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
	"github.com/truechain/truechain-engineering-code/cmd/truekey/types"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/rlp"
	"math/big"
)

// ReadIndexKey retrieves the number of account path index
func ReadIndexKey(db DatabaseReader) uint64 {
	data, _ := db.Get(indexKey)
	if len(data) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(data).Uint64()
}

// WriteIndexKey stores the number of account path index
func WriteIndexKey(db DatabaseWriter, count uint64) {
	if err := db.Put(indexKey, new(big.Int).SetUint64(count).Bytes()); err != nil {
		log.Crit("Failed to store fast sync trie progress", "err", err)
	}
}

// ReadCanonicalHash retrieves the hash assigned to a canonical block number.
func ReadAdminPassword(db DatabaseReader, key common.Hash) []common.Hash {
	data, _ := db.Get(adminKey(key))
	if len(data) == 0 {
		return []common.Hash{}
	}
	var admins []common.Hash
	if err := rlp.Decode(bytes.NewReader(data), &admins); err != nil {
		log.Error("Invalid admin password RLP", "hash", key, "err", err)
		return nil
	}

	return admins
}

// WriteAdminPassword stores the hash assigned to a admin password.
func WriteAdminPassword(db DatabaseWriter, key common.Hash, admins []common.Hash) {
	data, err := rlp.EncodeToBytes(admins)
	if err != nil {
		log.Crit("Failed to RLP encode admins", "err", err)
	}
	if err := db.Put(adminKey(key), data); err != nil {
		log.Crit("Failed to store admin hash", "err", err)
	}
}

// HasAdminPassword verifies the existence of a admin password corresponding to the hash.
func HasAdminPassword(db DatabaseReader, hash common.Hash) bool {
	if has, err := db.Has(adminKey(hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadAdminWalletRLP retrieves the admin wallet in RLP encoding.
func readAdminWalletRLP(db DatabaseReader, hash common.Hash) rlp.RawValue {
	data, _ := db.Get(adminWalletKey(hash))
	return data
}

// WriteAdminWalletRLP stores an RLP encoded admin wallet into the database.
func writeAdminWalletRLP(db DatabaseWriter, hash common.Hash, rlp rlp.RawValue) {
	if err := db.Put(adminWalletKey(hash), rlp); err != nil {
		log.Crit("Failed to store admin wallet", "err", err)
	}
}

// HasAdminWallet verifies the existence of a admin wallet corresponding to the hash.
func HasAdminWallet(db DatabaseReader, hash common.Hash) bool {
	if has, err := db.Has(adminWalletKey(hash)); !has || err != nil {
		return false
	}
	return true
}

// ReadAdminWallet retrieves the admin wallet corresponding to the hash.
func ReadAdminWallet(db DatabaseReader, hash common.Hash) *types.AdminWallet {
	data := readAdminWalletRLP(db, hash)
	if len(data) == 0 {
		return nil
	}
	body := new(types.AdminWallet)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		log.Error("Invalid block body RLP", "hash", hash, "err", err)
		return nil
	}
	return body
}

// WriteAdminWallet store a admin wallet into the database.
func WriteAdminWallet(db DatabaseWriter, hash common.Hash, wallet *types.AdminWallet) {
	data, err := rlp.EncodeToBytes(wallet)
	if err != nil {
		log.Crit("Failed to RLP encode admin wallet", "err", err)
	}
	writeAdminWalletRLP(db, hash, data)
}

// DeleteAdminWallet removes admin wallet data associated with a hash.
func DeleteAdminWallet(db DatabaseDeleter, hash common.Hash) {
	if err := db.Delete(adminWalletKey(hash)); err != nil {
		log.Crit("Failed to delete admin wallet", "err", err)
	}
}
