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
	headHeaderKey_Fast = []byte("LastHeader_Fast")

	// headBlockKey tracks the latest know full block's hash.
	headBlockKey_Fast = []byte("LastBlock_Fast")

	// headFastBlockKey tracks the latest known incomplete block's hash duirng fast sync.
	headFastBlockKey_Fast = []byte("LastFast_Fast")

	// fastTrieProgressKey tracks the number of trie entries imported during fast sync.
	fastTrieProgressKey_Fast = []byte("TrieSync_Fast")

	// Data item prefixes (use single byte to avoid mixing data types, avoid `i`, used for indexes).
	headerPrefix_Fast       = []byte("h_f") // headerPrefix + num (uint64 big endian) + hash -> header
	headerTDSuffix_Fast     = []byte("t_f") // headerPrefix + num (uint64 big endian) + hash + headerTDSuffix -> td
	headerHashSuffix_Fast   = []byte("n_f") // headerPrefix + num (uint64 big endian) + headerHashSuffix -> hash
	headerNumberPrefix_Fast = []byte("H_f") // headerNumberPrefix + hash -> num (uint64 big endian)

	blockBodyPrefix_Fast     = []byte("b_f") // blockBodyPrefix + num (uint64 big endian) + hash -> block body
	blockReceiptsPrefix_Fast = []byte("r_f") // blockReceiptsPrefix + num (uint64 big endian) + hash -> block receipts

	txLookupPrefix_Fast  = []byte("l_f") // txLookupPrefix + hash -> transaction/receipt lookup metadata
	bloomBitsPrefix_Fast = []byte("B_f") // bloomBitsPrefix + bit (uint16 big endian) + section (uint64 big endian) + hash -> bloom bits

	preimagePrefix_Fast = []byte("secure-key-_f")      // preimagePrefix + hash -> preimage
	configPrefix_Fast   = []byte("ethereum-config-_f") // config prefix for the db

	// Chain index prefixes (use `i` + single byte to avoid mixing data types).
	BloomBitsIndexPrefix_Fast = []byte("iB_f") // BloomBitsIndexPrefix is the data table of a chain indexer to track its progress

	preimageCounter_Fast    = metrics.NewRegisteredCounter("db/preimage/total", nil)
	preimageHitCounter_Fast = metrics.NewRegisteredCounter("db/preimage/hits", nil)
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
func headerKey_Fast(number uint64, hash common.Hash) []byte {
	return append(append(headerPrefix_Fast, encodeBlockNumber(number)...), hash.Bytes()...)
}

// headerTDKey = headerPrefix + num (uint64 big endian) + hash + headerTDSuffix
func headerTDKey_Fast(number uint64, hash common.Hash) []byte {
	return append(headerKey_Fast(number, hash), headerTDSuffix_Fast...)
}

// headerHashKey = headerPrefix + num (uint64 big endian) + headerHashSuffix
func headerHashKey_Fast(number uint64) []byte {
	return append(append(headerPrefix_Fast, encodeBlockNumber(number)...), headerHashSuffix_Fast...)
}

// headerNumberKey = headerNumberPrefix + hash
func headerNumberKey_Fast(hash common.Hash) []byte {
	return append(headerNumberPrefix_Fast, hash.Bytes()...)
}

// blockBodyKey = blockBodyPrefix + num (uint64 big endian) + hash
func blockBodyKey_Fast(number uint64, hash common.Hash) []byte {
	return append(append(blockBodyPrefix_Fast, encodeBlockNumber(number)...), hash.Bytes()...)
}

// blockReceiptsKey = blockReceiptsPrefix + num (uint64 big endian) + hash
func blockReceiptsKey_Fast(number uint64, hash common.Hash) []byte {
	return append(append(blockReceiptsPrefix_Fast, encodeBlockNumber(number)...), hash.Bytes()...)
}

// txLookupKey = txLookupPrefix + hash
func txLookupKey_Fast(hash common.Hash) []byte {
	return append(txLookupPrefix_Fast, hash.Bytes()...)
}

// bloomBitsKey = bloomBitsPrefix + bit (uint16 big endian) + section (uint64 big endian) + hash
func bloomBitsKey_Fast(bit uint, section uint64, hash common.Hash) []byte {
	key := append(append(bloomBitsPrefix_Fast, make([]byte, 10)...), hash.Bytes()...)

	binary.BigEndian.PutUint16(key[1:], uint16(bit))
	binary.BigEndian.PutUint64(key[3:], section)

	return key
}

// preimageKey = preimagePrefix + hash
func preimageKey_Fast(hash common.Hash) []byte {
	return append(preimagePrefix_Fast, hash.Bytes()...)
}

// configKey = configPrefix + hash
func configKey_Fast(hash common.Hash) []byte {
	return append(configPrefix_Fast, hash.Bytes()...)
}