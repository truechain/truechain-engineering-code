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

package etrue

import (
	"testing"

	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/core"
)

func makeTestBlock() *types.Block {
	db := ethdb.NewMemDatabase()
	BaseGenesis := new(core.Genesis)
	genesis := BaseGenesis.MustFastCommit(db)
	header := &types.Header{
		ParentHash: genesis.Hash(),
		Number:     common.Big1,
		GasLimit:   core.FastCalcGasLimit(genesis),
	}
	fb := types.NewBlock(header, nil, nil, nil)
	return fb
}

func TestElectionTestMode(t *testing.T) {
	// TestMode election return a local static committee, whose members are generated barely
	// by local node
	election := NewFakeElection()
	members := election.GetCommittee(common.Big1)
	if len(members) != 4 {
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
