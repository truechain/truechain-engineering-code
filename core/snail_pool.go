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

package core

import (
	"errors"
	"math"
	"math/big"
	"sync"
	"time"

	"container/list"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	//"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	//recordChanSize = 100
	fastBlockChanSize = 100
	fruitChanSize  = 100
)

// freshFruitSize is the freshness of fruit according to the paper
var fruitFreshness *big.Int = big.NewInt(17)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSign = errors.New("invalid sign")

	ErrInvalidPointer = errors.New("invalid pointer block")

	ErrExist = errors.New("already exist")

	ErrNotExist = errors.New("not exist")

	ErrInvalidHash = errors.New("invalid hash")

	ErrFreshness = errors.New("fruit not fresh")
)

// TxPoolConfig are the configuration parameters of the transaction pool.
type HybridPoolConfig struct {
	NoLocals  bool          // Whether local transaction handling should be disabled
	Journal   string        // Journal of local transactions to survive node restarts
	Rejournal time.Duration // Time interval to regenerate the local transaction journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Minimum number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

// DefaultTxPoolConfig contains the default configurations for the transaction
// pool.
var DefaultHybridPoolConfig = HybridPoolConfig{
	//Journal:   "records.rlp",
	Journal:   "fastBlocks.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}

// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (config *HybridPoolConfig) sanitize() HybridPoolConfig {
	conf := *config
	if conf.Rejournal < time.Second {
		log.Warn("Sanitizing invalid txpool journal time", "provided", conf.Rejournal, "updated", time.Second)
		conf.Rejournal = time.Second
	}
	if conf.PriceLimit < 1 {
		log.Warn("Sanitizing invalid txpool price limit", "provided", conf.PriceLimit, "updated", DefaultTxPoolConfig.PriceLimit)
		conf.PriceLimit = DefaultHybridPoolConfig.PriceLimit
	}
	if conf.PriceBump < 1 {
		log.Warn("Sanitizing invalid txpool price bump", "provided", conf.PriceBump, "updated", DefaultTxPoolConfig.PriceBump)
		conf.PriceBump = DefaultHybridPoolConfig.PriceBump
	}
	return conf
}

// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
type SnailPool struct {
	config      HybridPoolConfig
	chainconfig *params.ChainConfig
	//chain       *BlockChain
	chain       *snailchain.SnailBlockChain
	gasPrice    *big.Int

	scope event.SubscriptionScope

	fruitFeed  event.Feed
	//recordFeed event.Feed
	fastBlockFeed event.Feed
	mu sync.RWMutex

	//chainHeadCh  chan ChainHeadEvent
	chainHeadCh  chan snailchain.ChainHeadEvent
	chainHeadSub event.Subscription

	signer types.Signer

	currentState  *state.StateDB      // Current state in the blockchain head
	pendingState  *state.ManagedState // Pending state tracking virtual nonces
	currentMaxGas uint64              // Current gas limit for transaction caps

	engine consensus.Engine // Consensus engine used for validating

	muFruit  sync.RWMutex
	//muRecord sync.RWMutex
	muFastBlock sync.RWMutex

	//allRecords    map[common.Hash]*types.PbftRecord
	allFastBlocks    map[common.Hash]*types.FastBlock

	//fruitRecords  map[common.Hash]*types.PbftRecord // the records have fruit
	fruitFastBlocks  map[common.Hash]*types.FastBlock // the fastBlocks have fruit
	//recordList    *list.List
	fastBlockList    *list.List
	//recordPending *list.List
	fastBlockPending *list.List
	//newRecordCh   chan *types.PbftRecord
	newFastBlockCh   chan *types.FastBlock

	allFruits    map[common.Hash]*types.SnailBlock
	fruitPending map[common.Hash]*types.SnailBlock

	newFruitCh chan *types.SnailBlock

	//header *types.Block
	header *types.SnailBlock

	gasUsed uint64
	gasPool *GasPool // available gas used to pack transactions

	wg sync.WaitGroup // for shutdown sync

	homestead bool
}

// NewTxPool creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewSnailPool(chainconfig *params.ChainConfig, chain *snailchain.SnailBlockChain) *SnailPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	//config = (&config).sanitize()
	config := DefaultHybridPoolConfig

	// Create the transaction pool with its initial settings
	pool := &SnailPool{
		config:      config,
		chainconfig: chainconfig,
		chain:       chain,
		signer:      types.NewEIP155Signer(chainconfig.ChainID),
		chainHeadCh: make(chan snailchain.ChainHeadEvent, chainHeadChanSize),
		gasPrice:    new(big.Int).SetUint64(config.PriceLimit),

		//newRecordCh: make(chan *types.PbftRecord, recordChanSize),
		newFastBlockCh: make(chan *types.FastBlock, fastBlockChanSize),

		//allRecords:    make(map[common.Hash]*types.PbftRecord),
		allFastBlocks:    make(map[common.Hash]*types.FastBlock),
		//recordList:    list.New(),
		fastBlockList: 	list.New(),
		fastBlockPending:  list.New(),

		newFruitCh: make(chan *types.SnailBlock, fruitChanSize),
		allFruits:  make(map[common.Hash]*types.SnailBlock),
		fruitPending:make(map[common.Hash]*types.SnailBlock),
	}
	pool.reset(nil, chain.CurrentBlock())

	// If local transactions and journaling is enabled, load from disk
	if !config.NoLocals && config.Journal != "" {
	}
	// Subscribe events from blockchain
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	//pool.minedFruitSub = pool.eventMux.Subscribe(NewMinedFruitEvent{})

	pool.header = pool.chain.CurrentBlock()
	statedb, err := pool.chain.StateAt(pool.header.Root())
	if err != nil {
		log.Error("Failed to reset pool state", "err", err)
		return nil
	}
	pool.currentState = statedb
	pool.pendingState = state.ManageState(statedb)
	//pool.currentMaxGas = pool.header.GasLimit()

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()

	//eth.NewRecord(pool)
	return pool
}

/*func (pool *SnailPool) getRecord(hash common.Hash, number *big.Int) *types.PbftRecord {
	pool.muRecord.Lock()
	defer pool.muRecord.Unlock()

	for lr := pool.recordPending.Front(); lr != nil; lr = lr.Next() {
		r := lr.Value.(*types.PbftRecord)
		if r.Number().Cmp(number) > 0 {
			// rest records are greater than number
			return nil
		} else if r.Number().Cmp(number) == 0 {
			if r.Hash() != hash {
				return nil
			} else {
				return r
			}
		}
	}

	return nil
}*/

func (pool *SnailPool) getFastBlock(hash common.Hash, number *big.Int) *types.FastBlock {
	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()

	for lr := pool.fastBlockPending.Front(); lr != nil; lr = lr.Next() {
		r := lr.Value.(*types.FastBlock)
		if r.Number().Cmp(number) > 0 {
			// rest records are greater than number
			return nil
		} else if r.Number().Cmp(number) == 0 {
			if r.Hash() != hash {
				return nil
			} else {
				return r
			}
		}
	}

	return nil
}

// move the fruit of execute record to pending list
func (pool *SnailPool) updateFruit(fastBlock *types.FastBlock, toLock bool) error {
	if toLock {
		pool.muFruit.Lock()
		defer pool.muFruit.Unlock()
	}

	f := pool.allFruits[fastBlock.Hash()]
	if f == nil {
		return ErrNotExist
	} else {
		if f.TxHash() != fastBlock.TxHash() {
			// fruit txs is invalid
			delete(pool.allFruits, fastBlock.Hash())
			delete(pool.fruitPending, fastBlock.Hash())
			return ErrInvalidHash
		} else {
			pool.fruitPending[fastBlock.Hash()] = f
		}
	}
	return nil
}

// add
func (pool *SnailPool) addFruit(fruit *types.SnailBlock) error {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	//check
	r := pool.getFastBlock(fruit.FastHash(), fruit.FastNumber())
	f := pool.allFruits[fruit.FastHash()]
	if f == nil {
		pool.allFruits[fruit.FastHash()] = fruit
		if r != nil {
			pool.muFastBlock.Lock()
			pool.removeFastBlockWithLock(pool.fastBlockPending, fruit.FastHash())
			pool.muFastBlock.Unlock()
			pool.fruitPending[fruit.FastHash()] = fruit
		}

		return nil
	} else {
		if fruit.Hash().Big().Cmp(f.Hash().Big()) > 0 {
			// new fruit hash is greater than old one
			return nil
		} else {
			pool.allFruits[fruit.FastHash()] = fruit
			if r != nil {
				pool.muFastBlock.Lock()
				pool.removeFastBlockWithLock(pool.fastBlockPending, fruit.FastHash())
				pool.muFastBlock.Unlock()
				pool.fruitPending[fruit.FastHash()] = fruit
			} else if _,ok := pool.fruitPending[fruit.FastHash()]; ok {
				pool.fruitPending[fruit.FastHash()] = fruit
			}
		}
	}

	return nil
}

func (pool *SnailPool) commitTransaction(tx *types.Transaction, coinbase common.Address, gp *GasPool, gasUsed *uint64) (error, *types.Receipt) {
	//snap := pool.currentState.Snapshot()

	// TODO: commit tx
	//return nil, nil

	//receipt, _, err := ApplyTransaction(pool.chainconfig, pool.chain, &coinbase, gp, pool.currentState, pool.header.Header(), tx, gasUsed, vm.Config{})
	/*if err != nil {
		//pool.currentState.RevertToSnapshot(snap)
		log.Info("apply transaction field ", "err", err.Error())
		return err, nil
	}*/

	return nil, nil
}

/*func (pool *SnailPool) commitRecord(record *types.PbftRecord, coinbase common.Address) (error, []*types.Receipt) {
	var receipts []*types.Receipt

	if pool.gasPool == nil {
		pool.gasPool = new(GasPool).AddGas(pool.header.GasLimit())
	}
	//return nil, nil

	snap := pool.currentState.Snapshot()

	for _, tx := range record.Transactions() {
		err, receipt := pool.commitTransaction(tx, coinbase, pool.gasPool, &pool.gasUsed)
		if err != nil {
			pool.currentState.RevertToSnapshot(snap)
			return err, nil
		}
		receipts = append(receipts, receipt)
	}

	return nil, receipts
}*/

func (pool *SnailPool) commitFastBlock(fastBlock *types.FastBlock, coinbase common.Address) (error, []*types.Receipt) {
	var receipts []*types.Receipt

	/*if pool.gasPool == nil {
		pool.gasPool = new(GasPool).AddGas(pool.header.GasLimit())
	}*/
	//return nil, nil

	snap := pool.currentState.Snapshot()

	for _, tx := range fastBlock.Transactions() {
		err, receipt := pool.commitTransaction(tx, coinbase, pool.gasPool, &pool.gasUsed)
		if err != nil {
			pool.currentState.RevertToSnapshot(snap)
			return err, nil
		}
		receipts = append(receipts, receipt)
	}

	return nil, receipts
}

// re execute records whose number are larger than the give on
// maybe they can execute now
/*func (pool *SnailPool) updateRecordsWithLock(number *big.Int, lockFruits bool) {
	// TODO:
	var remove []*list.Element
	for lr := pool.recordList.Front(); lr != nil; lr = lr.Next() {
		r := lr.Value.(*types.PbftRecord)
		if r == nil {
			return
		} else if number.Cmp(r.Number()) > 0 {
			continue
		} else {
			err, _ := pool.commitRecord(r, common.Address{})
			if err == nil {
				errf := pool.updateFruit(r, lockFruits)
				if errf != nil {
					pool.insertRecordWithLock(pool.recordPending, r)
					var records []*types.PbftRecord
					records = append(records, r)
					go pool.recordFeed.Send(NewRecordsEvent{records})
				}

				remove = append(remove, lr)
			} else {
				break
			}
		}
	}

	for _, e := range remove {
		pool.recordList.Remove(e)
	}
}*/


func (pool *SnailPool) updateFastBlocksWithLock(number *big.Int, lockFruits bool) {
	// TODO:
	var remove []*list.Element
	for lr := pool.fastBlockList.Front(); lr != nil; lr = lr.Next() {
		f := lr.Value.(*types.FastBlock)
		if f == nil {
			return
		} else if number.Cmp(f.Number()) > 0 {
			continue
		} else {
			err, _ := pool.commitFastBlock(f, common.Address{})
			if err == nil {
				errf := pool.updateFruit(f, lockFruits)
				if errf != nil {
					pool.insertFastBlockWithLock(pool.fastBlockPending, f)
					var fastBlocks []*types.FastBlock
					fastBlocks = append(fastBlocks, f)
					go pool.fastBlockFeed.Send(snailchain.NewFastBlocksEvent{fastBlocks})
				}

				remove = append(remove, lr)
			} else {
				break
			}
		}
	}

	for _, e := range remove {
		pool.fastBlockList.Remove(e)
	}
}


/*func (pool *SnailPool) addRecord(record *types.PbftRecord) error {
	pool.muRecord.Lock()
	defer pool.muRecord.Unlock()

	//check
	f := pool.allRecords[record.Hash()]
	if f != nil {
		return ErrExist
	}

	pool.allRecords[record.Hash()] = record

	// TODO: execute all the txs in the record
	err, _ := pool.commitRecord(record, common.Address{})
	if err != nil {
		pool.insertRecordWithLock(pool.recordList, record)
	} else {
		err := pool.updateFruit(record, true)
		if err != nil {
			// insert pending list to send to mine
			pool.insertRecordWithLock(pool.recordPending, record)
		}
        var records []*types.PbftRecord
		records = append(records, record)
		go pool.recordFeed.Send(NewRecordsEvent{records})

		pool.updateRecordsWithLock(record.Number(), true)
	}

	return nil
}*/

func (pool *SnailPool) addFastBlock(fastBlock *types.FastBlock) error {
	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()

	//check
	f := pool.allFastBlocks[fastBlock.Hash()]
	if f != nil {
		return ErrExist
	}

	pool.allFastBlocks[fastBlock.Hash()] = fastBlock

	// TODO: execute all the txs in the record
	err, _ := pool.commitFastBlock(fastBlock, common.Address{})
	if err != nil {
		pool.insertFastBlockWithLock(pool.fastBlockList, fastBlock)
	} else {
		err := pool.updateFruit(fastBlock, true)
		if err != nil {
			// insert pending list to send to mine
			pool.insertFastBlockWithLock(pool.fastBlockPending, fastBlock)
		}
		var fastBlocks []*types.FastBlock
		fastBlocks = append(fastBlocks, fastBlock)
		go pool.fastBlockFeed.Send(snailchain.NewFastBlocksEvent{fastBlocks})

		pool.updateFastBlocksWithLock(fastBlock.Number(), true)
	}

	return nil
}

// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *SnailPool) loop() {
	defer pool.wg.Done()

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	journal := time.NewTicker(pool.config.Rejournal)
	defer journal.Stop()

	// Track the previous head headers for transaction reorgs
	head := pool.chain.CurrentBlock()

	// Keep waiting for and reacting to the various events
	for {
		select {
		// Handle ChainHeadEvent
		case ev := <-pool.chainHeadCh:
			if ev.Block != nil {
				pool.mu.Lock()
				if pool.chainconfig.IsHomestead(ev.Block.Number()) {
					pool.homestead = true
				}
				pool.reset(head, ev.Block)
				head = ev.Block

				pool.mu.Unlock()
			}

		case fruit := <-pool.newFruitCh:
			if fruit != nil {
				pool.addFruit(fruit)
			}

		/*case record := <-pool.newRecordCh:
			if record != nil {
				pool.addRecord(record)
			}*/
		case fastBlock := <-pool.newFastBlockCh:
			if fastBlock != nil {
				pool.addFastBlock(fastBlock)
			}

			// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return

			// Handle stats reporting ticks
		case <-report.C:
			// TODO: pool report

			// Handle inactive account transaction eviction
		case <-evict.C:
			pool.mu.Lock()
			pool.mu.Unlock()

			// Handle local transaction journal rotation
		case <-journal.C:
			// TODO: support journal

		}
	}
}

// TxDifference returns a new set t which is the difference between a to b.
func fruitsDifference(a, b []*types.SnailBlock) []*types.SnailBlock {
	keep := make([]*types.SnailBlock, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, f := range b {
		remove[f.Hash()] = struct{}{}
	}

	for _, f := range a {
		if _, ok := remove[f.Hash()]; !ok {
			keep = append(keep, f)
		}
	}

	return keep
}

// remove the record from pending list and unexecutable list
/*func (pool *SnailPool) removeRecordWithLock(fastBlockList *list.List, hash common.Hash) {
	for e := fastBlockList.Front(); e != nil; e = e.Next() {
		r := e.Value.(*types.PbftRecord)
		if r.Hash() == hash {
			fastBlockList.Remove(e)
			break
		}
	}
}*/

// remove the record from pending list and unexecutable list
func (pool *SnailPool) removeFastBlockWithLock(fastBlockList *list.List, hash common.Hash) {
	for e := fastBlockList.Front(); e != nil; e = e.Next() {
		r := e.Value.(*types.FastBlock)
		if r.Hash() == hash {
			fastBlockList.Remove(e)
			break
		}
	}
}

// remove all the fruits and fastBlocks included in the new block
func (pool *SnailPool) removeWithLock(fruits []*types.SnailBlock) {
	for _, fruit := range fruits {
		delete(pool.fruitPending, fruit.FastHash())
		delete(pool.allFruits, fruit.FastHash())

		if _, ok := pool.allFastBlocks[fruit.FastHash()]; ok {
			pool.removeFastBlockWithLock(pool.fastBlockList, fruit.FastHash())
			pool.removeFastBlockWithLock(pool.fastBlockPending, fruit.FastHash())
			delete(pool.allFastBlocks, fruit.FastHash())
		}
	}
}


/*func (pool *SnailPool) resetRecordsWithLock() {
	pool.fruitPending = make(map[common.Hash]*types.SnailBlock)

	pool.fastBlockList = list.New()
	pool.fastBlockPending = list.New()

	for _, record := range pool.allFastBlocks {
		pool.insertFastBlockWithLock(pool.fastBlockList, record)
	}
}*/

func (pool *SnailPool) resetFastBlocksWithLock() {
	pool.fruitPending = make(map[common.Hash]*types.SnailBlock)

	pool.fastBlockList = list.New()
	pool.fastBlockPending = list.New()

	for _, fastBlock := range pool.allFastBlocks {
		pool.insertFastBlockWithLock(pool.fastBlockList, fastBlock)
	}
}

// reset retrieves the current state of the blockchain and ensures the content
// of the transaction pool is valid with regard to the chain state.
func (pool *SnailPool) reset(oldHead, newHead *types.SnailBlock) {
	// If we're reorging an old state, reinject all dropped transactions
	var reinject []*types.SnailBlock

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash() {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.Number().Uint64()
		newNum := newHead.Number().Uint64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all transactions into memory
			var discarded, included []*types.SnailBlock

			var (
				rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.Number().Uint64())
				add = pool.chain.GetBlock(newHead.Hash(), newHead.Number().Uint64())
			)
			for rem.NumberU64() > add.NumberU64() {
				discarded = append(discarded, rem.Fruits()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number(), "hash", oldHead.Hash())
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				included = append(included, add.Fruits()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number(), "hash", newHead.Hash())
					return
				}
			}
			for rem.Hash() != add.Hash() {
				discarded = append(discarded, rem.Fruits()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by tx pool", "block", oldHead.Number(), "hash", oldHead.Hash())
					return
				}
				included = append(included, add.Fruits()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by tx pool", "block", newHead.Number(), "hash", newHead.Hash())
					return
				}
			}
			reinject = fruitsDifference(discarded, included)
		}
	}
	// Initialize the internal state to the current head
	if newHead == nil {
		newHead = pool.chain.CurrentBlock() // Special case during testing
	}
	statedb, err := pool.chain.StateAt(newHead.Root())
	if err != nil {
		log.Error("Failed to reset txpool state", "err", err)
		return
	}
	pool.currentState = statedb
	pool.pendingState = state.ManageState(statedb)
	//pool.currentMaxGas = newHead.GasLimit()

	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))

	// validate the pool of pending transactions, this will remove
	// any transactions that have been included in the block or
	// have been invalidated because of another transaction (e.g.
	// higher gas price)
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()

	pool.removeWithLock(newHead.Fruits())

	// reset pool state by re-verify all the records, including pending records
	// TODO: reset pool state using pendingState, refer to tx_pool
	pool.resetFastBlocksWithLock()

	// Check the queue and move transactions over to the pending if possible
	// or remove those that have become invalid
	pool.updateFastBlocksWithLock(common.Big0, false)
}

// Stop terminates the transaction pool.
func (pool *SnailPool) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

	// TODO: journal close
	//if pool.journal != nil {
	//	pool.journal.close()
	//}
	log.Info("Transaction pool stopped")
}

// GasPrice returns the current gas price enforced by the transaction pool.
func (pool *SnailPool) GasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return new(big.Int).Set(pool.gasPrice)
}

// State returns the virtual managed state of the transaction pool.
func (pool *SnailPool) State() *state.ManagedState {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return pool.pendingState
}

// AddLocals enqueues a batch of transactions into the pool if they are valid,
// marking the senders as a local ones in the mean time, ensuring they go around
// the local pricing constraints.
func (pool *SnailPool) AddRemoteFruits(fruits []*types.SnailBlock) []error {

	errs := make([]error, len(fruits))

	// TODO: check fruits
	for i, fruit := range fruits {
		if err := pool.validateFruit(fruit); err != nil {
			errs[i] = err
			continue
		}

		f := types.CopyFruit(fruit)
		pool.newFruitCh <- f
	}

	return errs
}

// Pending retrieves all currently processable allFruits, sorted by record number.
// The returned fruit set is a copy and can be freely modified by calling code.
func (pool *SnailPool) PendingFruits() (map[common.Hash]*types.SnailBlock, error) {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	pending := make(map[common.Hash]*types.SnailBlock)
	for addr, fruit := range pool.fruitPending {
		pending[addr] = types.CopyFruit(fruit)
	}

	return pending, nil
}

// SubscribeNewFruitsEvent registers a subscription of NewFruitEvent and
// starts sending event to the given channel.
func (pool *SnailPool) SubscribeNewFruitEvent(ch chan<- snailchain.NewFruitsEvent) event.Subscription {
	return pool.scope.Track(pool.fruitFeed.Subscribe(ch))
}

// Insert record into list order by record number
/*func (pool *SnailPool) insertRecordWithLock(recordList *list.List, record *types.PbftRecord) error {

	log.Info("++insert record pending", "number", record.Number(), "hash", record.Hash())

	for lr := recordList.Front(); lr != nil; lr = lr.Next() {
		r := lr.Value.(*types.PbftRecord)
		if r.Number().Cmp(record.Number()) > 0 {
			recordList.InsertBefore(record, lr)
			return nil
		}
	}
	recordList.PushBack(record)

	return nil
}*/

// Insert record into list order by record number
func (pool *SnailPool) insertFastBlockWithLock(fastBlockList *list.List, fastBlock *types.FastBlock) error {

	log.Info("++insert fastBlock pending", "number", fastBlock.Number(), "hash", fastBlock.Hash())

	for lr := fastBlockList.Front(); lr != nil; lr = lr.Next() {
		f := lr.Value.(*types.FastBlock)
		if f.Number().Cmp(fastBlock.Number()) > 0 {
			fastBlockList.InsertBefore(fastBlock, lr)
			return nil
		}
	}
	fastBlockList.PushBack(fastBlock)

	return nil
}

// AddLocals enqueues a batch of transactions into the pool if they are valid,
// marking the senders as a local ones in the mean time, ensuring they go around
// the local pricing constraints.
/*func (pool *SnailPool) AddRemoteRecords(records []*types.PbftRecord) []error {
	errs := make([]error, len(records))

	// TODO: check record signatures
	for i, record := range records {
		if err := pool.validateRecord(record); err != nil {
			errs[i] = err
			continue
		}

		r := types.CopyRecord(record)
		pool.newRecordCh <- r
	}

	return errs
}
*/

// Pending retrieves one currently record.
// The returned record is a copy and can be freely modified by calling code.
/*func (pool *SnailPool) PendingRecords() (*types.PbftRecord, error) {
	pool.muRecord.Lock()
	defer pool.muRecord.Unlock()

	first := pool.recordPending.Front()
	if first == nil {
		return nil, nil
	}
	record := types.CopyRecord(first.Value.(*types.PbftRecord))

	return record, nil
}*/

func (pool *SnailPool) PendingFastBlocks() (*types.FastBlock, error) {
	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()

	first := pool.fastBlockPending.Front()
	if first == nil {
		return nil, nil
	}
	block := first.Value.(*types.FastBlock)
	fastBlock := types.NewFastBlockWithHeader(block.Header()).WithBody(block.Transactions())

	return fastBlock, nil
}

// SubscribeNewRecordsEvent registers a subscription of NewRecordEvent and
// starts sending event to the given channel.
/*func (pool *SnailPool) SubscribeNewRecordEvent(ch chan<- NewRecordsEvent) event.Subscription {
	return pool.scope.Track(pool.recordFeed.Subscribe(ch))
}*/
func (pool *SnailPool) SubscribeNewFastBlockEvent(ch chan<- snailchain.NewFastBlocksEvent) event.Subscription {
	return pool.scope.Track(pool.fastBlockFeed.Subscribe(ch))
}
// validateRecord checks whether a Record is valid.
/*func (pool *SnailPool) validateRecord(record *types.PbftRecord) error {
	// TODO: check the record is signed properly
	//from, err := types.Sender(pool.signer, tx)
	//if err != nil {
	//	return ErrInvalidSender
	//}

	txHash := types.DeriveSha(record.Transactions())
	if txHash != record.TxHash() {
		return ErrInvalidHash
	}

	hash := record.CalcHash()
	if hash != record.Hash() {
		return ErrInvalidHash
	}

	return nil
}*/


func (pool *SnailPool) validateFruit(fruit *types.SnailBlock) error {
	// TODO: checks whether the fruit is valid

	// check freshness
	pointer := pool.chain.GetBlockByHash(fruit.PointerHash())
	if pointer == nil {
		return ErrInvalidPointer
	}
	//freshNumber := pool.header.Number().Sub(pool.header.Number(), pointer.Number())
	freshNumber :=new(big.Int).Sub(pool.header.Number(),pointer.Number())
	if freshNumber.Cmp(fruitFreshness) > 0 {
		return ErrFreshness
	}

	//header := fruit.Header()
	//if err := pool.engine.VerifyHeader(pool.chain, header, true); err != nil {
	//	return err
	//}

	//TODO snail chain not need use transactions 20180804
	/*
	if hash := types.DeriveSha(fruit.Transactions()); hash != header.TxHash {
		return ErrInvalidHash
	}
	*/

	return nil
}

// PostChainEvents iterates over the events generated by a chain insertion and
// posts them into the event feed.
// TODO: Should not expose PostChainEvents. The chain events should be posted in WriteBlock.
/*func (pool *SnailPool) PostNewRecordEvents(event interface{}) {
	pool.recordFeed.Send(event)
}*/

func (pool *SnailPool) PostNewFastBlockEvents(event interface{}) {
	pool.fastBlockFeed.Send(event)
}