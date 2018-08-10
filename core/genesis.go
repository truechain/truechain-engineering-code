// Copyright 2014 The go-ethereum Authors
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

package core

import (
	"github.com/truechain/truechain-engineering-code/core/types"

	"errors"

	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/core/fastchain"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/params"
)

//go:generate gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	snailGenesis snailchain.Genesis
	fastGenesis  fastchain.Genesis
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil {
		if &genesis.snailGenesis != nil && genesis.snailGenesis.Config == nil {
			return params.AllEthashProtocolChanges, common.Hash{}, errGenesisNoConfig
		}
		if &genesis.fastGenesis != nil && genesis.fastGenesis.Config == nil {
			return params.AllEthashProtocolChanges, common.Hash{}, errGenesisNoConfig
		}
	}

	snailchain.SetupGenesisBlock(db, &genesis.snailGenesis)
	return fastchain.SetupGenesisBlock(db, &genesis.fastGenesis)
}

// // GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
// func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
// 	g := Genesis{Alloc: GenesisAlloc{addr: {Balance: balance}}}
// 	return g.MustCommit(db)
// }

// be seeded with the
func DeveloperGenesisBlock(period uint64, faucet common.Address) *Genesis {
	// Override the default period to the user requested one
	config := *params.AllCliqueProtocolChanges
	config.Clique.Period = period

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	snailGenesis := snailchain.Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, 65)...),
		GasLimit:   6283185,
		Difficulty: big.NewInt(1),
		// Alloc: map[common.Address]types.GenesisAccount{
		// 	common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
		// 	common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
		// 	common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
		// 	common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
		// 	common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
		// 	common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
		// 	common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
		// 	common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
		// 	faucet: {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		// },
	}
	fastGenesis := fastchain.Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, 65)...),
		GasLimit:   6283185,
		Difficulty: big.NewInt(1),
		Alloc: map[common.Address]types.GenesisAccount{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			faucet: {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
	return &Genesis{
		snailGenesis: snailGenesis,
		fastGenesis:  fastGenesis,
	}
}

// DefaultRinkebyGenesisBlock returns the Rinkeby network genesis block.
func DefaultRinkebyGenesisBlock() *Genesis {
	snailGenesis := snailchain.Genesis{
		Config:     params.RinkebyChainConfig,
		Timestamp:  1492009146,
		ExtraData:  hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e42eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		// Alloc:      types.DecodePrealloc(rinkebyAllocData),
	}
	fastGenesis := fastchain.Genesis{
		Config:     params.RinkebyChainConfig,
		Timestamp:  1492009146,
		ExtraData:  hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e42eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		Alloc:      types.DecodePrealloc(rinkebyAllocData),
	}
	return &Genesis{
		snailGenesis: snailGenesis,
		fastGenesis:  fastGenesis,
	}
}

// DefaultTestnetGenesisBlock returns the Ropsten network genesis block.
func DefaultTestnetGenesisBlock() *Genesis {
	snailGenesis := snailchain.Genesis{
		Config:     params.TestnetChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		GasLimit:   16777216,
		Difficulty: big.NewInt(1048576),
		// Alloc:      types. DecodePrealloc(testnetAllocData),
	}
	fastGenesis := fastchain.Genesis{
		Config:     params.TestnetChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		GasLimit:   16777216,
		Difficulty: big.NewInt(1048576),
		Alloc:      types.DecodePrealloc(testnetAllocData),
	}
	return &Genesis{
		snailGenesis: snailGenesis,
		fastGenesis:  fastGenesis,
	}
}
