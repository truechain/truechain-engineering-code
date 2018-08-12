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
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/rlp"
)

// ReadFtLookupEntry retrieves the positional metadata associated with a fruit
// hash to allow retrieving the fruit by hash.
func ReadFtLookupEntry(db DatabaseReader, hash common.Hash) (common.Hash, uint64, uint64) {
	data, _ := db.Get(txLookupKey(hash))
	if len(data) == 0 {
		return common.Hash{}, 0, 0
	}
	var entry FtLookupEntry
	if err := rlp.DecodeBytes(data, &entry); err != nil {
		log.Error("Invalid fruit lookup entry RLP", "hash", hash, "err", err)
		return common.Hash{}, 0, 0
	}
	return entry.BlockHash, entry.BlockIndex, entry.Index
}

// WriteFtLookupEntries stores a positional metadata for every fruit from
// a block, enabling hash based fruit lookups.
func WriteFtLookupEntries(db DatabaseWriter, block *types.SnailBlock) {
	for i, tx := range block.Fruits() {
		entry := FtLookupEntry{
			BlockHash:  block.Hash(),
			BlockIndex: block.NumberU64(),
			Index:      uint64(i),
		}
		data, err := rlp.EncodeToBytes(entry)
		if err != nil {
			log.Crit("Failed to encode fruit lookup entry", "err", err)
		}
		if err := db.Put(txLookupKey(tx.Hash()), data); err != nil {
			log.Crit("Failed to store fruit lookup entry", "err", err)
		}
	}
}

// DeleteFtLookupEntry removes all fruit data associated with a hash.
func DeleteFtLookupEntry(db DatabaseDeleter, hash common.Hash) {
	db.Delete(txLookupKey(hash))
}

// ReadFruit retrieves a specific fruit from the database, along with
// its added positional metadata.
func ReadFruit(db DatabaseReader, hash common.Hash) (*types.SnailBlock, common.Hash, uint64, uint64) {
	blockHash, blockNumber, txIndex := ReadFtLookupEntry(db, hash)

	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}

	body := ReadBody(db, blockHash, blockNumber)

	if body == nil || len(body.Fruits) <= int(txIndex) {
		log.Error("Fruit referenced missing", "number", blockNumber, "hash", blockHash, "index", txIndex)
		return nil, common.Hash{}, 0, 0
	}
	return body.Fruits[txIndex], blockHash, blockNumber, txIndex
}

// ReadReceipt retrieves a specific transaction receipt from the database, along with
// its added positional metadata.
func ReadReceipt(db DatabaseReader, hash common.Hash) (*types.Receipt, common.Hash, uint64, uint64) {
	blockHash, blockNumber, receiptIndex := ReadFtLookupEntry(db, hash)
	if blockHash == (common.Hash{}) {
		return nil, common.Hash{}, 0, 0
	}
	receipts := ReadReceipts(db, blockHash, blockNumber)
	if len(receipts) <= int(receiptIndex) {
		log.Error("Receipt refereced missing", "number", blockNumber, "hash", blockHash, "index", receiptIndex)
		return nil, common.Hash{}, 0, 0
	}
	return receipts[receiptIndex], blockHash, blockNumber, receiptIndex
}

// ReadBloomBits retrieves the compressed bloom bit vector belonging to the given
// section and bit index from the.
func ReadBloomBits(db DatabaseReader, bit uint, section uint64, head common.Hash) ([]byte, error) {
	return db.Get(bloomBitsKey(bit, section, head))
}

// WriteBloomBits stores the compressed bloom bits vector belonging to the given
// section and bit index.
func WriteBloomBits(db DatabaseWriter, bit uint, section uint64, head common.Hash, bits []byte) {
	if err := db.Put(bloomBitsKey(bit, section, head), bits); err != nil {
		log.Crit("Failed to store bloom bits", "err", err)
	}
}
