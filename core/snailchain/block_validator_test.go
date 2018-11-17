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
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
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

	tests := []struct {
		name    string
		fn      func() error
		wantErr error
	}{
		{
			name: "valid",
			fn: func() error {
				snail, fast, block := makeChain(1, 0)
				validator := NewBlockValidator(snail.chainConfig, fast, snail, snail.Engine())
				return validator.ValidateBody(block)
			},
			wantErr: nil,
		},
		{
			name: "HasBlockAndState",
			fn: func() error {
				snail, fast, _ := makeChain(1, 0)
				validator := NewBlockValidator(snail.chainConfig, fast, snail, snail.Engine())
				return validator.ValidateBody(validator.bc.CurrentBlock())
			},
			wantErr: ErrKnownBlock,
		},
		{
			name: "ErrInvalidFruits",
			fn: func() error {
				snail, fast, block := makeChain(2, 1)
				validator := NewBlockValidator(snail.chainConfig, fast, snail, snail.Engine())
				return validator.ValidateBody(block)
			},
			wantErr: ErrInvalidFruits,
		},
	}

	for _, test := range tests {

		err := test.fn()
		// Check the return values.
		if !reflect.DeepEqual(err, test.wantErr) {
			spew := spew.ConfigState{DisablePointerAddresses: true, DisableCapacities: true}
			t.Errorf("%s: returned error %#v, want %#v", test.name, spew.NewFormatter(err), spew.NewFormatter(test.wantErr))
		}

	}
}

func makeChain(n int, i int) (*SnailBlockChain, *core.BlockChain, *types.SnailBlock) {
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

	blocks1, err := MakeSnailBlockFruits(snailChain, fastchain, 1, n, 1, n*params.MinimumFruits, snailGenesis.PublicKey(), snailGenesis.Coinbase(), true, nil)
	if err != nil {
		return nil, nil, nil
	}
	//snailChain.InsertChain(blocks1)

	//InsertChain(blocks)

	return snailChain, fastchain, blocks1[i]
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
