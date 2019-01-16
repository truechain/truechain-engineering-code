//// Copyright 2017 The go-ethereum Authors
//// This file is part of the go-ethereum library.
////
//// The go-ethereum library is free software: you can redistribute it and/or modify
//// it under the terms of the GNU Lesser General Public License as published by
//// the Free Software Foundation, either version 3 of the License, or
//// (at your option) any later version.
////
//// The go-ethereum library is distributed in the hope that it will be useful,
//// but WITHOUT ANY WARRANTY; without even the implied warranty of
//// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
//// GNU Lesser General Public License for more details.
////
//// You should have received a copy of the GNU Lesser General Public License
//// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
//
package core

//
import (
	"math/big"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	snaildb "github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
)

func TestDefaultGenesisBlock(t *testing.T) {
	block := DefaultGenesisBlock().ToFastBlock(nil)
	if block.Hash() != params.MainnetGenesisHash {
		t.Errorf("wrong mainnet genesis hash, got %v, want %v", common.ToHex(block.Hash().Bytes()), params.MainnetGenesisHash)
	}
	block = DefaultTestnetGenesisBlock().ToFastBlock(nil)
	if block.Hash() != params.TestnetGenesisHash {
		t.Errorf("wrong testnet genesis hash, got %v, want %v", common.ToHex(block.Hash().Bytes()), params.TestnetGenesisHash)
	}
}

func TestSetupGenesis(t *testing.T) {
	var (
		customghash = common.HexToHash("0x03f61b60215e655a0e79c5242a4ddbeb6dc6d7f8b82009d5c0931494b1f20455")
		customg     = Genesis{
			Alloc: types.GenesisAlloc{
				{1}: {Balance: big.NewInt(1), Storage: map[common.Hash]common.Hash{{1}: {1}}},
			},
		}
		oldcustomg = customg
	)
	oldcustomg.Config = &params.ChainConfig{}
	tests := []struct {
		name       string
		fn         func(etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error)
		wantConfig *params.ChainConfig
		wantHash   common.Hash
		wantErr    error
	}{
		{
			name: "genesis without ChainConfig",
			fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
				return SetupGenesisBlock(db, new(Genesis))
			},
			wantErr:    errGenesisNoConfig,
			wantConfig: params.AllMinervaProtocolChanges,
		},
		{
			name: "no block in DB, genesis == nil",
			fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "mainnet block in DB, genesis == nil",
			fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
				DefaultGenesisBlock().MustFastCommit(db)
				DefaultGenesisBlock().MustSnailCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "custom block in DB, genesis == testnet",
			fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
				customg.MustFastCommit(db)
				customg.MustSnailCommit(db)
				return SetupGenesisBlock(db, DefaultTestnetGenesisBlock())
			},
			wantErr:    &GenesisMismatchError{Stored: customghash, New: params.TestnetGenesisHash},
			wantHash:   params.TestnetGenesisHash,
			wantConfig: params.TestnetChainConfig,
		},
		// {
		// 	name: "compatible config in DB",
		// 	fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, error, *params.ChainConfig, common.Hash, error) {
		// 		oldcustomg.MustFastCommit(db)
		// 		oldcustomg.MustSnailCommit(db)
		// 		return SetupGenesisBlock(db, &customg)
		// 	},
		// 	wantHash:   customghash,
		// 	wantConfig: customg.Config,
		// },
		// {
		// 	name: "incompatible config in DB",
		// 	fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, error, *params.ChainConfig, common.Hash, error) {
		// 		// Commit the 'old' genesis block with Homestead transition at #2.
		// 		// Advance to block #4, past the homestead transition block of customg.
		// 		genesis := oldcustomg.MustFastCommit(db)

		// 		// bc, _ := NewFastBlockChain(db, nil, oldcustomg.Config, ethash.NewFullFaker(), vm.Config{})
		// 		// defer bc.Stop()

		// 		blocks, _ := GenerateChain(oldcustomg.Config, genesis, ethash.NewFaker(), db, 4, nil)
		// 		// bc.InsertChain(blocks)
		// 		// bc.CurrentBlock()
		// 		// This should return a compatibility error.
		// 		return SetupGenesisBlock(db, &customg)
		// 	},
		// 	wantHash:   customghash,
		// 	wantConfig: customg.Config,
		// 	wantErr: &params.ConfigCompatError{
		// 		What:         "Homestead fork block",
		// 		StoredConfig: big.NewInt(2),
		// 		NewConfig:    big.NewInt(3),
		// 		RewindTo:     1,
		// 	},
		// },
	}

	for _, test := range tests {
		db := etruedb.NewMemDatabase()
		config, hash, _, err := test.fn(db)
		// Check the return values.
		if !reflect.DeepEqual(err, test.wantErr) {
			spew := spew.ConfigState{DisablePointerAddresses: true, DisableCapacities: true}
			t.Errorf("%s: returned error %#v, want %#v", test.name, spew.NewFormatter(err), spew.NewFormatter(test.wantErr))
		}
		if !reflect.DeepEqual(config, test.wantConfig) {
			t.Errorf("%s:\nreturned %v\nwant     %v", test.name, config, test.wantConfig)
		}
		if hash != test.wantHash {
			t.Errorf("%s: returned hash %s, want %s", test.name, hash.Hex(), test.wantHash.Hex())
		} else if err == nil {
			// Check database content.
			stored := rawdb.ReadBlock(db, test.wantHash, 0)
			if stored.Hash() != test.wantHash {
				t.Errorf("%s: block in DB has hash %s, want %s", test.name, stored.Hash(), test.wantHash)
			}
		}
	}
}

func TestDefaultSnailGenesisBlock(t *testing.T) {
	block := DefaultGenesisBlock().ToSnailBlock(nil)
	if block.Hash() != params.MainnetSnailGenesisHash {
		t.Errorf("wrong mainnet genesis hash, got %v, want %v", common.ToHex(block.Hash().Bytes()), params.MainnetSnailGenesisHash)
	}
	block = DefaultTestnetGenesisBlock().ToSnailBlock(nil)
	if block.Hash() != params.TestnetSnailGenesisHash {
		t.Errorf("wrong testnet genesis hash, got %v, want %v", common.ToHex(block.Hash().Bytes()), params.TestnetSnailGenesisHash)
	}
}

func TestSetupSnailGenesis(t *testing.T) {
	var (
		//customghash = common.HexToHash("0x62e8674fcc8df82c74aad443e97c4cfdb748652ea117c8afe86cd4a04e5f44f8")
		customg = Genesis{
			Alloc: types.GenesisAlloc{
				{1}: {Balance: big.NewInt(1), Storage: map[common.Hash]common.Hash{{1}: {1}}},
			},
		}
		oldcustomg = customg
	)
	oldcustomg.Config = &params.ChainConfig{}
	tests := []struct {
		name       string
		fn         func(etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error)
		wantConfig *params.ChainConfig
		wantHash   common.Hash
		wantErr    error
	}{
		{
			name: "genesis without ChainConfig",
			fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
				return SetupGenesisBlock(db, new(Genesis))
			},
			wantErr:    errGenesisNoConfig,
			wantConfig: params.AllMinervaProtocolChanges,
		},
		{
			name: "no block in DB, genesis == nil",
			fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetSnailGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "mainnet block in DB, genesis == nil",
			fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
				DefaultGenesisBlock().MustFastCommit(db)
				DefaultGenesisBlock().MustSnailCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetSnailGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		// {
		// 	name: "custom block in DB, genesis == testnet",
		// 	fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, common.Hash, error) {
		// 		//customg.MustFastCommit(db)
		// 		customg.MustSnailCommit(db)
		// 		return SetupGenesisBlock(db, DefaultTestnetGenesisBlock())
		// 	},
		// 	wantErr:    &GenesisMismatchError{Stored: customghash, New: params.TestnetSnailGenesisHash},
		// 	wantHash:   params.TestnetSnailGenesisHash,
		// 	wantConfig: params.TestnetChainConfig,
		// },
		// {
		// 	name: "compatible config in DB",
		// 	fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, error, *params.ChainConfig, common.Hash, error) {
		// 		oldcustomg.MustFastCommit(db)
		// 		oldcustomg.MustSnailCommit(db)
		// 		return SetupGenesisBlock(db, &customg)
		// 	},
		// 	wantHash:   customghash,
		// 	wantConfig: customg.Config,
		// },
		// {
		// 	name: "incompatible config in DB",
		// 	fn: func(db etruedb.Database) (*params.ChainConfig, common.Hash, error, *params.ChainConfig, common.Hash, error) {
		// 		// Commit the 'old' genesis block with Homestead transition at #2.
		// 		// Advance to block #4, past the homestead transition block of customg.
		// 		genesis := oldcustomg.MustFastCommit(db)

		// 		// bc, _ := NewFastBlockChain(db, nil, oldcustomg.Config, ethash.NewFullFaker(), vm.Config{})
		// 		// defer bc.Stop()

		// 		blocks, _ := GenerateChain(oldcustomg.Config, genesis, ethash.NewFaker(), db, 4, nil)
		// 		// bc.InsertChain(blocks)
		// 		// bc.CurrentBlock()
		// 		// This should return a compatibility error.
		// 		return SetupGenesisBlock(db, &customg)
		// 	},
		// 	wantHash:   customghash,
		// 	wantConfig: customg.Config,
		// 	wantErr: &params.ConfigCompatError{
		// 		What:         "Homestead fork block",
		// 		StoredConfig: big.NewInt(2),
		// 		NewConfig:    big.NewInt(3),
		// 		RewindTo:     1,
		// 	},
		// },
	}

	for _, test := range tests {
		db := etruedb.NewMemDatabase()
		config, _, hash, err := test.fn(db)
		// Check the return values.
		if !reflect.DeepEqual(err, test.wantErr) {
			spew := spew.ConfigState{DisablePointerAddresses: true, DisableCapacities: true}
			t.Errorf("%s: returned error %#v, want %#v", test.name, spew.NewFormatter(err), spew.NewFormatter(test.wantErr))
		}
		if !reflect.DeepEqual(config, test.wantConfig) {
			t.Errorf("%s:\nreturned %v\nwant     %v", test.name, config, test.wantConfig)
		}
		if hash != test.wantHash {
			t.Errorf("%s: returned hash %s, want %s", test.name, hash.Hex(), test.wantHash.Hex())
		} else if err == nil {
			// Check database content.
			stored := snaildb.ReadBlock(db, test.wantHash, 0)
			if stored.Hash() != test.wantHash {
				t.Errorf("%s: block in DB has hash %s, want %s", test.name, stored.Hash(), test.wantHash)
			}
		}
	}
}
