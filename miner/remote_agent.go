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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"encoding/hex"
	"errors"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
)

type hashrate struct {
	ping time.Time
	rate uint64
}

const UPDATABLOCKLENGTH = 12000 //12000  3000
const DATASETHEADLENGH = 10240

var maxUint128 = new(big.Int).Exp(big.NewInt(2), big.NewInt(128), big.NewInt(0))

// RemoteAgent for Remote mine
type RemoteAgent struct {
	mu sync.Mutex

	quitCh   chan struct{}
	workCh   chan *Work
	returnCh chan<- *Result

	chain       consensus.ChainReader
	snailchain  consensus.SnailChainReader
	engine      consensus.Engine
	currentWork *Work
	work        map[common.Hash]*Work

	hashrateMu sync.RWMutex
	hashrate   map[common.Hash]hashrate

	running int32 // running indicates whether the agent is active. Call atomically
}

//NewRemoteAgent create remote agent object
func NewRemoteAgent(chain consensus.ChainReader, snailchain consensus.SnailChainReader, engine consensus.Engine) *RemoteAgent {

	return &RemoteAgent{
		chain:      chain,
		snailchain: snailchain,
		engine:     engine,
		work:       make(map[common.Hash]*Work),
		hashrate:   make(map[common.Hash]hashrate),
	}
}

//SubmitHashrate return the HashRate for remote agent
func (a *RemoteAgent) SubmitHashrate(id common.Hash, rate uint64) {
	a.hashrateMu.Lock()
	defer a.hashrateMu.Unlock()

	a.hashrate[id] = hashrate{time.Now(), rate}
}

// Work return a work chan
func (a *RemoteAgent) Work() chan<- *Work {
	return a.workCh
}

// SetReturnCh return a mine result for return chan
func (a *RemoteAgent) SetReturnCh(returnCh chan<- *Result) {
	a.returnCh = returnCh
}

//Start remote control the start mine
func (a *RemoteAgent) Start() {
	if !atomic.CompareAndSwapInt32(&a.running, 0, 1) {
		return
	}
	a.quitCh = make(chan struct{})
	a.workCh = make(chan *Work, 1)
	go a.loop(a.workCh, a.quitCh)
}

//Stop remote control the stop mine
func (a *RemoteAgent) Stop() {
	if !atomic.CompareAndSwapInt32(&a.running, 1, 0) {
		return
	}
	close(a.quitCh)
	close(a.workCh)
}

// GetHashRate returns the accumulated hashrate of all identifier combined
func (a *RemoteAgent) GetHashRate() (tot int64) {
	a.hashrateMu.RLock()
	defer a.hashrateMu.RUnlock()

	// this could overflow
	for _, hashrate := range a.hashrate {
		tot += int64(hashrate.rate)
	}
	return
}

//GetWork return the current block hash without nonce
func (a *RemoteAgent) GetWork() ([4]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var res [4]string
	var fruitTarget *big.Int
	var blockTarget *big.Int

	if a.currentWork != nil {
		block := a.currentWork.Block
		epoch := uint64((block.Number().Uint64() - 1) / UPDATABLOCKLENGTH)
		block.Number()
		res[0] = block.HashNoNonce().Hex()
		//DatasetHash := a.engine.DataSetHash(block.NumberU64())
		//res[1] = "0x" + hex.EncodeToString(DatasetHash)
		res[1] = a.engine.DataSetHash(epoch)
		// Calculate the "target" to be returned to the external miner
		block.Fruits()
		if block.IsFruit() {
			// is fruit  so the block target set zore
			fruitTarget = new(big.Int).Div(maxUint128, block.FruitDifficulty())
			blockTarget = new(big.Int).SetInt64(0)
		} else {

			if block.FastNumber().Cmp(big.NewInt(0)) == 0 {
				// only block
				fruitTarget = new(big.Int).SetInt64(0)
				blockTarget = new(big.Int).Div(maxUint128, block.BlockDifficulty())
			} else {
				fruitTarget = new(big.Int).Div(maxUint128, block.FruitDifficulty())
				blockTarget = new(big.Int).Div(maxUint128, block.BlockDifficulty())
			}
		}
		res[2] = a.CompletionHexString(32, hex.EncodeToString(fruitTarget.Bytes()))
		res[3] = a.CompletionHexString(32, hex.EncodeToString(blockTarget.Bytes()))
		a.work[block.HashNoNonce()] = a.currentWork
		return res, nil
	}
	return res, errors.New("No work available yet, Don't panic.")
}

func (a *RemoteAgent) CompletionHexString(n int, src string) string {
	var res string
	if n <= 0 || len(src) > n {
		return src
	}
	var needString []byte
	for i := 0; i < n-len(src); i++ {
		needString = append(needString, '0')
	}
	res = "0x" + string(needString) + src
	return res
}

// SubmitWork tries to inject a pow solution into the remote agent, returning
// whether the solution was accepted or not (not can be both a bad pow as well as
// any other error, like no work pending).
func (a *RemoteAgent) SubmitWork(nonce types.BlockNonce, mixDigest, hash common.Hash) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	var isFinish bool
	var isFruit bool

	// Make sure the work submitted is present
	work := a.work[hash]
	if work == nil {
		log.Info("Work submitted but none pending", "hash", hash)
		return false
	}
	// Make sure the Engine solutions is indeed valid
	result := work.Block.Header()
	result.Nonce = nonce
	result.MixDigest = mixDigest

	errFruit := a.engine.VerifySnailSeal(a.snailchain, result, true)

	errBlock := a.engine.VerifySnailSeal(a.snailchain, result, false)

	if errBlock != nil && errFruit != nil {
		// not find fruit or block for mine
		log.Warn("Invalid proof-of-work submitted", "hash", hash, "errBlock", errBlock, "errFruit", errFruit)
		return false
	} else {

		if work.Block.IsFruit() {
			// only fruit
			if errFruit != nil {
				log.Warn("Invalid proof-of-work submitted", "hash", hash, "errFruit", errFruit)
				return false
			}
			isFruit = true
			isFinish = true
		} else {
			// fruit or block
			if errFruit == nil {
				// mine
				isFruit = true
			} else {
				if errBlock == nil {
					// mine block
					isFruit = false
					isFinish = true
				} else {
					log.Warn("Invalid proof-of-work submitted not all", "hash", hash, "errBlock", errBlock, "errFruit", errFruit)

				}
			}

		}

	}

	block := work.Block.WithSeal(result)

	if isFruit {
		block.SetSnailBlockFruits(nil)
	} else {
		block.SetSnailBlockSigns(nil)
	}

	a.returnCh <- &Result{work, block}

	if isFinish {
		delete(a.work, hash)
	}

	return true
}

//GetWork return the current block hash without nonce
func (a *RemoteAgent) GetDataset() ([DATASETHEADLENGH]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	//var res [DATASETHEADLENGH][]byte
	var res [DATASETHEADLENGH]string

	if a.currentWork != nil {
		block := a.currentWork.Block
		blockNum := block.Number().Uint64()
		tip9 := a.chain.Config().TIP9.SnailNumber.Uint64()
		if tip9 < blockNum {
			blockNum = tip9
		}
		epoch := uint64((blockNum - 1) / UPDATABLOCKLENGTH)
		if epoch == 0 {
			return res, errors.New("the epoch is zore not need dataset")
		}
		st_block_num := uint64((epoch-1)*UPDATABLOCKLENGTH + 1)

		for i := 0; i < DATASETHEADLENGH; i++ {
			header := a.snailchain.GetHeaderByNumber(uint64(i) + st_block_num)
			if header == nil {
				log.Error("header is nill  ", "blockNum is:  ", (uint64(i) + st_block_num))
				return res, errors.New("GetDataset get heard fial")
			}
			res[i] = "0x" + hex.EncodeToString(header.Hash().Bytes()[:16])
		}
		return res, nil
	}

	return res, errors.New("No work available yet, Don't panic.")
}

//GetWork return the current block hash without nonce
func (a *RemoteAgent) GetDatasetBySeedHash(seedHash string) ([DATASETHEADLENGH]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	//var res [DATASETHEADLENGH][]byte
	var res [DATASETHEADLENGH]string

	if a.currentWork != nil {
		var seed1, seed2 string
		var tarepoch uint64
		block := a.currentWork.Block
		epoch := uint64((block.Number().Uint64() - 1) / UPDATABLOCKLENGTH)
		if epoch == 0 {
			return res, errors.New("the epoch is zore not need dataset")
		}
		// only return the current dataset or befor one if epch>=2
		if epoch >= 2 {
			seed1 = a.engine.DataSetHash(epoch)
			seed2 = a.engine.DataSetHash(epoch - 1)
		} else {
			seed1 = a.engine.DataSetHash(epoch)
		}

		if strings.Compare(seedHash, seed1) == 0 {
			tarepoch = epoch
		} else if strings.Compare(seedHash, seed2) == 0 {
			tarepoch = epoch - 1
		} else {
			return res, errors.New("GetDataset get heard fial")
		}

		st_block_num := uint64((tarepoch-1)*UPDATABLOCKLENGTH + 1)

		for i := 0; i < DATASETHEADLENGH; i++ {
			header := a.snailchain.GetHeaderByNumber(uint64(i) + st_block_num)
			if header == nil {
				log.Error("header is nill  ", "blockNum is:  ", (uint64(i) + st_block_num))
				return res, errors.New("GetDataset get heard fial")
			}
			res[i] = "0x" + hex.EncodeToString(header.Hash().Bytes()[:16])
		}
		return res, nil
	}

	return res, errors.New("No work available yet, Don't panic.")
}

// loop monitors mining events on the work and quit channels, updating the internal
// state of the remote miner until a termination is requested.
//
// Note, the reason the work and quit channels are passed as parameters is because
// RemoteAgent.Start() constantly recreates these channels, so the loop code cannot
// assume data stability in these member fields.
func (a *RemoteAgent) loop(workCh chan *Work, quitCh chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-quitCh:
			return
		case work := <-workCh:
			a.mu.Lock()
			a.currentWork = work
			a.mu.Unlock()
		case <-ticker.C:
			// cleanup

			a.mu.Lock()
			for hash, work := range a.work {
				if time.Since(work.createdAt) > 6*(600*time.Second) {
					delete(a.work, hash)
				}
			}
			a.mu.Unlock()

			a.hashrateMu.Lock()
			for id, hashrate := range a.hashrate {
				if time.Since(hashrate.ping) > 10*time.Second {
					delete(a.hashrate, id)
				}
			}
			a.hashrateMu.Unlock()
		}
	}
}
