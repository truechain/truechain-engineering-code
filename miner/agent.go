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

// NewCPUAgent create a Agent for miner
func NewCPUAgent(chain consensus.SnailChainReader, engine consensus.Engine) *CPUAgent {
	miner := &CPUAgent{
		chain:  chain,
		engine: engine,
		stop:   make(chan struct{}, 1),
		workCh: make(chan *Work, 1),
	}
	return miner
}

//Work is Agent return monitor work chan
func (agent *CPUAgent) Work() chan<- *Work { return agent.workCh }

//SetReturnCh is Agent return monitor result chan after the miner
func (agent *CPUAgent) SetReturnCh(ch chan<- *Result) { agent.returnCh = ch }

//Stop is a interface the work can control the Agent to stop miner
func (agent *CPUAgent) Stop() {
	if !atomic.CompareAndSwapInt32(&agent.isMining, 1, 0) {
		return // agent already stopped
	}
	agent.stop <- struct{}{}
done:
	// Empty work channel
	for {
		select {
		case <-agent.workCh:
		default:
			break done
		}
	}
}

// Start is a interface the work can control the Agent to start miner
func (agent *CPUAgent) Start() {
	if !atomic.CompareAndSwapInt32(&agent.isMining, 0, 1) {
		return // agent already started
	}
	go agent.update()
}

func (agent *CPUAgent) update() {
out:
	for {
		select {
		case work := <-agent.workCh:
			agent.mu.Lock()
			if agent.quitCurrentOp != nil {
				close(agent.quitCurrentOp)
			}
			agent.quitCurrentOp = make(chan struct{})
			go agent.mine(work, agent.quitCurrentOp)
			agent.mu.Unlock()
		case <-agent.stop:
			agent.mu.Lock()
			if agent.quitCurrentOp != nil {
				close(agent.quitCurrentOp)
				agent.quitCurrentOp = nil
			}
			agent.mu.Unlock()
			break out
		}
	}
}

func (agent *CPUAgent) mine(work *Work, stop <-chan struct{}) {
	log.Info("start to mine", "block", work.Block.Number(), "fruits", len(work.Block.Fruits()),
		" fast", work.Block.FastNumber(), "diff", work.Block.BlockDifficulty(), "fdiff", work.Block.FruitDifficulty())
	// the mine with consensus

	// the new flow for fruit and block 20180624
	send := make(chan *types.SnailBlock, 10)
	abort := make(chan struct{})
	go agent.engine.ConSeal(agent.chain, work.Block, abort, send)

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
			agent.returnCh <- &Result{work, result}
			// when get a fruit, to stop or continue
			if !result.IsFruit() {
				break mineloop
			}
			break
		}
	}

}

// GetHashRate return the cpu miner rate
func (agent *CPUAgent) GetHashRate() int64 {

	if pow, ok := agent.engine.(consensus.PoW); ok {
		return int64(pow.Hashrate())
	}

	return 0
}
