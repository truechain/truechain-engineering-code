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
	"testing"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/election"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/params"
)

func TestValidateBody(t *testing.T) {
	snail, fast, block := makeChain(1)
	validator := NewBlockValidator(snail.chainConfig, fast, snail, snail.Engine())
	validator.ValidateBody(block)
}

func makeChain(n int) (*SnailBlockChain, *core.BlockChain, *types.SnailBlock) {
	var (
		testdb = ethdb.NewMemDatabase()
		// genesis = new(core.Genesis).MustSnailCommit(testdb)
		genesis = core.DefaultGenesisBlock()
		engine  = minerva.NewFaker()
	)

	//blocks := make(types.SnailBlocks, 2)
	cache := &core.CacheConfig{}
	fastGenesis := genesis.MustFastCommit(testdb)
	fastchain, _ := core.NewBlockChain(testdb, cache, params.AllMinervaProtocolChanges, engine, vm.Config{})
	fastblocks := makeFast(fastGenesis, n*params.MinimumFruits, engine, testdb, canonicalSeed)
	fastchain.InsertChain(fastblocks)

	snailGenesis := genesis.MustSnailCommit(testdb)
	snailChain, _ := NewSnailBlockChain(testdb, nil, params.TestChainConfig, engine, vm.Config{})

	blocks1, err := MakeSnailBlockFruits(snailChain, fastchain, 1, 2, 1, 120, snailGenesis.PublicKey(), snailGenesis.Coinbase(), true, nil)
	if err != nil {
		return nil, nil, nil
	}
	//snailChain.InsertChain(blocks1)

	//InsertChain(blocks)

	return snailChain, fastchain, blocks1[0]
}

func makeSnail(fastChain *core.BlockChain, parent *types.SnailBlock, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.SnailBlock {
	blocks := GenerateChain(params.TestChainConfig, fastChain, parent, engine, db, n, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})
	return blocks
}

// makeBlockChain creates a deterministic chain of blocks rooted at parent.
func makeFast(parent *types.Block, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.Block {
	engine.SetElection(election.NewFakeElection())
	blocks, _ := core.GenerateChain(params.TestChainConfig, parent, engine, db, n, func(i int, b *core.BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})

	return blocks
}
