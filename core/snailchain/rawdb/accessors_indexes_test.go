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
	"testing"

	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/ethdb"
)

// Tests that positional lookup metadata can be stored and retrieved.
func TestLookupStorage(t *testing.T) {
	db := ethdb.NewMemDatabase()

	ft1 := types.NewSnailBlockWithHeader(&types.SnailHeader{Extra: []byte("fruit1 header")})
	ft2 := types.NewSnailBlockWithHeader(&types.SnailHeader{Extra: []byte("fruit2 header")})
	ft3 := types.NewSnailBlockWithHeader(&types.SnailHeader{Extra: []byte("fruit3 header")})
	fts := []*types.SnailBlock{ft1, ft2, ft3}

	snailHeader := types.SnailHeader{Extra: []byte("test header")}
	block := types.NewSnailBlock(&snailHeader, fts, []*types.PbftSign{},  []*types.SnailHeader{})

	// Check that no transactions entries are in a pristine database
	for i, tx := range fts {
		if txn, _, _, _ := ReadFruit(db, tx.Hash()); txn != nil {
			t.Fatalf("tx #%d [%x]: non existent fruit returned: %v", i, tx.Hash(), txn)
		}
	}
	// Insert all the transactions into the database, and verify contents
	WriteBlock(db, block)
	WriteFtLookupEntries(db, block)

	for i, tx := range fts {
		if txn, hash, number, index := ReadFruit(db, tx.Hash()); txn == nil {
			t.Fatalf("tx #%d [%x]: fruit not found", i, tx.Hash())
		} else {
			if hash != block.Hash() || number != block.NumberU64() || index != uint64(i) {
				t.Fatalf("tx #%d [%x]: positional metadata mismatch: have %x/%d/%d, want %x/%v/%v", i, tx.Hash(), hash, number, index, block.Hash(), block.NumberU64(), i)
			}
			if tx.Hash() != txn.Hash() {
				t.Fatalf("tx #%d [%x]: transaction mismatch: have %v, want %v", i, tx.Hash(), txn, tx)
			}
		}
	}
	// Delete the transactions and check purge
	for i, tx := range fts {
		DeleteFtLookupEntry(db, tx.Hash())
		if txn, _, _, _ := ReadFruit(db, tx.Hash()); txn != nil {
			t.Fatalf("tx #%d [%x]: deleted transaction returned: %v", i, tx.Hash(), txn)
		}
	}
}
