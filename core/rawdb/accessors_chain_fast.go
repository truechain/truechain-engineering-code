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
	"encoding/binary"
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/rlp"
)

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadCanonicalHash_Fast(db DatabaseReader, number uint64) common.Hash {
	data, _ := db.Get(headerHashKey_Fast(number))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteCanonicalHash_Fast(db DatabaseWriter, hash common.Hash, number uint64) {
	if err := db.Put(headerHashKey_Fast(number), hash.Bytes()); err != nil {
		log.Crit("Failed to store number to hash mapping", "err", err)
	}
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHash_Fast(db DatabaseDeleter, number uint64) {
	if err := db.Delete(headerHashKey_Fast(number)); err != nil {
		log.Crit("Failed to delete number to hash mapping", "err", err)
	}
}

// ReadHeaderNumber returns the header number assigned to a hash.
func ReadHeaderNumber_Fast(db DatabaseReader, hash common.Hash) *uint64 {
	data, _ := db.Get(headerNumberKey_Fast(hash))
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// ReadHeadHeaderHash retrieves the hash of the current canonical head header.
func ReadHeadHeaderHash_Fast(db DatabaseReader) common.Hash {
	data, _ := db.Get(headHeaderKey_Fast)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadHeaderHash stores the hash of the current canonical head header.
func WriteHeadHeaderHash_Fast(db DatabaseWriter, hash common.Hash) {
	if err := db.Put(headHeaderKey_Fast, hash.Bytes()); err != nil {
		log.Crit("Failed to store last header's hash", "err", err)
	}
}

// ReadHeadBlockHash retrieves the hash of the current canonical head block.
func ReadHeadBlockHash_Fast(db DatabaseReader) common.Hash {
	data, _ := db.Get(headBlockKey_Fast)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHash_Fast(db DatabaseWriter, hash common.Hash) {
	if err := db.Put(headBlockKey_Fast, hash.Bytes()); err != nil {
		log.Crit("Failed to store last block's hash", "err", err)
	}
}

// ReadFastTrieProgress retrieves the number of tries nodes fast synced to allow
// reporting correct numbers across restarts.
func ReadFastTrieProgress_Fast(db DatabaseReader) uint64 {
	data, _ := db.Get(fastTrieProgressKey_Fast)
	if len(data) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(data).Uint64()
}

// ReadHeadFastBlockHash retrieves the hash of the current fast-sync head block.
func ReadHeadFastBlockHash_Fast(db DatabaseReader) common.Hash {
	data, _ := db.Get(headFastBlockKey_Fast)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

func WriteHeadFastBlockHash_Fast(db DatabaseWriter, hash common.Hash) {
	if err := db.Put(headFastBlockKey_Fast, hash.Bytes()); err != nil {
		log.Crit("Failed to store last fast block's hash", "err", err)
	}
}

// WriteFastTrieProgress stores the fast sync trie process counter to support
// retrieving it across restarts.
func WriteFastTrieProgress_Fast(db DatabaseWriter, count uint64) {
	if err := db.Put(fastTrieProgressKey_Fast, new(big.Int).SetUint64(count).Bytes()); err != nil {
		log.Crit("Failed to store fast sync trie progress", "err", err)
	}
}

// ReadHeaderRLP retrieves a block header in its raw RLP database encoding.
func ReadHeaderRLP_Fast(db DatabaseReader, hash common.Hash, number uint64) rlp.RawValue {
	data, _ := db.Get(headerKey_Fast(number, hash))
	return data
}

// HasHeader verifies the existence of a block header corresponding to the hash.
func HasHeader_Fast(db DatabaseReader, hash common.Hash, number uint64) bool {
	if has, err := db.Has(headerKey_Fast(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeader_Fast(db DatabaseReader, hash common.Hash, number uint64) *types.FastHeader {
	data := ReadHeaderRLP_Fast(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	header := new(types.FastHeader)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Invalid block header RLP", "hash", hash, "err", err)
		return nil
	}
	return header
}

// WriteHeader stores a block header into the database and also stores the hash-
// to-number mapping.
func WriteHeader_Fast(db DatabaseWriter, header *types.FastHeader) {
	// Write the hash -> number mapping
	var (
		hash    = header.Hash()
		number  = header.Number.Uint64()
		encoded = encodeBlockNumber(number)
	)
	key := headerNumberKey_Fast(hash)
	if err := db.Put(key, encoded); err != nil {
		log.Crit("Failed to store hash to number mapping", "err", err)
	}
	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	key = headerKey_Fast(number, hash)
	if err := db.Put(key, data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeader_Fast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(headerKey_Fast(number, hash)); err != nil {
		log.Crit("Failed to delete header", "err", err)
	}
	if err := db.Delete(headerNumberKey_Fast(hash)); err != nil {
		log.Crit("Failed to delete hash to number mapping", "err", err)
	}
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLP_Fast(db DatabaseReader, hash common.Hash, number uint64) rlp.RawValue {
	data, _ := db.Get(blockBodyKey_Fast(number, hash))
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLP_Fast(db DatabaseWriter, hash common.Hash, number uint64, rlp rlp.RawValue) {
	if err := db.Put(blockBodyKey_Fast(number, hash), rlp); err != nil {
		log.Crit("Failed to store block body", "err", err)
	}
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBody_Fast(db DatabaseReader, hash common.Hash, number uint64) bool {
	if has, err := db.Has(blockBodyKey_Fast(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBody_Fast(db DatabaseReader, hash common.Hash, number uint64) *types.FastBody {
	data := ReadBodyRLP_Fast(db, hash, number)
	if len(data) == 0 {
		return nil
	}
	body := new(types.FastBody)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}
	return body
}

// WriteBody storea a block body into the database.
func WriteBody_Fast(db DatabaseWriter, hash common.Hash, number uint64, body *types.FastBody) {
	data, err := rlp.EncodeToBytes(body)
	if err != nil {
		log.Crit("Failed to RLP encode body", "err", err)
	}
	WriteBodyRLP_Fast(db, hash, number, data)
}

// DeleteBody removes all block body data associated with a hash.
func DeleteBody_Fast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(blockBodyKey_Fast(number, hash)); err != nil {
		log.Crit("Failed to delete block body", "err", err)
	}
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTd_Fast(db DatabaseReader, hash common.Hash, number uint64) *big.Int {
	data, _ := db.Get(headerTDKey_Fast(number, hash))
	if len(data) == 0 {
		return nil
	}
	td := new(big.Int)
	if err := rlp.Decode(bytes.NewReader(data), td); err != nil {
		log.Error("Invalid block total difficulty RLP", "hash", hash, "err", err)
		return nil
	}
	return td
}

// WriteTd stores the total difficulty of a block into the database.
func WriteTd_Fast(db DatabaseWriter, hash common.Hash, number uint64, td *big.Int) {
	data, err := rlp.EncodeToBytes(td)
	if err != nil {
		log.Crit("Failed to RLP encode block total difficulty", "err", err)
	}
	if err := db.Put(headerTDKey_Fast(number, hash), data); err != nil {
		log.Crit("Failed to store block total difficulty", "err", err)
	}
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTd_Fast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(headerTDKey_Fast(number, hash)); err != nil {
		log.Crit("Failed to delete block total difficulty", "err", err)
	}
}

// ReadReceipts retrieves all the transaction receipts belonging to a block.
func ReadReceipts_Fast(db DatabaseReader, hash common.Hash, number uint64) types.Receipts {
	// Retrieve the flattened receipt slice
	data, _ := db.Get(blockReceiptsKey_Fast(number, hash))
	if len(data) == 0 {
		return nil
	}
	// Convert the revceipts from their storage form to their internal representation
	storageReceipts := []*types.ReceiptForStorage{}
	if err := rlp.DecodeBytes(data, &storageReceipts); err != nil {
		log.Error("Invalid receipt array RLP", "hash", hash, "err", err)
		return nil
	}
	receipts := make(types.Receipts, len(storageReceipts))
	for i, receipt := range storageReceipts {
		receipts[i] = (*types.Receipt)(receipt)
	}
	return receipts
}

// WriteReceipts stores all the transaction receipts belonging to a block.
func WriteReceipts_Fast(db DatabaseWriter, hash common.Hash, number uint64, receipts types.Receipts) {
	// Convert the receipts into their storage form and serialize them
	storageReceipts := make([]*types.ReceiptForStorage, len(receipts))
	for i, receipt := range receipts {
		storageReceipts[i] = (*types.ReceiptForStorage)(receipt)
	}
	bytes, err := rlp.EncodeToBytes(storageReceipts)
	if err != nil {
		log.Crit("Failed to encode block receipts", "err", err)
	}
	// Store the flattened receipt slice
	if err := db.Put(blockReceiptsKey_Fast(number, hash), bytes); err != nil {
		log.Crit("Failed to store block receipts", "err", err)
	}
}

// DeleteReceipts removes all receipt data associated with a block hash.
func DeleteReceipts_Fast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(blockReceiptsKey_Fast(number, hash)); err != nil {
		log.Crit("Failed to delete block receipts", "err", err)
	}
}

// ReadBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body. If either the header or body could not
// be retrieved nil is returned.
//
// Note, due to concurrent download of header and block body the header and thus
// canonical hash can be stored in the database but the body data not (yet).
func ReadBlock_Fast(db DatabaseReader, hash common.Hash, number uint64) *types.FastBlock {
	header := ReadHeader_Fast(db, hash, number)
	if header == nil {
		return nil
	}
	body := ReadBody_Fast(db, hash, number)
	if body == nil {
		return nil
	}
	signInfo :=  make([]*types.SignInfo, 0)
	return types.NewFastBlockWithHeader(header).WithBody(body.Transactions, signInfo)
}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlock_Fast(db DatabaseWriter, block *types.FastBlock) {
	WriteBody_Fast(db, block.Hash(), block.NumberU64(), block.Body())
	WriteHeader_Fast(db, block.Header())
}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlock_Fast(db DatabaseDeleter, hash common.Hash, number uint64) {
	DeleteReceipts_Fast(db, hash, number)
	DeleteHeader_Fast(db, hash, number)
	DeleteBody_Fast(db, hash, number)
	DeleteTd_Fast(db, hash, number)
}

// FindCommonAncestor returns the last common ancestor of two block headers
func FindCommonAncestor_Fast(db DatabaseReader, a, b *types.FastHeader) *types.FastHeader {
	for bn := b.Number.Uint64(); a.Number.Uint64() > bn; {
		a = ReadFastHeader(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
	}
	for an := a.Number.Uint64(); an < b.Number.Uint64(); {
		b = ReadFastHeader(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	for a.Hash() != b.Hash() {
		a = ReadFastHeader(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
		b = ReadFastHeader(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	return a
}
