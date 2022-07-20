// Copyright 2020 The go-ethereum Authors
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

package gasprice

import (
	"context"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"math"
	"math/big"
	"testing"

	"github.com/truechain/truechain-engineering-code/common"
	ethash "github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
)

type testBackend struct {
	chain *core.BlockChain
}

func (b *testBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	if number == rpc.LatestBlockNumber {
		return b.chain.CurrentBlock().Header(), nil
	}
	return b.chain.GetHeaderByNumber(uint64(number)), nil
}

func (b *testBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	if number == rpc.LatestBlockNumber {
		return b.chain.CurrentBlock(), nil
	}
	return b.chain.GetBlockByNumber(uint64(number)), nil
}

func (b *testBackend) ChainConfig() *params.ChainConfig {
	return b.chain.Config()
}

func newTestBackend(t *testing.T) *testBackend {
	var (
		key, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr   = crypto.PubkeyToAddress(key.PublicKey)
		gspec  = &core.Genesis{
			Config: params.TestChainConfig,
			Alloc:  types.GenesisAlloc{addr: {Balance: big.NewInt(math.MaxInt64)}},
		}
		signer = types.NewTIP1Signer(gspec.Config.ChainID)
	)

	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)
	gspec.Config.TIP7 = &params.BlockConfig{FastNumber: big.NewInt(10000)}
	gspec.Config.TIP8 = &params.BlockConfig{FastNumber: big.NewInt(1000), CID: big.NewInt(10)}
	gspec.Config.TIP9 = &params.BlockConfig{SnailNumber: big.NewInt(1000)}
	gspec.Config.TIP10 = &params.BlockConfig{FastNumber: big.NewInt(1000)}
	gspec.Config.TIP13 = &params.BlockConfig{FastNumber: big.NewInt(0), SnailNumber: big.NewInt(0)}

	engine := ethash.NewFaker()
	db := etruedb.NewMemDatabase()
	genesis, _ := gspec.CommitFast(db)

	// Generate testing blocks
	blocks, _ := core.GenerateChain(gspec.Config, genesis, engine, db, 32, func(i int, b *core.BlockGen) {
		b.SetCoinbase(common.Address{1})
		tx, err := types.SignTx(types.NewTransaction(b.TxNonce(addr), common.HexToAddress("deadbeef"), big.NewInt(100), 21000, big.NewInt(params.Babbage), nil), signer, key)
		if err != nil {
			t.Fatalf("failed to create tx: %v", err)
		}
		b.AddTx(tx)
		tx, err = types.SignTx(types.NewTransaction(b.TxNonce(addr), common.HexToAddress("deadbeef"), big.NewInt(100), 21000, big.NewInt(int64(i+20)*params.Babbage), nil), signer, key)
		if err != nil {
			t.Fatalf("failed to create tx: %v", err)
		}
		b.AddTx(tx)
	})
	// Construct testing chain
	diskdb := etruedb.NewMemDatabase()
	gspec.CommitFast(diskdb)

	chain, err := core.NewBlockChain(diskdb, nil, gspec.Config, engine, vm.Config{})
	if err != nil {
		t.Fatalf("Failed to create local chain, %v", err)
	}
	chain.InsertChain(blocks)
	return &testBackend{chain: chain}
}

func (b *testBackend) CurrentHeader() *types.Header {
	return b.chain.CurrentHeader()
}

func (b *testBackend) GetBlockByNumber(number uint64) *types.Block {
	return b.chain.GetBlockByNumber(number)
}

func TestSuggestPrice(t *testing.T) {
	config := Config{
		Blocks:     20,
		Percentile: 60,
		Default:    big.NewInt(1 * params.Babbage),
	}
	backend := newTestBackend(t)
	oracle := NewOracle(backend, config)

	// The gas price sampled is: 32G, 31G, 30G, 29G, 28G, 27G
	got, err := oracle.SuggestPrice(context.Background())
	if err != nil {
		t.Fatalf("Failed to retrieve recommended gas price: %v", err)
	}
	expect := big.NewInt(params.Babbage * int64(30))
	if got.Cmp(expect) != 0 {
		t.Fatalf("Gas price mismatch, want %d, got %d", expect, got)
	}
}
