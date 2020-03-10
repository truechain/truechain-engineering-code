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
	"github.com/truechain/truechain-engineering-code/cmd/truekey/types"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/rlp"
)

// WriteAccountLookupEntries stores a positional metadata for every account from
// a wallet, enabling hash based account  lookups.
func WriteAccountLookupEntries(db DatabaseWriter, wallet *types.AdminWallet) {
	for _, accout := range wallet.AccountArray() {
		entry := AccountLookup{
			WalletHash: wallet.Hash,
			Index:      accout.ID,
		}
		data, err := rlp.EncodeToBytes(entry)
		if err != nil {
			log.Crit("Failed to encode account lookup entry", "err", err)
		}
		if err := db.Put(accountLookupKey(accout.Address.Hash()), data); err != nil {
			log.Crit("Failed to store account lookup entry", "err", err)
		}
	}
}

// DeleteAccountLookupEntry removes all account data associated with a hash.
func DeleteAccountLookupEntry(db DatabaseDeleter, hash common.Hash) {
	db.Delete(accountLookupKey(hash))
}

// HasAccountLookupEntry verifies the existence of a accountLook entry corresponding to the hash.
func HasAccountLookupEntry(db DatabaseReader, hash common.Hash) bool {
	if has, err := db.Has(accountLookupKey(hash)); !has || err != nil {
		return false
	}
	return true
}
