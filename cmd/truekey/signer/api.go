// Copyright 2018 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package signer

import (
	"context"
	"errors"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/cmd/truekey/hdwallet"
	"github.com/truechain/truechain-engineering-code/cmd/truekey/rawdb"
	"github.com/truechain/truechain-engineering-code/cmd/truekey/types"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/log"
	"net"
	"os"
	"sync"
	"time"
)

var (
	ErrNotRegisterAdmin = errors.New("please call RegisterAdmin")
)

const (
	DefaultBaseDerivationPath = "m/44'/60'/0'/0/"
)

// SignerAPI defines the actual implementation of ExternalAPI
type SignerAPI struct {
	db          etruedb.Database
	wallet      *hdwallet.Wallet
	adminWallet map[common.Hash]*types.AdminWallet
	rootLoc     string
	index       uint64
	lightKDF    bool
	seedHash    common.Hash
	indexMu     sync.RWMutex
}

// NewSignerAPI creates a new API that can be used for Account management.
// ksLocation specifies the directory where to store the password protected private
// key that is generated when a new Account is created.
func NewSignerAPI(db etruedb.Database, seed []byte, rootLoc string, lightKdf bool) (*SignerAPI, error) {
	wallet, err := hdwallet.NewFromSeed(seed)
	if err != nil {
		return nil, err
	}
	signer := &SignerAPI{
		db:          db,
		wallet:      wallet,
		adminWallet: make(map[common.Hash]*types.AdminWallet),
		index:       uint64(0),
		rootLoc:     rootLoc,
		lightKDF:    lightKdf,
		seedHash:    crypto.Keccak256Hash(seed),
	}
	signer.init()
	return signer, nil
}

func (api *SignerAPI) init() {
	api.index = rawdb.ReadIndexKey(api.db)
	admins := rawdb.ReadAdminPassword(api.db, api.seedHash)
	if len(admins) > 0 {
		for _, hash := range admins {
			wallet := rawdb.ReadAdminWallet(api.db, hash)
			if wallet != nil {
				continue
			}
			api.adminWallet[hash] = wallet
			location := getKeyStoreDir(api.rootLoc, hash)
			api.adminWallet[hash].SetAm(startTrueKeyAccountManager(location, api.lightKDF))
		}
	}
}

func (api *SignerAPI) registerAdmin(passphrase string, metadata Metadata) error {

	hash := crypto.Keccak256Hash([]byte(passphrase))
	location := getKeyStoreDir(api.rootLoc, hash)

	err := os.Mkdir(location, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}

	api.adminWallet[hash] = types.NewAdminWallet(metadata.String(), startTrueKeyAccountManager(location, api.lightKDF), hash)
	var admins []common.Hash
	if rawdb.HasAdminPassword(api.db, api.seedHash) {
		admins = append(admins, rawdb.ReadAdminPassword(api.db, api.seedHash)...)
	}
	admins = append(admins, hash)
	rawdb.WriteAdminPassword(api.db, api.seedHash, admins)
	return nil
}

func (api *SignerAPI) deriveAccounts(passphrase string, count uint64, metadata Metadata) ([]accounts.Account, error) {
	hash := crypto.Keccak256Hash([]byte(passphrase))
	v, exists := api.adminWallet[hash]
	if !exists {
		return nil, ErrNotRegisterAdmin
	}
	api.indexMu.Lock()
	defer api.indexMu.RUnlock()
	var ats []accounts.Account
	for i := uint64(0); i < count; i++ {
		index := i + api.index
		account, err := api.wallet.Derive(getDerivationPath(index), true)
		if err != nil {
			api.index += i
			log.Info("Derive accounts", "err", err)
			return nil, err
		}
		v.Accounts[account.Address] = &types.ChildAccount{
			ID:        index,
			Account:   account,
			Lock:      false,
			Timestamp: time.Now(),
		}
		ats = append(ats, account)
		privateKey, err := api.wallet.PrivateKey(account)
		if err != nil {
			log.Info("Derive accounts", "err", err)
			return nil, err
		}
		fetchKeystore(v.Am).ImportECDSA(privateKey, passphrase)
	}
	api.index += count
	rawdb.WriteIndexKey(api.db, api.index)
	rawdb.WriteAdminWallet(api.db, hash, v)
	return ats, nil
}

// Get private key
func (api *SignerAPI) exportKey(id uint64, addr common.Address, metadata Metadata) (hexutil.Bytes, error) {
	return nil, nil
}

func (api *SignerAPI) childAddress(passphrase string, metadata Metadata) ([]types.ChildAccount, error) {
	hash := crypto.Keccak256Hash([]byte(passphrase))
	v, exists := api.adminWallet[hash]
	if !exists {
		return nil, ErrNotRegisterAdmin
	}
	var accs []types.ChildAccount
	for _, account := range v.Accounts {
		accs = append(accs, *account)
	}
	return accs, nil
}

func (api *SignerAPI) updateAccount(passphrase string, id uint64, content types.AccountState, metadata Metadata) error {
	hash := crypto.Keccak256Hash([]byte(passphrase))
	v, exists := api.adminWallet[hash]
	if !exists {
		return ErrNotRegisterAdmin
	}
	account, err := api.wallet.Derive(getDerivationPath(id), false)
	if err != nil {
		log.Info("Update account", "err", err)
		return err
	}
	realAccount := v.Accounts[account.Address]
	realAccount.Lock, realAccount.Note = content.Lock, content.Note
	realAccount.IPs = append(realAccount.IPs, content.IPs...)
	return nil
}

func (api *SignerAPI) changeAdmin(passphrase string, newPassphrase string, metadata Metadata) error {
	hash := crypto.Keccak256Hash([]byte(passphrase))
	v, exists := api.adminWallet[hash]
	if !exists {
		return ErrNotRegisterAdmin
	}
	newHash := crypto.Keccak256Hash([]byte(newPassphrase))
	rawdb.WriteAdminWallet(api.db, newHash, v)
	api.adminWallet[newHash] = v

	delete(api.adminWallet, hash)
	rawdb.DeleteAdminWallet(api.db, hash)
	return nil
}

// -------------------------------------------------------------------------------
// List returns the set of wallet this signer manages. Each wallet can contain
// multiple accounts.
func (api *SignerAPI) List(ctx context.Context) ([]common.Address, error) {
	ip := net.ParseIP(MetadataFromContext(ctx).Remote)

	var accs []accounts.Account
	for _, wallet := range api.adminWallet {
		for _, account := range wallet.Accounts {
			if !account.Lock {
				for _, aip := range account.IPs {
					if ip.Equal(aip) {
						accs = append(accs, account.Account)
						break
					}
				}
			}
		}
	}

	addresses := make([]common.Address, 0)
	for _, acc := range accs {
		addresses = append(addresses, acc.Address)
	}

	return addresses, nil
}

func (api *SignerAPI) SignHash(ctx context.Context, addr common.Address, hash hexutil.Bytes) (hexutil.Bytes, error) {
	ip := net.ParseIP(MetadataFromContext(ctx).Remote)
	var acc accounts.Account
out:
	for _, wallet := range api.adminWallet {
		for _, account := range wallet.Accounts {
			if !account.Lock && account.Account.Address == addr {
				for _, aip := range account.IPs {
					if ip.Equal(aip) {
						acc = account.Account
						break out
					}
				}
			}
		}
	}
	return api.wallet.SignHash(acc, hash)
}

// Returns the external api version. This method does not require user acceptance. Available methods are
// available via enumeration anyway, and this info does not contain user-specific data
func (api *SignerAPI) Version(ctx context.Context) (string, error) {
	return types.ExternalAPIVersion, nil
}
