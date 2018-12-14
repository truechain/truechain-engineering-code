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
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
)

// CPUAgent is for agent to mine
type CPUAgent struct {
	mu sync.Mutex

	workCh        chan *Work
	stop          chan struct{}
	quitCurrentOp chan struct{}
	returnCh      chan<- *Result

	chain  consensus.SnailChainReader
	engine consensus.Engine

	isMining int32 // isMining indicates whether the agent is currently mining
}

func NewCpuAgent(chain consensus.SnailChainReader, engine consensus.Engine) *CPUAgent {
	miner := &CPUAgent{
		chain:  chain,
		engine: engine,
		stop:   make(chan struct{}, 1),
		workCh: make(chan *Work, 1),
	}
	return miner
}

func (self *CPUAgent) Work() chan<- *Work            { return self.workCh }
func (self *CPUAgent) SetReturnCh(ch chan<- *Result) { self.returnCh = ch }

func (self *CPUAgent) Stop() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 1, 0) {
		return // agent already stopped
	}
	self.stop <- struct{}{}
done:
	// Empty work channel
	for {
		select {
		case <-self.workCh:
		default:
			break done
		}
	}
}

func (self *CPUAgent) Start() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 0, 1) {
		return // agent already started
	}
	go self.update()
}

func (self *CPUAgent) update() {
out:
	for {
		select {
		case work := <-self.workCh:
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
			}
			self.quitCurrentOp = make(chan struct{})
			go self.mine(work, self.quitCurrentOp)
			self.mu.Unlock()
		case <-self.stop:
			self.mu.Lock()
			if self.quitCurrentOp != nil {
				close(self.quitCurrentOp)
				self.quitCurrentOp = nil
			}
			self.mu.Unlock()
			break out
		}
	}
}

func (self *CPUAgent) mine(work *Work, stop <-chan struct{}) {
	log.Info("start to mine", "block", work.Block.Number(), "fruits", len(work.Block.Fruits()),
		" fast", work.Block.FastNumber(), "diff", work.Block.BlockDifficulty(), "fdiff", work.Block.FruitDifficulty())
	// the mine with consensus
	/*
		if result, err := self.engine.Seal(self.chain, work.Block, stop); result != nil {
			log.Info("Successfully sealed new block", "number", result.Number(), "hash", result.Hash())
			self.returnCh <- &Result{work, result}
		} else {
			if err != nil {
				log.Warn("Block sealing failed", "err", err)
			}
			self.returnCh <- nil
		}
	*/

	// the new flow for fruit and block 20180624
	send := make(chan *types.SnailBlock, 10)
	abort := make(chan struct{})
	go self.engine.ConSeal(self.chain, work.Block, abort, send)

	var result *types.SnailBlock
mineloop:
	for {
		select {
		case <-stop:
			// Outside abort, stop all miner threads
			close(abort)
			break mineloop
		case result = <-send:
			// One of the threads found a block or fruit return it
			self.returnCh <- &Result{work, result}
			// when get a fruit, to stop or continue
			if !result.IsFruit() {
				break mineloop
			}
			break
		}
	}

}

func (self *CPUAgent) GetHashRate() int64 {

	if pow, ok := self.engine.(consensus.PoW); ok {
		return int64(pow.Hashrate())
	}

	return 0
}
