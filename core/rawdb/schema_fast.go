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

// Package rawdb contains a collection of low level database accessors.
package rawdb

import (
	"encoding/binary"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/metrics"
)

// The fields below define the low level database schema prefixing.
var (
	// databaseVerisionKey tracks the current database version.
	//databaseVerisionKey = []byte("DatabaseVersion")

	// headHeaderKey tracks the latest know header's hash.
	headHeaderKeyFast = []byte("LastHeaderFast")

	// headBlockKey tracks the latest know full block's hash.
	headBlockKeyFast = []byte("LastBlockFast")

	// headFastBlockKey tracks the latest known incomplete block's hash duirng fast sync.
	headFastBlockKeyFast = []byte("LastFastFast")

	// fastTrieProgressKey tracks the number of trie entries imported during fast sync.
	fastTrieProgressKeyFast = []byte("TrieSyncFast")

	// Data item prefixes (use single byte to avoid mixing data types, avoid `i`, used for indexes).
	headerPrefixFast       = []byte("h_f") // headerPrefix + num (uint64 big endian) + hash -> header
	headerTDSuffixFast     = []byte("t_f") // headerPrefix + num (uint64 big endian) + hash + headerTDSuffix -> td
	headerHashSuffixFast   = []byte("n_f") // headerPrefix + num (uint64 big endian) + headerHashSuffix -> hash
	headerNumberPrefixFast = []byte("H_f") // headerNumberPrefix + hash -> num (uint64 big endian)

	blockBodyPrefixFast     = []byte("b_f") // blockBodyPrefix + num (uint64 big endian) + hash -> block body
	blockReceiptsPrefixFast = []byte("r_f") // blockReceiptsPrefix + num (uint64 big endian) + hash -> block receipts

	txLookupPrefixFast  = []byte("l_f") // txLookupPrefix + hash -> transaction/receipt lookup metadata
	bloomBitsPrefixFast = []byte("B_f") // bloomBitsPrefix + bit (uint16 big endian) + section (uint64 big endian) + hash -> bloom bits

	preimagePrefixFast = []byte("secure-key-_f")      // preimagePrefix + hash -> preimage
	configPrefixFast   = []byte("ethereum-config-_f") // config prefix for the db

	// Chain index prefixes (use `i` + single byte to avoid mixing data types).
	BloomBitsIndexPrefixFast = []byte("iB_f") // BloomBitsIndexPrefix is the data table of a chain indexer to track its progress

	preimageCounterFast    = metrics.NewRegisteredCounter("db/preimage/total", nil)
	preimageHitCounterFast = metrics.NewRegisteredCounter("db/preimage/hits", nil)
)

/*type TxLookupEntry struct {
	BlockHash  common.Hash
	BlockIndex uint64
	Index      uint64
}

func encodeBlockNumber(number uint64) []byte {
	enc := make([]byte, 8)
	binary.BigEndian.PutUint64(enc, number)
	return enc
}*/

// headerKey = headerPrefix + num (uint64 big endian) + hash
func headerKeyFast(number uint64, hash common.Hash) []byte {
	return append(append(headerPrefixFast, encodeBlockNumber(number)...), hash.Bytes()...)
}

// headerTDKey = headerPrefix + num (uint64 big endian) + hash + headerTDSuffix
func headerTDKeyFast(number uint64, hash common.Hash) []byte {
	return append(headerKeyFast(number, hash), headerTDSuffixFast...)
}

// headerHashKey = headerPrefix + num (uint64 big endian) + headerHashSuffix
func headerHashKeyFast(number uint64) []byte {
	return append(append(headerPrefixFast, encodeBlockNumber(number)...), headerHashSuffixFast...)
}

// headerNumberKey = headerNumberPrefix + hash
func headerNumberKeyFast(hash common.Hash) []byte {
	return append(headerNumberPrefixFast, hash.Bytes()...)
}

// blockBodyKey = blockBodyPrefix + num (uint64 big endian) + hash
func blockBodyKeyFast(number uint64, hash common.Hash) []byte {
	return append(append(blockBodyPrefixFast, encodeBlockNumber(number)...), hash.Bytes()...)
}

// blockReceiptsKey = blockReceiptsPrefix + num (uint64 big endian) + hash
func blockReceiptsKeyFast(number uint64, hash common.Hash) []byte {
	return append(append(blockReceiptsPrefixFast, encodeBlockNumber(number)...), hash.Bytes()...)
}

// txLookupKey = txLookupPrefix + hash
func txLookupKeyFast(hash common.Hash) []byte {
	return append(txLookupPrefixFast, hash.Bytes()...)
}

// bloomBitsKey = bloomBitsPrefix + bit (uint16 big endian) + section (uint64 big endian) + hash
func bloomBitsKeyFast(bit uint, section uint64, hash common.Hash) []byte {
	key := append(append(bloomBitsPrefixFast, make([]byte, 10)...), hash.Bytes()...)

	binary.BigEndian.PutUint16(key[1:], uint16(bit))
	binary.BigEndian.PutUint64(key[3:], section)

	return key
}

// preimageKey = preimagePrefix + hash
func preimageKeyFast(hash common.Hash) []byte {
	return append(preimagePrefixFast, hash.Bytes()...)
}

// configKey = configPrefix + hash
func configKeyFast(hash common.Hash) []byte {
	return append(configPrefixFast, hash.Bytes()...)
}