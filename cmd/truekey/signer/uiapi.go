// Copyright 2019 The go-ethereum Authors
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
//

package signer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"github.com/truechain/truechain-engineering-code/cmd/truekey/hdwallet"
	"github.com/truechain/truechain-engineering-code/cmd/truekey/types"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"path/filepath"
)

// SignerUIAPI implements methods Clef provides for a UI to query, in the bidirectional communication
// channel.
// This API is considered secure, since a request can only
// ever arrive from the UI -- and the UI is capable of approving any action, thus we can consider these
// requests pre-approved.
// NB: It's very important that these methods are not ever exposed on the external service
// registry.
type UIServerAPI struct {
	extApi      *SignerAPI
	adminWallet map[common.Hash]*types.AdminWallet
}

// NewUIServerAPI creates a new UIServerAPI
func NewUIServerAPI(extapi *SignerAPI) *UIServerAPI {
	return &UIServerAPI{extapi, extapi.adminWallet}
}

func (s *UIServerAPI) RegisterAdmin(ctx context.Context, passphrase string) error {
	if err := ValidatePasswordFormat(passphrase); err != nil {
		fmt.Printf("invalid password: %v\n", err)
	}
	return s.extApi.registerAdmin(passphrase, MetadataFromContext(ctx))
}

// DeriveAccounts requests a HD wallet to derive a new account, optionally pinning
// it for later reuse.
// Example call
// {"jsonrpc":"2.0","method":"clef_deriveAccount","params":["ledger://","m/44'/60'/0'", false], "id":6}
func (s *UIServerAPI) DeriveAccounts(ctx context.Context, count uint64, passphrase string) ([]accounts.Account, error) {
	if err := ValidatePasswordFormat(passphrase); err != nil {
		fmt.Printf("invalid password: %v\n", err)
	}
	return s.extApi.deriveAccounts(passphrase, count, MetadataFromContext(ctx))
}

func (s *UIServerAPI) ExportKey(ctx context.Context, id uint64, addr common.Address) (hexutil.Bytes, error) {
	return s.extApi.exportKey(id, addr, MetadataFromContext(ctx))
}

// List available accounts. As opposed to the external API definition, this method delivers
// the full Account object and not only Address.
// Example call
// {"jsonrpc":"2.0","method":"clef_listAccounts","params":[], "id":4}
func (s *UIServerAPI) ChildAddress(ctx context.Context, passphrase string) ([]types.ChildAccount, error) {
	if err := ValidatePasswordFormat(passphrase); err != nil {
		fmt.Printf("invalid password: %v\n", err)
	}
	return s.extApi.childAddress(passphrase, MetadataFromContext(ctx))
}

func (s *UIServerAPI) UpdateAccount(ctx context.Context, passphrase string, id uint64, content types.AccountState) error {
	if err := ValidatePasswordFormat(passphrase); err != nil {
		fmt.Printf("invalid password: %v\n", err)
	}
	return s.extApi.updateAccount(passphrase, id, content, MetadataFromContext(ctx))
}

func (s *UIServerAPI) ChangeAdmin(ctx context.Context, passphrase string, newPassphrase string) error {
	if err := ValidatePasswordFormat(passphrase); err != nil {
		fmt.Printf("invalid password: %v\n", err)
	}
	if err := ValidatePasswordFormat(newPassphrase); err != nil {
		fmt.Printf("invalid new password: %v\n", err)
	}
	return s.extApi.changeAdmin(passphrase, newPassphrase, MetadataFromContext(ctx))
}

// fetchKeystore retrives the encrypted keystore from the account manager.
func fetchKeystore(am *accounts.Manager) *keystore.KeyStore {
	return am.Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
}

func getKeyStoreDir(root string, hash common.Hash) string {
	return filepath.Join(root, common.Bytes2Hex(hash.Bytes()))
}

func getDerivationPath(index uint64) accounts.DerivationPath {
	return hdwallet.MustParseDerivationPath(DefaultBaseDerivationPath + fmt.Sprintf("%x", index))
}

func startTrueKeyAccountManager(ksLocation string, lightKDF bool) *accounts.Manager {
	var (
		backends []accounts.Backend
		n, p     = keystore.StandardScryptN, keystore.StandardScryptP
	)
	if lightKDF {
		n, p = keystore.LightScryptN, keystore.LightScryptP
	}
	// support password based accounts
	if len(ksLocation) > 0 {
		backends = append(backends, keystore.NewKeyStore(ksLocation, n, p))
	}

	// TrueKey doesn't allow insecure http account unlock.
	return accounts.NewManager(&accounts.Config{InsecureUnlockAllowed: false}, backends...)
}

// types for the requests/response types between signer and UI
type (
	SignDataResponse struct {
		Approved bool `json:"approved"`
	}
	NewAccountRequest struct {
		Meta Metadata `json:"meta"`
	}
	NewAccountResponse struct {
		Approved bool `json:"approved"`
	}
	ListRequest struct {
		Accounts []accounts.Account `json:"accounts"`
		Meta     Metadata           `json:"meta"`
	}
	ListResponse struct {
		Accounts []accounts.Account `json:"accounts"`
	}
	Message struct {
		Text string `json:"text"`
	}
	StartupInfo struct {
		Info map[string]interface{} `json:"info"`
	}
	UserInputRequest struct {
		Title      string `json:"title"`
		Prompt     string `json:"prompt"`
		IsPassword bool   `json:"isPassword"`
	}
	UserInputResponse struct {
		Text string `json:"text"`
	}
)

// Metadata about a request
type Metadata struct {
	Remote    string `json:"remote"`
	Local     string `json:"local"`
	Scheme    string `json:"scheme"`
	UserAgent string `json:"User-Agent"`
	Origin    string `json:"Origin"`
}

// MetadataFromContext extracts Metadata from a given context.Context
func MetadataFromContext(ctx context.Context) Metadata {
	m := Metadata{"NA", "NA", "NA", "", ""} // batman

	if v := ctx.Value("remote"); v != nil {
		m.Remote = v.(string)
	}
	if v := ctx.Value("scheme"); v != nil {
		m.Scheme = v.(string)
	}
	if v := ctx.Value("local"); v != nil {
		m.Local = v.(string)
	}
	if v := ctx.Value("Origin"); v != nil {
		m.Origin = v.(string)
	}
	if v := ctx.Value("User-Agent"); v != nil {
		m.UserAgent = v.(string)
	}
	return m
}

// String implements Stringer interface
func (m Metadata) String() string {
	s, err := json.Marshal(m)
	if err == nil {
		return string(s)
	}
	return err.Error()
}
