// Copyright 2016 The go-ethereum Authors
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

// Package light implements on-demand retrieval capable state and chain objects
// for the Ethereum Light Client.
package light

import (
	"context"
	"errors"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/light/fast"
	"github.com/truechain/truechain-engineering-code/light/public"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	bodyCacheLimit  = 256
	blockCacheLimit = 256
	fruitCacheLimit = 1024
)

// LightChain represents a canonical chain that by default only handles block
// headers, downloading block bodies and receipts on demand through an ODR
// interface. It only does header validation during chain insertion.
type LightChain struct {
	hc            *snailchain.HeaderChain
	fastchain     *fast.LightChain
	indexerConfig *public.IndexerConfig
	chainDb       etruedb.Database
	engine        consensus.Engine
	odr           OdrBackend
	chainFeed     event.Feed
	chainSideFeed event.Feed
	chainHeadFeed event.Feed
	scope         event.SubscriptionScope
	genesisBlock  *types.SnailBlock
	bodyCache     *lru.Cache // Cache for the most recent block bodies
	bodyRLPCache  *lru.Cache // Cache for the most recent block bodies in RLP encoded format
	blockCache    *lru.Cache // Cache for the most recent entire blocks
	fruitCache    *lru.Cache // Cache for the most recent entire fruits

	chainmu sync.RWMutex
	quit    chan struct{}
	wg      sync.WaitGroup

	// Atomic boolean switches:
	running          int32 // whether LightChain is running or stopped
	procInterrupt    int32 // interrupts chain insert
	disableCheckFreq int32 // disables header verification
}

// NewLightChain returns a fully initialised light chain using information
// available in the database. It initialises the default Ethereum header
// validator.
func NewLightChain(fastchain *fast.LightChain, odr OdrBackend, config *params.ChainConfig, engine consensus.Engine, checkpoint *params.TrustedCheckpoint) (*LightChain, error) {
	bodyCache, _ := lru.New(bodyCacheLimit)
	bodyRLPCache, _ := lru.New(bodyCacheLimit)
	blockCache, _ := lru.New(blockCacheLimit)
	fruitCache, _ := lru.New(fruitCacheLimit)

	bc := &LightChain{
		chainDb:       odr.Database(),
		indexerConfig: odr.IndexerConfig(),
		odr:           odr,
		quit:          make(chan struct{}),
		bodyCache:     bodyCache,
		bodyRLPCache:  bodyRLPCache,
		blockCache:    blockCache,
		engine:        engine,
		fruitCache:    fruitCache,
		fastchain:     fastchain,
	}
	var err error
	bc.hc, err = snailchain.NewHeaderChain(odr.Database(), config, bc.engine, bc.getProcInterrupt)
	if err != nil {
		return nil, err
	}
	bc.genesisBlock, _ = bc.GetBlockByNumber(NoOdr, 0)
	if bc.genesisBlock == nil {
		return nil, core.ErrNoGenesis
	}
	if checkpoint != nil {
		bc.AddTrustedCheckpoint(checkpoint)
	}
	if err := bc.loadLastState(); err != nil {
		return nil, err
	}
	// Check the current state of the block hashes and make sure that we do not have any of the bad blocks in our chain
	for hash := range core.BadHashes {
		if header := bc.GetHeaderByHash(hash); header != nil {
			log.Error("Found bad hash, rewinding chain", "number", header.Number, "hash", header.ParentHash)
			bc.SetHead(header.Number.Uint64() - 1)
			log.Error("Chain rewind was successful, resuming normal operation")
		}
	}
	return bc, nil
}

// AddTrustedCheckpoint adds a trusted checkpoint to the blockchain
func (lc *LightChain) AddTrustedCheckpoint(cp *params.TrustedCheckpoint) {
	if lc.odr.ChtIndexer() != nil {
		StoreChtRoot(lc.chainDb, cp.SectionIndex, cp.SectionHead, cp.CHTRoot)
		lc.odr.ChtIndexer().AddCheckpoint(cp.SectionIndex, cp.SectionHead)
		lc.odr.IndexerConfig().SetDatasetRoot(cp.DSRoot)
	}
	log.Info("Added trusted checkpoint", "block", (cp.SectionIndex+1)*lc.indexerConfig.ChtSize-1, "hash", cp.SectionHead)
}

func (lc *LightChain) getProcInterrupt() bool {
	return atomic.LoadInt32(&lc.procInterrupt) == 1
}

// Odr returns the ODR backend of the chain
func (lc *LightChain) Odr() OdrBackend {
	return lc.odr
}

// loadLastState loads the last known chain state from the database. This method
// assumes that the chain manager mutex is held.
func (lc *LightChain) loadLastState() error {
	if head := rawdb.ReadHeadHeaderHash(lc.chainDb); head == (common.Hash{}) {
		// Corrupt or empty database, init from scratch
		lc.Reset()
	} else {
		if header := lc.GetHeaderByHash(head); header != nil {
			lc.hc.SetCurrentHeader(header)
		}
	}

	// Issue a status log and return
	header := lc.hc.CurrentHeader()
	headerTd := lc.GetTd(header.Hash(), header.Number.Uint64())

	for i := header.Number.Uint64(); i > header.Number.Uint64()-uint64(12); i-- {
		head := lc.GetHeaderByNumber(i)
		if head == nil {
			break
		}
		td := rawdb.ReadTd(lc.chainDb, head.Hash(), head.Number.Uint64())
		log.Info("Read td", "td", td, "number", head.Number, "hash", head.Hash())
	}

	log.Info("Loaded most recent snail header", "number", header.Number, "hash", header.Hash(), "td", headerTd, "age", common.PrettyAge(time.Unix(header.Time.Int64(), 0)))

	return nil
}

// SetHead rewinds the local chain to a new head. Everything above the new
// head will be deleted and the new one set.
func (lc *LightChain) SetHead(head uint64) {
	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	lc.hc.SetHead(head, nil)
	lc.loadLastState()
}

// Reset purges the entire blockchain, restoring it to its genesis state.
func (lc *LightChain) Reset() {
	lc.ResetWithGenesisBlock(lc.genesisBlock)
}

// ResetWithGenesisBlock purges the entire blockchain, restoring it to the
// specified genesis state.
func (lc *LightChain) ResetWithGenesisBlock(genesis *types.SnailBlock) {
	// Dump the entire block chain and purge the caches
	lc.SetHead(0)

	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	// Prepare the genesis block and reinitialise the chain
	rawdb.WriteTd(lc.chainDb, genesis.Hash(), genesis.NumberU64(), genesis.Difficulty())
	rawdb.WriteBlock(lc.chainDb, genesis)

	lc.genesisBlock = genesis
	lc.hc.SetGenesis(lc.genesisBlock.Header())
	lc.hc.SetCurrentHeader(lc.genesisBlock.Header())
}

// Accessors

// Engine retrieves the light chain's consensus engine.
func (lc *LightChain) Engine() consensus.Engine { return lc.engine }

// Genesis returns the genesis block
func (lc *LightChain) Genesis() *types.SnailBlock {
	return lc.genesisBlock
}

// GetBody retrieves a block body (transactions and uncles) from the database
// or ODR service by hash, caching it if found.
func (lc *LightChain) GetBody(ctx context.Context, hash common.Hash) (*types.SnailBody, error) {
	// Short circuit if the body's already in the cache, retrieve otherwise
	if cached, ok := lc.bodyCache.Get(hash); ok {
		body := cached.(*types.SnailBody)
		return body, nil
	}
	number := lc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil, errors.New("unknown block")
	}
	body, err := GetBody(ctx, lc.odr, hash, *number)
	if err != nil {
		return nil, err
	}
	// Cache the found body for next time and return
	lc.bodyCache.Add(hash, body)
	return body, nil
}

// GetBodyRLP retrieves a block body in RLP encoding from the database or
// ODR service by hash, caching it if found.
func (lc *LightChain) GetBodyRLP(ctx context.Context, hash common.Hash) (rlp.RawValue, error) {
	// Short circuit if the body's already in the cache, retrieve otherwise
	if cached, ok := lc.bodyRLPCache.Get(hash); ok {
		return cached.(rlp.RawValue), nil
	}
	number := lc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil, errors.New("unknown block")
	}
	body, err := GetBodyRLP(ctx, lc.odr, hash, *number)
	if err != nil {
		return nil, err
	}
	// Cache the found body for next time and return
	lc.bodyRLPCache.Add(hash, body)
	return body, nil
}

// HasBlock checks if a block is fully present in the database or not, caching
// it if present.
func (lc *LightChain) HasBlock(hash common.Hash, number uint64) bool {
	blk, _ := lc.GetBlock(NoOdr, hash, number)
	return blk != nil
}

// GetBlock retrieves a block from the database or ODR service by hash and number,
// caching it if found.
func (lc *LightChain) GetBlock(ctx context.Context, hash common.Hash, number uint64) (*types.SnailBlock, error) {
	// Short circuit if the block's already in the cache, retrieve otherwise
	if block, ok := lc.blockCache.Get(hash); ok {
		return block.(*types.SnailBlock), nil
	}
	block, err := GetBlock(ctx, lc.odr, hash, number)
	if err != nil {
		return nil, err
	}
	// Cache the found block for next time and return
	lc.blockCache.Add(block.Hash(), block)
	return block, nil
}

// GetBlockByHash retrieves a block from the database or ODR service by hash,
// caching it if found.
func (lc *LightChain) GetBlockByHash(ctx context.Context, hash common.Hash) (*types.SnailBlock, error) {
	number := lc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil, errors.New("unknown block")
	}
	return lc.GetBlock(ctx, hash, *number)
}

// GetFruit retrieves a fruit from the database by FastHash
func (lc *LightChain) GetFruit(ctx context.Context, hash common.Hash) (*types.SnailBlock, error) {

	// Short circuit if the block's already in the cache, retrieve otherwise
	if fruit, ok := lc.fruitCache.Get(hash); ok {
		return fruit.(*types.SnailBlock), nil
	}

	fruit, err := GetFruit(ctx, lc.odr, hash, lc.fastchain.GetHeaderByHash(hash).Number.Uint64())
	if err != nil {
		return nil, err
	}
	// Cache the found block for next time and return
	lc.fruitCache.Add(fruit.Hash(), fruit)
	return fruit, nil
}

// GetBlockByNumber retrieves a block from the database or ODR service by
// number, caching it (associated with its hash) if found.
func (lc *LightChain) GetBlockByNumber(ctx context.Context, number uint64) (*types.SnailBlock, error) {
	hash, err := GetCanonicalHash(ctx, lc.odr, number)
	if hash == (common.Hash{}) || err != nil {
		return nil, err
	}
	return lc.GetBlock(ctx, hash, number)
}

// Stop stops the blockchain service. If any imports are currently in progress
// it will abort them using the procInterrupt.
func (lc *LightChain) Stop() {
	if !atomic.CompareAndSwapInt32(&lc.running, 0, 1) {
		return
	}
	close(lc.quit)
	atomic.StoreInt32(&lc.procInterrupt, 1)

	lc.wg.Wait()
	log.Info("Blockchain manager stopped")
}

// Rollback is designed to remove a chain of links from the database that aren't
// certain enough to be valid.
func (lc *LightChain) Rollback(chain []common.Hash) {
	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	for i := len(chain) - 1; i >= 0; i-- {
		hash := chain[i]

		if head := lc.hc.CurrentHeader(); head.Hash() == hash {
			lc.hc.SetCurrentHeader(lc.GetHeader(head.ParentHash, head.Number.Uint64()-1))
		}
	}
}

// postChainEvents iterates over the events generated by a chain insertion and
// posts them into the event feed.
func (lc *LightChain) postChainEvents(events []interface{}) {
	for _, event := range events {
		switch ev := event.(type) {
		case types.SnailChainEvent:
			if lc.CurrentHeader().Hash() == ev.Hash {
				lc.chainHeadFeed.Send(types.SnailChainHeadEvent{Block: ev.Block})
			}
			lc.chainFeed.Send(ev)
		case types.SnailChainSideEvent:
			lc.chainSideFeed.Send(ev)
		}
	}
}

// InsertHeaderChain attempts to insert the given header chain in to the local
// chain, possibly creating a reorg. If an error is returned, it will return the
// index number of the failing header as well an error describing what went wrong.
//
// The verify parameter can be used to fine tune whether nonce verification
// should be done or not. The reason behind the optional check is because some
// of the header retrieval mechanisms already need to verfy nonces, as well as
// because nonces can be verified sparsely, not needing to check each.
//
// In the case of a light chain, InsertHeaderChain also creates and posts light
// chain events when necessary.
func (lc *LightChain) InsertHeaderChain(chain []*types.SnailHeader, fruitHeads [][]*types.SnailHeader, checkFreq int) (int, error) {
	if atomic.LoadInt32(&lc.disableCheckFreq) == 1 {
		checkFreq = 0
	}
	start := time.Now()
	if i, err := lc.hc.ValidateHeaderChain(chain, fruitHeads, checkFreq, lc.fastchain.GetHeaderChain(), rawdb.ReadLightCheckPoint(lc.chainDb)); err != nil {
		return i, err
	}

	// Make sure only one thread manipulates the chain at once
	lc.chainmu.Lock()
	defer lc.chainmu.Unlock()

	lc.wg.Add(1)
	defer lc.wg.Done()

	var events []interface{}
	whFunc := func(header *types.SnailHeader, fruitHeads []*types.SnailHeader) error {

		status, err := lc.hc.WriteHeader(header, fruitHeads)

		switch status {
		case snailchain.CanonStatTy:
			log.Debug("Inserted new snail header", "number", header.Number, "td", lc.hc.GetTd(header.Hash(), header.Number.Uint64()), "fruits", len(fruitHeads), "hash", header.Hash().String())
			events = append(events, types.SnailChainEvent{Block: types.NewSnailBlockWithHeader(header), Hash: header.Hash()})

		case snailchain.SideStatTy:
			log.Debug("Inserted forked header", "number", header.Number, "hash", header.Hash())
			events = append(events, types.SnailChainSideEvent{Block: types.NewSnailBlockWithHeader(header)})
		}
		return err
	}
	i, err := lc.hc.InsertHeaderChain(chain, fruitHeads, whFunc, start)
	lc.postChainEvents(events)
	return i, err
}

// CurrentHeader retrieves the current head header of the canonical chain. The
// header is retrieved from the HeaderChain's internal cache.
func (lc *LightChain) CurrentHeader() *types.SnailHeader {
	return lc.hc.CurrentHeader()
}

// GetTd retrieves a block's total difficulty in the canonical chain from the
// database by hash and number, caching it if found.
func (lc *LightChain) GetTd(hash common.Hash, number uint64) *big.Int {
	return lc.hc.GetTd(hash, number)
}

// GetTdByHash retrieves a block's total difficulty in the canonical chain from the
// database by hash, caching it if found.
func (lc *LightChain) GetTdByHash(hash common.Hash) *big.Int {
	return lc.hc.GetTdByHash(hash)
}

// GetHeader retrieves a block header from the database by hash and number,
// caching it if found.
func (lc *LightChain) GetHeader(hash common.Hash, number uint64) *types.SnailHeader {
	return lc.hc.GetHeader(hash, number)
}

// GetHeaderByHash retrieves a block header from the database by hash, caching it if
// found.
func (lc *LightChain) GetHeaderByHash(hash common.Hash) *types.SnailHeader {
	return lc.hc.GetHeaderByHash(hash)
}

// GetHeaderByHash retrieves a block header from the database by hash, caching it if
// found.
func (lc *LightChain) GetFruitHeaderByHash(hash common.Hash) *types.SnailHeader {
	fruitHead, _, _, _ := rawdb.ReadFruitHead(lc.chainDb, hash)
	return fruitHead
}

func (lc *LightChain) GetFruitsHead(number uint64) []*types.SnailHeader {
	hash := rawdb.ReadCanonicalHash(lc.chainDb, number)
	if hash == (common.Hash{}) {
		return nil
	}
	heads := rawdb.ReadFruitsHead(lc.chainDb, hash, number)
	return heads
}

// HasHeader checks if a block header is present in the database or not, caching
// it if present.
func (lc *LightChain) HasHeader(hash common.Hash, number uint64) bool {
	return lc.hc.HasHeader(hash, number)
}

// GetBlockHashesFromHash retrieves a number of block hashes starting at a given
// hash, fetching towards the genesis block.
func (lc *LightChain) GetBlockHashesFromHash(hash common.Hash, max uint64) []common.Hash {
	return lc.hc.GetBlockHashesFromHash(hash, max)
}

// GetAncestor retrieves the Nth ancestor of a given block. It assumes that either the given block or
// a close ancestor of it is canonical. maxNonCanonical points to a downwards counter limiting the
// number of blocks to be individually checked before we reach the canonical chain.
//
// Note: ancestor == 0 returns the same block, 1 returns its parent and so on.
func (lc *LightChain) GetAncestor(hash common.Hash, number, ancestor uint64, maxNonCanonical *uint64) (common.Hash, uint64) {
	lc.chainmu.RLock()
	defer lc.chainmu.RUnlock()

	return lc.hc.GetAncestor(hash, number, ancestor, maxNonCanonical)
}

// GetHeaderByNumber retrieves a block header from the database by number,
// caching it (associated with its hash) if found.
func (lc *LightChain) GetHeaderByNumber(number uint64) *types.SnailHeader {
	return lc.hc.GetHeaderByNumber(number)
}

// GetHeaderByNumberOdr retrieves a block header from the database or network
// by number, caching it (associated with its hash) if found.
func (lc *LightChain) GetHeaderByNumberOdr(ctx context.Context, number uint64) (*types.SnailHeader, error) {
	if header := lc.hc.GetHeaderByNumber(number); header != nil {
		return header, nil
	}
	return GetHeaderByNumber(ctx, lc.odr, number)
}

// Config retrieves the header chain's chain configuration.
func (lc *LightChain) Config() *params.ChainConfig { return lc.hc.Config() }

func (lc *LightChain) SyncCht(ctx context.Context) bool {
	// If we don't have a CHT indexer, abort
	if lc.odr.ChtIndexer() == nil {
		return false
	}
	// Ensure the remote CHT head is ahead of us
	head := lc.CurrentHeader().Number.Uint64()
	sections, _, _ := lc.odr.ChtIndexer().Sections()

	latest := sections*lc.indexerConfig.ChtSize - 1
	if head >= latest {
		return false
	}
	// Retrieve the latest useful header and update to it
	if header, err := GetHeaderByNumber(ctx, lc.odr, latest); header != nil && err == nil {
		lc.chainmu.Lock()
		defer lc.chainmu.Unlock()

		// Ensure the chain didn't move past the latest block while retrieving it
		if lc.hc.CurrentHeader().Number.Uint64() < header.Number.Uint64() {
			log.Info("Updated latest header based on CHT", "number", header.Number, "hash", header.Hash(), "age", common.PrettyAge(time.Unix(header.Time.Int64(), 0)))
			lc.hc.SetCurrentHeader(header)
			lc.fastchain.LoadLastState()
		}
		return true
	}
	return false
}

// LockChain locks the chain mutex for reading so that multiple canonical hashes can be
// retrieved while it is guaranteed that they belong to the same version of the chain
func (lc *LightChain) LockChain() {
	lc.chainmu.RLock()
}

// UnlockChain unlocks the chain mutex
func (lc *LightChain) UnlockChain() {
	lc.chainmu.RUnlock()
}

// SubscribeChainEvent registers a subscription of ChainEvent.
func (lc *LightChain) SubscribeChainEvent(ch chan<- types.SnailChainEvent) event.Subscription {
	return lc.scope.Track(lc.chainFeed.Subscribe(ch))
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (lc *LightChain) SubscribeChainHeadEvent(ch chan<- types.SnailChainHeadEvent) event.Subscription {
	return lc.scope.Track(lc.chainHeadFeed.Subscribe(ch))
}

// SubscribeChainSideEvent registers a subscription of ChainSideEvent.
func (lc *LightChain) SubscribeChainSideEvent(ch chan<- types.SnailChainSideEvent) event.Subscription {
	return lc.scope.Track(lc.chainSideFeed.Subscribe(ch))
}

// DisableCheckFreq disables header validation. This is used for ultralight mode.
func (lc *LightChain) DisableCheckFreq() {
	atomic.StoreInt32(&lc.disableCheckFreq, 1)
}

// EnableCheckFreq enables header validation.
func (lc *LightChain) EnableCheckFreq() {
	atomic.StoreInt32(&lc.disableCheckFreq, 0)
}

// GetHeaderChain loads the last known chain state from the database. This method
func (lc *LightChain) GetHeaderChain() *snailchain.HeaderChain {
	return lc.hc
}
