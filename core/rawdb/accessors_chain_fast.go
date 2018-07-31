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
func ReadCanonicalHashFast(db DatabaseReader, number uint64) common.Hash {
	data, _ := db.Get(headerHashKeyFast(number))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteCanonicalHashFast(db DatabaseWriter, hash common.Hash, number uint64) {
	if err := db.Put(headerHashKeyFast(number), hash.Bytes()); err != nil {
		log.Crit("Failed to store number to hash mapping", "err", err)
	}
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteCanonicalHashFast(db DatabaseDeleter, number uint64) {
	if err := db.Delete(headerHashKeyFast(number)); err != nil {
		log.Crit("Failed to delete number to hash mapping", "err", err)
	}
}

// ReadHeaderNumber returns the header number assigned to a hash.
func ReadHeaderNumberFast(db DatabaseReader, hash common.Hash) *uint64 {
	data, _ := db.Get(headerNumberKeyFast(hash))
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// ReadHeadHeaderHash retrieves the hash of the current canonical head header.
func ReadHeadHeaderHashFast(db DatabaseReader) common.Hash {
	data, _ := db.Get(headHeaderKeyFast)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadHeaderHash stores the hash of the current canonical head header.
func WriteHeadHeaderHashFast(db DatabaseWriter, hash common.Hash) {
	if err := db.Put(headHeaderKeyFast, hash.Bytes()); err != nil {
		log.Crit("Failed to store last header's hash", "err", err)
	}
}

// ReadHeadBlockHash retrieves the hash of the current canonical head block.
func ReadHeadBlockHashFast(db DatabaseReader) common.Hash {
	data, _ := db.Get(headBlockKeyFast)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteHeadBlockHash stores the head block's hash.
func WriteHeadBlockHashFast(db DatabaseWriter, hash common.Hash) {
	if err := db.Put(headBlockKeyFast, hash.Bytes()); err != nil {
		log.Crit("Failed to store last block's hash", "err", err)
	}
}

// ReadFastTrieProgress retrieves the number of tries nodes fast synced to allow
// reporting correct numbers across restarts.
func ReadFastTrieProgressFast(db DatabaseReader) uint64 {
	data, _ := db.Get(fastTrieProgressKeyFast)
	if len(data) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(data).Uint64()
}

// ReadHeadFastBlockHash retrieves the hash of the current fast-sync head block.
func ReadHeadFastBlockHashFast(db DatabaseReader) common.Hash {
	data, _ := db.Get(headFastBlockKeyFast)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

func WriteHeadFastBlockHashFast(db DatabaseWriter, hash common.Hash) {
	if err := db.Put(headFastBlockKeyFast, hash.Bytes()); err != nil {
		log.Crit("Failed to store last fast block's hash", "err", err)
	}
}

// WriteFastTrieProgress stores the fast sync trie process counter to support
// retrieving it across restarts.
func WriteFastTrieProgressFast(db DatabaseWriter, count uint64) {
	if err := db.Put(fastTrieProgressKeyFast, new(big.Int).SetUint64(count).Bytes()); err != nil {
		log.Crit("Failed to store fast sync trie progress", "err", err)
	}
}

// ReadHeaderRLP retrieves a block header in its raw RLP database encoding.
func ReadHeaderRLPFast(db DatabaseReader, hash common.Hash, number uint64) rlp.RawValue {
	data, _ := db.Get(headerKeyFast(number, hash))
	return data
}

// HasHeader verifies the existence of a block header corresponding to the hash.
func HasHeaderFast(db DatabaseReader, hash common.Hash, number uint64) bool {
	if has, err := db.Has(headerKeyFast(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadHeader retrieves the block header corresponding to the hash.
func ReadHeaderFast(db DatabaseReader, hash common.Hash, number uint64) *types.FastHeader {
	data := ReadHeaderRLPFast(db, hash, number)
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
func WriteHeaderFast(db DatabaseWriter, header *types.FastHeader) {
	// Write the hash -> number mapping
	var (
		hash    = header.Hash()
		number  = header.Number.Uint64()
		encoded = encodeBlockNumber(number)
	)
	key := headerNumberKeyFast(hash)
	if err := db.Put(key, encoded); err != nil {
		log.Crit("Failed to store hash to number mapping", "err", err)
	}
	// Write the encoded header
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		log.Crit("Failed to RLP encode header", "err", err)
	}
	key = headerKeyFast(number, hash)
	if err := db.Put(key, data); err != nil {
		log.Crit("Failed to store header", "err", err)
	}
}

// DeleteHeader removes all block header data associated with a hash.
func DeleteHeaderFast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(headerKeyFast(number, hash)); err != nil {
		log.Crit("Failed to delete header", "err", err)
	}
	if err := db.Delete(headerNumberKeyFast(hash)); err != nil {
		log.Crit("Failed to delete hash to number mapping", "err", err)
	}
}

// ReadBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func ReadBodyRLPFast(db DatabaseReader, hash common.Hash, number uint64) rlp.RawValue {
	data, _ := db.Get(blockBodyKeyFast(number, hash))
	return data
}

// WriteBodyRLP stores an RLP encoded block body into the database.
func WriteBodyRLPFast(db DatabaseWriter, hash common.Hash, number uint64, rlp rlp.RawValue) {
	if err := db.Put(blockBodyKeyFast(number, hash), rlp); err != nil {
		log.Crit("Failed to store block body", "err", err)
	}
}

// HasBody verifies the existence of a block body corresponding to the hash.
func HasBodyFast(db DatabaseReader, hash common.Hash, number uint64) bool {
	if has, err := db.Has(blockBodyKeyFast(number, hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadBody retrieves the block body corresponding to the hash.
func ReadBodyFast(db DatabaseReader, hash common.Hash, number uint64) *types.FastBody {
	data := ReadBodyRLPFast(db, hash, number)
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
func WriteBodyFast(db DatabaseWriter, hash common.Hash, number uint64, body *types.FastBody) {
	data, err := rlp.EncodeToBytes(body)
	if err != nil {
		log.Crit("Failed to RLP encode body", "err", err)
	}
	WriteBodyRLPFast(db, hash, number, data)
}

// DeleteBody removes all block body data associated with a hash.
func DeleteBodyFast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(blockBodyKeyFast(number, hash)); err != nil {
		log.Crit("Failed to delete block body", "err", err)
	}
}

// ReadTd retrieves a block's total difficulty corresponding to the hash.
func ReadTdFast(db DatabaseReader, hash common.Hash, number uint64) *big.Int {
	data, _ := db.Get(headerTDKeyFast(number, hash))
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
func WriteTdFast(db DatabaseWriter, hash common.Hash, number uint64, td *big.Int) {
	data, err := rlp.EncodeToBytes(td)
	if err != nil {
		log.Crit("Failed to RLP encode block total difficulty", "err", err)
	}
	if err := db.Put(headerTDKeyFast(number, hash), data); err != nil {
		log.Crit("Failed to store block total difficulty", "err", err)
	}
}

// DeleteTd removes all block total difficulty data associated with a hash.
func DeleteTdFast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(headerTDKeyFast(number, hash)); err != nil {
		log.Crit("Failed to delete block total difficulty", "err", err)
	}
}

// ReadReceipts retrieves all the transaction receipts belonging to a block.
func ReadReceiptsFast(db DatabaseReader, hash common.Hash, number uint64) types.Receipts {
	// Retrieve the flattened receipt slice
	data, _ := db.Get(blockReceiptsKeyFast(number, hash))
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
func WriteReceiptsFast(db DatabaseWriter, hash common.Hash, number uint64, receipts types.Receipts) {
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
	if err := db.Put(blockReceiptsKeyFast(number, hash), bytes); err != nil {
		log.Crit("Failed to store block receipts", "err", err)
	}
}

// DeleteReceipts removes all receipt data associated with a block hash.
func DeleteReceiptsFast(db DatabaseDeleter, hash common.Hash, number uint64) {
	if err := db.Delete(blockReceiptsKeyFast(number, hash)); err != nil {
		log.Crit("Failed to delete block receipts", "err", err)
	}
}

// ReadBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body. If either the header or body could not
// be retrieved nil is returned.
//
// Note, due to concurrent download of header and block body the header and thus
// canonical hash can be stored in the database but the body data not (yet).
func ReadBlockFast(db DatabaseReader, hash common.Hash, number uint64) *types.FastBlock {
	header := ReadHeaderFast(db, hash, number)
	if header == nil {
		return nil
	}
	body := ReadBodyFast(db, hash, number)
	if body == nil {
		return nil
	}
	signInfo :=  make([]*types.SignInfo, 0)
	return types.NewFastBlockWithHeader(header).WithBody(body.Transactions, signInfo)
}

// WriteBlock serializes a block into the database, header and body separately.
func WriteBlockFast(db DatabaseWriter, block *types.FastBlock) {
	WriteBodyFast(db, block.Hash(), block.NumberU64(), block.Body())
	WriteHeaderFast(db, block.Header())
}

// DeleteBlock removes all block data associated with a hash.
func DeleteBlockFast(db DatabaseDeleter, hash common.Hash, number uint64) {
	DeleteReceiptsFast(db, hash, number)
	DeleteHeaderFast(db, hash, number)
	DeleteBodyFast(db, hash, number)
	DeleteTdFast(db, hash, number)
}

// FindCommonAncestor returns the last common ancestor of two block headers
func FindCommonAncestorFast(db DatabaseReader, a, b *types.FastHeader) *types.FastHeader {
	for bn := b.Number.Uint64(); a.Number.Uint64() > bn; {
		a = ReadHeaderFast(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
	}
	for an := a.Number.Uint64(); an < b.Number.Uint64(); {
		b = ReadHeaderFast(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	for a.Hash() != b.Hash() {
		a = ReadHeaderFast(db, a.ParentHash, a.Number.Uint64()-1)
		if a == nil {
			return nil
		}
		b = ReadHeaderFast(db, b.ParentHash, b.Number.Uint64()-1)
		if b == nil {
			return nil
		}
	}
	return a
}
