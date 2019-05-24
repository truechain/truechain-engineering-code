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

// Package tests implements execution of Ethereum JSON tests.
package tests

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
)

// A BlockTest checks handling of entire blocks.
type BlockTest struct {
	json btJSON
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (t *BlockTest) UnmarshalJSON(in []byte) error {
	return json.Unmarshal(in, &t.json)
}

type btJSON struct {
	FastBlocks  []btBlock    `json:"fastBlocks"`
	SnailBlocks []snailBlock `json:"snailBlocks"`

	FastGenesis btHeader    `json:"genesisFastBlockHeader"`
	Genesis     snailHeader `json:"genesisBlockHeader"`

	Pre        types.GenesisAlloc     `json:"pre"`
	Post       types.GenesisAlloc     `json:"postState"`
	BestBlock  common.UnprefixedHash  `json:"lastblockhash"`
	Network    string                 `json:"network"`
	Committee  types.CommitteeMembers `json:"committee"`
	SealEngine string                 `json:"sealEngine"`
}

type btBlock struct {
	BlockHeader *btHeader
	Txs         []*types.Transaction
	Signs       []*types.PbftSign
	Infos       []*types.CommitteeMember
	Rlp         string
}

type snailBlock struct {
	BlockHeader *snailHeader
	Fruits      []*types.SnailBlock
	Signs       []*types.PbftSign
	Rlp         string
}

//go:generate gencodec -type btHeader -field-override btHeaderMarshaling -out gen_btheader.go
type btHeader struct {
	SnailHash        common.Hash
	SnailNumber      *big.Int
	CommitteeHash    common.Hash
	Bloom            types.Bloom
	Number           *big.Int
	Hash             common.Hash
	ParentHash       common.Hash
	ReceiptsRoot     common.Hash
	StateRoot        common.Hash
	TransactionsRoot common.Hash
	ExtraData        []byte
	GasLimit         uint64
	GasUsed          uint64
	Timestamp        *big.Int
}

type btHeaderMarshaling struct {
	ExtraData  hexutil.Bytes
	Number     *math.HexOrDecimal256
	Difficulty *math.HexOrDecimal256
	GasLimit   math.HexOrDecimal64
	GasUsed    math.HexOrDecimal64
	Timestamp  *math.HexOrDecimal256
}

//go:generate gencodec -type snailHeader -field-override snailHeaderMarshaling -out gen_snailheader.go
type snailHeader struct {
	ParentHash      common.Hash
	Miner           common.Address
	PointerHash     common.Hash
	PointerNumber   *big.Int
	FruitsHash      common.Hash
	FastHash        common.Hash
	FastNumber      *big.Int
	SignHash        common.Hash
	Difficulty      *big.Int
	FruitDifficulty *big.Int
	Number          *big.Int
	PublicKey       []byte
	Timestamp       *big.Int
	ExtraData       []byte
	MixHash         common.Hash
	Nonce           types.BlockNonce
}

type snailHeaderMarshaling struct {
	ExtraData  hexutil.Bytes
	Number     *math.HexOrDecimal256
	Difficulty *math.HexOrDecimal256
	Timestamp  *math.HexOrDecimal256
}

func (t *BlockTest) Run() error {
	config, ok := Forks[t.json.Network]
	if !ok {
		return UnsupportedForkError{t.json.Network}
	}

	// import pre accounts & construct test genesis block & state root
	db := etruedb.NewMemDatabase()
	genesis := core.DefaultGenesisBlock()
	gblock := genesis.MustFastCommit(db)

	if gblock.Hash() != t.json.FastGenesis.Hash {
		return fmt.Errorf("genesis block hash doesn't match test: computed=%x, test=%x", gblock.Hash().Bytes()[:6], t.json.FastGenesis.Hash[:6])
	}
	if gblock.Root() != t.json.FastGenesis.StateRoot {
		return fmt.Errorf("genesis block state root does not match test: computed=%x, test=%x", gblock.Root().Bytes()[:6], t.json.FastGenesis.StateRoot[:6])
	}

	var engine consensus.Engine
	if t.json.SealEngine == "NoProof" {
		engine = minerva.NewFaker()
	} else {
		engine = minerva.NewShared()
	}

	fastChain, err := core.NewBlockChain(db, nil, config, engine, vm.Config{})

	genesis.MustSnailCommit(db)
	// Initialize a fresh chain with only a genesis block
	blockchain, err := snailchain.NewSnailBlockChain(db, params.TestChainConfig, engine, fastChain)

	if err != nil {
		return err
	}

	defer fastChain.Stop()
	_, err = t.insertFastBlocks(fastChain)

	if err != nil {
		return err
	}

	_, err = t.insertSnailBlocks(blockchain)

	if err != nil {
		return err
	}

	newDB, err := fastChain.State()
	if err != nil {
		return err
	}
	if err = t.validatePostState(newDB); err != nil {
		return fmt.Errorf("post state validation failed: %v", err)
	}
	//return t.validateImportedHeaders(fastChain, validBlocks)

	return nil
}

func (t *BlockTest) genesis(config *params.ChainConfig) *core.Genesis {
	return &core.Genesis{
		Config:     config,
		Timestamp:  t.json.Genesis.Timestamp.Uint64(),
		ParentHash: t.json.Genesis.ParentHash,
		ExtraData:  t.json.Genesis.ExtraData,
		GasLimit:   t.json.FastGenesis.GasLimit,
		GasUsed:    t.json.FastGenesis.GasUsed,
		Alloc:      t.json.Pre,
		Committee:  t.json.Committee,
	}
}

/* See https://github.com/ethereum/tests/wiki/Blockchain-Tests-II

   Whether a block is valid or not is a bit subtle, it's defined by presence of
   blockHeader, transactions and uncleHeaders fields. If they are missing, the block is
   invalid and we must verify that we do not accept it.

   Since some tests mix valid and invalid blocks we need to check this for every block.

   If a block is invalid it does not necessarily fail the test, if it's invalidness is
   expected we are expected to ignore it and continue processing and then validate the
   post state.
*/
func (t *BlockTest) insertFastBlocks(blockchain *core.BlockChain) ([]btBlock, error) {
	validBlocks := make([]btBlock, 0)
	// insert the test blocks, which will execute all transactions
	for _, b := range t.json.FastBlocks {
		cb, err := b.decode()
		if err != nil {
			if b.BlockHeader == nil {
				continue // OK - block is supposed to be invalid, continue with next block
			} else {
				return nil, fmt.Errorf("Block RLP decoding failed when expected to succeed: %v", err)
			}
		}
		// RLP decoding worked, try to insert into chain:
		blocks := types.Blocks{cb}
		i, err := blockchain.InsertChain(blocks)
		if err != nil {
			if b.BlockHeader == nil {
				continue // OK - block is supposed to be invalid, continue with next block
			} else {
				return nil, fmt.Errorf("Block #%v insertion into chain failed: %v", blocks[i].Number(), err)
			}
		}
		if b.BlockHeader == nil {
			return nil, fmt.Errorf("Block insertion should have failed")
		}

		// validate RLP decoding by checking all values against test file JSON
		if err = validateHeader(b.BlockHeader, cb.Header()); err != nil {
			return nil, fmt.Errorf("Deserialised block header validation failed: %v", err)
		}
		validBlocks = append(validBlocks, b)
	}
	return validBlocks, nil
}

/* See https://github.com/ethereum/tests/wiki/Blockchain-Tests-II

   Whether a block is valid or not is a bit subtle, it's defined by presence of
   blockHeader, transactions and uncleHeaders fields. If they are missing, the block is
   invalid and we must verify that we do not accept it.

   Since some tests mix valid and invalid blocks we need to check this for every block.

   If a block is invalid it does not necessarily fail the test, if it's invalidness is
   expected we are expected to ignore it and continue processing and then validate the
   post state.
*/
func (t *BlockTest) insertSnailBlocks(blockchain *snailchain.SnailBlockChain) ([]snailBlock, error) {
	validBlocks := make([]snailBlock, 0)
	// insert the test blocks, which will execute all transactions
	for _, b := range t.json.SnailBlocks {
		cb, err := b.decode()
		if err != nil {
			if b.BlockHeader == nil {
				continue // OK - block is supposed to be invalid, continue with next block
			} else {
				return nil, fmt.Errorf("Block RLP decoding failed when expected to succeed: %v", err)
			}
		}
		// RLP decoding worked, try to insert into chain:
		blocks := types.SnailBlocks{cb}
		i, err := blockchain.InsertChain(blocks)
		if err != nil {
			if b.BlockHeader == nil {
				continue // OK - block is supposed to be invalid, continue with next block
			} else {
				return nil, fmt.Errorf("Block #%v insertion into chain failed: %v", blocks[i].Number(), err)
			}
		}
		if b.BlockHeader == nil {
			return nil, fmt.Errorf("Block insertion should have failed")
		}

		// validate RLP decoding by checking all values against test file JSON
		//if err = validateHeader(b.BlockHeader, cb.Header()); err != nil {
		//	return nil, fmt.Errorf("Deserialised block header validation failed: %v", err)
		//}
		validBlocks = append(validBlocks, b)
	}
	return validBlocks, nil
}

func validateHeader(h *btHeader, h2 *types.Header) error {
	if h.Bloom != h2.Bloom {
		return fmt.Errorf("Bloom: want: %x have: %x", h.Bloom, h2.Bloom)
	}
	if h.Number.Cmp(h2.Number) != 0 {
		return fmt.Errorf("Number: want: %v have: %v", h.Number, h2.Number)
	}
	if h.ParentHash != h2.ParentHash {
		return fmt.Errorf("Parent hash: want: %x have: %x", h.ParentHash, h2.ParentHash)
	}
	if h.ReceiptsRoot != h2.ReceiptHash {
		return fmt.Errorf("Receipt hash: want: %x have: %x", h.ReceiptsRoot, h2.ReceiptHash)
	}
	if h.TransactionsRoot != h2.TxHash {
		return fmt.Errorf("Tx hash: want: %x have: %x", h.TransactionsRoot, h2.TxHash)
	}
	if h.StateRoot != h2.Root {
		return fmt.Errorf("State hash: want: %x have: %x", h.StateRoot, h2.Root)
	}
	if !bytes.Equal(h.ExtraData, h2.Extra) {
		return fmt.Errorf("Extra data: want: %x have: %x", h.ExtraData, h2.Extra)
	}
	if h.GasLimit != h2.GasLimit {
		return fmt.Errorf("GasLimit: want: %d have: %d", h.GasLimit, h2.GasLimit)
	}
	if h.GasUsed != h2.GasUsed {
		return fmt.Errorf("GasUsed: want: %d have: %d", h.GasUsed, h2.GasUsed)
	}
	if h.Timestamp.Cmp(h2.Time) != 0 {
		return fmt.Errorf("Timestamp: want: %v have: %v", h.Timestamp, h2.Time)
	}
	return nil
}

func (t *BlockTest) validatePostState(statedb *state.StateDB) error {
	// validate post state accounts in test file against what we have in state db
	for addr, acct := range t.json.Post {
		// address is indirectly verified by the other fields, as it's the db key
		code2 := statedb.GetCode(addr)
		balance2 := statedb.GetBalance(addr)
		nonce2 := statedb.GetNonce(addr)
		if !bytes.Equal(code2, acct.Code) {
			return fmt.Errorf("account code mismatch for addr: %s want: %v have: %s", addr.String(), acct.Code, hex.EncodeToString(code2))
		}
		if balance2.Cmp(acct.Balance) != 0 {
			return fmt.Errorf("account balance mismatch for addr: %s, want: %d, have: %d", addr.String(), acct.Balance, balance2)
		}
		if nonce2 != acct.Nonce {
			return fmt.Errorf("account nonce mismatch for addr: %s want: %d have: %d", addr.String(), acct.Nonce, nonce2)
		}
	}
	return nil
}

func (t *BlockTest) validateImportedHeaders(cm *core.BlockChain, validBlocks []btBlock) error {
	// to get constant lookup when verifying block headers by hash (some tests have many blocks)
	bmap := make(map[common.Hash]btBlock, len(t.json.FastBlocks))
	for _, b := range validBlocks {
		bmap[b.BlockHeader.Hash] = b
	}
	// iterate over blocks backwards from HEAD and validate imported
	// headers vs test file. some tests have reorgs, and we import
	// block-by-block, so we can only validate imported headers after
	// all blocks have been processed by BlockChain, as they may not
	// be part of the longest chain until last block is imported.
	for b := cm.CurrentBlock(); b != nil && b.NumberU64() != 0; b = cm.GetBlockByHash(b.Header().ParentHash) {
		if err := validateHeader(bmap[b.Hash()].BlockHeader, b.Header()); err != nil {
			return fmt.Errorf("Imported block header validation failed: %v", err)
		}
	}
	return nil
}

func (bb *btBlock) decode() (*types.Block, error) {
	data, err := hexutil.Decode(bb.Rlp)
	if err != nil {
		return nil, err
	}
	var b types.Block
	err = rlp.DecodeBytes(data, &b)
	return &b, err
}

func (bb *snailBlock) decode() (*types.SnailBlock, error) {
	data, err := hexutil.Decode(bb.Rlp)
	if err != nil {
		return nil, err
	}
	var b types.SnailBlock
	err = rlp.DecodeBytes(data, &b)
	return &b, err
}
