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

package snailchain

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"

	"math/big"
	"os"
	"testing"
)

const (
	geneSnailBlockNumber = 11
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

// This test checks that received transactions are added to the local pool.
func TestExampleGenerateChain(t *testing.T) { testExampleGenerateChain(t, 128) }

func testExampleGenerateChain(t *testing.T, n int) {
	params.MinimumFruits = 1
	params.MinTimeGap = big.NewInt(0)
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		db      = etruedb.NewMemDatabase()
		pow     = minerva.NewFaker()
		gspec   = &core.Genesis{
			Config:     params.TestChainConfig,
			Alloc:      types.GenesisAlloc{addr1: {Balance: big.NewInt(3000000)}},
			Difficulty: big.NewInt(20000),
		}
		genesis      = gspec.MustFastCommit(db)
		snailGenesis = gspec.MustSnailCommit(db)
	)

	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.
	chain, _ := core.GenerateChain(gspec.Config, genesis, pow, db, n*params.MinimumFruits, nil)

	// Import the chain. This runs all block validation rules.
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, pow, vm.Config{})
	defer blockchain.Stop()

	if i, err := blockchain.InsertChain(chain); err != nil {
		fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
		return
	}

	snailChain, _ := NewSnailBlockChain(db, gspec.Config, pow, blockchain)
	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.

	schain := GenerateChain(gspec.Config, blockchain, []*types.SnailBlock{snailGenesis}, n, 7, nil)
	if _, err := snailChain.InsertChain(schain); err != nil {
		panic(err)
	}
	defer snailChain.Stop()
}
func TestReward(t *testing.T) {
	testReward(t, 1)
}

func testReward(t *testing.T, n int) {
	//params.MinimumFruits = 1
	params.MinTimeGap = big.NewInt(0)
	var (
		db      = etruedb.NewMemDatabase()
		pow     = minerva.NewFaker()

		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		gspec   = &core.Genesis{
			Config: params.TestChainConfig,
			//Alloc:      types.GenesisAlloc{addr1: {Balance: big.NewInt(3000000)}},
			Difficulty: big.NewInt(20000),
			Coinbase:   addr1,
		}
		genesis      = gspec.MustFastCommit(db)
		snailGenesis = gspec.MustSnailCommit(db)
	)

	var (
		balance_given    = new(big.Int)
		balance_get      = new(big.Int)
		minerAddress     = make(map[common.Address]int)
		snailRewardBlock *types.SnailBlock
		snailBlocks      []*types.SnailBlock
		fastParent       = genesis
	)
	//generate blockchain
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, pow, vm.Config{})
	defer blockchain.Stop()
	snailChain, _ := NewSnailBlockChain(db, gspec.Config, pow, blockchain)
	defer snailChain.Stop()
	pow.SetSnailChainReader(snailChain)

	for i := 1; i < geneSnailBlockNumber; i++ {
		log.Info("getInfo", "i", i, "blockchain", fastParent.NumberU64(), "snailNumber", snailChain.CurrentBlock().NumberU64())

		fastBlocks, _ := core.GenerateChainWithReward(gspec.Config, fastParent, snailRewardBlock, pow, db, n*params.MinimumFruits, nil)
		if i, err := blockchain.InsertChain(fastBlocks); err != nil {
			fmt.Printf("insert error (block %d): %v\n", fastBlocks[i].NumberU64(), err)
			return
		}
		fastParent = blockchain.CurrentBlock()

		if i == 1 {
			snailBlocks = GenerateChain(gspec.Config, blockchain, []*types.SnailBlock{snailGenesis}, n, 7, nil)
		} else {
			snailBlocks = GenerateChain(gspec.Config, blockchain, snailChain.GetBlocksFromNumber(0), n, 7, nil)
		}
		if _, err := snailChain.InsertChain(snailBlocks); err != nil {
			panic(err)
		}
		snailRewardBlock = snailChain.CurrentBlock()
	}
	//get statedb
	statedb, err := state.New(blockchain.CurrentBlock().Root(), state.NewDatabase(db))
	if err != nil {
		log.Error("new state error", err, "err")
	}

	for _, snail := range snailChain.GetBlocksFromNumber(1) {
		if minerAddress[snail.Coinbase()] == 0 {
			balance_get = new(big.Int).Add(balance_get, statedb.GetBalance(snail.Coinbase()))
			log.Info("snail coinbase", "number", snail.NumberU64(), "addr", snail.Coinbase())
		}
		minerAddress[snail.Coinbase()] = minerAddress[snail.Coinbase()] + 1
		fruits := snail.Fruits()
		for _, fruit := range fruits {
			if minerAddress[snail.Coinbase()] == 0 {
				balance_get = new(big.Int).Add(balance_get, statedb.GetBalance(fruit.Coinbase()))
				log.Info("miner coinbase", "number", fruit.NumberU64(), "addr", fruit.Coinbase())
			}
			minerAddress[fruit.Coinbase()] = minerAddress[fruit.Coinbase()] + 1
		}
	}

	members := pow.GetElection().GetCommittee(big.NewInt(1))
	for _, member := range members {
		if minerAddress[member.Coinbase] == 0 {
			balance_get = new(big.Int).Add(balance_get, statedb.GetBalance(member.Coinbase))
		}
		minerAddress[member.Coinbase] = minerAddress[member.Coinbase] + 1
		log.Info("getBalance[committe member]", "addr", member.Coinbase, "balance", statedb.GetBalance(member.Coinbase))
	}

	for i := 1; i < geneSnailBlockNumber-1; i++ {
		committeeAward, minerAward, minerFruitAward, _ := minerva.GetBlockReward(big.NewInt(int64(i)))
		fmt.Println("committeeAward:", committeeAward, "minerAward:", minerAward, "minerFruitAward:", minerFruitAward)
		balance_given = new(big.Int).Add(balance_given, committeeAward)
		balance_given = new(big.Int).Add(balance_given, minerAward)
		balance_given = new(big.Int).Add(balance_given, minerFruitAward)
	}

	for addr, times := range minerAddress {
		log.Info("miner address", "addr", addr, "times", times)
	}

	balance := new(big.Int).Sub(balance_get,balance_given)
	log.Warn("all balance", "balance_get", balance_get, "balance_given", balance_given, "balance", balance)
	if balance.Uint64() <= 10000 {
		log.Info("testReward success ...")
	}else{
		log.Info("testReward fail ...")
	}
}

