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
	"encoding/hex"
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
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/etruedb"
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
	ChainDb() etruedb.Database
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

type CommitteeStartUpdate struct{}
type CommitteeStop struct{}

var committeemembers []*types.CommitteeMember

// New is create a miner object
func New(truechain Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine,
	election CommitteeElection, mineFruit bool, singleNode bool, remoteMining bool) *Miner {
	miner := &Miner{
		truechain:  truechain,
		mux:        mux,
		engine:     engine,
		election:   election,
		fruitOnly:  mineFruit, // set fruit only
		singleNode: singleNode,
		electionCh: make(chan types.ElectionEvent, fruitChanSize),
		worker:     newWorker(config, engine, common.Address{}, truechain, mux),
		commitFlag: 1,
		canStart:   1,
	}

	if !remoteMining {
		miner.Register(NewCPUAgent(truechain.SnailBlockChain(), engine))
	}
	miner.electionSub = miner.election.SubscribeElectionEvent(miner.electionCh)

	go miner.SetFruitOnly(mineFruit)

	// single node and remote agent not need care about the election
	if !miner.singleNode || remoteMining {
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
			case types.CommitteeStart, types.CommitteeUpdate:
				// already to start mining need stop
				committeemembers = nil
				committeemembers = append(committeemembers, ch.CommitteeMembers...)
				committeemembers = append(committeemembers, ch.BackupMembers...)

				miner.mux.Post(CommitteeStartUpdate{})
				log.Info("get  election  msg  1 CommitteeStart", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining)
			case types.CommitteeStop:
				miner.mux.Post(CommitteeStop{})
				log.Info("get  election  msg  3 CommitteeStop", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining)
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

	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{}, CommitteeStartUpdate{}, CommitteeStop{})
	defer events.Unsubscribe()
	//out:
	for {
		select {
		case ev := <-events.Chan():
			if ev == nil {
				return
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				log.Info("Miner update get download info startEvent", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining, "commit", miner.commitFlag)
				atomic.StoreInt32(&miner.canStart, 0)
				if miner.Mining() {
					miner.Stop()
				}
				atomic.StoreInt32(&miner.shouldStart, 1)
				log.Info("Mining aborted due to sync")

			case downloader.DoneEvent, downloader.FailedEvent:
				log.Info("Miner update get download info DoneEvent,FailedEvent", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining, "mining", miner.commitFlag)
				shouldStart := atomic.LoadInt32(&miner.shouldStart) == 1

				atomic.StoreInt32(&miner.canStart, 1)
				if shouldStart {
					miner.Start(miner.coinbase)
				}

			case CommitteeStartUpdate:
				log.Info("Miner update committee start update")

				if miner.election.IsCommitteeMember(committeemembers, miner.publickey) {
					// i am committee
					atomic.StoreInt32(&miner.commitFlag, 0)

					if miner.Mining() {
						miner.Stop()
						//atomic.StoreInt32(&miner.shouldStart, 1)
						log.Info("Mining aborted due to CommitteeUpdate")
					}

				} else {
					log.Info("Miner update not in commiteer munber so start to miner")
					atomic.StoreInt32(&miner.commitFlag, 1)

					miner.Start(miner.coinbase)

				}
				log.Info("Miner update get  election  msg  1 CommitteeStart", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining, "mining", miner.commitFlag)

			case CommitteeStop:
				log.Info("Miner update committee stop")
			}
		}
	}

}

//Start miner
func (miner *Miner) Start(coinbase common.Address) {
	log.Debug("start miner --miner start function")

	miner.SetEtherbase(coinbase)

	if atomic.LoadInt32(&miner.canStart) == 0 || atomic.LoadInt32(&miner.commitFlag) == 0 {
		log.Info("start to miner", "canstart", miner.canStart, "commitflag", miner.commitFlag)
		return
	}
	atomic.StoreInt32(&miner.shouldStart, 1)
	atomic.StoreInt32(&miner.mining, 1)

	miner.worker.start()
	//miner.worker.commitNewWork()

	var events []interface{}
	events = append(events, types.NewMinedFruitEvent{Block: nil})
	miner.worker.chain.PostChainEvents(events)

}

//Stop stop miner
func (miner *Miner) Stop() {
	log.Debug(" miner stop miner funtion")
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
	miner.publickey = make([]byte, len(pubkey))
	copy(miner.publickey, pubkey)

	if toElect {
		miner.worker.setElection(toElect, pubkey)
		log.Info("Set worker publickey", "pubkey", hex.EncodeToString(miner.publickey))
	}

	log.Info("Set election success", "pubkey", hex.EncodeToString(miner.publickey))
}

// SetFruitOnly allow the mine only mined fruit
func (miner *Miner) SetFruitOnly(FruitOnly bool) {
	miner.fruitOnly = FruitOnly
	miner.worker.SetFruitOnly(FruitOnly)
}
