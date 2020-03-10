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
	"encoding/json"
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
	"io/ioutil"
	"net"
	"os"
	"sync"
	"time"
)

var (
	ErrNotRegisterAdmin     = errors.New("please call RegisterAdmin")
	ErrAlreadyRegisterAdmin = errors.New("already RegisterAdmin")
	ErrUpdateIDError        = errors.New("id too big")
	ErrAdminAlready         = errors.New("admin already exist")
	ErrAccountNotExist      = errors.New("account not exist")
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
	indexMutex  *sync.Mutex //block mutex
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
		indexMutex:  new(sync.Mutex),
	}
	signer.init()
	return signer, nil
}

func (api *SignerAPI) init() {
	api.index = rawdb.ReadIndexKey(api.db)
	admins := rawdb.ReadAdminPassword(api.db, api.seedHash)
	log.Info("init", "api.seedHash", api.seedHash.String(), "count", len(admins))
	if len(admins) > 0 {
		for _, hash := range admins {
			wallet := rawdb.ReadAdminWallet(api.db, hash)
			log.Info("init", "hash", hash.String(), "wallet", wallet)
			if wallet == nil {
				continue
			}
			location := getKeyStoreDir(api.rootLoc, hash)

			realWallet := types.NewAdminWallet(wallet.Info, startTrueKeyKeyStore(location, api.lightKDF), hash)
			for k, v := range wallet.Accounts {
				realWallet.Accounts[k] = v
			}
			api.adminWallet[hash] = realWallet
		}
	}
}

func (api *SignerAPI) registerAdmin(passphrase string, metadata Metadata) error {

	hash := crypto.Keccak256Hash([]byte(passphrase))
	if _, exists := api.adminWallet[hash]; exists {
		return ErrAlreadyRegisterAdmin
	}

	location := getKeyStoreDir(api.rootLoc, hash)
	err := os.Mkdir(location, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}

	api.adminWallet[hash] = types.NewAdminWallet(metadata.String(), startTrueKeyKeyStore(location, api.lightKDF), hash)
	var admins []common.Hash
	if rawdb.HasAdminPassword(api.db, api.seedHash) {
		admins = append(admins, rawdb.ReadAdminPassword(api.db, api.seedHash)...)
	}
	admins = append(admins, hash)
	rawdb.WriteAdminPassword(api.db, api.seedHash, admins)
	rawdb.WriteAdminWallet(api.db, hash, api.adminWallet[hash])
	log.Info("WriteAdminPassword", "api.seedHash", api.seedHash.String(), "count", len(admins), "hash", hash.String())
	return nil
}

func (api *SignerAPI) deriveAccounts(passphrase string, count uint64, metadata Metadata) ([]accounts.Account, error) {
	api.indexMutex.Lock()
	defer api.indexMutex.Unlock()
	hash := crypto.Keccak256Hash([]byte(passphrase))
	v, exists := api.adminWallet[hash]
	if !exists {
		return nil, ErrNotRegisterAdmin
	}
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
			Address:   account.Address,
			Lock:      false,
			Timestamp: time.Now(),
		}
		ats = append(ats, account)
		privateKey, err := api.wallet.PrivateKey(account)
		if err != nil {
			log.Info("Derive accounts", "err", err)
			return nil, err
		}
		v.Keystore().ImportECDSA(privateKey, passphrase)
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
	if id > api.index {
		return ErrUpdateIDError
	}

	account, err := api.wallet.Derive(getDerivationPath(id), false)
	if err != nil {
		log.Info("Update account", "err", err)
		return err
	}
	realAccount, ok := v.Accounts[account.Address]

	if !ok {
		return ErrAccountNotExist
	}

	realAccount.Lock, realAccount.Note = content.Lock, content.Note
	for _, ip := range content.IPs {
		if net.ParseIP(ip.String()) != nil {
			find := false
			for _, exist := range realAccount.IPs {
				if ip.Equal(exist) {
					find = true
				}
			}
			if find {
				continue
			}
			realAccount.IPs = append(realAccount.IPs, ip)
		}
	}
	return nil
}

func (api *SignerAPI) changeAdmin(passphrase string, newPassphrase string, metadata Metadata) error {
	api.indexMutex.Lock()
	defer api.indexMutex.Unlock()
	hash := crypto.Keccak256Hash([]byte(passphrase))
	v, exists := api.adminWallet[hash]
	if !exists {
		return ErrNotRegisterAdmin
	}
	newHash := crypto.Keccak256Hash([]byte(newPassphrase))

	if _, exists := api.adminWallet[newHash]; exists {
		return ErrAdminAlready
	}

	location := getKeyStoreDir(api.rootLoc, hash)
	newLocation := getKeyStoreDir(api.rootLoc, newHash)

	err := os.Rename(location, newLocation)
	if err != nil {
		return err
	}

	realWallet := types.NewAdminWallet(v.Info, startTrueKeyKeyStore(newLocation, api.lightKDF), newHash)
	for k, v := range v.Accounts {
		realWallet.Accounts[k] = v
	}

	api.adminWallet[newHash] = realWallet
	delete(api.adminWallet, hash)

	rawdb.WriteAdminWallet(api.db, newHash, realWallet)
	rawdb.DeleteAdminWallet(api.db, hash)

	var hashs []common.Hash
	for _, v := range rawdb.ReadAdminPassword(api.db, api.seedHash) {
		if v == hash {
			continue
		}
		hashs = append(hashs, v)
	}
	rawdb.WriteAdminPassword(api.db, api.seedHash, append(hashs, newHash))
	return nil
}

// -------------------------------------------------------------------------------
// List returns the set of wallet this signer manages. Each wallet can contain
// multiple accounts.
func (api *SignerAPI) List(ctx context.Context) ([]common.Address, error) {
	ip := net.ParseIP(MetadataFromContext(ctx).Remote)

	addresses := make([]common.Address, 0)
	for _, wallet := range api.adminWallet {
		for _, account := range wallet.Accounts {
			if !account.Lock {
				for _, aip := range account.IPs {
					if ip.Equal(aip) {
						addresses = append(addresses, account.Address)
						break
					}
				}
			}
		}
	}

	return addresses, nil
}

func (api *SignerAPI) SignHash(ctx context.Context, addr common.Address, hash hexutil.Bytes) (hexutil.Bytes, error) {
	ip := net.ParseIP(MetadataFromContext(ctx).Remote)
	var acc accounts.Account
out:
	for _, wallet := range api.adminWallet {
		for _, account := range wallet.Accounts {
			if !account.Lock && account.Address == addr {
				for _, aip := range account.IPs {
					if ip.Equal(aip) {
						acc.Address = account.Address
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

const jsonIndent = "    "

// Config is the config.json file format. It holds a set of node records
// as a JSON object.
type Config struct {
	Config []*AdminConfig `json:"admins"`
}

type AdminConfig struct {
	Admin common.Hash   `json:"admin"`
	Keys  []common.Hash `json:"keys"`
}

func loadNodesJSON(file string) Config {
	var config Config
	if isExist(file) {
		if err := common.LoadJSON(file, &config); err != nil {
			log.Info("loadNodesJSON", "error", err)
		}
	}
	return config
}

func writeNodesJSON(file string, config Config) {
	for _, v := range loadNodesJSON(file).Config {
		for _, n := range config.Config {
			if v.Admin == n.Admin {
				n.Keys = append(n.Keys, v.Keys...)
			}
		}
	}

	nodesJSON, err := json.MarshalIndent(config, "", jsonIndent)
	if err != nil {
		log.Info("writeNodesJSON MarshalIndent", "error", err)
	}
	if file == "-" {
		os.Stdout.Write(nodesJSON)
		return
	}
	if err := ioutil.WriteFile(file, nodesJSON, 0644); err != nil {
		log.Info("writeNodesJSON writeFile", "error", err)
	}
}

func isExist(f string) bool {
	_, err := os.Stat(f)
	return err == nil || os.IsExist(err)
}
