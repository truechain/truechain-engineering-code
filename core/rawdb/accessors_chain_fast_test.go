// Copyright 2018 The go-ethereum Authors
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

package rawdb

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/rlp"
)

// Tests block header storage and retrieval operations.
func TestHeaderStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()

	// Create a test header to move around the database and make sure it's really new
	header := &types.FastHeader{Number: big.NewInt(42), Extra: []byte("test header")}
	if entry := ReadHeaderFast(db, header.Hash(), header.Number.Uint64()); entry != nil {
		t.Fatalf("Non existent header returned: %v", entry)
	}

	// Write and verify the header in the database
	WriteHeaderFast(db, header)
	if entry := ReadHeaderFast(db, header.Hash(), header.Number.Uint64()); entry == nil {
		t.Fatalf("Stored header not found")
	} else if entry.Hash() != header.Hash() {
		t.Fatalf("Retrieved header mismatch: have %v, want %v", entry, header)
	}
	if entry := ReadHeaderRLPFast(db, header.Hash(), header.Number.Uint64()); entry == nil {
		t.Fatalf("Stored header RLP not found")
	} else {
		hasher := sha3.NewKeccak256()
		hasher.Write(entry)

		if hash := common.BytesToHash(hasher.Sum(nil)); hash != header.Hash() {
			t.Fatalf("Retrieved RLP header mismatch: have %v, want %v", entry, header)
		}
	}
	// Delete the header and verify the execution
	DeleteHeaderFast(db, header.Hash(), header.Number.Uint64())
	if entry := ReadHeaderFast(db, header.Hash(), header.Number.Uint64()); entry != nil {
		t.Fatalf("Deleted header returned: %v", entry)
	}
}


// Tests block body storage and retrieval operations.
func TestBodyStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()

	// Create a test body to move around the database and make sure it's really new

	//body := &types.FastBody{Uncles: []*types.Header{{Extra: []byte("test header")}}}
	body := &types.FastBody{Transactions:nil}
	hasher := sha3.NewKeccak256()
	rlp.Encode(hasher, body)
	hash := common.BytesToHash(hasher.Sum(nil))

	if entry := ReadBodyFast(db, hash, 0); entry != nil {
		t.Fatalf("Non existent body returned: %v", entry)
	}
	// Write and verify the body in the database
	WriteBodyFast(db, hash, 0, body)
	if entry := ReadBodyFast(db, hash, 0); entry == nil {
		t.Fatalf("Stored body not found")
	} else if types.DeriveSha(types.Transactions(entry.Transactions)) != types.DeriveSha(types.Transactions(body.Transactions)) /*|| types.CalcUncleHash(entry.Transactions) != types.CalcUncleHash(body.Transactions)*/ {
		t.Fatalf("Retrieved body mismatch: have %v, want %v", entry, body)
	}
	if entry := ReadBodyRLPFast(db, hash, 0); entry == nil {
		t.Fatalf("Stored body RLP not found")
	} else {
		hasher := sha3.NewKeccak256()
		hasher.Write(entry)

		if calc := common.BytesToHash(hasher.Sum(nil)); calc != hash {
			t.Fatalf("Retrieved RLP body mismatch: have %v, want %v", entry, body)
		}
	}
	// Delete the body and verify the execution
	DeleteBodyFast(db, hash, 0)
	if entry := ReadBodyFast(db, hash, 0); entry != nil {
		t.Fatalf("Deleted body returned: %v", entry)
	}
}

// Tests block storage and retrieval operations.
func TestBlockStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()

	// Create a test block to move around the database and make sure it's really new
	block := types.NewFastBlockWithHeader(&types.FastHeader{
		Extra:       []byte("test block"),
		//UncleHash:   types.EmptyUncleHash,
		TxHash:      types.EmptyRootHash,
		ReceiptHash: types.EmptyRootHash,
	})
	if entry := ReadBlockFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Non existent block returned: %v", entry)
	}
	if entry := ReadHeaderFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Non existent header returned: %v", entry)
	}
	if entry := ReadBodyFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Non existent body returned: %v", entry)
	}
	// Write and verify the block in the database
	WriteBlockFast(db, block)
	if entry := ReadBlockFast(db, block.Hash(), block.NumberU64()); entry == nil {
		t.Fatalf("Stored block not found")
	} else if entry.Hash() != block.Hash() {
		t.Fatalf("Retrieved block mismatch: have %v, want %v", entry, block)
	}
	if entry := ReadHeaderFast(db, block.Hash(), block.NumberU64()); entry == nil {
		t.Fatalf("Stored header not found")
	} else if entry.Hash() != block.Header().Hash() {
		t.Fatalf("Retrieved header mismatch: have %v, want %v", entry, block.Header())
	}
	if entry := ReadBodyFast(db, block.Hash(), block.NumberU64()); entry == nil {
		t.Fatalf("Stored body not found")
	} else if types.DeriveSha(types.Transactions(entry.Transactions)) != types.DeriveSha(block.Transactions()) /*|| types.CalcUncleHash(entry.Uncles) != types.CalcUncleHash(block.Uncles())*/ {
		t.Fatalf("Retrieved body mismatch: have %v, want %v", entry, block.Body())
	}
	// Delete the block and verify the execution
	DeleteBlockFast(db, block.Hash(), block.NumberU64())
	if entry := ReadBlockFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Deleted block returned: %v", entry)
	}
	if entry := ReadHeaderFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Deleted header returned: %v", entry)
	}
	if entry := ReadBodyFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Deleted body returned: %v", entry)
	}
}

// Tests that partial block contents don't get reassembled into full blocks.
func TestPartialBlockStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()
	block := types.NewFastBlockWithHeader(&types.FastHeader{
		Extra:       []byte("test block"),
		//UncleHash:   types.EmptyUncleHash,
		TxHash:      types.EmptyRootHash,
		ReceiptHash: types.EmptyRootHash,
	})
	// Store a header and check that it's not recognized as a block
	WriteHeaderFast(db, block.Header())
	if entry := ReadBlockFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Non existent block returned: %v", entry)
	}
	DeleteHeaderFast(db, block.Hash(), block.NumberU64())

	// Store a body and check that it's not recognized as a block
	WriteBodyFast(db, block.Hash(), block.NumberU64(), block.Body())
	if entry := ReadBlockFast(db, block.Hash(), block.NumberU64()); entry != nil {
		t.Fatalf("Non existent block returned: %v", entry)
	}
	DeleteBodyFast(db, block.Hash(), block.NumberU64())

	// Store a header and a body separately and check reassembly
	WriteHeaderFast(db, block.Header())
	WriteBodyFast(db, block.Hash(), block.NumberU64(), block.Body())

	if entry := ReadBlockFast(db, block.Hash(), block.NumberU64()); entry == nil {
		t.Fatalf("Stored block not found")
	} else if entry.Hash() != block.Hash() {
		t.Fatalf("Retrieved block mismatch: have %v, want %v", entry, block)
	}
}

// Tests block total difficulty storage and retrieval operations.
func TestTdStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()

	// Create a test TD to move around the database and make sure it's really new
	hash, td := common.Hash{}, big.NewInt(314)
	if entry := ReadTdFast(db, hash, 0); entry != nil {
		t.Fatalf("Non existent TD returned: %v", entry)
	}
	// Write and verify the TD in the database
	WriteTdFast(db, hash, 0, td)
	if entry := ReadTdFast(db, hash, 0); entry == nil {
		t.Fatalf("Stored TD not found")
	} else if entry.Cmp(td) != 0 {
		t.Fatalf("Retrieved TD mismatch: have %v, want %v", entry, td)
	}
	// Delete the TD and verify the execution
	DeleteTdFast(db, hash, 0)
	if entry := ReadTdFast(db, hash, 0); entry != nil {
		t.Fatalf("Deleted TD returned: %v", entry)
	}
}

// Tests that canonical numbers can be mapped to hashes and retrieved.
func TestCanonicalMappingStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()

	// Create a test canonical number and assinged hash to move around
	hash, number := common.Hash{0: 0xff}, uint64(314)
	if entry := ReadCanonicalHashFast(db, number); entry != (common.Hash{}) {
		t.Fatalf("Non existent canonical mapping returned: %v", entry)
	}
	// Write and verify the TD in the database
	WriteCanonicalHashFast(db, hash, number)
	if entry := ReadCanonicalHashFast(db, number); entry == (common.Hash{}) {
		t.Fatalf("Stored canonical mapping not found")
	} else if entry != hash {
		t.Fatalf("Retrieved canonical mapping mismatch: have %v, want %v", entry, hash)
	}
	// Delete the TD and verify the execution
	DeleteCanonicalHashFast(db, number)
	if entry := ReadCanonicalHashFast(db, number); entry != (common.Hash{}) {
		t.Fatalf("Deleted canonical mapping returned: %v", entry)
	}
}

// Tests that head headers and head blocks can be assigned, individually.
func TestHeadStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()

	blockHead := types.NewFastBlockWithHeader(&types.FastHeader{Extra: []byte("test block header")})
	blockFull := types.NewFastBlockWithHeader(&types.FastHeader{Extra: []byte("test block full")})
	blockFast := types.NewFastBlockWithHeader(&types.FastHeader{Extra: []byte("test block fast")})

	// Check that no head entries are in a pristine database
	if entry := ReadHeadHeaderHashFast(db); entry != (common.Hash{}) {
		t.Fatalf("Non head header entry returned: %v", entry)
	}
	if entry := ReadHeadBlockHashFast(db); entry != (common.Hash{}) {
		t.Fatalf("Non head block entry returned: %v", entry)
	}
	if entry := ReadHeadFastBlockHashFast(db); entry != (common.Hash{}) {
		t.Fatalf("Non fast head block entry returned: %v", entry)
	}
	// Assign separate entries for the head header and block
	WriteHeadHeaderHashFast(db, blockHead.Hash())
	WriteHeadBlockHashFast(db, blockFull.Hash())
	WriteHeadFastBlockHashFast(db, blockFast.Hash())

	// Check that both heads are present, and different (i.e. two heads maintained)
	if entry := ReadHeadHeaderHashFast(db); entry != blockHead.Hash() {
		t.Fatalf("Head header hash mismatch: have %v, want %v", entry, blockHead.Hash())
	}
	if entry := ReadHeadBlockHashFast(db); entry != blockFull.Hash() {
		t.Fatalf("Head block hash mismatch: have %v, want %v", entry, blockFull.Hash())
	}
	if entry := ReadHeadFastBlockHashFast(db); entry != blockFast.Hash() {
		t.Fatalf("Fast head block hash mismatch: have %v, want %v", entry, blockFast.Hash())
	}
}

// Tests that receipts associated with a single block can be stored and retrieved.
func TestBlockReceiptStorageFast(t *testing.T) {
	db := ethdb.NewMemDatabase()

	receipt1 := &types.Receipt{
		Status:            types.ReceiptStatusFailed,
		CumulativeGasUsed: 1,
		Logs: []*types.Log{
			{Address: common.BytesToAddress([]byte{0x11})},
			{Address: common.BytesToAddress([]byte{0x01, 0x11})},
		},
		TxHash:          common.BytesToHash([]byte{0x11, 0x11}),
		ContractAddress: common.BytesToAddress([]byte{0x01, 0x11, 0x11}),
		GasUsed:         111111,
	}
	receipt2 := &types.Receipt{
		PostState:         common.Hash{2}.Bytes(),
		CumulativeGasUsed: 2,
		Logs: []*types.Log{
			{Address: common.BytesToAddress([]byte{0x22})},
			{Address: common.BytesToAddress([]byte{0x02, 0x22})},
		},
		TxHash:          common.BytesToHash([]byte{0x22, 0x22}),
		ContractAddress: common.BytesToAddress([]byte{0x02, 0x22, 0x22}),
		GasUsed:         222222,
	}
	receipts := []*types.Receipt{receipt1, receipt2}

	// Check that no receipt entries are in a pristine database
	hash := common.BytesToHash([]byte{0x03, 0x14})
	if rs := ReadReceiptsFast(db, hash, 0); len(rs) != 0 {
		t.Fatalf("non existent receipts returned: %v", rs)
	}
	// Insert the receipt slice into the database and check presence
	WriteReceiptsFast(db, hash, 0, receipts)
	if rs := ReadReceiptsFast(db, hash, 0); len(rs) == 0 {
		t.Fatalf("no receipts returned")
	} else {
		for i := 0; i < len(receipts); i++ {
			rlpHave, _ := rlp.EncodeToBytes(rs[i])
			rlpWant, _ := rlp.EncodeToBytes(receipts[i])

			if !bytes.Equal(rlpHave, rlpWant) {
				t.Fatalf("receipt #%d: receipt mismatch: have %v, want %v", i, rs[i], receipts[i])
			}
		}
	}
	// Delete the receipt slice and check purge
	DeleteReceiptsFast(db, hash, 0)
	if rs := ReadReceiptsFast(db, hash, 0); len(rs) != 0 {
		t.Fatalf("deleted receipts returned: %v", rs)
	}
}
