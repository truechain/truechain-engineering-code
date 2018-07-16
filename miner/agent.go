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
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/eth/truechain"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"

)

type CpuAgent struct {
	mu sync.Mutex

	workCh        chan *Work
	stop          chan struct{}
	quitCurrentOp chan struct{}
	returnCh      chan<- *Result

	chain  consensus.ChainReader
	engine consensus.Engine
	tc 	*truechain.TrueHybrid
	isMining int32 // isMining indicates whether the agent is currently mining
	eth Backend
}

func NewCpuAgent(chain consensus.ChainReader, engine consensus.Engine,tc *truechain.TrueHybrid,eth Backend) *CpuAgent {
	miner := &CpuAgent{
		chain:  chain,
		engine: engine,
		stop:   make(chan struct{}, 1),
		workCh: make(chan *Work, 1),
		tc:tc,
		eth:eth,
	}
	return miner
}

func (self *CpuAgent) Work() chan<- *Work            { return self.workCh }
func (self *CpuAgent) SetReturnCh(ch chan<- *Result) { self.returnCh = ch }

func (self *CpuAgent) Stop() {
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

func (self *CpuAgent) Start() {
	if !atomic.CompareAndSwapInt32(&self.isMining, 0, 1) {
		return // agent already started
	}
	go self.update()
}

func (self *CpuAgent) update() {
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

func (self *CpuAgent) mine(work *Work, stop <-chan struct{}) {
	if result, err := self.engine.Seal(self.chain, work.Block, stop); result != nil {
		log.Info("Successfully sealed new block", "number", result.Number(), "hash", result.Hash())
		self.MakeSigned(work)
		self.returnCh <- &Result{work, result}

	} else {
		if err != nil {
			log.Warn("Block sealing failed", "err", err)
		}
		self.returnCh <- nil
	}
}

func (self *CpuAgent) MakeSigned(work *Work){


	//添加当前节点到预选委员会队列
	fmt.Print("Add to the candidate committee。。。")
	//获取节点信息
	ip,pub,_ := work.tc.GetNodeID()

	//构建候选节点的信息
	cm := truechain.CdMember{}
	cm.Height=work.Block.Number()
	cm.Addr = ip
	cm.Nodeid = pub
	cm.Coinbase	= string(work.Block.Coinbase().Bytes())				//挖矿奖励地址)
	cm.Comfire	= false
	cm.Port	= 16745


	//创建候选的消息
	cem := &truechain.CdEncryptionMsg{}

	//Height		*big.Int
	//Msg			[]byte		上一个结构体的字节码
	//Sig 			[]byte		Coinbase 私钥签名后
	//use			bool		默认值：false
	coninbase :=work.Block.Coinbase().Bytes()
	account := accounts.Account{Address: common.BytesToAddress(coninbase)}
	wallet,_ := self.eth.AccountManager().Find(account)
	signed, _ := wallet.SignHash(account,cem.Msg)

	//wallet
	cem.Height = work.Block.Number()
	byt ,_ := truechain.ToByte(cm)
	cem.Msg = byt
	cem.Sig = signed
	//work.Block.Coinbase().
	//cem.Height = work.Block.Number()
	//cem.Height = work.Block.Number()
	//cem.Msg = cem.ToByte()
	//Add to the candidate committee
	work.tc.ReceiveSdmMsg(cem)


}


func (self *CpuAgent) GetHashRate() int64 {
	if pow, ok := self.engine.(consensus.PoW); ok {
		return int64(pow.Hashrate())
	}
	return 0
}
