// Copyright 2015 The go-ethereum Authors
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

package miner

import (
	"bytes"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	//	"github.com/ethereum/go-ethereum/consensus/truepow"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"gopkg.in/fatih/set.v0"
)

const (
	resultQueueSize  = 10
	miningLogAtDepth = 5

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
	// chainSideChanSize is the size of channel listening to ChainSideEvent.
	chainSideChanSize = 10
)

var (
	// fruit freshness
	fruitFreshness *big.Int = big.NewInt(17)
)

// Agent can register themself with the worker
type Agent interface {
	Work() chan<- *Work
	SetReturnCh(chan<- *Result)
	Stop()
	Start()
	GetHashRate() int64
}

// Work is the workers current environment and holds
// all of the current state information
type Work struct {
	config *params.ChainConfig
	signer types.Signer

	state     *state.StateDB // apply state changes here
	ancestors *set.Set       // ancestor set (used for checking uncle parent validity)
	family    *set.Set       // family set (used for checking uncle invalidity)
	uncles    *set.Set       // uncle set
	tcount    int            // tx count in cycle
	gasPool   *core.GasPool  // available gas used to pack transactions

	Block *types.Block // the new block

	FruitSet []*types.Block //the for fruitset

	header   *types.Header
	txs      []*types.Transaction
	receipts []*types.Receipt
	fruits   []*types.Block // for the fresh neo20180627

	createdAt time.Time
}

type Result struct {
	Work  *Work
	Block *types.Block
}

/*
//result for fruit and block  Neo 20180624
type Result struct {
	Work  *Work
	Block *types.Block
	Fruit *types.Block
}
*/

// worker is the main object which takes care of applying messages to the new state
type worker struct {
	config *params.ChainConfig
	engine consensus.Engine

	mu sync.Mutex

	// update loop
	mux *event.TypeMux
	// neo add to event record and fruit
	fruitSub  event.Subscription // for fruit
	recordSub event.Subscription //for record
	fruitCh   chan core.NewFruitsEvent
	recordCh  chan core.NewRecordsEvent

	txsCh        chan core.NewTxsEvent
	txsSub       event.Subscription
	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription
	chainSideCh  chan core.ChainSideEvent
	chainSideSub event.Subscription
	wg           sync.WaitGroup

	agents map[Agent]struct{}
	recv   chan *Result

	eth     Backend
	chain   *core.BlockChain
	proc    core.Validator
	chainDb ethdb.Database

	coinbase common.Address
	extra    []byte

	currentMu sync.Mutex
	current   *Work

	snapshotMu    sync.RWMutex
	snapshotBlock *types.Block
	snapshotState *state.StateDB

	uncleMu        sync.Mutex
	possibleUncles map[common.Hash]*types.Block

	unconfirmed *unconfirmedBlocks // set of locally mined blocks pending canonicalness confirmations

	fruitPoolSet []*types.Block //the for  fruitset  pool neo  20180627
	// atomic status counters
	mining int32
	atWork int32
}

func newWorker(config *params.ChainConfig, engine consensus.Engine, coinbase common.Address, eth Backend, mux *event.TypeMux) *worker {
	worker := &worker{
		config:         config,
		engine:         engine,
		eth:            eth,
		mux:            mux,
		txsCh:          make(chan core.NewTxsEvent, txChanSize),
		fruitCh:        make(chan core.NewFruitsEvent, txChanSize),  //neo 20180626 for fruit
		recordCh:       make(chan core.NewRecordsEvent, txChanSize), //neo 20180626 for record
		chainHeadCh:    make(chan core.ChainHeadEvent, chainHeadChanSize),
		chainSideCh:    make(chan core.ChainSideEvent, chainSideChanSize),
		chainDb:        eth.ChainDb(),
		recv:           make(chan *Result, resultQueueSize),
		chain:          eth.BlockChain(),
		proc:           eth.BlockChain().Validator(),
		possibleUncles: make(map[common.Hash]*types.Block),
		coinbase:       coinbase,
		agents:         make(map[Agent]struct{}),
		unconfirmed:    newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
	}
	// Subscribe NewTxsEvent for tx pool
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainSideSub = eth.BlockChain().SubscribeChainSideEvent(worker.chainSideCh)

	// Neo subscribe event for fruit and record
	worker.fruitSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh) // Neo 20180628 we need change the txsCh for fruit and record
	worker.recordSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)

	go worker.update()

	go worker.wait()
	worker.commitNewWork()

	return worker
}

func (self *worker) setEtherbase(addr common.Address) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.coinbase = addr
}

func (self *worker) setExtra(extra []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.extra = extra
}

func (self *worker) pending() (*types.Block, *state.StateDB) {
	if atomic.LoadInt32(&self.mining) == 0 {
		// return a snapshot to avoid contention on currentMu mutex
		self.snapshotMu.RLock()
		defer self.snapshotMu.RUnlock()
		return self.snapshotBlock, self.snapshotState.Copy()
	}

	self.currentMu.Lock()
	defer self.currentMu.Unlock()
	return self.current.Block, self.current.state.Copy()
}

func (self *worker) pendingBlock() *types.Block {
	if atomic.LoadInt32(&self.mining) == 0 {
		// return a snapshot to avoid contention on currentMu mutex
		self.snapshotMu.RLock()
		defer self.snapshotMu.RUnlock()
		return self.snapshotBlock
	}

	self.currentMu.Lock()
	defer self.currentMu.Unlock()
	return self.current.Block
}

func (self *worker) start() {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.StoreInt32(&self.mining, 1)

	// spin up agents
	for agent := range self.agents {
		agent.Start()
	}
}

func (self *worker) stop() {
	self.wg.Wait()

	self.mu.Lock()
	defer self.mu.Unlock()
	if atomic.LoadInt32(&self.mining) == 1 {
		for agent := range self.agents {
			agent.Stop()
		}
	}
	atomic.StoreInt32(&self.mining, 0)
	atomic.StoreInt32(&self.atWork, 0)
}

func (self *worker) register(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.agents[agent] = struct{}{}
	agent.SetReturnCh(self.recv)
}

func (self *worker) unregister(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	delete(self.agents, agent)
	agent.Stop()
}

//Neo 20180626 for fruit pool event
func (self *worker) updateofFruitTx([]*types.Block) {

}

//Neo 20180626 for record pool event
func (self *worker) updateofRecordTx([]*types.PbftRecord) {

}

func (self *worker) update() {
	defer self.txsSub.Unsubscribe()
	defer self.chainHeadSub.Unsubscribe()
	defer self.chainSideSub.Unsubscribe()

	// for fruit and record Neo 20180626
	defer self.fruitSub.Unsubscribe()
	defer self.recordSub.Unsubscribe()

	for {
		// A real event arrived, process interesting content
		select {
		// Handle ChainHeadEvent
		case <-self.chainHeadCh:
			self.commitNewWork()

		// Handle ChainSideEvent
		case ev := <-self.chainSideCh:
			self.uncleMu.Lock()
			self.possibleUncles[ev.Block.Hash()] = ev.Block
			self.uncleMu.Unlock()

		// Handle NewTxsEvent
		case ev := <-self.txsCh:
			// Apply transactions to the pending state if we're not mining.
			//
			// Note all transactions received may not be continuous with transactions
			// already included in the current mining block. These transactions will
			// be automatically eliminated.
			if atomic.LoadInt32(&self.mining) == 0 {
				self.currentMu.Lock()

				txs := make(map[common.Address]types.Transactions)

				for _, tx := range ev.Txs {
					acc, _ := types.Sender(self.current.signer, tx)
					txs[acc] = append(txs[acc], tx)
				}

				//txset := types.NewTransactionsByPriceAndNonce(self.current.signer, txs)
				//self.current.commitTransactions(self.mux, txset, self.chain, self.coinbase)
				self.updateSnapshot()
				self.currentMu.Unlock()
			} else {
				// If we're mining, but nothing is being processed, wake on new transactions
				if self.config.Clique != nil && self.config.Clique.Period == 0 {
					self.commitNewWork()
				}
			}

		//Neo 20180626 for fruit and record pool
		case ev := <-self.fruitCh:

			self.updateofFruitTx(ev.Fruits)
			//return

		case ev := <-self.recordCh:
			self.updateofRecordTx(ev.Records)

			//return
		// System stopped
		case <-self.txsSub.Err():
			return
		case <-self.chainHeadSub.Err():
			return
		case <-self.chainSideSub.Err():
			return
		}
	}
}

func (self *worker) wait() {
	for {

		for result := range self.recv {
			atomic.AddInt32(&self.atWork, -1)

			if result == nil {
				continue
			}

			block := result.Block
			work := result.Work

			//neo 20180624 that for fruit
			if block.IsFruit() {
				if block.RecordNumber() == nil {
					// if it does't include a record, it's not a fruit
					continue
				}
				if block.RecordNumber().Cmp(common.Big0) == 0 {
					continue
				}
				//log.Info("mined fruit", "record number", block.RecordNumber(), "hash", block.Hash())
				//neo 20180628
				// put it into pool first
				// Broadcast the new fruit event
				self.mux.Post(core.NewMinedFruitEvent{Block: block})

				var newFruits []*types.Block
				newFruits = append(newFruits, block)
				self.eth.HybridPool().AddRemoteFruits(newFruits)

			} else {
				// Update the block hash in all logs since it is now available and not when the
				// receipt/log of individual transactions were created.
				for _, r := range work.receipts {
					for _, l := range r.Logs {
						l.BlockHash = block.Hash()
						// neo add fruit 20180624
					}
				}
				for _, log := range work.state.Logs() {
					log.BlockHash = block.Hash()
				}

				stat, err := self.chain.WriteBlockWithState(block, work.receipts, work.state)
				if err != nil {
					log.Error("Failed writing block to chain", "err", err)
					continue
				}

				// Broadcast the block and announce chain insertion event
				self.mux.Post(core.NewMinedBlockEvent{Block: block})
				var (
					events []interface{}
					logs   = work.state.Logs()
				)
				events = append(events, core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})
				if stat == core.CanonStatTy {
					events = append(events, core.ChainHeadEvent{Block: block})
				}
				self.chain.PostChainEvents(events, logs)

				// Insert the block into the set of pending ones to wait for confirmations
				self.unconfirmed.Insert(block.NumberU64(), block.Hash())
			}

		}
	}
}

// push sends a new work task to currently live miner agents.
func (self *worker) push(work *Work) {
	if atomic.LoadInt32(&self.mining) != 1 {
		return //neo mark 20180701
	}
	for agent := range self.agents {
		atomic.AddInt32(&self.atWork, 1)
		if ch := agent.Work(); ch != nil {
			ch <- work
		}
	}
}

// makeCurrent creates a new environment for the current cycle.
func (self *worker) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := self.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &Work{
		config:    self.config,
		signer:    types.NewEIP155Signer(self.config.ChainID),
		state:     state,
		ancestors: set.New(),
		family:    set.New(),
		uncles:    set.New(),
		header:    header,
		createdAt: time.Now(),
	}

	// when 08 is processed ancestors contain 07 (quick block)
	for _, ancestor := range self.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			work.family.Add(uncle.Hash())
		}
		work.family.Add(ancestor.Hash())
		work.ancestors.Add(ancestor.Hash())
	}

	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	self.current = work

	return nil
}

func (self *worker) commitNewWork() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.uncleMu.Lock()
	defer self.uncleMu.Unlock()
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	tstart := time.Now()
	parent := self.chain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	header := &types.Header{
		ParentHash:  parent.Hash(),
		PointerHash: parent.Hash(),
		Number:      num.Add(num, common.Big1),
		GasLimit:    core.CalcGasLimit(parent),
		Extra:       self.extra,
		Time:        big.NewInt(tstamp),
	}
	// Only set the coinbase if we are mining (avoid spurious block rewards)
	if atomic.LoadInt32(&self.mining) == 1 {
		header.Coinbase = self.coinbase
	}
	if err := self.engine.Prepare(self.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}
	// If we are care about TheDAO hard-fork check whether to override the extra-data or not
	if daoBlock := self.config.DAOForkBlock; daoBlock != nil {
		// Check whether the block is among the fork extra-override range
		limit := new(big.Int).Add(daoBlock, params.DAOForkExtraRange)
		if header.Number.Cmp(daoBlock) >= 0 && header.Number.Cmp(limit) < 0 {
			// Depending whether we support or oppose the fork, override differently
			if self.config.DAOForkSupport {
				header.Extra = common.CopyBytes(params.DAOForkBlockExtra)
			} else if bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
				header.Extra = []byte{} // If miner opposes, don't let it use the reserved extra-data
			}
		}
	}
	// Could potentially happen if starting to mine in an odd state.
	err := self.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// Create the current work task and check any fork transitions needed
	work := self.current
	if self.config.DAOForkSupport && self.config.DAOForkBlock != nil && self.config.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(work.state)
	}
	PendingRecord, err := self.eth.HybridPool().PendingRecords()
	if err != nil {
		return
	}
	if PendingRecord != nil {
		for _, tx := range PendingRecord.Transactions() {
			work.txs = append(work.txs, tx)
		}
		work.header.RecordHash = PendingRecord.Hash()
		work.header.RecordNumber = PendingRecord.Number()

		log.Info("commit record", "number", PendingRecord.Number())
	}

	PendingFruits, err := self.eth.HybridPool().PendingFruits()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return
	}
	fruits := FruitsByNumber(PendingFruits)
	work.commitFruits(fruits, self.chain, self.coinbase)
	// compute uncles for the new block.
	var (
		uncles    []*types.Header
		badUncles []common.Hash
	)
	for hash, uncle := range self.possibleUncles {
		if len(uncles) == 2 {
			break
		}
		if err := self.commitUncle(work, uncle.Header()); err != nil {
			log.Trace("Bad uncle found and will be removed", "hash", hash)
			log.Trace(fmt.Sprint(uncle))

			badUncles = append(badUncles, hash)
		} else {
			log.Debug("Committing new uncle to block", "hash", hash)
			uncles = append(uncles, uncle.Header())
		}
	}
	for _, hash := range badUncles {
		delete(self.possibleUncles, hash)
	}
	// TODO: get fruits from tx pool
	// Create the new block to seal with the consensus engine
	if work.Block, err = self.engine.Finalize(self.chain, header, work.state, work.txs, uncles, work.receipts, work.fruits); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return
	}
	// We only care about logging if we're actually mining.
	if atomic.LoadInt32(&self.mining) == 1 {
		log.Info("Commit new mining work", "number", work.Block.Number(), "txs", len(work.txs), "uncles", len(uncles), "fruits", len(work.fruits), "elapsed", common.PrettyDuration(time.Since(tstart)))
		self.unconfirmed.Shift(work.Block.NumberU64() - 1)
	}
	self.push(work)
	self.updateSnapshot()
}

func (self *worker) commitUncle(work *Work, uncle *types.Header) error {
	hash := uncle.Hash()
	if work.uncles.Has(hash) {
		return fmt.Errorf("uncle not unique")
	}
	if !work.ancestors.Has(uncle.ParentHash) {
		return fmt.Errorf("uncle's parent unknown (%x)", uncle.ParentHash[0:4])
	}
	if work.family.Has(hash) {
		return fmt.Errorf("uncle already in family (%x)", hash)
	}
	work.uncles.Add(uncle.Hash())
	return nil
}

func (self *worker) updateSnapshot() {
	self.snapshotMu.Lock()
	defer self.snapshotMu.Unlock()

	/*
		self.snapshotBlock = types.NewBlock(
			self.current.header,
			self.current.txs,
			nil,
			self.current.receipts,
		)*/

	//NewBlock(header *Header, txs []*Transaction, uncles []*Header, receipts []*Receipt, fruits []*Block)

	self.snapshotBlock = types.NewBlock(
		self.current.header,
		self.current.txs,
		nil,
		self.current.receipts,
		self.current.fruits,
	)

	self.snapshotState = self.current.state.Copy()
}

func (env *Work) commitTransaction(tx *types.Transaction, bc *core.BlockChain, coinbase common.Address, gp *core.GasPool) (error, *types.Receipt) {
	//snap := env.state.Snapshot()

	receipt, _, err := core.ApplyTransaction(env.config, bc, &coinbase, gp, env.state, env.header, tx, &env.header.GasUsed, vm.Config{})
	if err != nil {
		//env.state.RevertToSnapshot(snap)
		return err, nil
	}
	//env.txs = append(env.txs, tx)

	//env.receipts = append(env.receipts, receipt)
	return nil, receipt
}

func (env *Work) commitFruit(fruit *types.Block, bc *core.BlockChain, coinbase common.Address) (error, []*types.Receipt) {
	var receipts []*types.Receipt

	pointer := bc.GetBlockByHash(fruit.PointerHash())
	if pointer == nil {
		return core.ErrInvalidPointer, nil
	}

	freshNumber := new(big.Int).Sub(env.header.Number, pointer.Number())

	if freshNumber.Cmp(fruitFreshness) > 0 {
		return core.ErrFreshness, nil
	}

	snap := env.state.Snapshot()

	for _, tx := range fruit.Transactions() {
		err, receipt := env.commitTransaction(tx, bc, coinbase, env.gasPool)
		if err == nil {
			receipts = append(receipts, receipt)
		} else {
			env.state.RevertToSnapshot(snap)
			return err, nil
		}
	}

	return nil, receipts
}

func FruitsByNumber(fruits map[common.Hash]*types.Block) []*types.Block {
	var fruitset []*types.Block
	// TODO: order by record number
	for _, fruit := range fruits {

		fruitset = append(fruitset, fruit)
	}
	
	lenfruits := len(fruitset)
	for i :=0; i<lenfruits; i++{
		mixRecordNumberIdx := i
		for j :=i+1; j<lenfruits; j++{
			if fruitset[j].RecordNumber().Uint64() < fruitset[mixRecordNumberIdx].RecordNumber().Uint64(){
				mixRecordNumberIdx =  j
			}
		}
		tempfruit := fruitset[i]
		fruitset[i] =fruitset[mixRecordNumberIdx]
		fruitset[mixRecordNumberIdx] = tempfruit
	}


	return fruitset
}

func (env *Work) commitFruits(fruits []*types.Block, bc *core.BlockChain, coinbase common.Address) {
	if env.gasPool == nil {
		env.gasPool = new(core.GasPool).AddGas(env.header.GasLimit)
	}

	for _, fruit := range fruits {
		err, receipts := env.commitFruit(fruit, bc, coinbase)
		if err == nil {
			for _, receipt := range receipts {
				env.receipts = append(env.receipts, receipt)
			}
			env.fruits = append(env.fruits, fruit)
		}
	}

}
