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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"os"
	"testing"
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
