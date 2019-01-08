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

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/params"
)

// Backend wraps all methods required for mining.
type Backend interface {
	AccountManager() *accounts.Manager
	SnailBlockChain() *snailchain.SnailBlockChain
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
	SnailPool() *snailchain.SnailPool
	ChainDb() ethdb.Database
}

//CommitteeElection interface is Election module implementation committee interface
type CommitteeElection interface {
	//VerifySigns verify the fast chain committee signatures in batches
	VerifySigns(pvs []*types.PbftSign) ([]*types.CommitteeMember, []error)

	//Get a list of committee members
	GetCommittee(fastNumber *big.Int) []*types.CommitteeMember

	SubscribeElectionEvent(ch chan<- types.ElectionEvent) event.Subscription

	IsCommitteeMember(members []*types.CommitteeMember, publickey []byte) bool
}

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	mux *event.TypeMux

	worker *worker

	toElect    bool   // for elect
	publickey  []byte // for publickey
	fruitOnly  bool   // only for miner fruit
	singleNode bool   // for single node mode

	coinbase  common.Address
	mining    int32
	truechain Backend
	engine    consensus.Engine
	election  CommitteeElection

	//election
	electionCh  chan types.ElectionEvent
	electionSub event.Subscription

	canStart    int32 // can start indicates whether we can start the mining operation
	shouldStart int32 // should start indicates whether we should start after sync
	commitFlag  int32
}

// New is create a miner object
func New(truechain Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine,
	election CommitteeElection, mineFruit bool, singleNode bool) *Miner {
	miner := &Miner{
		truechain:  truechain,
		mux:        mux,
		engine:     engine,
		election:   election,
		fruitOnly:  mineFruit, // set fruit only
		singleNode: singleNode,
		electionCh: make(chan types.ElectionEvent, txChanSize),
		worker:     newWorker(config, engine, common.Address{}, truechain, mux),
		canStart:   1,
		commitFlag: 1,
	}

	miner.Register(NewCPUAgent(truechain.SnailBlockChain(), engine))
	miner.electionSub = miner.election.SubscribeElectionEvent(miner.electionCh)

	go miner.SetFruitOnly(mineFruit)

	// single node not need care about the election
	if !miner.singleNode {
		go miner.loop()
	}

	go miner.update()
	return miner
}

func (miner *Miner) loop() {

	defer miner.electionSub.Unsubscribe()
	for {
		select {
		case ch := <-miner.electionCh:
			switch ch.Option {
			case types.CommitteeStart:
				// alread to start mining need stop

				if miner.election.IsCommitteeMember(ch.CommitteeMembers, miner.publickey) {
					// i am committee
					if miner.Mining() {
						atomic.StoreInt32(&miner.commitFlag, 0)
						miner.Stop()
					}
					atomic.StoreInt32(&miner.commitFlag, 0)
				} else {
					log.Debug("not in commiteer munber so start to miner")

					atomic.StoreInt32(&miner.commitFlag, 1)
					miner.Start(miner.coinbase)

				}
				log.Debug("==================get  election  msg  1 CommitteeStart", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining)
			case types.CommitteeStop:

				log.Debug("==================get  election  msg  3 CommitteeStop", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining)
				atomic.StoreInt32(&miner.commitFlag, 1)
				miner.Start(miner.coinbase)
			}
		case <-miner.electionSub.Err():
			return

		}
	}

}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (miner *Miner) update() {
	//defer self.electionSub.Unsubscribe()
	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{}, types.ElectionEvent{})
out:
	for ev := range events.Chan() {
		switch ev.Data.(type) {
		case downloader.StartEvent:
			log.Info("-----------------get download info startEvent")
			atomic.StoreInt32(&miner.canStart, 0)
			if miner.Mining() {
				miner.Stop()
				atomic.StoreInt32(&miner.shouldStart, 1)
				log.Info("Mining aborted due to sync")
			}
		case downloader.DoneEvent, downloader.FailedEvent:
			log.Info("-----------------get download info DoneEvent,FailedEvent")
			shouldStart := atomic.LoadInt32(&miner.shouldStart) == 1

			atomic.StoreInt32(&miner.canStart, 1)
			atomic.StoreInt32(&miner.shouldStart, 0)
			if shouldStart {
				miner.Start(miner.coinbase)
			}
			// unsubscribe. we're only interested in this event once
			events.Unsubscribe()
			// stop immediately and ignore all further pending events
			break out
		}
	}
}

//Start miner
func (miner *Miner) Start(coinbase common.Address) {
	log.Debug("start miner --miner start function")
	atomic.StoreInt32(&miner.shouldStart, 1)
	miner.SetEtherbase(coinbase)

	if atomic.LoadInt32(&miner.canStart) == 0 || atomic.LoadInt32(&miner.commitFlag) == 0 {
		log.Info("start to miner", "canstart", miner.canStart, "commitflag", miner.commitFlag)
		return
	}
	atomic.StoreInt32(&miner.mining, 1)

	miner.worker.start()
	miner.worker.commitNewWork()
}

//Stop stop miner
func (miner *Miner) Stop() {
	log.Debug(" miner   ---stop miner funtion")
	miner.worker.stop()
	atomic.StoreInt32(&miner.mining, 0)
	atomic.StoreInt32(&miner.shouldStart, 0)

}

//Register is for register Agent to start or stop Agent
func (miner *Miner) Register(agent Agent) {
	if miner.Mining() {
		agent.Start()
	}
	miner.worker.register(agent)
}

//Unregister is Unregister the Agent
func (miner *Miner) Unregister(agent Agent) {
	miner.worker.unregister(agent)
}

//Mining start mining set flage
func (miner *Miner) Mining() bool {
	return atomic.LoadInt32(&miner.mining) > 0
}

// HashRate can calc the Mine cpu hash rate
func (miner *Miner) HashRate() (tot int64) {
	if pow, ok := miner.engine.(consensus.PoW); ok {
		tot += int64(pow.Hashrate())

	}
	// do we care this might race? is it worth we're rewriting some
	// aspects of the worker/locking up agents so we can get an accurate
	// hashrate?
	for agent := range miner.worker.agents {
		if _, ok := agent.(*CPUAgent); !ok {
			tot += agent.GetHashRate()
		}
	}
	return tot
}

//SetExtra set Extra data
func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("Extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
}

// Pending returns the currently pending block and associated state.
func (miner *Miner) Pending() (*types.Block, *state.StateDB) {
	return miner.worker.pending()
}

// PendingSnail returns the currently pending Snailblock and associated state.
func (miner *Miner) PendingSnail() (*types.SnailBlock, *state.StateDB) {
	return miner.worker.pendingSnail()
}

// PendingBlock returns the currently pending block.
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
func (miner *Miner) PendingBlock() *types.Block {
	return miner.worker.pendingBlock()
}

// PendingSnailBlock returns the currently pending block.
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
func (miner *Miner) PendingSnailBlock() *types.SnailBlock {
	return miner.worker.pendingSnailBlock()
}

// SetEtherbase  for reward
func (miner *Miner) SetEtherbase(addr common.Address) {
	miner.coinbase = addr
	miner.worker.setEtherbase(addr)
}

// SetElection Election is after mine the miner can be committee number
func (miner *Miner) SetElection(toElect bool, pubkey []byte) {

	if len(pubkey) <= 0 {
		log.Info("Set election failed, pubkey is nil")
		return
	}
	miner.toElect = toElect

	if toElect {

		miner.publickey = make([]byte, len(pubkey))

		copy(miner.publickey, pubkey)
		miner.worker.setElection(toElect, pubkey)
		log.Info("--------------------set publickey ------", "pubkey", miner.publickey)
	}

	log.Info("Set election success", "len pubkey", len(miner.publickey))
}

// SetFruitOnly allow the mine only mined fruit
func (miner *Miner) SetFruitOnly(FruitOnly bool) {
	miner.fruitOnly = FruitOnly
	miner.worker.SetFruitOnly(FruitOnly)
}
