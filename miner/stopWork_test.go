// Copyright 2018 The go-ethereum Authors
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
	"fmt"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
	"testing"
)

type WorkerBackend struct {
	db             etruedb.Database
	txPool         *core.TxPool
	chain          *snailchain.SnailBlockChain
	fastchain      *core.BlockChain
	uncleBlock     *types.Block
	snailPool      *snailchain.SnailPool
	accountManager *accounts.Manager
}

func (b *WorkerBackend) SnailBlockChain() *snailchain.SnailBlockChain { return b.chain }
func (b *WorkerBackend) AccountManager() *accounts.Manager            { return b.accountManager }
func (b *WorkerBackend) SnailGenesis() *types.SnailBlock              { return b.chain.GetBlockByNumber(0) }
func (b *WorkerBackend) TxPool() *core.TxPool                         { return b.txPool }
func (b *WorkerBackend) BlockChain() *core.BlockChain                 { return b.fastchain }
func (b *WorkerBackend) ChainDb() etruedb.Database                    { return b.db }
func (b *WorkerBackend) SnailPool() *snailchain.SnailPool             { return b.snailPool }

func workerBackend(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, n int) *WorkerBackend {
	var (
		db      = etruedb.NewMemDatabase()
		genesis = core.DefaultGenesisBlock()
	)
	blockNum = 5
	fastChainHight = 500

	snailChainLocal, fastChainLocal = snailchain.MakeChainAndBlocks(fastChainHight, blockNum, genesis, minerva.NewFaker())

	//sv := snailchain.NewBlockValidator(chainConfig, fastChainLocal, snailChainLocal, engine)

	return &WorkerBackend{
		db:        db,
		chain:     snailChainLocal,
		fastchain: fastChainLocal,
		snailPool: snailchain.NewSnailPool(snailchain.DefaultSnailPoolConfig, fastChainLocal, snailChainLocal, engine),
	}
}

func testWorker(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, blocks int) (*worker, *WorkerBackend) {
	backend := workerBackend(t, chainConfig, engine, blocks)

	w := newWorker(chainConfig, engine, coinbase, backend, nil)

	return w, backend
}

func TestStopCommitFastBlock(t *testing.T) {
	fmt.Println("it's stop more than two snail block ")
	var (
		//fruitset1 []*types.SnailBlock  // nil situation
		fruitset2 []*types.SnailBlock // contine but not have 60
		fruitset3 []*types.SnailBlock // not contine   1 2 3  5 7 8
		fruitset4 []*types.SnailBlock // contine and langer then 60
		fruitset5 []*types.SnailBlock // frist one big then snailfruitslast fast numbe 10000 10001...
	)
	engine := minerva.NewFaker()

	chainDb := etruedb.NewMemDatabase()
	chainConfig, _, _, _ := core.SetupGenesisBlock(chainDb, core.DefaultGenesisBlock())
	//Miner := New(snailChainLocal, nil, nil, snailChainLocal.Engine(), nil, false, nil)
	worker, _ := testWorker(t, chainConfig, engine, 1)

	startFastNum := blockNum*params.MinimumFruits + 1
	gensisSnail := snailChainLocal.GetBlockByNumber(0)
	worker.commitNewWork()
	// situation 1   nil
	//fruitset1 = nil
	err0 := worker.CommitFastBlocksByWoker(nil, snailChainLocal, fastChainLocal, nil)
	if err0 != nil {
		fmt.Println("1 is err", err0)
	}

	// situation 2   1 2 3 4
	for i := startFastNum; i < (10 + startFastNum); i++ {

		fruit, _ := snailchain.MakeSnailBlockFruit(snailChainLocal, fastChainLocal, blockNum, i, gensisSnail.PublicKey(), gensisSnail.Coinbase(), false, nil)
		if fruit == nil {
			fmt.Println("fruit is nil  2")
		}
		fruitset2 = append(fruitset2, fruit)
	}

	err := worker.CommitFastBlocksByWoker(fruitset2, snailChainLocal, fastChainLocal, nil)
	if err != nil {
		fmt.Println("2 is err", err)
	}

	// situation 3   1 2 3 5 7
	j := 0
	for i := startFastNum; i < startFastNum+20; i++ {
		j++
		if j == 10 {
			continue
		}
		fruit, _ := snailchain.MakeSnailBlockFruit(snailChainLocal, fastChainLocal, blockNum, i, gensisSnail.PublicKey(), gensisSnail.Coinbase(), false, nil)
		if fruit == nil {
			fmt.Println("fruit is nil  3")
		}
		fruitset3 = append(fruitset3, fruit)
	}

	err2 := worker.CommitFastBlocksByWoker(fruitset3, snailChainLocal, fastChainLocal, nil)
	if err != nil {
		fmt.Println("3 is err", err2)
	}
	// situation 4   1 2 3...60
	for i := startFastNum; i < startFastNum+60; i++ {

		fruit, _ := snailchain.MakeSnailBlockFruit(snailChainLocal, fastChainLocal, blockNum, i, gensisSnail.PublicKey(), gensisSnail.Coinbase(), false, nil)
		if fruit == nil {
			fmt.Println("fruit is nil 4 ")
		}
		fruitset4 = append(fruitset4, fruit)
	}
	err3 := worker.CommitFastBlocksByWoker(fruitset4, snailChainLocal, fastChainLocal, nil)
	if err != nil {
		fmt.Println("4 is err", err3)
	}

	// situation 5   10000 10001...
	for i := fastChainHight; i < startFastNum+60; i++ {

		fruit, _ := snailchain.MakeSnailBlockFruit(snailChainLocal, fastChainLocal, blockNum, i, gensisSnail.PublicKey(), gensisSnail.Coinbase(), false, nil)
		if fruit == nil {
			fmt.Println("fruit is nil  5")
		}
		fruitset5 = append(fruitset5, fruit)
	}
	err5 := worker.CommitFastBlocksByWoker(fruitset5, snailChainLocal, fastChainLocal, nil)
	if err != nil {
		fmt.Println("5 is err", err5)
	}

	snail_blocks := snailChainLocal.GetBlocksFromNumber(1)
	for _, block := range snail_blocks {
		fmt.Printf("snail %d => %x\n", block.Number(), block.Hash())
	}

	for i := uint64(0); i <= fastChainLocal.CurrentBlock().Number().Uint64(); i++ {
		block := fastChainLocal.GetBlockByNumber(i)
		if block == nil {
			break
		} else {
			fmt.Printf("fast %d => %x\n", block.Number(), block.Hash())
		}
	}
}

func TestStopCommitFruits(t *testing.T) {

	var (
		//fruitset1 []*types.SnailBlock  // nil situation
		fruitset []*types.SnailBlock // contine but not have 60

	)
	engine := minerva.NewFaker()

	chainDb := etruedb.NewMemDatabase()
	chainConfig, _, _, _ := core.SetupGenesisBlock(chainDb, core.DefaultGenesisBlock())
	//Miner := New(snailChainLocal, nil, nil, snailChainLocal.Engine(), nil, false, nil)
	worker, _ := testWorker(t, chainConfig, engine, 1)

	startFastNum := blockNum*params.MinimumFruits + 1
	gensisSnail := snailChainLocal.GetBlockByNumber(0)

	//create some fruits but less then cureent block

	fruitNofresh, _ := snailchain.MakeSnailBlockFruit(snailChainLocal, fastChainLocal, 1, startFastNum+params.MinimumFruits+1, gensisSnail.PublicKey(), gensisSnail.Coinbase(), false, nil)

	for i := startFastNum; i < startFastNum+params.MinimumFruits; i++ {
		fruit, _ := snailchain.MakeSnailBlockFruit(snailChainLocal, fastChainLocal, startFastNum, i, gensisSnail.PublicKey(), gensisSnail.Coinbase(), false, nil)
		fruitset = append(fruitset, fruit)
	}
	fruitset = append(fruitset, fruitNofresh)

	worker.CommitFruits(fruitset, snailChainLocal, fastChainLocal, engine)
}
