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
	"github.com/ethereum/go-ethereum/common"
)

// Tests that positional lookup metadata can be stored and retrieved.
func TestLookupStorage(t *testing.T) {
	db := ethdb.NewMemDatabase()

	ft1 := types.NewSnailBlockWithHeader(&types.SnailHeader{FastHash: common.HexToHash("0x17a333ecba3dd040a0ab06d131a4b47e0d261fe8a8f0d43c8dd79f0f9d99020f"), Extra: []byte("fruit1 header")})
	ft2 := types.NewSnailBlockWithHeader(&types.SnailHeader{FastHash: common.HexToHash("0x27a333ecba3dd040a0ab06d131a4b47e0d261fe8a8f0d43c8dd79f0f9d99020f"), Extra: []byte("fruit2 header")})
	ft3 := types.NewSnailBlockWithHeader(&types.SnailHeader{FastHash: common.HexToHash("0x37a333ecba3dd040a0ab06d131a4b47e0d261fe8a8f0d43c8dd79f0f9d99020f"), Extra: []byte("fruit3 header")})
	fts := []*types.SnailBlock{ft1, ft2, ft3}

	snailHeader := types.SnailHeader{Extra: []byte("test header")}
	block := types.NewSnailBlock(&snailHeader, fts, []*types.PbftSign{},  []*types.SnailHeader{})

	// Check that no transactions entries are in a pristine database
	for i, ft := range fts {
		if ftn, _, _, _ := ReadFruit(db, ft.FastHash()); ftn != nil {
			t.Fatalf("ft #%d [%x]: non existent fruit returned: %v", i, ft.FastHash(), ftn)
		}
	}
	// Insert all the transactions into the database, and verify contents
	WriteBlock(db, block)
	WriteFtLookupEntries(db, block)

	for i, ft := range fts {
		if ftn, hash, number, index := ReadFruit(db, ft.FastHash()); ftn == nil {
			t.Fatalf("ft #%d [%x]: fruit not found", i, ft.FastHash())
		} else {
			if hash != block.Hash() || number != block.NumberU64() || index != uint64(i) {
				t.Fatalf("ft #%d [%x]: positional metadata mismatch: have %x/%d/%d, want %x/%v/%v", i, ft.FastHash(), hash, number, index, block.Hash(), block.NumberU64(), i)
			}
			if ft.FastHash() != ftn.FastHash() {
				t.Fatalf("ft #%d [%x]: transaction mismatch: have %v, want %v", i, ft.FastHash(), ftn, ft)
			}
		}
	}
	// Delete the transactions and check purge
	for i, ft := range fts {
		DeleteFtLookupEntry(db, ft.FastHash())
		if ftn, _, _, _ := ReadFruit(db, ft.FastHash()); ftn != nil {
			t.Fatalf("ft #%d [%x]: deleted transaction returned: %v", i, ft.FastHash(), ftn)
		}
	}
}
