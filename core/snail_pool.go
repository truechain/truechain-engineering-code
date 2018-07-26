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
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	recordChanSize = 100
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
	Journal:   "records.rlp",
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
	chain       *BlockChain
	gasPrice    *big.Int

	scope event.SubscriptionScope

	fruitFeed  event.Feed
	recordFeed event.Feed

	mu sync.RWMutex

	chainHeadCh  chan ChainHeadEvent
	chainHeadSub event.Subscription

	signer types.Signer

	currentState  *state.StateDB      // Current state in the blockchain head
	pendingState  *state.ManagedState // Pending state tracking virtual nonces
	currentMaxGas uint64              // Current gas limit for transaction caps

	engine consensus.Engine // Consensus engine used for validating

	muFruit  sync.RWMutex
	muRecord sync.RWMutex

	allRecords    map[common.Hash]*types.PbftRecord
	fruitRecords  map[common.Hash]*types.PbftRecord // the records have fruit
	recordList    *list.List
	recordPending *list.List
	newRecordCh   chan *types.PbftRecord

	allFruits    map[common.Hash]*types.Block
	fruitPending map[common.Hash]*types.Block

	newFruitCh chan *types.Block

	header *types.Block

	gasUsed uint64
	gasPool *GasPool // available gas used to pack transactions

	wg sync.WaitGroup // for shutdown sync

	homestead bool
}

// NewTxPool creates a new transaction pool to gather, sort and filter inbound
// transactions from the network.
func NewSnailPool(chainconfig *params.ChainConfig, chain *BlockChain) *SnailPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	//config = (&config).sanitize()
	config := DefaultHybridPoolConfig

	// Create the transaction pool with its initial settings
	pool := &SnailPool{
		config:      config,
		chainconfig: chainconfig,
		chain:       chain,
		signer:      types.NewEIP155Signer(chainconfig.ChainID),
		chainHeadCh: make(chan ChainHeadEvent, chainHeadChanSize),
		gasPrice:    new(big.Int).SetUint64(config.PriceLimit),

		newRecordCh: make(chan *types.PbftRecord, recordChanSize),

		allRecords:    make(map[common.Hash]*types.PbftRecord),
		recordList:    list.New(),
		recordPending: list.New(),

		newFruitCh: make(chan *types.Block, fruitChanSize),
		allFruits:  make(map[common.Hash]*types.Block),
		fruitPending:make(map[common.Hash]*types.Block),
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
	pool.currentMaxGas = pool.header.GasLimit()

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()

	//eth.NewRecord(pool)
	return pool
}

func (pool *SnailPool) getRecord(hash common.Hash, number *big.Int) *types.PbftRecord {
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
}

// move the fruit of execute record to pending list
func (pool *SnailPool) updateFruit(record *types.PbftRecord, toLock bool) error {
	if toLock {
		pool.muFruit.Lock()
		defer pool.muFruit.Unlock()
	}

	f := pool.allFruits[record.Hash()]
	if f == nil {
		return ErrNotExist
	} else {
		if f.TxHash() != record.TxHash() {
			// fruit txs is invalid
			delete(pool.allFruits, record.Hash())
			delete(pool.fruitPending, record.Hash())
			return ErrInvalidHash
		} else {
			pool.fruitPending[record.Hash()] = f
		}
	}
	return nil
}

// add
func (pool *SnailPool) addFruit(fruit *types.Block) error {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	//check
	r := pool.getRecord(fruit.RecordHash(), fruit.RecordNumber())
	f := pool.allFruits[fruit.RecordHash()]
	if f == nil {
		pool.allFruits[fruit.RecordHash()] = fruit
		if r != nil {
			pool.muRecord.Lock()
			pool.removeRecordWithLock(pool.recordPending, fruit.RecordHash())
			pool.muRecord.Unlock()
			pool.fruitPending[fruit.RecordHash()] = fruit
		}

		return nil
	} else {
		if fruit.Hash().Big().Cmp(f.Hash().Big()) > 0 {
			// new fruit hash is greater than old one
			return nil
		} else {
			pool.allFruits[fruit.RecordHash()] = fruit
			if r != nil {
				pool.muRecord.Lock()
				pool.removeRecordWithLock(pool.recordPending, fruit.RecordHash())
				pool.muRecord.Unlock()
				pool.fruitPending[fruit.RecordHash()] = fruit
			} else if _,ok := pool.fruitPending[fruit.RecordHash()]; ok {
				pool.fruitPending[fruit.RecordHash()] = fruit
			}
		}
	}

	return nil
}

func (pool *SnailPool) commitTransaction(tx *types.Transaction, coinbase common.Address, gp *GasPool, gasUsed *uint64) (error, *types.Receipt) {
	//snap := pool.currentState.Snapshot()

	// TODO: commit tx
	//return nil, nil

	receipt, _, err := ApplyTransaction(pool.chainconfig, pool.chain, &coinbase, gp, pool.currentState, pool.header.Header(), tx, gasUsed, vm.Config{})
	if err != nil {
		//pool.currentState.RevertToSnapshot(snap)
		log.Info("apply transaction field ", "err", err.Error())
		return err, nil
	}

	return nil, receipt
}

func (pool *SnailPool) commitRecord(record *types.PbftRecord, coinbase common.Address) (error, []*types.Receipt) {
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
}

// re execute records whose number are larger than the give on
// maybe they can execute now
func (pool *SnailPool) updateRecordsWithLock(number *big.Int, lockFruits bool) {
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
}

func (pool *SnailPool) addRecord(record *types.PbftRecord) error {
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

		case record := <-pool.newRecordCh:
			if record != nil {
				pool.addRecord(record)
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
func fruitsDifference(a, b []*types.Block) []*types.Block {
	keep := make([]*types.Block, 0, len(a))

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
func (pool *SnailPool) removeRecordWithLock(recordList *list.List, hash common.Hash) {
	for e := recordList.Front(); e != nil; e = e.Next() {
		r := e.Value.(*types.PbftRecord)
		if r.Hash() == hash {
			recordList.Remove(e)
			break
		}
	}
}

// remove all the fruits and records included in the new block
func (pool *SnailPool) removeWithLock(fruits []*types.Block) {
	for _, fruit := range fruits {
		delete(pool.fruitPending, fruit.RecordHash())
		delete(pool.allFruits, fruit.RecordHash())

		if _, ok := pool.allRecords[fruit.RecordHash()]; ok {
			pool.removeRecordWithLock(pool.recordList, fruit.RecordHash())
			pool.removeRecordWithLock(pool.recordPending, fruit.RecordHash())
			delete(pool.allRecords, fruit.RecordHash())
		}
	}
}


func (pool *SnailPool) resetRecordsWithLock() {
	pool.fruitPending = make(map[common.Hash]*types.Block)

	pool.recordList = list.New()
	pool.recordPending = list.New()

	for _, record := range pool.allRecords {
		pool.insertRecordWithLock(pool.recordList, record)
	}
}

// reset retrieves the current state of the blockchain and ensures the content
// of the transaction pool is valid with regard to the chain state.
func (pool *SnailPool) reset(oldHead, newHead *types.Block) {
	// If we're reorging an old state, reinject all dropped transactions
	var reinject []*types.Block

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash() {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.Number().Uint64()
		newNum := newHead.Number().Uint64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all transactions into memory
			var discarded, included []*types.Block

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
	pool.currentMaxGas = newHead.GasLimit()

	// Inject any transactions discarded due to reorgs
	log.Debug("Reinjecting stale transactions", "count", len(reinject))

	// validate the pool of pending transactions, this will remove
	// any transactions that have been included in the block or
	// have been invalidated because of another transaction (e.g.
	// higher gas price)
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	pool.muRecord.Lock()
	defer pool.muRecord.Unlock()

	pool.removeWithLock(newHead.Fruits())

	// reset pool state by re-verify all the records, including pending records
	// TODO: reset pool state using pendingState, refer to tx_pool
	pool.resetRecordsWithLock()

	// Check the queue and move transactions over to the pending if possible
	// or remove those that have become invalid
	pool.updateRecordsWithLock(common.Big0, false)
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
func (pool *SnailPool) AddRemoteFruits(fruits []*types.Block) []error {

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
func (pool *SnailPool) PendingFruits() (map[common.Hash]*types.Block, error) {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	pending := make(map[common.Hash]*types.Block)
	for addr, fruit := range pool.fruitPending {
		pending[addr] = types.CopyFruit(fruit)
	}

	return pending, nil
}

// SubscribeNewFruitsEvent registers a subscription of NewFruitEvent and
// starts sending event to the given channel.
func (pool *SnailPool) SubscribeNewFruitEvent(ch chan<- NewFruitsEvent) event.Subscription {
	return pool.scope.Track(pool.fruitFeed.Subscribe(ch))
}

// Insert record into list order by record number
func (pool *SnailPool) insertRecordWithLock(recordList *list.List, record *types.PbftRecord) error {

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
}

// AddLocals enqueues a batch of transactions into the pool if they are valid,
// marking the senders as a local ones in the mean time, ensuring they go around
// the local pricing constraints.
func (pool *SnailPool) AddRemoteRecords(records []*types.PbftRecord) []error {
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

// Pending retrieves one currently record.
// The returned record is a copy and can be freely modified by calling code.
func (pool *SnailPool) PendingRecords() (*types.PbftRecord, error) {
	pool.muRecord.Lock()
	defer pool.muRecord.Unlock()

	first := pool.recordPending.Front()
	if first == nil {
		return nil, nil
	}
	record := types.CopyRecord(first.Value.(*types.PbftRecord))

	return record, nil
}

// SubscribeNewRecordsEvent registers a subscription of NewRecordEvent and
// starts sending event to the given channel.
func (pool *SnailPool) SubscribeNewRecordEvent(ch chan<- NewRecordsEvent) event.Subscription {
	return pool.scope.Track(pool.recordFeed.Subscribe(ch))
}

// validateRecord checks whether a Record is valid.
func (pool *SnailPool) validateRecord(record *types.PbftRecord) error {
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
}

func (pool *SnailPool) validateFruit(fruit *types.Block) error {
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

	header := fruit.Header()
	//if err := pool.engine.VerifyHeader(pool.chain, header, true); err != nil {
	//	return err
	//}

	if hash := types.DeriveSha(fruit.Transactions()); hash != header.TxHash {
		return ErrInvalidHash
	}

	return nil
}

// PostChainEvents iterates over the events generated by a chain insertion and
// posts them into the event feed.
// TODO: Should not expose PostChainEvents. The chain events should be posted in WriteBlock.
func (pool *SnailPool) PostNewRecordEvents(event interface{}) {
	pool.recordFeed.Send(event)
}
