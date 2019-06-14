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

const (
	electionChanSize = 16
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

	downloaderStartCh  chan downloaderStartEvent
	downloaderStartSub event.Subscription

	canStart     int32 // can start indicates whether we can start the mining operation
	shouldStart  int32 // should start indicates whether we should start after sync
	commitFlag   int32
	remoteMining bool
}

type downloaderStartEvent struct{ start bool }

// New is create a miner object
func New(truechain Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine,
	election CommitteeElection, mineFruit bool, singleNode bool, remoteMining bool, mining bool) *Miner {
	miner := &Miner{
		truechain:         truechain,
		mux:               mux,
		engine:            engine,
		election:          election,
		fruitOnly:         mineFruit, // set fruit only
		singleNode:        singleNode,
		worker:            newWorker(config, engine, common.Address{}, truechain, mux),
		downloaderStartCh: make(chan downloaderStartEvent, electionChanSize),
		commitFlag:        0, // not committee
		canStart:          0, // not start downlad
		mining:            0,
		remoteMining:      remoteMining,
	}

	// without the --mine can not start mine,no matter remote or fruitonly
	if mining || mineFruit {
		atomic.StoreInt32(&miner.shouldStart, 1)
	}

	go miner.SetFruitOnly(mineFruit)

	// single node and remote agent not need care about the election
	if !remoteMining {
		miner.Register(NewCPUAgent(truechain.SnailBlockChain(), engine))
		if !miner.singleNode {
			miner.electionCh = make(chan types.ElectionEvent, electionChanSize)
			miner.electionSub = miner.election.SubscribeElectionEvent(miner.electionCh)
			go miner.loop()
		}
	}

	go miner.update()
	return miner
}

func (miner *Miner) loop() {

	defer miner.electionSub.Unsubscribe()

	for {
		select {
		case downloader := <-miner.downloaderStartCh:
			if downloader.start {
				log.Info("download start msg")
				atomic.StoreInt32(&miner.canStart, 1)
				if miner.Mining() {
					miner.Stop()
				}
			} else {
				log.Info("download done and fail msg")
				atomic.StoreInt32(&miner.canStart, 0)
				miner.Start(miner.coinbase)
			}
		case ch := <-miner.electionCh:
			switch ch.Option {
			case types.CommitteeStart, types.CommitteeUpdate:
				// already to start mining need stop
				var committeemembers []*types.CommitteeMember
				committeemembers = nil
				committeemembers = append(committeemembers, ch.CommitteeMembers...)
				committeemembers = append(committeemembers, ch.BackupMembers...)

				//miner.mux.Post(CommitteeStartUpdate{})
				log.Info("get  election  msg  1 CommitteeStart", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining)
				if miner.election.IsCommitteeMember(committeemembers, miner.publickey) {
					// i am committee
					atomic.StoreInt32(&miner.commitFlag, 1)

					if miner.Mining() {
						miner.Stop()
						log.Info("Mining aborted due to CommitteeUpdate")
					}

				} else {
					log.Info("Miner update not in commiteer munber so start to miner")
					atomic.StoreInt32(&miner.commitFlag, 0)
					miner.Start(miner.coinbase)

				}
				log.Info("Miner update get  election  msg  1 CommitteeStart", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining, "mining", miner.commitFlag)

			case types.CommitteeStop:
				//miner.mux.Post(CommitteeStop{})
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

	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer events.Unsubscribe()

	for {
		select {
		case ev := <-events.Chan():
			if ev == nil {
				return
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:

				log.Info("Miner update get download info startEvent", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining, "commit", miner.commitFlag)
				if miner.remoteMining {
					atomic.StoreInt32(&miner.canStart, 1)
					if miner.Mining() {
						miner.Stop()
					}
					log.Info("Mining aborted due to sync")
				} else {
					miner.downloaderStartCh <- downloaderStartEvent{true}
				}

			case downloader.DoneEvent, downloader.FailedEvent:
				log.Info("Miner update get download info DoneEvent,FailedEvent", "canStart", miner.canStart, "shoutstart", miner.shouldStart, "mining", miner.mining, "mining", miner.commitFlag)
				if miner.remoteMining {
					atomic.StoreInt32(&miner.canStart, 0)
					miner.Start(miner.coinbase)
				} else {
					miner.downloaderStartCh <- downloaderStartEvent{false}
				}
				return
			}
		}
	}

}

//Start miner
func (miner *Miner) Start(coinbase common.Address) {
	log.Debug("start miner --miner start function")

	miner.SetEtherbase(coinbase)

	if atomic.LoadInt32(&miner.canStart) == 0 && atomic.LoadInt32(&miner.commitFlag) == 0 && atomic.LoadInt32(&miner.mining) == 0 && atomic.LoadInt32(&miner.shouldStart) == 1 {

		atomic.StoreInt32(&miner.mining, 1)

		miner.worker.start()

		var events []interface{}
		events = append(events, types.NewMinedFruitEvent{Block: nil})
		miner.worker.chain.PostChainEvents(events)
	} else {
		log.Info("start to miner fail", "canstart", miner.canStart, "commitflag", miner.commitFlag, "shout start", miner.shouldStart, "mining", miner.mining)
		return
	}

}

//Stop stop miner
func (miner *Miner) SetShouldStartMining(start bool) {
	if start {
		atomic.StoreInt32(&miner.shouldStart, 1)
	} else {
		atomic.StoreInt32(&miner.shouldStart, 0)
	}
}

//Stop stop miner
func (miner *Miner) Stop() {
	log.Debug(" miner stop miner funtion")
	miner.worker.stop()
	atomic.StoreInt32(&miner.mining, 0)
	//atomic.StoreInt32(&miner.shouldStart, 0)

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

// GetCurrentBlock return the fruit or block it is mining
func (miner *Miner) GetCurrentBlock() *types.SnailBlock {
	return miner.worker.current.Block
}
