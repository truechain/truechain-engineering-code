// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package snailchain implements the truechain snailchain protocol.
package snailchain

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/hashicorp/golang-lru"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/metrics"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	blockInsertTimer = metrics.NewRegisteredTimer("snailchain/inserts", nil)
	blockWriteTimer  = metrics.NewRegisteredTimer("snailchain/write", nil)
	//ErrNoGenesis is returned if the Genesis not found in chain.
	ErrNoGenesis = errors.New("Genesis not found in chain")
)

const (
	bodyCacheLimit      = 256
	blockCacheLimit     = 256
	maxFutureBlocks     = 256
	maxTimeFutureBlocks = 30
	badBlockLimit       = 10
)

// SnailBlockChain represents the canonical chain given a database with a genesis
// block. The Blockchain manages chain imports, reverts, chain reorganisations.
//
// Importing blocks in to the block chain happens according to the set of rules
// defined by the two stage Validator. Processing of blocks is done using the
// Processor which processes the included transaction. The validation of the state
// is done in the second part of the Validator. Failing results in aborting of
// the import.
//
// The BlockChain also helps in returning blocks from **any** chain included
// in the database as well as blocks that represents the canonical chain. It's
// important to note that GetBlock can return any block and does not need to be
// included in the canonical one where as GetBlockByNumber always represents the
// canonical chain.
type SnailBlockChain struct {
	chainConfig *params.ChainConfig // Chain & network configuration
	db          etruedb.Database    // Low level persistent database to store final content in
	gcproc      time.Duration       // Accumulates canonical block processing for trie dumping

	hc            *HeaderChain
	chainFeed     event.Feed
	chainSideFeed event.Feed
	chainHeadFeed event.Feed
	fastBlockFeed event.Feed
	fruitFeed     event.Feed // for worker mined fruit
	logsFeed      event.Feed
	scope         event.SubscriptionScope
	genesisBlock  *types.SnailBlock

	chainmu sync.RWMutex // blockchain insertion lock
	procmu  sync.RWMutex // block processor lock

	checkpoint       int          // checkpoint counts towards the new checkpoint
	currentBlock     atomic.Value // Current head of the block chain
	currentFastBlock atomic.Value // Current head of the fast-sync chain (may be above the block chain!)

	stateCache   state.Database // State database to reuse between imports (contains state cache)
	bodyCache    *lru.Cache     // Cache for the most recent block bodies
	bodyRLPCache *lru.Cache     // Cache for the most recent block bodies in RLP encoded format
	blockCache   *lru.Cache     // Cache for the most recent entire blocks
	futureBlocks *lru.Cache     // future blocks are blocks added for later processing

	quit    chan struct{} // blockchain quit channel
	running int32         // running must be called atomically
	// procInterrupt must be atomically called
	procInterrupt int32          // interrupt signaler for block processing
	wg            sync.WaitGroup // chain processing wait group for shutting down

	engine    consensus.Engine
	validator core.SnailValidator // block and state validator interface
	vmConfig  vm.Config

	blockchain *core.BlockChain

	badBlocks *lru.Cache // Bad block cache
}

// NewSnailBlockChain returns a fully initialised block chain using information
// available in the database. It initialises the default Ethereum Validator and
// Processor.
func NewSnailBlockChain(db etruedb.Database, chainConfig *params.ChainConfig, engine consensus.Engine, vmConfig vm.Config, blockchain *core.BlockChain) (*SnailBlockChain, error) {

	bodyCache, _ := lru.New(bodyCacheLimit)
	bodyRLPCache, _ := lru.New(bodyCacheLimit)
	blockCache, _ := lru.New(blockCacheLimit)
	futureBlocks, _ := lru.New(maxFutureBlocks)
	badBlocks, _ := lru.New(badBlockLimit)

	bc := &SnailBlockChain{
		chainConfig:  chainConfig,
		db:           db,
		quit:         make(chan struct{}),
		bodyCache:    bodyCache,
		bodyRLPCache: bodyRLPCache,
		blockCache:   blockCache,
		futureBlocks: futureBlocks,
		engine:       engine,
		vmConfig:     vmConfig,
		badBlocks:    badBlocks,
		blockchain:   blockchain,
	}
	bc.SetValidator(NewBlockValidator(chainConfig, blockchain, bc, engine))

	var err error
	bc.hc, err = NewHeaderChain(db, chainConfig, engine, bc.getProcInterrupt)
	if err != nil {
		return nil, err
	}

	bc.genesisBlock = bc.GetBlockByNumber(0)
	if bc.genesisBlock == nil {
		return nil, ErrNoGenesis
	}

	if err := bc.loadLastState(); err != nil {
		return nil, err
	}
	// Check the current state of the block hashes and make sure that we do not have any of the bad blocks in our chain
	for hash := range BadHashes {
		if header := bc.GetHeaderByHash(hash); header != nil {
			// get the canonical block corresponding to the offending header's number
			headerByNumber := bc.GetHeaderByNumber(header.Number.Uint64())
			// make sure the headerByNumber (if present) is in our current canonical chain
			if headerByNumber != nil && headerByNumber.Hash() == header.Hash() {
				log.Error("Found bad hash, rewinding chain", "number", header.Number, "hash", header.ParentHash)
				bc.SetHead(header.Number.Uint64() - 1)
				log.Error("Chain rewind was successful, resuming normal operation")
			}
		}
	}
	// Take ownership of this particular state
	go bc.update()
	return bc, nil
}

func (bc *SnailBlockChain) getProcInterrupt() bool {
	return atomic.LoadInt32(&bc.procInterrupt) == 1
}

// loadLastState loads the last known chain state from the database. This method
// assumes that the chain manager mutex is held.
func (bc *SnailBlockChain) loadLastState() error {
	// Restore the last known head block
	head := rawdb.ReadHeadBlockHash(bc.db)
	if head == (common.Hash{}) {
		// Corrupt or empty database, init from scratch
		log.Warn("Empty database, resetting chain")
		return bc.Reset()
	}
	// Make sure the entire head block is available
	currentBlock := bc.GetBlockByHash(head)
	if currentBlock == nil {
		// Corrupt or empty database, init from scratch
		log.Warn("Head block missing, resetting chain", "hash", head)
		return bc.Reset()
	}
	remove := make(types.Fruits, 0, len(currentBlock.Fruits()))
	maxFruitNumber := currentBlock.Fruits()[len(currentBlock.Fruits())-1].FastNumber()
	for maxFruitNumber != nil && maxFruitNumber.Cmp(bc.blockchain.CurrentBlock().Number()) > 0 {
		log.Debug("rollback snailBlock", "snailBlock number", currentBlock.Number(), "maxFruitNumber", maxFruitNumber, "current fastblock number", bc.blockchain.CurrentBlock().Number())
		parentHash := currentBlock.ParentHash()
		for _, ft := range currentBlock.Fruits() {
			remove = append(remove, ft)
		}
		rawdb.DeleteCanonicalHash(bc.db, currentBlock.NumberU64())
		currentBlock = bc.GetBlockByHash(parentHash)
		maxFruitNumber = currentBlock.Fruits()[len(currentBlock.Fruits())-1].FastNumber()
	}
	rawdb.WriteHeadBlockHash(bc.db, currentBlock.Hash())
	rawdb.WriteHeadHeaderHash(bc.db, currentBlock.Header().Hash())
	rawdb.WriteHeadFastBlockHash(bc.db, bc.blockchain.CurrentBlock().Hash())
	batch := bc.db.NewBatch()
	for _, ft := range remove {
		rawdb.DeleteFtLookupEntry(batch, ft.FastHash())
	}
	batch.Write()
	// Everything seems to be fine, set as the head block
	bc.currentBlock.Store(currentBlock)

	// Restore the last known head header
	currentHeader := currentBlock.Header()
	if head := rawdb.ReadHeadHeaderHash(bc.db); head != (common.Hash{}) {
		if header := bc.GetHeaderByHash(head); header != nil {
			currentHeader = header
		}
	}
	bc.hc.SetCurrentHeader(currentHeader)

	// Restore the last known head fast block
	bc.currentFastBlock.Store(currentBlock)
	if head := rawdb.ReadHeadFastBlockHash(bc.db); head != (common.Hash{}) {
		if block := bc.GetBlockByHash(head); block != nil {
			bc.currentFastBlock.Store(block)
		}
	}

	// Issue a status log for the user
	currentFastBlock := bc.CurrentFastBlock()

	headerTd := bc.GetTd(currentHeader.Hash(), currentHeader.Number.Uint64())
	blockTd := bc.GetTd(currentBlock.Hash(), currentBlock.NumberU64())
	fastTd := bc.GetTd(currentFastBlock.Hash(), currentFastBlock.NumberU64())

	log.Info("Loaded most recent local header", "number", currentHeader.Number, "hash", currentHeader.Hash(), "td", headerTd)
	log.Info("Loaded most recent local full block", "number", currentBlock.Number(), "hash", currentBlock.Hash(), "td", blockTd)
	log.Info("Loaded most recent local fast block", "number", currentFastBlock.Number(), "hash", currentFastBlock.Hash(), "td", fastTd)

	return nil
}

// SetHead rewinds the local chain to a new head. In the case of headers, everything
// above the new head will be deleted and the new one set. In the case of blocks
// though, the head may be further rewound if block bodies are missing (non-archive
// nodes after a fast sync).
func (bc *SnailBlockChain) SetHead(head uint64) error {
	log.Warn("Rewinding blockchain", "target", head)
	if head > bc.currentBlock.Load().(*types.SnailBlock).Number().Uint64() {
		log.Error("the height can't set,because it is higher than current height", "height", head, "current height", bc.currentBlock.Load().(*types.SnailBlock).Number().Uint64())
		return errors.New("the height you give is too high,can not be set")
	}
	/*	err := bc.Validator().ValidateRewarded(head + 1)
		if err != nil {
			log.Error("the hight can't set,because it's next block is already rewarded", "hight", head)
			return err
		}*/
	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()
	//retroversion fastchain
	fastNumber := bc.GetBlockByNumber(head).Fruits()[len(bc.GetBlockByNumber(head).Fruits())-1].FastNumber()
	if err := bc.blockchain.SetHead(fastNumber.Uint64()); err != nil {
		return err
	}
	// Rewind the header chain, deleting all block bodies and FtLookupEntry until then
	delFn := func(db rawdb.DatabaseDeleter, hash common.Hash, num uint64) {
		rawdb.DeleteBody(db, hash, num)
		block := bc.GetBlockByNumber(num)
		for _, ft := range block.Fruits() {
			rawdb.DeleteFtLookupEntry(db, ft.FastHash())
		}
	}

	bc.hc.SetHead(head, delFn)
	currentHeader := bc.hc.CurrentHeader()

	// Clear out any stale content from the caches
	bc.bodyCache.Purge()
	bc.bodyRLPCache.Purge()
	bc.blockCache.Purge()
	bc.futureBlocks.Purge()

	// Rewind the block chain, ensuring we don't end up with a stateless head block
	if currentBlock := bc.CurrentBlock(); currentBlock != nil && currentHeader.Number.Uint64() < currentBlock.NumberU64() {
		bc.currentBlock.Store(bc.GetBlock(currentHeader.Hash(), currentHeader.Number.Uint64()))
	}

	// Rewind the fast block in a simpleton way to the target head
	if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock != nil && currentHeader.Number.Uint64() < currentFastBlock.NumberU64() {
		bc.currentFastBlock.Store(bc.GetBlock(currentHeader.Hash(), currentHeader.Number.Uint64()))
	}
	// If either blocks reached nil, reset to the genesis state
	if currentBlock := bc.CurrentBlock(); currentBlock == nil {
		bc.currentBlock.Store(bc.genesisBlock)
	}
	if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock == nil {
		bc.currentFastBlock.Store(bc.genesisBlock)
	}
	currentBlock := bc.CurrentBlock()
	currentFastBlock := bc.CurrentFastBlock()

	rawdb.WriteHeadBlockHash(bc.db, currentBlock.Hash())
	rawdb.WriteHeadFastBlockHash(bc.db, currentFastBlock.Hash())

	// Append a single chain head event if we've progressed the chain
	lastCanon := bc.GetBlockByNumber(head)
	if lastCanon != nil && bc.CurrentBlock().Hash() == lastCanon.Hash() {
		events := make([]interface{}, 0, 1)
		events = append(events, types.ChainSnailHeadEvent{lastCanon})
		bc.PostChainEvents(events)
	}

	return bc.loadLastState()

}

// FastSyncCommitHead sets the current head block to the one defined by the hash
// irrelevant what the chain contents were prior.
func (bc *SnailBlockChain) FastSyncCommitHead(hash common.Hash) error {
	// Make sure that both the block as well at its state trie exists
	block := bc.GetBlockByHash(hash)
	if block == nil {
		return fmt.Errorf("non existent block [%x…]", hash[:4])
	}

	// If all checks out, manually set the head block
	bc.chainmu.Lock()
	bc.currentBlock.Store(block)
	bc.chainmu.Unlock()

	log.Info("Committed new head block", "number", block.Number(), "hash", hash)
	return nil
}

// CurrentBlock retrieves the current head block of the canonical chain. The
// block is retrieved from the blockchain's internal cache.
func (bc *SnailBlockChain) CurrentBlock() *types.SnailBlock {
	return bc.currentBlock.Load().(*types.SnailBlock)
}

// CurrentFastBlock retrieves the current fast-sync head block of the canonical
// chain. The block is retrieved from the blockchain's internal cache.
func (bc *SnailBlockChain) CurrentFastBlock() *types.SnailBlock {
	return bc.currentFastBlock.Load().(*types.SnailBlock)
}

// SetValidator sets the validator which is used to validate incoming blocks.
func (bc *SnailBlockChain) SetValidator(validator core.SnailValidator) {
	bc.procmu.Lock()
	defer bc.procmu.Unlock()
	bc.validator = validator
}

// Validator returns the current validator.
func (bc *SnailBlockChain) Validator() core.SnailValidator {
	bc.procmu.RLock()
	defer bc.procmu.RUnlock()
	return bc.validator
}

// StateAt returns a new mutable state based on a particular point in time.
func (bc *SnailBlockChain) StateAt(root common.Hash) (*state.StateDB, error) {
	return state.New(root, bc.stateCache)
}

// Reset purges the entire blockchain, restoring it to its genesis state.
func (bc *SnailBlockChain) Reset() error {
	return bc.ResetWithGenesisBlock(bc.genesisBlock)
}

// ResetWithGenesisBlock purges the entire blockchain, restoring it to the
// specified genesis state.
func (bc *SnailBlockChain) ResetWithGenesisBlock(genesis *types.SnailBlock) error {
	// Dump the entire block chain and purge the caches
	if err := bc.SetHead(0); err != nil {
		return err
	}
	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	// Prepare the genesis block and reinitialise the chain
	/*if err := bc.hc.WriteSnailTd(genesis.Hash(), genesis.NumberU64(), genesis.Difficulty()); err != nil {
		log.Crit("Failed to write genesis block TD", "err", err)
	}*/
	//rawdb.WriteBlock(bc.db, genesis)

	bc.genesisBlock = genesis
	bc.insert(bc.genesisBlock)
	bc.currentBlock.Store(bc.genesisBlock)
	bc.hc.SetGenesis(bc.genesisBlock.Header())
	bc.hc.SetCurrentHeader(bc.genesisBlock.Header())
	bc.currentFastBlock.Store(bc.genesisBlock)

	return nil
}

// repair tries to repair the current blockchain by rolling back the current block
// until one with associated state is found. This is needed to fix incomplete db
// writes caused either by crashes/power outages, or simply non-committed tries.
//
// This method only rolls back the current block. The current header and current
// fast block are left intact.
func (bc *SnailBlockChain) repair(head **types.SnailBlock) error {
	for {
		// Otherwise rewind one block and recheck state availability there
		(*head) = bc.GetBlock((*head).ParentHash(), (*head).NumberU64()-1)
	}
}

// Export writes the active chain to the given writer.
func (bc *SnailBlockChain) Export(w io.Writer) error {
	return bc.ExportN(w, uint64(0), bc.CurrentBlock().NumberU64())
}

// ExportN writes a subset of the active chain to the given writer.
func (bc *SnailBlockChain) ExportN(w io.Writer, first uint64, last uint64) error {
	bc.chainmu.RLock()
	defer bc.chainmu.RUnlock()

	if first > last {
		return fmt.Errorf("export failed: first (%d) is greater than last (%d)", first, last)
	}
	log.Info("Exporting batch of blocks", "count", last-first+1)

	for nr := first; nr <= last; nr++ {
		block := bc.GetBlockByNumber(nr)
		if block == nil {
			return fmt.Errorf("export failed on #%d: not found", nr)
		}

		if err := block.EncodeRLP(w); err != nil {
			return err
		}
	}

	return nil
}

// insert injects a new head block into the current block chain. This method
// assumes that the block is indeed a true head. It will also reset the head
// header and the head fast sync block to this very same block if they are older
// or if they are on a different side chain.
//
// Note, this function assumes that the `mu` mutex is held!
func (bc *SnailBlockChain) insert(block *types.SnailBlock) {
	// If the block is on a side chain or an unknown one, force other heads onto it too
	updateHeads := rawdb.ReadCanonicalHash(bc.db, block.NumberU64()) != block.Hash()

	// Add the block to the canonical chain number scheme and mark as the head
	rawdb.WriteCanonicalHash(bc.db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(bc.db, block.Hash())

	bc.currentBlock.Store(block)

	// If the block is better than our head or is on a different chain, force update heads
	if updateHeads {
		bc.hc.SetCurrentHeader(block.Header())

		rawdb.WriteHeadFastBlockHash(bc.db, block.Hash())

		bc.currentFastBlock.Store(block)
	}
}

// Genesis retrieves the chain's genesis block.
func (bc *SnailBlockChain) Genesis() *types.SnailBlock {
	return bc.genesisBlock
}

// GetBody retrieves a block body (transactions and uncles) from the database by
// hash, caching it if found.
func (bc *SnailBlockChain) GetBody(hash common.Hash) *types.SnailBody {
	// Short circuit if the body's already in the cache, retrieve otherwise
	if cached, ok := bc.bodyCache.Get(hash); ok {
		body := cached.(*types.SnailBody)
		return body
	}
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	body := rawdb.ReadBody(bc.db, hash, *number)
	if body == nil {
		return nil
	}
	// Cache the found body for next time and return
	bc.bodyCache.Add(hash, body)
	return body
}

// GetBodyRLP retrieves a block body in RLP encoding from the database by hash,
// caching it if found.
func (bc *SnailBlockChain) GetBodyRLP(hash common.Hash) rlp.RawValue {
	// Short circuit if the body's already in the cache, retrieve otherwise
	if cached, ok := bc.bodyRLPCache.Get(hash); ok {
		return cached.(rlp.RawValue)
	}
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	body := rawdb.ReadBodyRLP(bc.db, hash, *number)
	if len(body) == 0 {
		return nil
	}
	// Cache the found body for next time and return
	bc.bodyRLPCache.Add(hash, body)
	return body
}

// HasBlock checks if a block is fully present in the database or not.
func (bc *SnailBlockChain) HasBlock(hash common.Hash, number uint64) bool {
	if bc.blockCache.Contains(hash) {
		return true
	}
	return rawdb.HasBody(bc.db, hash, number)
}

// HasConfirmedBlock checks if a block is fully present in the database or not.and number must bigger than currentBlockNumber
func (bc *SnailBlockChain) HasConfirmedBlock(hash common.Hash, number uint64) bool {
	if number > bc.currentBlock.Load().(*types.SnailBlock).Number().Uint64() {
		return false
	}
	if bc.blockCache.Contains(hash) {
		return true
	}
	return rawdb.HasBody(bc.db, hash, number)
}

// HasState checks if state trie is fully present in the database or not.
func (bc *SnailBlockChain) HasState(hash common.Hash) bool {
	_, err := bc.stateCache.OpenTrie(hash)
	return err == nil
}

// HasBlockAndState checks if a block and associated state trie is fully present
// in the database or not, caching it if present.
func (bc *SnailBlockChain) HasBlockAndState(hash common.Hash, number uint64) bool {
	//if get number bigger than currentNumber return nil
	if number > bc.currentBlock.Load().(*types.SnailBlock).Number().Uint64() {
		return false
	}
	// Check first that the block itself is known
	block := bc.GetBlock(hash, number)
	if block == nil {
		return false
	}
	return rawdb.ReadCanonicalHash(bc.db, number) == hash
}

// GetBlock retrieves a block from the database by hash and number,
// caching it if found.
func (bc *SnailBlockChain) GetBlock(hash common.Hash, number uint64) *types.SnailBlock {
	// Short circuit if the block's already in the cache, retrieve otherwise
	if block, ok := bc.blockCache.Get(hash); ok {
		return block.(*types.SnailBlock)
	}
	block := rawdb.ReadBlock(bc.db, hash, number)
	if block == nil {
		return nil
	}
	// Cache the found block for next time and return
	bc.blockCache.Add(block.Hash(), block)
	return block
}

// GetBlockByHash retrieves a block from the database by hash, caching it if found.
func (bc *SnailBlockChain) GetBlockByHash(hash common.Hash) *types.SnailBlock {
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	return bc.GetBlock(hash, *number)
}

// GetBlockByNumber retrieves a block from the database by number, caching it
// (associated with its hash) if found.
func (bc *SnailBlockChain) GetBlockByNumber(number uint64) *types.SnailBlock {
	hash := rawdb.ReadCanonicalHash(bc.db, number)
	if hash == (common.Hash{}) {
		return nil
	}
	return bc.GetBlock(hash, number)
}

// GetBlocksFromHash returns the block corresponding to hash and up to n-1 ancestors.
// [deprecated by eth/62]
func (bc *SnailBlockChain) GetBlocksFromHash(hash common.Hash, n int) (blocks []*types.SnailBlock) {
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	for i := 0; i < n; i++ {
		block := bc.GetBlock(hash, *number)
		if block == nil {
			break
		}
		blocks = append(blocks, block)
		hash = block.ParentHash()
		*number--
	}
	return
}

//GetBlocksFromNumber return snailblocks between given number to currentNumber
//just for test
func (bc *SnailBlockChain) GetBlocksFromNumber(fromNumber uint64) (blocks []*types.SnailBlock) {
	currentNumber := bc.CurrentBlock().Number()
	for i := fromNumber; i <= currentNumber.Uint64(); i++ {
		block := bc.GetBlockByNumber(i)
		if block == nil {
			break
		}
		blocks = append(blocks, block)
	}
	return
}

// TrieNode retrieves a blob of data associated with a trie node (or code hash)
// either from ephemeral in-memory cache, or from persistent storage.
func (bc *SnailBlockChain) TrieNode(hash common.Hash) ([]byte, error) {
	return bc.stateCache.TrieDB().Node(hash)
}

// Stop stops the blockchain service. If any imports are currently in progress
// it will abort them using the procInterrupt.
func (bc *SnailBlockChain) Stop() {
	if !atomic.CompareAndSwapInt32(&bc.running, 0, 1) {
		return
	}
	// Unsubscribe all subscriptions registered from blockchain
	bc.scope.Close()
	close(bc.quit)
	atomic.StoreInt32(&bc.procInterrupt, 1)

	bc.wg.Wait()

	log.Info("Blockchain manager stopped")
}

func (bc *SnailBlockChain) procFutureBlocks() {
	blocks := make([]*types.SnailBlock, 0, bc.futureBlocks.Len())
	for _, hash := range bc.futureBlocks.Keys() {
		if block, exist := bc.futureBlocks.Peek(hash); exist {
			blocks = append(blocks, block.(*types.SnailBlock))
		}
	}
	if len(blocks) > 0 {
		types.SnailBlockBy(types.SnailNumber).Sort(blocks)

		// Insert one by one as chain insertion needs contiguous ancestry between blocks
		for i := range blocks {
			bc.InsertChain(blocks[i : i+1])
		}
	}
}

// WriteStatus status of write
type WriteStatus byte

//the three kind of status
const (
	NonStatTy WriteStatus = iota
	CanonStatTy
	SideStatTy
)

// Rollback is designed to remove a chain of links from the database that aren't
// certain enough to be valid.
func (bc *SnailBlockChain) Rollback(chain []common.Hash) {
	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	for i := len(chain) - 1; i >= 0; i-- {
		hash := chain[i]

		currentHeader := bc.hc.CurrentHeader()
		if currentHeader.Hash() == hash {
			bc.hc.SetCurrentHeader(bc.GetHeader(currentHeader.ParentHash, currentHeader.Number.Uint64()-1))
		}
		if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock.Hash() == hash {
			newFastBlock := bc.GetBlock(currentFastBlock.ParentHash(), currentFastBlock.NumberU64()-1)
			bc.currentFastBlock.Store(newFastBlock)
			//TODO Write Fast Block Hash
			rawdb.WriteHeadFastBlockHash(bc.db, newFastBlock.Hash())
		}
		if currentBlock := bc.CurrentBlock(); currentBlock.Hash() == hash {
			newBlock := bc.GetBlock(currentBlock.ParentHash(), currentBlock.NumberU64()-1)
			bc.currentBlock.Store(newBlock)
			rawdb.WriteHeadBlockHash(bc.db, newBlock.Hash())
		}
	}
}

// GetDatabase Get lowlevel persistence database
func (bc *SnailBlockChain) GetDatabase() etruedb.Database {
	return bc.db
}

// WriteBlock writes only the block and its metadata to the database,
// but does not write any state. This is used to construct competing side forks
// up to the point where they exceed the canonical total difficulty.
func (bc *SnailBlockChain) WriteBlock(block *types.SnailBlock, td *big.Int) (err error) {
	bc.wg.Add(1)
	defer bc.wg.Done()

	if err := bc.hc.WriteTd(block.Hash(), block.NumberU64(), td); err != nil {
		return err
	}
	rawdb.WriteBlock(bc.db, block)

	return nil
}

// WriteMinedCanonicalBlock writes the minedblock to the database.
func (bc *SnailBlockChain) WriteMinedCanonicalBlock(block *types.SnailBlock) (status WriteStatus, err error) {
	bc.chainmu.Lock()
	bc.chainmu.Unlock()
	log.Debug("WriteMinedCanonicalBlock", "number", block.Number(), "hash", block.Hash())
	return bc.writeCanonicalBlock(block)
}

func (bc *SnailBlockChain) writeCanonicalBlock(block *types.SnailBlock) (status WriteStatus, err error) {
	bc.wg.Add(1)
	defer bc.wg.Done()

	log.Info("Write new snail canonical block...", "number", block.Number(), "hash", block.Hash())
	// Calculate the total difficulty of the block
	ptd := bc.GetTd(block.ParentHash(), block.NumberU64()-1)
	if ptd == nil {
		return NonStatTy, consensus.ErrUnknownAncestor
	}
	// Make sure no inconsistent state is leaked during insertion

	currentBlock := bc.CurrentBlock()
	localTd := bc.GetTd(currentBlock.Hash(), currentBlock.NumberU64())
	externTd := new(big.Int).Add(block.Difficulty(), ptd)

	// Irrelevant of the canonical status, write the block itself to the database
	if err := bc.hc.WriteTd(block.Hash(), block.NumberU64(), externTd); err != nil {
		return NonStatTy, err
	}
	// Write other block data using a batch.
	rawdb.WriteBlock(bc.db, block)

	// If the total difficulty is higher than our known, add it to the canonical chain
	// Second clause in the if statement reduces the vulnerability to selfish mining.
	// Please refer to http://www.cs.cornell.edu/~ie53/publications/btcProcFC.pdf
	reorg := externTd.Cmp(localTd) > 0
	currentBlock = bc.CurrentBlock()
	if !reorg && externTd.Cmp(localTd) == 0 {
		// Split same-difficulty blocks by number, then at random
		reorg = block.NumberU64() < currentBlock.NumberU64() || (block.NumberU64() == currentBlock.NumberU64() && mrand.Float64() < 0.5)
	}
	if reorg {
		// Reorganise the chain if the parent is not the head block
		if block.ParentHash() != currentBlock.Hash() {
			log.Debug("Reorganise the chain sine the parent is not the head block")
			if err := bc.reorg(currentBlock, block); err != nil {
				return NonStatTy, err
			}
		}
		// Write the positional metadata for fruit lookups
		rawdb.WriteFtLookupEntries(bc.db, block)

		status = CanonStatTy
	} else {
		status = SideStatTy
	}

	//if err := batch.Write(); err != nil {
	//	return NonStatTy, err
	//}

	// Set new head.
	if status == CanonStatTy {
		bc.insert(block)
	}
	bc.futureBlocks.Remove(block.Hash())
	return status, nil
}

// addFutureBlock checks if the block is within the max allowed window to get
// accepted for future processing, and returns an error if the block is too far
// ahead and was not added.
func (bc *SnailBlockChain) addFutureBlock(block *types.SnailBlock) error {
	max := big.NewInt(time.Now().Unix() + maxTimeFutureBlocks)
	if block.Time().Cmp(max) > 0 {
		return fmt.Errorf("future block timestamp %v > allowed %v", block.Time(), max)
	}
	bc.futureBlocks.Add(block.Hash(), block)
	return nil
}

// InsertChain attempts to insert the given batch of blocks in to the canonical
// chain or, otherwise, create a fork. If an error is returned it will return
// the index number of the failing block as well an error describing what went
// wrong.
//
// After insertion is done, all accumulated events will be fired.
func (bc *SnailBlockChain) InsertChain(chain types.SnailBlocks) (int, error) {
	// Sanity check that we have something meaningful to import
	if len(chain) == 0 {
		return 0, nil
	}
	// Do a sanity check that the provided chain is actually ordered and linked
	for i := 1; i < len(chain); i++ {
		if chain[i].NumberU64() != chain[i-1].NumberU64()+1 || chain[i].ParentHash() != chain[i-1].Hash() {
			// Chain broke ancestry, log a message (programming error) and skip insertion
			log.Error("Non contiguous block insert", "number", chain[i].Number(), "hash", chain[i].Hash(),
				"parent", chain[i].ParentHash(), "prevnumber", chain[i-1].Number(), "prevhash", chain[i-1].Hash())

			return 0, fmt.Errorf("non contiguous insert: item %d is #%d [%x…], item %d is #%d [%x…] (parent [%x…])", i-1, chain[i-1].NumberU64(),
				chain[i-1].Hash().Bytes()[:4], i, chain[i].NumberU64(), chain[i].Hash().Bytes()[:4], chain[i].ParentHash().Bytes()[:4])
		}
	}
	// Pre-checks passed, start the full block imports
	bc.wg.Add(1)
	bc.chainmu.Lock()
	log.Debug("InsertChain...", "start", chain[0].NumberU64(), "end", chain[len(chain)-1].NumberU64())
	n, events, err := bc.insertChain(chain, true)
	bc.chainmu.Unlock()
	bc.wg.Done()

	bc.PostChainEvents(events)
	return n, err
}

// insertChain is the internal implementation of insertChain, which assumes that
// 1) chains are contiguous, and 2) The chain mutex is held.
//
// This method is split out so that import batches that require re-injecting
// historical blocks can do so without releasing the lock, which could lead to
// racey behaviour. If a sidechain import is in progress, and the historic state
// is imported, but then new canon-head is added before the actual sidechain
// completes, then the historic state could be pruned again
func (bc *SnailBlockChain) insertChain(chain types.SnailBlocks, verifySeals bool) (int, []interface{}, error) {
	// If the chain is terminating, don't even bother starting u
	if atomic.LoadInt32(&bc.procInterrupt) == 1 {
		return 0, nil, nil
	}

	// A queued approach to delivering events. This is generally
	// faster than direct delivery and requires much less mutex
	// acquiring.
	var (
		stats     = insertSnailStats{startTime: mclock.Now()}
		events    = make([]interface{}, 0, len(chain))
		lastCanon *types.SnailBlock
	)
	// Start the parallel header verifier
	headers := make([]*types.SnailHeader, len(chain))
	seals := make([]bool, len(chain))

	for i, block := range chain {
		headers[i] = block.Header()
		seals[i] = verifySeals
	}
	//bc.CurrentSnailHeader
	abort, results := bc.engine.VerifySnailHeaders(bc, headers, seals)
	defer close(abort)

	// Peek the error for the first block to decide the directing import logic
	it := newInsertIterator(chain, results, bc.Validator())

	block, err := it.next()
	bstart := time.Now()
	switch {
	// First block is pruned, insert as sidechain and reorg only if TD grows enough
	case err == consensus.ErrPrunedAncestor:
		return bc.insertSidechain(block, it)

		// First block is future, shove it (and all children) to the future queue (unknown ancestor)
	case err == consensus.ErrFutureBlock || (err == consensus.ErrUnknownAncestor && bc.futureBlocks.Contains(it.first().ParentHash())):
		for block != nil && (it.index == 0 || err == consensus.ErrUnknownAncestor) {
			if err := bc.addFutureBlock(block); err != nil {
				return it.index, events, err
			}
			block, err = it.next()
		}
		stats.queued += it.processed()
		stats.ignored += it.remaining()

		// If there are any still remaining, mark as ignored
		return it.index, events, err

		// First block (and state) is known
		//   1. We did a roll-back, and should now do a re-import
		//   2. The block is stored as a sidechain, and is lying about it's stateroot, and passes a stateroot
		// 	    from the canonical chain, which has not been verified.
	case err == ErrKnownBlock:
		// Skip all known blocks that behind us
		current := bc.CurrentBlock().NumberU64()

		for block != nil && err == ErrKnownBlock && current >= block.NumberU64() {
			stats.ignored++
			block, err = it.next()
		}
		// Falls through to the block import

		// Some other error occurred, abort
	case err != nil:
		stats.ignored += len(it.chain)
		bc.reportBlock(block, err)
		return it.index, events, err
	}
	// No validation errors for the first block (or chain prefix skipped)
	for ; block != nil && (err == nil || err == consensus.ErrPrunedAncestor); block, err = it.next() {
		// If the chain is terminating, stop processing blocks
		if atomic.LoadInt32(&bc.procInterrupt) == 1 {
			log.Debug("Premature abort during blocks processing")
			break
		}
		// If the header is a banned one, straight out abort
		if BadHashes[block.Hash()] {
			bc.reportBlock(block, ErrBlacklistedHash)
			return it.index, events, ErrBlacklistedHash
		}
		t0 := time.Now()
		proctime := time.Since(bstart)

		// Write the block to the chain and get the status.
		status, err := bc.writeCanonicalBlock(block)
		t1 := time.Now()
		if err != nil {
			log.Info("Write new snail canonical block error", "number", block.Number(), "hash", block.Hash(), "err", err)
			return it.index, events, err
		}
		blockWriteTimer.Update(t1.Sub(t0))
		switch status {
		case CanonStatTy:

			log.Info("Inserted new snail block", "number", block.Number(), "hash", block.Hash(),
				"fts", len(block.Fruits()), "elapsed", common.PrettyDuration(time.Since(bstart)))

			//coalescedLogs = append(coalescedLogs, logs...)

			blockInsertTimer.UpdateSince(bstart)
			events = append(events, types.ChainSnailEvent{block, block.Hash()})
			lastCanon = block

			// Only count canonical blocks for GC processing time
			bc.gcproc += proctime

		case SideStatTy:

			log.Info("Inserted new snail forked block", "number", block.Number(), "hash", block.Hash(), "diff", block.Difficulty(), "elapsed",
				common.PrettyDuration(time.Since(bstart)), "fts", len(block.Fruits()))

			blockInsertTimer.UpdateSince(bstart)
			events = append(events, types.ChainSnailSideEvent{block})
		}
		stats.processed++

		stats.report(chain, it.index)
	}

	// Any blocks remaining here? The only ones we care about are the future ones
	if block != nil && err == consensus.ErrFutureBlock {
		if err := bc.addFutureBlock(block); err != nil {
			return it.index, events, err
		}
		block, err = it.next()

		for ; block != nil && err == consensus.ErrUnknownAncestor; block, err = it.next() {
			if err := bc.addFutureBlock(block); err != nil {
				return it.index, events, err
			}
			stats.queued++
		}
	}
	stats.ignored += it.remaining()

	// Append a single chain head event if we've progressed the chain
	if lastCanon != nil && bc.CurrentBlock().Hash() == lastCanon.Hash() {
		events = append(events, types.ChainSnailHeadEvent{lastCanon})
	}
	return it.index, events, err
}

// insertSidechain is called when an import batch hits upon a pruned ancestor
// error, which happens when a sidechain with a sufficiently old fork-block is
// found.
//
// The method writes all (header-and-body-valid) blocks to disk, then tries to
// switch over to the new chain if the TD exceeded the current chain.
func (bc *SnailBlockChain) insertSidechain(block *types.SnailBlock, it *insertIterator) (int, []interface{}, error) {
	var (
		externTd *big.Int
		current  = bc.CurrentBlock()
	)
	// The first sidechain block error is already verified to be ErrPrunedAncestor.
	// Since we don't import them here, we expect ErrUnknownAncestor for the remaining
	// ones. Any other errors means that the block is invalid, and should not be written
	// to disk.
	err := consensus.ErrPrunedAncestor
	for ; block != nil && (err == consensus.ErrPrunedAncestor); block, err = it.next() {
		if externTd == nil {
			externTd = bc.GetTd(block.ParentHash(), block.NumberU64()-1)
		}
		externTd = new(big.Int).Add(externTd, block.Difficulty())

		if !bc.HasBlock(block.Hash(), block.NumberU64()) {
			start := time.Now()
			if err := bc.WriteBlock(block, externTd); err != nil {
				return it.index, nil, err
			}
			log.Debug("Injected sidechain block", "number", block.Number(), "hash", block.Hash(),
				"diff", block.Difficulty(), "elapsed", common.PrettyDuration(time.Since(start)))
		}
	}
	// At this point, we've written all sidechain blocks to database. Loop ended
	// either on some other error or all were processed. If there was some other
	// error, we can ignore the rest of those blocks.
	//
	// If the externTd was larger than our local TD, we now need to reimport the previous
	// blocks to regenerate the required state
	localTd := bc.GetTd(current.Hash(), current.NumberU64())
	if localTd.Cmp(externTd) > 0 {
		log.Debug("Sidechain written to disk", "start", it.first().NumberU64(), "end", it.previous().NumberU64(), "sidetd", externTd, "localtd", localTd)
		return it.index, nil, err
	}
	// Gather all the sidechain hashes (full blocks may be memory heavy)
	var (
		hashes  []common.Hash
		numbers []uint64
	)
	parent := bc.GetHeader(it.previous().Hash(), it.previous().NumberU64())
	for parent != nil && rawdb.ReadCanonicalHash(bc.db, parent.Number.Uint64()) != parent.Hash() {
		hashes = append(hashes, parent.Hash())
		numbers = append(numbers, parent.Number.Uint64())

		parent = bc.GetHeader(parent.ParentHash, parent.Number.Uint64()-1)
		log.Warn("number and hash", "number", parent.Number.Uint64(), "CanonicalHash", rawdb.ReadCanonicalHash(bc.db, parent.Number.Uint64()), "Sidechainhash", parent.Hash())
	}
	if parent == nil {
		return it.index, nil, errors.New("missing parent")
	}
	// Import all the pruned blocks to make the state available
	var (
		blocks []*types.SnailBlock
		memory common.StorageSize
	)
	for i := len(hashes) - 1; i >= 0; i-- {
		// Append the next block to our batch
		block := bc.GetBlock(hashes[i], numbers[i])

		blocks = append(blocks, block)
		memory += block.Size()

		// If memory use grew too large, import and continue. Sadly we need to discard
		// all raised events and logs from notifications since we're too heavy on the
		// memory here.
		if len(blocks) >= 2048 || memory > 64*1024*1024 {
			log.Info("Importing heavy sidechain segment", "blocks", len(blocks), "start", blocks[0].NumberU64(), "end", block.NumberU64())
			if _, _, err := bc.insertChain(blocks, false); err != nil {
				return 0, nil, err
			}
			blocks, memory = blocks[:0], 0

			// If the chain is terminating, stop processing blocks
			if atomic.LoadInt32(&bc.procInterrupt) == 1 {
				log.Debug("Premature abort during blocks processing")
				return 0, nil, nil
			}
		}
	}
	if len(blocks) > 0 {
		log.Info("Importing sidechain segment", "start", blocks[0].NumberU64(), "end", blocks[len(blocks)-1].NumberU64())
		return bc.insertChain(blocks, false)
	}
	return 0, nil, nil
}

func countSnailFruits(chain []*types.SnailBlock) (c int) {

	for _, b := range chain {
		c += len(b.Fruits())
	}

	return c
}

// reorgs takes two blocks, an old chain and a new chain and will reconstruct the blocks and inserts them
// to be part of the new canonical chain and accumulates potential missing fruits and post an
// event about them
func (bc *SnailBlockChain) reorg(oldBlock, newBlock *types.SnailBlock) error {
	var (
		newChain    types.SnailBlocks
		oldChain    types.SnailBlocks
		commonBlock *types.SnailBlock
		deletedFts  types.Fruits
	)

	if oldBlock == nil {
		return fmt.Errorf("Invalid old chain")
	}
	if newBlock == nil {
		return fmt.Errorf("Invalid new chain")
	}

	// first reduce whoever is higher bound
	if oldBlock.NumberU64() > newBlock.NumberU64() {
		// reduce old chain
		for ; oldBlock != nil && oldBlock.NumberU64() != newBlock.NumberU64(); oldBlock = bc.GetBlock(oldBlock.ParentHash(), oldBlock.NumberU64()-1) {
			oldChain = append(oldChain, oldBlock)
			deletedFts = append(deletedFts, oldBlock.Fruits()...)
		}
	} else {
		// reduce new chain and append new chain blocks for inserting later on
		for ; newBlock != nil && newBlock.NumberU64() != oldBlock.NumberU64(); newBlock = bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1) {
			newChain = append(newChain, newBlock)
		}
	}

	for {
		if oldBlock.Hash() == newBlock.Hash() {
			commonBlock = oldBlock
			break
		}

		oldChain = append(oldChain, oldBlock)
		newChain = append(newChain, newBlock)
		deletedFts = append(deletedFts, oldBlock.Fruits()...)

		oldBlock, newBlock = bc.GetBlock(oldBlock.ParentHash(), oldBlock.NumberU64()-1), bc.GetBlock(newBlock.ParentHash(), newBlock.NumberU64()-1)
		if oldBlock == nil {
			return fmt.Errorf("Invalid old chain")
		}
		if newBlock == nil {
			return fmt.Errorf("Invalid new chain")
		}
	}
	// Ensure the user sees large reorgs
	if len(oldChain) > 0 && len(newChain) > 0 {
		logFn := log.Debug
		if len(oldChain) > 63 {
			logFn = log.Warn
		}
		logFn("Chain split detected", "number", commonBlock.Number(), "hash", commonBlock.Hash(),
			"drop", len(oldChain), "dropfrom", oldChain[0].Hash(), "add", len(newChain), "addfrom", newChain[0].Hash())
	} else {
		log.Error("Impossible reorg, please file an issue", "oldnum", oldBlock.Number(), "oldhash", oldBlock.Hash(), "newnum", newBlock.Number(), "newhash", newBlock.Hash())
	}
	// Insert the new chain, taking care of the proper incremental order
	var addedFts types.Fruits
	for i := len(newChain) - 1; i >= 0; i-- {
		// insert the block in the canonical way, re-writing history
		bc.insert(newChain[i])
		// write lookup entries for hash based fruits
		rawdb.WriteFtLookupEntries(bc.db, newChain[i])
		addedFts = append(addedFts, newChain[i].Fruits()...)
	}

	// calculate the difference between deleted and added fruits
	diff := bc.ftDifference(deletedFts, addedFts)

	batch := bc.db.NewBatch()

	for _, ft := range diff {
		rawdb.DeleteFtLookupEntry(batch, ft.FastHash())
	}

	batch.Write()

	if len(oldChain) > 0 {
		go func() {
			for _, block := range oldChain {
				bc.chainSideFeed.Send(types.ChainSnailSideEvent{Block: block})
			}
		}()
	}

	if len(newChain) > 1 {
		go func() {
			for i := len(newChain) - 1; i > 0; i-- {
				log.Info("reorg snail block", "number", newChain[i].Number(), "hash", newChain[i].Hash())
				bc.chainFeed.Send(types.ChainSnailEvent{Block: newChain[i]})
			}
		}()
	}

	return nil
}

// ftDifference returns a new set t which is the difference between a to b.
func (bc *SnailBlockChain) ftDifference(a, b types.Fruits) (keep types.Fruits) {
	keep = make(types.Fruits, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, ft := range b {
		remove[ft.FastHash()] = struct{}{}
	}

	for _, ft := range a {
		if _, ok := remove[ft.FastHash()]; !ok {
			keep = append(keep, ft)
		}
	}

	return keep
}

// PostChainEvents iterates over the events generated by a chain insertion and
// posts them into the event feed.
// TODO: Should not expose PostChainEvents. The chain events should be posted in WriteBlock.
func (bc *SnailBlockChain) PostChainEvents(events []interface{}) {
	for _, event := range events {
		switch ev := event.(type) {
		case types.ChainSnailEvent:
			bc.chainFeed.Send(ev)

		case types.ChainSnailHeadEvent:
			bc.chainHeadFeed.Send(ev)

		case types.ChainSnailSideEvent:
			bc.chainSideFeed.Send(ev)

		case types.NewFastBlocksEvent:
			bc.fastBlockFeed.Send(ev)

		case types.NewMinedFruitEvent:
			bc.fruitFeed.Send(ev)

		}
	}
}

func (bc *SnailBlockChain) update() {
	futureTimer := time.NewTicker(5 * time.Second)
	defer futureTimer.Stop()
	for {
		select {
		case <-futureTimer.C:
			bc.procFutureBlocks()
		case <-bc.quit:
			return
		}
	}
}

// BadBlocks returns a list of the last 'bad blocks' that the client has seen on the network
func (bc *SnailBlockChain) BadBlocks() []*types.SnailBlock {
	blocks := make([]*types.SnailBlock, 0, bc.badBlocks.Len())
	for _, hash := range bc.badBlocks.Keys() {
		if blk, exist := bc.badBlocks.Peek(hash); exist {
			block := blk.(*types.SnailBlock)
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// addBadBlock adds a bad block to the bad-block LRU cache
func (bc *SnailBlockChain) addBadBlock(block *types.SnailBlock) {
	bc.badBlocks.Add(block.Hash(), block)
}

// reportBlock logs a bad block error.
func (bc *SnailBlockChain) reportBlock(block *types.SnailBlock, err error) {
	bc.addBadBlock(block)

	log.Error(fmt.Sprintf(`
########## BAD SNAIL BLOCK #########
Chain config: %v

Number: %v
Hash: 0x%x

Error: %v
##############################
`, bc.chainConfig, block.Number(), block.Hash(), err))
}

// InsertHeaderChain attempts to insert the given header chain in to the local
// chain, possibly creating a reorg. If an error is returned, it will return the
// index number of the failing header as well an error describing what went wrong.
//
// The verify parameter can be used to fine tune whether nonce verification
// should be done or not. The reason behind the optional check is because some
// of the header retrieval mechanisms already need to verify nonces, as well as
// because nonces can be verified sparsely, not needing to check each.
func (bc *SnailBlockChain) InsertHeaderChain(chain []*types.SnailHeader, checkFreq int) (int, error) {
	start := time.Now()
	if i, err := bc.hc.ValidateHeaderChain(chain, checkFreq); err != nil {
		return i, err
	}

	// Make sure only one thread manipulates the chain at once
	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	bc.wg.Add(1)
	defer bc.wg.Done()

	whFunc := func(header *types.SnailHeader) error {
		_, err := bc.hc.WriteHeader(header)
		return err
	}

	return bc.hc.InsertHeaderChain(chain, whFunc, start)
}

// CurrentHeader retrieves the current head header of the canonical chain. The
// header is retrieved from the HeaderChain's internal cache.
func (bc *SnailBlockChain) CurrentHeader() *types.SnailHeader {
	return bc.hc.CurrentHeader()
}

// GetTd retrieves a block's total difficulty in the canonical chain from the
// database by hash and number, caching it if found.
func (bc *SnailBlockChain) GetTd(hash common.Hash, number uint64) *big.Int {
	return bc.hc.GetTd(hash, number)
}

// GetTdByHash retrieves a block's total difficulty in the canonical chain from the
// database by hash, caching it if found.
func (bc *SnailBlockChain) GetTdByHash(hash common.Hash) *big.Int {
	return bc.hc.GetTdByHash(hash)
}

// GetHeader retrieves a block header from the database by hash and number,
// caching it if found.
func (bc *SnailBlockChain) GetHeader(hash common.Hash, number uint64) *types.SnailHeader {
	return bc.hc.GetHeader(hash, number)
}

// GetHeaderByHash retrieves a block header from the database by hash, caching it if
// found.
func (bc *SnailBlockChain) GetHeaderByHash(hash common.Hash) *types.SnailHeader {
	return bc.hc.GetHeaderByHash(hash)
}

// HasHeader checks if a block header is present in the database or not, caching
// it if present.
func (bc *SnailBlockChain) HasHeader(hash common.Hash, number uint64) bool {
	return bc.hc.HasHeader(hash, number)
}

// GetBlockHashesFromHash retrieves a number of block hashes starting at a given
// hash, fetching towards the genesis block.
func (bc *SnailBlockChain) GetBlockHashesFromHash(hash common.Hash, max uint64) []common.Hash {
	return bc.hc.GetBlockHashesFromHash(hash, max)
}

// GetAncestor retrieves the Nth ancestor of a given block. It assumes that either the given block or
// a close ancestor of it is canonical. maxNonCanonical points to a downwards counter limiting the
// number of blocks to be individually checked before we reach the canonical chain.
//
// Note: ancestor == 0 returns the same block, 1 returns its parent and so on.
func (bc *SnailBlockChain) GetAncestor(hash common.Hash, number, ancestor uint64, maxNonCanonical *uint64) (common.Hash, uint64) {
	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	return bc.hc.GetAncestor(hash, number, ancestor, maxNonCanonical)
}

// GetHeaderByNumber retrieves a block header from the database by number,
// caching it (associated with its hash) if found.
func (bc *SnailBlockChain) GetHeaderByNumber(number uint64) *types.SnailHeader {
	return bc.hc.GetHeaderByNumber(number)
}

// GetFruitByFastHash retrieves a block from the database by FastHash
func (bc *SnailBlockChain) GetFruitByFastHash(fastHash common.Hash) (*types.SnailBlock, uint64) {
	fruit, hash, number, index := rawdb.ReadFruit(bc.db, fastHash)
	//log.Debug("Get fruit by fast hash", "fruit", fruit, "hash", hash, "number", number, "index", index, "fastHash",fastHash)

	if fruit == nil {
		return nil, 0
	}

	block := bc.GetBlock(hash, number)

	return block, index
}

// GetFruit retrieves a fruit from the database by FastHash
func (bc *SnailBlockChain) GetFruit(fastHash common.Hash) *types.SnailBlock {
	fruit, _, _, _ := rawdb.ReadFruit(bc.db, fastHash)
	return fruit
}

// Config retrieves the blockchain's chain configuration.
func (bc *SnailBlockChain) Config() *params.ChainConfig { return bc.chainConfig }

// Engine retrieves the blockchain's consensus engine.
func (bc *SnailBlockChain) Engine() consensus.Engine { return bc.engine }

// SubscribeChainEvent registers a subscription of ChainSnailEvent.
func (bc *SnailBlockChain) SubscribeChainEvent(ch chan<- types.ChainSnailEvent) event.Subscription {
	return bc.scope.Track(bc.chainFeed.Subscribe(ch))
}

// SubscribeChainHeadEvent registers a subscription of types.ChainSnailHeadEvent.
func (bc *SnailBlockChain) SubscribeChainHeadEvent(ch chan<- types.ChainSnailHeadEvent) event.Subscription {
	return bc.scope.Track(bc.chainHeadFeed.Subscribe(ch))
}

// SubscribeChainSideEvent registers a subscription of types.ChainSnailSideEvent.
func (bc *SnailBlockChain) SubscribeChainSideEvent(ch chan<- types.ChainSnailSideEvent) event.Subscription {
	return bc.scope.Track(bc.chainSideFeed.Subscribe(ch))
}

// SubscribeLogsEvent registers a subscription of []*types.Log.
func (bc *SnailBlockChain) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return bc.scope.Track(bc.logsFeed.Subscribe(ch))
}

// SubscribeFastBlockEvent registers a subscription of fruits.
func (bc *SnailBlockChain) SubscribeFastBlockEvent(ch chan<- types.NewFastBlocksEvent) event.Subscription {
	return bc.scope.Track(bc.fastBlockFeed.Subscribe(ch))
}

// SubscribeNewFruitEvent registers a subscription of fruits.
func (bc *SnailBlockChain) SubscribeNewFruitEvent(ch chan<- types.NewMinedFruitEvent) event.Subscription {
	return bc.scope.Track(bc.fruitFeed.Subscribe(ch))
}
