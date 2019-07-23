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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/core/types"
)

// ReadFtLookupEntry retrieves the positional metadata associated with a fruit
// hash to allow retrieving the fruit by hash.
func ReadFtLookupEntry(db DatabaseReader, fastHash common.Hash) (common.Hash, uint64, uint64) {
	data, _ := db.Get(ftLookupKey(fastHash))
	if len(data) == 0 {
		return common.Hash{}, 0, 0
	}
	var entry FtLookupEntry
	if err := rlp.DecodeBytes(data, &entry); err != nil {
		log.Error("Invalid fruit lookup entry RLP", "hash", fastHash, "err", err)
		return common.Hash{}, 0, 0
	}
	return entry.BlockHash, entry.BlockIndex, entry.Index
}

// WriteFtLookupEntries stores a positional metadata for every fruit from
// a block, enabling hash based fruit lookups.
func WriteFtLookupEntries(db DatabaseWriter, block *types.SnailBlock) {
	for i, ft := range block.Fruits() {
		entry := FtLookupEntry{
			BlockHash:  block.Hash(),
			BlockIndex: block.NumberU64(),
			Index:      uint64(i),
		}
		data, err := rlp.EncodeToBytes(entry)
		if err != nil {
			log.Crit("Failed to encode fruit lookup entry", "err", err)
		}
		if err := db.Put(ftLookupKey(ft.FastHash()), data); err != nil {
			log.Crit("Failed to store fruit lookup entry", "err", err)
		}
	}
}

// DeleteFtLookupEntry removes all fruit data associated with a hash.
func DeleteFtLookupEntry(db DatabaseDeleter, fastHash common.Hash) {
	db.Delete(ftLookupKey(fastHash))
}

// ReadFruit retrieves a specific fruit from the database, along with
// its added positional metadata.
func ReadFruit(db DatabaseReader, fastHash common.Hash) (*types.SnailBlock, common.Hash, uint64, uint64) {
	blockHash, blockNumber, ftIndex := ReadFtLookupEntry(db, fastHash)

	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}

	body := ReadBody(db, blockHash, blockNumber)

	if body == nil || len(body.Fruits) <= int(ftIndex) {
		log.Error("Fruit referenced missing", "number", blockNumber, "hash", blockHash, "index", ftIndex)
		return nil, common.Hash{}, 0, 0
	}
	return body.Fruits[ftIndex], blockHash, blockNumber, ftIndex
}

// WriteFtHeadLookupEntries stores a positional metadata for every fruit from
// a block, enabling hash based fruit lookups.
func WriteFtHeadLookupEntries(db DatabaseWriter, head *types.SnailHeader, fruitHeads []*types.SnailHeader) {
	for i, ft := range fruitHeads {
		entry := FtLookupEntry{
			BlockHash:  head.Hash(),
			BlockIndex: head.Number.Uint64(),
			Index:      uint64(i),
		}
		data, err := rlp.EncodeToBytes(entry)
		if err != nil {
			log.Crit("Failed to encode fruit lookup entry", "err", err)
		}
		if err := db.Put(ftLookupKey(ft.FastHash), data); err != nil {
			log.Crit("Failed to store fruit lookup entry", "err", err)
		}
	}
}

// ReadFruitHead retrieves a specific fruit from the database, along with
// its added positional metadata.
func ReadFruitHead(db DatabaseReader, fastHash common.Hash) (*types.SnailHeader, common.Hash, uint64, uint64) {
	blockHash, blockNumber, ftIndex := ReadFtLookupEntry(db, fastHash)

	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}

	heads := ReadFruitsHead(db, blockHash, blockNumber)

	if heads == nil || len(heads) <= int(ftIndex) {
		log.Error("Fruit head referenced missing", "number", blockNumber, "hash", blockHash, "index", ftIndex)
		return nil, common.Hash{}, 0, 0
	}
	return heads[ftIndex], blockHash, blockNumber, ftIndex
}
