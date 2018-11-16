// Copyright 2018 The TrueChain Authors
// This file is part of the truechain-engineering-code library.
//
// The truechain-engineering-code library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The truechain-engineering-code library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the truechain-engineering-code library. If not, see <http://www.gnu.org/licenses/>.

package election

import (
	"math/big"
	"testing"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/ethdb"
	// "github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	// canonicalSeed = 1
)

func makeTestBlock() *types.Block {
	db := ethdb.NewMemDatabase()
	BaseGenesis := new(Genesis)
	genesis := BaseGenesis.MustFastCommit(db)
	header := &types.Header{
		ParentHash: genesis.Hash(),
		Number:     common.Big1,
		GasLimit:   FastCalcGasLimit(genesis),
	}
	fb := types.NewBlock(header, nil, nil, nil)
	return fb
}

func TestElectionTestMode(t *testing.T) {
	// TestMode election return a local static committee, whose members are generated barely
	// by local node
	election := NewFakeElection()
	members := election.GetCommittee(common.Big1)
	if int64(len(members)) != params.MinimumCommitteeNumber.Int64() {
		t.Errorf("Commit members count error %v", len(members))
	}
}

func TestVerifySigns(t *testing.T) {
	// TestMode election return a local static committee, whose members are generated barely
	// by local node
	election := NewFakeElection()
	pbftSigns, err := election.GenerateFakeSigns(makeTestBlock())
	if err != nil {
		t.Errorf("Generate fake sign failed")
	}
	members, errs := election.VerifySigns(pbftSigns)

	for _, m := range members {
		if m == nil {
			t.Errorf("Pbft fake signs get invalid member")
		}
	}
	for _, err := range errs {
		if err != nil {
			t.Errorf("Pbft fake signs failed, error=%v", err)
		}
	}
}

func committeeEqual(left, right []*types.CommitteeMember) bool {
	var members map[common.Address]*types.CommitteeMember
	for _, l := range left {
		members[l.Coinbase] = l
	}
	for _, r := range right {
		if m, ok := members[r.Coinbase]; ok {
			if string(crypto.FromECDSAPub(m.Publickey)) != string(crypto.FromECDSAPub(r.Publickey)) {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

/*
func makeChain(n int) (*snailchain.SnailBlockChain, *core.BlockChain) {
	var (
		testdb  = ethdb.NewMemDatabase()
		// genesis = new(core.Genesis).MustSnailCommit(testdb)
		genesis = core.DefaultGenesisBlock()
		engine  = minerva.NewFaker()
	)
	cache := &core.CacheConfig{
		TrieNodeLimit: DefaultConfig.TrieCache,
		TrieTimeLimit: DefaultConfig.TrieTimeout,
	}
	fastGenesis := genesis.MustFastCommit(testdb)
	fastchain, err := core.NewBlockChain(testdb, cache, params.AllMinervaProtocolChanges, engine, vm.Config{})
	fastblocks := makeFast(fastGenesis, n * params.MinimumFruits, engine, testdb, canonicalSeed)
	fastchain.InsertChain(fastblocks)

	snailGenesis := genesis.MustSnailCommit(testdb)
	snailchain, _ := snailchain.NewSnailBlockChain(testdb, nil, params.TestChainConfig, engine, vm.Config{})
	blocks := makeSnail(fastchain, snailGenesis, n, engine, testdb, canonicalSeed)
	snailchain.InsertChain(blocks)

	return snailchain, fastchain
}

func makeSnail(fastChain *core.BlockChain, parent *types.SnailBlock, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.SnailBlock {
	blocks := snailchain.GenerateChain(params.TestChainConfig, fastChain, parent, engine, db, n, func(i int, b *snailchain.BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})
	return blocks
}

// makeBlockChain creates a deterministic chain of blocks rooted at parent.
func makeFast(parent *types.Block, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.Block {
	engine.SetElection(core.NewFakeElection())
	blocks, _ := core.GenerateChain(params.TestChainConfig, parent, engine, db, n, func(i int, b *core.BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})

	return blocks
}
*/

func TestElection1Members(t *testing.T) {
	// snail := newSnail(180)
	election := NewFakeElection()
	members := election.electCommittee(big.NewInt(1), big.NewInt(168))
	if len(members) == 0 {
		t.Errorf("Committee election get none member")
	}
	if int64(len(members)) > params.MaximumCommitteeNumber.Int64() {
		t.Errorf("Elected members exceed MAX member num")
	}
}

func TestElection2Members(t *testing.T) {
	// snail := newSnail(180)
	// defer snail.Stop()
	election := NewFakeElection()

	end := new(big.Int).Mul(big.NewInt(2), params.ElectionPeriodNumber)
	end.Sub(end, params.SnailConfirmInterval)
	begin := new(big.Int).Add(new(big.Int).Sub(end, params.ElectionPeriodNumber), common.Big1)

	members := election.electCommittee(begin, end)
	if len(members) == 0 {
		t.Errorf("Committee election get none member")
	}
	if int64(len(members)) > params.MaximumCommitteeNumber.Int64() {
		t.Errorf("Elected members exceed MAX member num")
	}
}

func TestGenesisCommittee(t *testing.T) {
	nums := []int64{1, 2, 3, 168, 179, 180}
	snail, fast := makeChain(180)
	// snail := newSnail(180)
	// defer snail.Stop()
	// t.Logf("create snail chain %v", snail.CurrentBlock().Number())
	election := core.NewElction(fast, snail, nil)

	// Get Genesis Committee
	for _, n := range nums {
		members := election.GetCommittee(big.NewInt(n))
		if !committeeEqual(members, snail.GetGenesisCommittee()) {
			t.Errorf("Elected members error for fast 1")
		}
	}
}
func TestGetCommittee(t *testing.T) {
	// snail := newSnail(360)
	// defer snail.Stop()
	// t.Logf("create snail chain %v", snail.CurrentBlock().Number())
	election := NewFakeElection()
	last := election.getLastNumber(big.NewInt(1), big.NewInt(168))
	members := election.electCommittee(big.NewInt(1), big.NewInt(168))

	if !committeeEqual(election.GetCommittee(last), election.snailchain.GetGenesisCommittee()) {
		t.Errorf("Elected members error for genesis committee last fast")
	}

	if !committeeEqual(election.GetCommittee(new(big.Int).Add(last, common.Big1)), members) {
		t.Errorf("Elected members error for committee1 first fast")
	}
}