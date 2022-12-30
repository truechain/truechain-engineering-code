// Copyright 2015 The go-ethereum Authors
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

package trueapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/metrics"
	"math/big"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/common/math"
	ethash "github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/rpc"
)

const (
	defaultGasPrice = 50 * params.Shannon
)

var (
	LocalTxMetrics = metrics.NewRegisteredMeter("etrue/prop/local_tx/in", nil)
)

// PublicTrueAPI provides an API to access True related information.
// It offers only methods that operate on public data that is freely available to anyone.
type PublicTrueAPI struct {
	b Backend
}

// NewPublicTrueAPI creates a new True protocol API.
func NewPublicTrueAPI(b Backend) *PublicTrueAPI {
	return &PublicTrueAPI{b}
}

// GasPrice returns a suggestion for a gas price.
func (s *PublicTrueAPI) GasPrice(ctx context.Context) (*hexutil.Big, error) {
	price, err := s.b.SuggestPrice(ctx)
	return (*hexutil.Big)(price), err
}

// ProtocolVersion returns the current True protocol version this node supports
func (s *PublicTrueAPI) ProtocolVersion() hexutil.Uint {
	return hexutil.Uint(s.b.ProtocolVersion())
}

// Syncing returns false in case the node is currently not syncing with the network. It can be up to date or has not
// yet received the latest block headers from its pears. In case it is synchronizing:
// - startingBlock: block number this node started to synchronise from
// - currentBlock:  block number this node is currently importing
// - highestBlock:  block number of the highest block header this node has received from peers
// - pulledStates:  number of state entries processed until now
// - knownStates:   number of known state entries that still need to be pulled
func (s *PublicTrueAPI) Syncing() (interface{}, error) {
	progress := s.b.Downloader().Progress()

	// Return not syncing if the synchronisation already completed
	if progress.CurrentSnailBlock >= progress.HighestSnailBlock {
		return false, nil
	}
	// Otherwise gather the block sync stats
	return map[string]interface{}{
		"startingFastBlock":  hexutil.Uint64(progress.StartingFastBlock),
		"currentFastBlock":   hexutil.Uint64(progress.CurrentFastBlock),
		"highestFastBlock":   hexutil.Uint64(progress.HighestFastBlock),
		"startingSnailBlock": hexutil.Uint64(progress.StartingSnailBlock),
		"currentSnailBlock":  hexutil.Uint64(progress.CurrentSnailBlock),
		"highestSnailBlock":  hexutil.Uint64(progress.HighestSnailBlock),
		"pulledStates":       hexutil.Uint64(progress.PulledStates),
		"knownStates":        hexutil.Uint64(progress.KnownStates),
	}, nil
}

// PublicTxPoolAPI offers and API for the transaction pool. It only operates on data that is non confidential.
type PublicTxPoolAPI struct {
	b Backend
}

// NewPublicTxPoolAPI creates a new tx pool service that gives information about the transaction pool.
func NewPublicTxPoolAPI(b Backend) *PublicTxPoolAPI {
	return &PublicTxPoolAPI{b}
}

// Content returns the transactions contained within the transaction pool.
func (s *PublicTxPoolAPI) Content() map[string]map[string]map[string]*RPCTransaction {
	content := map[string]map[string]map[string]*RPCTransaction{
		"pending": make(map[string]map[string]*RPCTransaction),
		"queued":  make(map[string]map[string]*RPCTransaction),
	}
	pending, queue := s.b.TxPoolContent()

	// Flatten the pending transactions
	for account, txs := range pending {
		dump := make(map[string]*RPCTransaction)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = newRPCPendingTransaction(tx)
		}
		content["pending"][account.Hex()] = dump
	}
	// Flatten the queued transactions
	for account, txs := range queue {
		dump := make(map[string]*RPCTransaction)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = newRPCPendingTransaction(tx)
		}
		content["queued"][account.Hex()] = dump
	}
	return content
}

// Status returns the number of pending and queued transaction in the pool.
func (s *PublicTxPoolAPI) Status() map[string]hexutil.Uint {
	pending, queue := s.b.Stats()
	return map[string]hexutil.Uint{
		"pending": hexutil.Uint(pending),
		"queued":  hexutil.Uint(queue),
	}
}

// Inspect retrieves the content of the transaction pool and flattens it into an
// easily inspectable list.
func (s *PublicTxPoolAPI) Inspect() map[string]map[string]map[string]string {
	content := map[string]map[string]map[string]string{
		"pending": make(map[string]map[string]string),
		"queued":  make(map[string]map[string]string),
	}
	pending, queue := s.b.TxPoolContent()

	// Define a formatter to flatten a transaction into a string
	var format = func(tx *types.Transaction) string {
		if to := tx.To(); to != nil {
			return fmt.Sprintf("%s: %v wei + %v gas × %v wei", tx.To().Hex(), tx.Value(), tx.Gas(), tx.GasPrice())
		}
		return fmt.Sprintf("contract creation: %v wei + %v gas × %v wei", tx.Value(), tx.Gas(), tx.GasPrice())
	}
	// Flatten the pending transactions
	for account, txs := range pending {
		dump := make(map[string]string)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = format(tx)
		}
		content["pending"][account.Hex()] = dump
	}
	// Flatten the queued transactions
	for account, txs := range queue {
		dump := make(map[string]string)
		for _, tx := range txs {
			dump[fmt.Sprintf("%d", tx.Nonce())] = format(tx)
		}
		content["queued"][account.Hex()] = dump
	}
	return content
}

// PublicAccountAPI provides an API to access accounts managed by this node.
// It offers only methods that can retrieve accounts.
type PublicAccountAPI struct {
	am *accounts.Manager
}

// NewPublicAccountAPI creates a new PublicAccountAPI.
func NewPublicAccountAPI(am *accounts.Manager) *PublicAccountAPI {
	return &PublicAccountAPI{am: am}
}

// Accounts returns the collection of accounts this node manages
func (s *PublicAccountAPI) Accounts() []common.Address {
	addresses := make([]common.Address, 0) // return [] instead of nil if empty
	for _, wallet := range s.am.Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address)
		}
	}
	return addresses
}

// RPCFruits represents a fruit that will serialize to the RPC representation of a fruit
type RPCFruit struct {
	Number          *hexutil.Big `json:"number"`
	FruitDifficulty *hexutil.Big `json:"fruitDifficulty"`
	FruitHash       common.Hash  `json:"fruitHash"`
	FastHash        common.Hash  `json:"fastHash"`
	FastNumber      *hexutil.Big `json:"fastNumber"`
	SignHash        common.Hash  `json:"signHash"`
	PointerHash     common.Hash  `json:"pointerHash"`
	PointerNumber   *hexutil.Big `json:"pointerNumber"`
}

// newRPCFruit returns a fruit that will serialize to the RPC
// representation, with the given location metadata set (if available).
func newRPCFruit(fruit *types.SnailBlock) *RPCFruit {

	result := &RPCFruit{
		Number:          (*hexutil.Big)(fruit.Header().Number),
		FruitDifficulty: (*hexutil.Big)(fruit.Header().FruitDifficulty),
		FruitHash:       fruit.Hash(),
		FastHash:        fruit.FastHash(),
		FastNumber:      (*hexutil.Big)(fruit.Header().FastNumber),
		SignHash:        fruit.Header().SignHash,
		PointerHash:     fruit.Header().PointerHash,
		PointerNumber:   (*hexutil.Big)(fruit.Header().PointerNumber),
	}
	return result
}

// PublicFruitPoolAPI offers and API for the snail pool. It only operates on data that is non confidential.
type PublicFruitPoolAPI struct {
	b Backend
}

// NewPublicFruitPoolAPI creates a new snail pool service that gives information about the snail pool.
func NewPublicFruitPoolAPI(b Backend) *PublicFruitPoolAPI {
	return &PublicFruitPoolAPI{b}
}

// Content returns the pendingFruits contained within the snail pool.
func (s *PublicFruitPoolAPI) Content() []*RPCFruit {
	//content := map[string]interface{}{}
	pending := s.b.SnailPoolContent()
	//content["count"] = count
	// Flatten the pending fruits
	//dump := make(map[common.Hash]*RPCFruit)
	var pendingFruits []*RPCFruit
	for _, fruit := range pending {
		pendingFruits = append(pendingFruits, newRPCFruit(fruit))
	}
	//content["pending"] = dump
	return pendingFruits
}

// Inspect returns the unVerifiedFruits contained within the snail pool.
func (s *PublicFruitPoolAPI) Inspect() []*RPCFruit {

	unVerified := s.b.SnailPoolInspect()
	var unVerifiedFruits []*RPCFruit
	for _, fruit := range unVerified {
		unVerifiedFruits = append(unVerifiedFruits, newRPCFruit(fruit))
	}
	return unVerifiedFruits
}

// Status returns the number of pending and unVerified Fruits in the pool.
func (s *PublicFruitPoolAPI) Status() map[string]hexutil.Uint {
	pending, unVerified := s.b.SnailPoolStats()
	return map[string]hexutil.Uint{
		"pending":    hexutil.Uint(pending),
		"unverified": hexutil.Uint(unVerified),
	}
}

// PrivateAccountAPI provides an API to access accounts managed by this node.
// It offers methods to create, (un)lock en list accounts. Some methods accept
// passwords and are therefore considered private by default.
type PrivateAccountAPI struct {
	am        *accounts.Manager
	nonceLock *AddrLocker
	b         Backend
}

// NewPrivateAccountAPI create a new PrivateAccountAPI.
func NewPrivateAccountAPI(b Backend, nonceLock *AddrLocker) *PrivateAccountAPI {
	return &PrivateAccountAPI{
		am:        b.AccountManager(),
		nonceLock: nonceLock,
		b:         b,
	}
}

// ListAccounts will return a list of addresses for accounts this node manages.
func (s *PrivateAccountAPI) ListAccounts() []common.Address {
	addresses := make([]common.Address, 0) // return [] instead of nil if empty
	for _, wallet := range s.am.Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address)
		}
	}
	return addresses
}

// rawWallet is a JSON representation of an accounts.Wallet interface, with its
// data contents extracted into plain fields.
type rawWallet struct {
	URL      string             `json:"url"`
	Status   string             `json:"status"`
	Failure  string             `json:"failure,omitempty"`
	Accounts []accounts.Account `json:"accounts,omitempty"`
}

// ListWallets will return a list of wallets this node manages.
func (s *PrivateAccountAPI) ListWallets() []rawWallet {
	wallets := make([]rawWallet, 0) // return [] instead of nil if empty
	for _, wallet := range s.am.Wallets() {
		status, failure := wallet.Status()

		raw := rawWallet{
			URL:      wallet.URL().String(),
			Status:   status,
			Accounts: wallet.Accounts(),
		}
		if failure != nil {
			raw.Failure = failure.Error()
		}
		wallets = append(wallets, raw)
	}
	return wallets
}

// OpenWallet initiates a hardware wallet opening procedure, establishing a USB
// connection and attempting to authenticate via the provided passphrase. Note,
// the method may return an extra challenge requiring a second open (e.g. the
// Trezor PIN matrix challenge).
func (s *PrivateAccountAPI) OpenWallet(url string, passphrase *string) error {
	wallet, err := s.am.Wallet(url)
	if err != nil {
		return err
	}
	pass := ""
	if passphrase != nil {
		pass = *passphrase
	}
	return wallet.Open(pass)
}

// DeriveAccount requests a HD wallet to derive a new account, optionally pinning
// it for later reuse.
func (s *PrivateAccountAPI) DeriveAccount(url string, path string, pin *bool) (accounts.Account, error) {
	wallet, err := s.am.Wallet(url)
	if err != nil {
		return accounts.Account{}, err
	}
	derivPath, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return accounts.Account{}, err
	}
	if pin == nil {
		pin = new(bool)
	}
	return wallet.Derive(derivPath, *pin)
}

// NewAccount will create a new account and returns the address for the new account.
func (s *PrivateAccountAPI) NewAccount(password string) (common.Address, error) {
	acc, err := fetchKeystore(s.am).NewAccount(password)
	if err == nil {
		return acc.Address, nil
	}
	return common.Address{}, err
}

// fetchKeystore retrives the encrypted keystore from the account manager.
func fetchKeystore(am *accounts.Manager) *keystore.KeyStore {
	return am.Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
}

// ImportRawKey stores the given hex encoded ECDSA key into the key directory,
// encrypting it with the passphrase.
func (s *PrivateAccountAPI) ImportRawKey(privkey string, password string) (common.Address, error) {
	key, err := crypto.HexToECDSA(privkey)
	if err != nil {
		return common.Address{}, err
	}
	acc, err := fetchKeystore(s.am).ImportECDSA(key, password)
	return acc.Address, err
}

// UnlockAccount will unlock the account associated with the given address with
// the given password for duration seconds. If duration is nil it will use a
// default of 300 seconds. It returns an indication if the account was unlocked.
func (s *PrivateAccountAPI) UnlockAccount(addr common.Address, password string, duration *uint64) (bool, error) {
	const max = uint64(time.Duration(math.MaxInt64) / time.Second)
	var d time.Duration
	if duration == nil {
		d = 300 * time.Second
	} else if *duration > max {
		return false, errors.New("unlock duration too large")
	} else {
		d = time.Duration(*duration) * time.Second
	}
	err := fetchKeystore(s.am).TimedUnlock(accounts.Account{Address: addr}, password, d)
	return err == nil, err
}

// LockAccount will lock the account associated with the given address when it's unlocked.
func (s *PrivateAccountAPI) LockAccount(addr common.Address) bool {
	return fetchKeystore(s.am).Lock(addr) == nil
}

// signTransactions sets defaults and signs the given transaction
// NOTE: the caller needs to ensure that the nonceLock is held, if applicable,
// and release it after the transaction has been submitted to the tx pool
func (s *PrivateAccountAPI) signTransaction(ctx context.Context, args SendTxArgs, passwd string) (*types.Transaction, error) {
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: args.From}
	wallet, err := s.am.Find(account)
	if err != nil {
		return nil, err
	}
	// Set some sanity defaults and terminate on failure
	if err := args.setDefaults(ctx, s.b); err != nil {
		return nil, err
	}
	// Assemble the transaction and sign with the wallet
	tx := args.toTransaction()

	return wallet.SignTxWithPassphrase(account, passwd, tx, s.b.ChainConfig().ChainID)
}

// SendTransaction will create a transaction from the given arguments and
// tries to sign it with the key associated with args.To. If the given passwd isn't
// able to decrypt the key it fails.
func (s *PrivateAccountAPI) SendTransaction(ctx context.Context, args SendTxArgs, passwd string) (common.Hash, error) {
	if args.Payment != (common.Address{}) { //personal.SendTransaction should not contain payment
		return common.Hash{}, fmt.Errorf("payment should not assigned")
	}
	if args.Nonce == nil {
		// Hold the addresse's mutex around signing to prevent concurrent assignment of
		// the same nonce to multiple accounts.
		s.nonceLock.LockAddr(args.From)
		defer s.nonceLock.UnlockAddr(args.From)
	}
	signed, err := s.signTransaction(ctx, args, passwd)
	if err != nil {
		return common.Hash{}, err
	}
	return submitTransaction(ctx, s.b, signed)
}

// SignTransaction will create a transaction from the given arguments and
// tries to sign it with the key associated with args.To. If the given passwd isn't
// able to decrypt the key it fails. The transaction is returned in RLP-form, not broadcast
// to other nodes
func (s *PrivateAccountAPI) SignTransaction(ctx context.Context, args SendTxArgs, passwd string) (*SignTransactionResult, error) {
	// No need to obtain the noncelock mutex, since we won't be sending this
	// tx into the transaction pool, but right back to the user
	if args.Payment != (common.Address{}) { //personal.SignTransaction should not contain payment
		return nil, fmt.Errorf("payment should not assigned")
	}
	if args.Gas == nil {
		return nil, fmt.Errorf("gas not specified")
	}
	if args.GasPrice == nil {
		return nil, fmt.Errorf("gasPrice not specified")
	}
	if args.Nonce == nil {
		return nil, fmt.Errorf("nonce not specified")
	}

	signed, err := s.signTransaction(ctx, args, passwd)
	if err != nil {
		return nil, err
	}
	if args.Fee == nil { //normal ethereum transaction
		//fmt.Println("into no fee")
		raw_tx_signed := signed.ConvertRawTransaction()
		if err != nil {
			return nil, err
		}
		data, err := rlp.EncodeToBytes(raw_tx_signed)
		if err != nil {
			return nil, err
		}
		return &SignTransactionResult{data, signed}, nil
	} else { //fee not nil
		//fmt.Println("into not nil of fee")
		data, err := rlp.EncodeToBytes(signed)
		if err != nil {
			return nil, err
		}
		return &SignTransactionResult{data, signed}, nil
	}
}

// signHash is a helper function that calculates a hash for the given message that can be
// safely used to calculate a signature from.
//
// The hash is calulcated as
//   keccak256("\x19True Signed Message:\n"${message length}${message}).
//
// This gives context to the signed message and prevents signing of transactions.
func signHash(data []byte) []byte {
	msg := fmt.Sprintf("\x19True Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256([]byte(msg))
}

// Sign calculates an True ECDSA signature for:
// keccack256("\x19True Signed Message:\n" + len(message) + message))
//
// Note, the produced signature conforms to the secp256k1 curve R, S and V values,
// where the V value will be 27 or 28 for legacy reasons.
//
// The key used to calculate the signature is decrypted with the given password.
//
// https://github.com/truechain/truechain-engineering-code/wiki/Management-APIs#personal_sign
func (s *PrivateAccountAPI) Sign(ctx context.Context, data hexutil.Bytes, addr common.Address, passwd string) (hexutil.Bytes, error) {
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}
	// Assemble sign the data with the wallet
	signature, err := wallet.SignTextWithPassphrase(account, passwd, signHash(data))
	if err != nil {
		return nil, err
	}
	signature[64] += 27 // Transform V from 0/1 to 27/28 according to the yellow paper
	return signature, nil
}

// EcRecover returns the address for the account that was used to create the signature.
// Note, this function is compatible with etrue__sign and personal_sign. As such it recovers
// the address of:
// hash = keccak256("\x19True Signed Message:\n"${message length}${message})
// addr = ecrecover(hash, signature)
//
// Note, the signature must conform to the secp256k1 curve R, S and V values, where
// the V value must be be 27 or 28 for legacy reasons.
//
// https://github.com/truechain/truechain-engineering-code/wiki/Management-APIs#personal_ecRecover
func (s *PrivateAccountAPI) EcRecover(ctx context.Context, data, sig hexutil.Bytes) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, fmt.Errorf("signature must be 65 bytes long")
	}
	if sig[64] != 27 && sig[64] != 28 {
		return common.Address{}, fmt.Errorf("invalid True signature (V is not 27 or 28)")
	}
	sig[64] -= 27 // Transform yellow paper V from 27/28 to 0/1

	rpk, err := crypto.SigToPub(signHash(data), sig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*rpk), nil
}

// SignAndSendTransaction was renamed to SendTransaction. This method is deprecated
// and will be removed in the future. It primary goal is to give clients time to update.
func (s *PrivateAccountAPI) SignAndSendTransaction(ctx context.Context, args SendTxArgs, passwd string) (common.Hash, error) {
	return s.SendTransaction(ctx, args, passwd)
}

// PublicBlockChainAPI provides an API to access the True blockchain.
// It offers only methods that operate on public data that is freely available to anyone.
type PublicBlockChainAPI struct {
	b Backend
}

// NewPublicBlockChainAPI creates a new True blockchain API.
func NewPublicBlockChainAPI(b Backend) *PublicBlockChainAPI {
	return &PublicBlockChainAPI{b}
}

// SnailBlockNumber returns the block number of the snailchain head.
func (s *PublicBlockChainAPI) SnailBlockNumber() hexutil.Uint64 {
	header, _ := s.b.SnailHeaderByNumber(context.Background(), rpc.LatestBlockNumber) // latest header should always be available
	return hexutil.Uint64(header.Number.Uint64())
}

func (s *PublicBlockChainAPI) FruitNumber() hexutil.Uint64 {
	block, _ := s.b.SnailBlockByNumber(context.Background(), rpc.LatestBlockNumber) // latest header should always be available
	if rpc.BlockNumber(block.NumberU64()) == rpc.EarliestBlockNumber {
		return 0
	}
	fruits := block.Fruits()
	return hexutil.Uint64(fruits[len(fruits)-1].FastNumber().Uint64())
}

// BlockNumber returns the block number of the chain head.
func (s *PublicBlockChainAPI) BlockNumber() hexutil.Uint64 {
	header, _ := s.b.HeaderByNumber(context.Background(), rpc.LatestBlockNumber) // latest header should always be available
	return hexutil.Uint64(header.Number.Uint64())
}

// GetBalance returns the amount of wei for the given address in the state of the
// given block number or hash. The rpc.LatestBlockNumber and rpc.PendingBlockNumber meta
// block numbers are also allowed.
func (s *PublicBlockChainAPI) GetBalance(ctx context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	state, _, err := s.b.StateAndHeaderByNumberOrHash(ctx, blockNrOrHash)
	if state == nil || err != nil {
		return nil, err
	}
	return (*hexutil.Big)(state.GetUnlockedBalance(address)), state.Error()
}

// GetLockBalance returns the amount of wei for the given address in pos state of the
// given block number or hash. The rpc.LatestBlockNumber and rpc.PendingBlockNumber meta
// block numbers are also allowed.
func (s *PublicBlockChainAPI) GetLockBalance(ctx context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	state, _, err := s.b.StateAndHeaderByNumberOrHash(ctx, blockNrOrHash)
	if state == nil || err != nil {
		return nil, err
	}
	return (*hexutil.Big)(state.GetPOSLocked(address)), state.Error()
}

// GetTotalBalance returns the amount of wei for the given address in the state of the
// given block number. The rpc.LatestBlockNumber and rpc.PendingBlockNumber meta
// block numbers are also allowed.
func (s *PublicBlockChainAPI) GetTotalBalance(ctx context.Context, address common.Address, blockNr rpc.BlockNumber) (*hexutil.Big, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	return (*hexutil.Big)(state.GetBalance(address)), state.Error()
}

// GetBlockByNumber returns the requested block. When blockNr is -1 the chain head is returned. When fullTx is true all
// transactions in the block are returned in full detail, otherwise only the transaction hash is returned.
func (s *PublicBlockChainAPI) GetBlockByNumber(ctx context.Context, blockNr rpc.BlockNumber, fullTx bool) (map[string]interface{}, error) {
	block, err := s.b.BlockByNumber(ctx, blockNr)
	if block != nil {
		response, err := s.rpcOutputBlock(block, true, fullTx)
		/*if err == nil && blockNr == rpc.PendingBlockNumber {
			// Pending blocks need to nil out a few fields
			for _, field := range []string{"hash", "nonce", "miner"} {
				response[field] = nil
			}
		}*/
		return response, err
	}
	return nil, err
}

// GetBlockByHash returns the requested block. When fullTx is true all transactions in the block are returned in full
// detail, otherwise only the transaction hash is returned.
func (s *PublicBlockChainAPI) GetBlockByHash(ctx context.Context, blockHash common.Hash, fullTx bool) (map[string]interface{}, error) {
	block, err := s.b.GetBlock(ctx, blockHash)
	if block != nil {
		return s.rpcOutputBlock(block, true, fullTx)
	}
	return nil, err
}

// GetSnailBlockByNumber returns the requested snail block. When blockNr is -1 the chain head is returned.
func (s *PublicBlockChainAPI) GetSnailBlockByNumber(ctx context.Context, blockNr rpc.BlockNumber, inclFruit bool) (map[string]interface{}, error) {
	block, err := s.b.SnailBlockByNumber(ctx, blockNr)
	if block != nil {
		response, err := s.rpcOutputSnailBlock(block, inclFruit)
		/*if err == nil && blockNr == rpc.PendingBlockNumber {
			// Pending blocks need to nil out a few fields
			for _, field := range []string{"hash", "nonce", "miner"} {
				response[field] = nil
			}
		}*/
		return response, err
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetSnailHashByNumber(ctx context.Context, blockNr rpc.BlockNumber) string {
	block, _ := s.b.SnailBlockByNumber(ctx, blockNr)
	if block != nil {
		return hexutil.Encode(block.Hash().Bytes())
	}
	return ""
}

// GetSnailBlockByHash returns the requested snail block.
func (s *PublicBlockChainAPI) GetSnailBlockByHash(ctx context.Context, blockHash common.Hash, inclFruit bool) (map[string]interface{}, error) {
	block, err := s.b.GetSnailBlock(ctx, blockHash)
	if block != nil {
		return s.rpcOutputSnailBlock(block, inclFruit)
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetStateChangeByFastNumber(fastNumber rpc.BlockNumber) *types.FastBalanceChangeContent {
	info := s.b.GetStateChangeByFastNumber(fastNumber)
	if info == nil || info.Balance == nil || len(info.Balance) == 0 {
		return nil
	}
	addrWithBalance := info.ToMap()
	return &types.FastBalanceChangeContent{addrWithBalance}
}

func (s *PublicBlockChainAPI) GetBalanceChangeBySnailNumber(snailNumber rpc.BlockNumber) *types.BalanceChangeContent {
	return s.b.GetBalanceChangeBySnailNumber(snailNumber)
}

func (s *PublicBlockChainAPI) GetFruitByNumber(ctx context.Context, fastblockNr rpc.BlockNumber, fullSigns bool) (map[string]interface{}, error) {
	if fastblockNr == rpc.LatestBlockNumber {
		fastHash := s.b.CurrentSnailBlock().Fruits()[len(s.b.CurrentSnailBlock().Fruits())-1].FastHash()
		return s.GetFruitByHash(ctx, fastHash, fullSigns)
	}
	block, err := s.b.BlockByNumber(ctx, fastblockNr)
	if block != nil {
		if err == nil {
			return s.GetFruitByHash(ctx, block.Hash(), fullSigns)
		}
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetFruitByHash(ctx context.Context, blockHash common.Hash, fullSigns bool) (map[string]interface{}, error) {
	block, err := s.b.GetFruit(ctx, blockHash)
	if block != nil {
		return s.rpcOutputFruit(block, fullSigns)
	}
	return nil, err
}

// GetUncleByBlockNumberAndIndex returns the uncle block for the given block hash and index. When fullTx is true
// all transactions in the block are returned in full detail, otherwise only the transaction hash is returned.
func (s *PublicBlockChainAPI) GetUncleByBlockNumberAndIndex(ctx context.Context, blockNr rpc.BlockNumber, index hexutil.Uint) (map[string]interface{}, error) {
	block, err := s.b.BlockByNumber(ctx, blockNr)
	if block != nil {
		return s.rpcOutputBlock(block, false, false)
	}
	return nil, err
}

// GetUncleByBlockHashAndIndex returns the uncle block for the given block hash and index. When fullTx is true
// all transactions in the block are returned in full detail, otherwise only the transaction hash is returned.
func (s *PublicBlockChainAPI) GetUncleByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) (map[string]interface{}, error) {
	block, err := s.b.GetBlock(ctx, blockHash)
	if block != nil {
		return s.rpcOutputBlock(block, false, false)
	}
	return nil, err
}

// GetUncleCountByBlockNumber returns number of uncles in the block for the given block number
func (s *PublicBlockChainAPI) GetUncleCountByBlockNumber(ctx context.Context, blockNr rpc.BlockNumber) *hexutil.Uint {
	return nil
}

// GetUncleCountByBlockHash returns number of uncles in the block for the given block hash
func (s *PublicBlockChainAPI) GetUncleCountByBlockHash(ctx context.Context, blockHash common.Hash) *hexutil.Uint {
	return nil
}

// GetCode returns the code stored at the given address in the state for the given block number or hash.
func (s *PublicBlockChainAPI) GetCode(ctx context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	state, _, err := s.b.StateAndHeaderByNumberOrHash(ctx, blockNrOrHash)
	if state == nil || err != nil {
		return nil, err
	}
	code := state.GetCode(address)
	return code, state.Error()
}

// GetStorageAt returns the storage from the state at the given address, key and
// block number or hash. The rpc.LatestBlockNumber and rpc.PendingBlockNumber meta block
// numbers are also allowed.
func (s *PublicBlockChainAPI) GetStorageAt(ctx context.Context, address common.Address, key string, blockNrOrHash rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	state, _, err := s.b.StateAndHeaderByNumberOrHash(ctx, blockNrOrHash)
	if state == nil || err != nil {
		return nil, err
	}
	res := state.GetState(address, common.HexToHash(key))
	return res[:], state.Error()
}

func newRevertError(result *core.ExecutionResult) *revertError {
	reason, errUnpack := abi.UnpackRevert(result.Revert())
	err := errors.New("execution reverted")
	if errUnpack == nil {
		err = fmt.Errorf("execution reverted: %v", reason)
	}
	return &revertError{
		error:  err,
		reason: hexutil.Encode(result.Revert()),
	}
}

// revertError is an API error that encompassas an EVM revertal with JSON error
// code and a binary data blob.
type revertError struct {
	error
	reason string // revert reason hex encoded
}

// ErrorCode returns the JSON error code for a revertal.
// See: https://github.com/ethereum/wiki/wiki/JSON-RPC-Error-Codes-Improvement-Proposal
func (e *revertError) ErrorCode() int {
	return 3
}

// ErrorData returns the hex encoded revert reason.
func (e *revertError) ErrorData() interface{} {
	return e.reason
}

// CallArgs represents the arguments for a call.
type CallArgs struct {
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Gas      hexutil.Uint64  `json:"gas"`
	GasPrice hexutil.Big     `json:"gasPrice"`
	Value    hexutil.Big     `json:"value"`
	Data     hexutil.Bytes   `json:"data"`
	Payer    common.Address  `json:"payer"`
	Fee      hexutil.Big     `json:"fee"`
}

func (s *PublicBlockChainAPI) doCall(ctx context.Context, args CallArgs, blockHr rpc.BlockNumberOrHash, vmCfg vm.Config, timeout time.Duration) (*core.ExecutionResult, error) {
	defer func(start time.Time) { log.Debug("Executing EVM call finished", "runtime", time.Since(start)) }(time.Now())

	state, header, err := s.b.StateAndHeaderByNumberOrHash(ctx, blockHr)
	if state == nil || err != nil {
		return nil, err
	}
	// Set sender address or use a default if none specified
	addr := args.From
	if addr == (common.Address{}) {
		if wallets := s.b.AccountManager().Wallets(); len(wallets) > 0 {
			if accounts := wallets[0].Accounts(); len(accounts) > 0 {
				addr = accounts[0].Address
			}
		}
	}
	// Set default gas & gas price if none were set
	gas, gasPrice := uint64(args.Gas), args.GasPrice.ToInt()
	if gas == 0 {
		gas = math.MaxUint64 / 2
	}
	if gasPrice.Sign() == 0 {
		gasPrice = new(big.Int).SetUint64(defaultGasPrice)
	}

	// Create new call message
	msg := types.NewMessage(addr, args.To, args.Payer, 0, args.Value.ToInt(), args.Fee.ToInt(), gas, gasPrice, args.Data, false)

	// Setup context so it may be cancelled the call has completed
	// or, in case of unmetered gas, setup a context with a timeout.
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	// Make sure the context is cancelled when the call has completed
	// this makes sure resources are cleaned up.
	defer cancel()

	// Get a new instance of the EVM.
	evm, vmError, err := s.b.GetEVM(ctx, msg, state, header, vmCfg)
	if err != nil {
		return nil, err
	}
	// Wait for the context to be done and cancel the evm. Even if the
	// EVM has finished, cancelling may be done (repeatedly)
	go func() {
		<-ctx.Done()
		evm.Cancel()
	}()

	// Setup the gas pool (also for unmetered requests)
	// and apply the message.
	gp := new(core.GasPool).AddGas(math.MaxUint64)
	result, err := core.ApplyMessage(evm, msg, gp)
	if err := vmError(); err != nil {
		return nil, err
	}
	// If the timer caused an abort, return an appropriate error message
	if evm.Cancelled() {
		return nil, fmt.Errorf("execution aborted (timeout = %v)", timeout)
	}
	if err != nil {
		return result, fmt.Errorf("err: %w (supplied gas %d)", err, msg.Gas())
	}
	return result, nil
}

// Call executes the given transaction on the state for the given block number.
// It doesn't make and changes in the state/blockchain and is useful to execute and retrieve values.
func (s *PublicBlockChainAPI) Call(ctx context.Context, args CallArgs, blockHr rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	result, err := s.doCall(ctx, args, blockHr, vm.Config{}, 5*time.Second)
	if err != nil {
		return nil, err
	}
	// If the result contains a revert reason, try to unpack and return it.
	if len(result.Revert()) > 0 {
		return nil, newRevertError(result)
	}
	return result.Return(), result.Err
}

// EstimateGas returns an estimate of the amount of gas needed to execute the
// given transaction against the current pending block.
func (s *PublicBlockChainAPI) EstimateGas(ctx context.Context, args CallArgs) (hexutil.Uint64, error) {
	// Binary search the gas requirement, as it may be higher than the amount used
	var (
		lo  uint64 = params.TxGas - 1
		hi  uint64
		cap uint64
	)
	if uint64(args.Gas) >= params.TxGas {
		hi = uint64(args.Gas)
	} else {
		// Retrieve the current pending block to act as the gas ceiling
		block, err := s.b.BlockByNumber(ctx, rpc.PendingBlockNumber)
		if err != nil {
			return 0, err
		}
		hi = block.GasLimit()
	}
	cap = hi

	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(gas uint64) (bool, *core.ExecutionResult, error) {
		args.Gas = hexutil.Uint64(gas)
		blockhr := rpc.BlockNumberOrHashWithNumber(rpc.PendingBlockNumber)
		result, err := s.doCall(ctx, args, blockhr, vm.Config{}, 0)
		if err != nil {
			if errors.Is(err, core.ErrIntrinsicGas) {
				return true, nil, nil // Special case, raise gas limit
			}
			return true, nil, err // Bail out
		}
		return result.Failed(), result, nil
	}
	// Execute the binary search and hone in on an executable gas limit
	for lo+1 < hi {
		mid := (hi + lo) / 2
		failed, _, err := executable(mid)

		// If the error is not nil(consensus error), it means the provided message
		// call or transaction will never be accepted no matter how much gas it is
		// assigned. Return the error directly, don't struggle any more.
		if err != nil {
			return 0, err
		}
		if failed {
			lo = mid
		} else {
			hi = mid
		}
	}
	// Reject the transaction as invalid if it still fails at the highest allowance
	if hi == cap {
		failed, result, err := executable(hi)
		if err != nil {
			return 0, err
		}
		if failed {
			if result != nil && result.Err != vm.ErrOutOfGas {
				if len(result.Revert()) > 0 {
					return 0, newRevertError(result)
				}
				return 0, result.Err
			}
			// Otherwise, the specified gas cap is too low
			return 0, fmt.Errorf("gas required exceeds allowance (%d)", cap)
		}
	}
	return hexutil.Uint64(hi), nil
}

func (s *PublicBlockChainAPI) GetCommittee(id rpc.BlockNumber) (map[string]interface{}, error) {
	detail, err := s.b.GetCommittee(id)
	return detail, err
}

// ExecutionResult groups all structured logs emitted by the EVM
// while replaying a transaction in debug mode as well as transaction
// execution status, the amount of gas used and the return value
type ExecutionResult struct {
	Gas         uint64         `json:"gas"`
	Failed      bool           `json:"failed"`
	ReturnValue string         `json:"returnValue"`
	StructLogs  []StructLogRes `json:"structLogs"`
}

// StructLogRes stores a structured log emitted by the EVM while replaying a
// transaction in debug mode
type StructLogRes struct {
	Pc      uint64             `json:"pc"`
	Op      string             `json:"op"`
	Gas     uint64             `json:"gas"`
	GasCost uint64             `json:"gasCost"`
	Depth   int                `json:"depth"`
	Error   string             `json:"error,omitempty"`
	Stack   *[]string          `json:"stack,omitempty"`
	Memory  *[]string          `json:"memory,omitempty"`
	Storage *map[string]string `json:"storage,omitempty"`
}

// formatLogs formats EVM returned structured logs for json output
func FormatLogs(logs []vm.StructLog) []StructLogRes {
	formatted := make([]StructLogRes, len(logs))
	for index, trace := range logs {
		formatted[index] = StructLogRes{
			Pc:      trace.Pc,
			Op:      trace.Op.String(),
			Gas:     trace.Gas,
			GasCost: trace.GasCost,
			Depth:   trace.Depth,
			Error:   trace.ErrorString(),
		}
		if trace.Stack != nil {
			stack := make([]string, len(trace.Stack))
			for i, stackValue := range trace.Stack {
				stack[i] = stackValue.Hex()
			}
			formatted[index].Stack = &stack
		}
		if trace.Memory != nil {
			memory := make([]string, 0, (len(trace.Memory)+31)/32)
			for i := 0; i+32 <= len(trace.Memory); i += 32 {
				memory = append(memory, fmt.Sprintf("%x", trace.Memory[i:i+32]))
			}
			formatted[index].Memory = &memory
		}
		if trace.Storage != nil {
			storage := make(map[string]string)
			for i, storageValue := range trace.Storage {
				storage[fmt.Sprintf("%x", i)] = fmt.Sprintf("%x", storageValue)
			}
			formatted[index].Storage = &storage
		}
	}
	return formatted
}


// RPCMarshalBlock converts the given block to the RPC output which depends on fullTx. If inclTx is true transactions are
// returned. When fullTx is true the returned block contains full transaction details, otherwise it will only contain
// transaction hashes.
func RPCMarshalBlock(b *types.Block, inclTx bool, fullTx bool) (map[string]interface{}, error) {
	head := b.Header() // copies the header once
	fields := map[string]interface{}{
		"number":           (*hexutil.Big)(head.Number),
		"hash":             b.Hash(),
		"parentHash":       head.ParentHash,
		"committeeRoot":    head.CommitteeHash,
		"miner":            head.Proposer,
		"logsBloom":        head.Bloom,
		"stateRoot":        head.Root,
		"snailHash":        head.SnailHash,
		"snailNumber":      (*hexutil.Big)(head.SnailNumber),
		"extraData":        hexutil.Bytes(head.Extra),
		"size":             hexutil.Uint64(b.Size()),
		"gasLimit":         hexutil.Uint64(head.GasLimit),
		"gasUsed":          hexutil.Uint64(head.GasUsed),
		"timestamp":        (*hexutil.Big)(head.Time),
		"transactionsRoot": head.TxHash,
		"receiptsRoot":     head.ReceiptHash,
	}

	formatSign := func(sign *types.PbftSign) (map[string]interface{}, error) {
		signmap := map[string]interface{}{
			"fastHeight": (*hexutil.Big)(sign.FastHeight),
			"fastHash":   sign.FastHash,
			"sign":       hexutil.Bytes(sign.Sign),
			"result":     sign.Result,
		}
		return signmap, nil
	}

	ss := b.Signs()
	signs := make([]interface{}, len(ss))
	var err error
	for i, sign := range ss {
		if signs[i], err = formatSign(sign); err != nil {
			return nil, err
		}
	}

	fields["signs"] = signs

	formatMembers := func(commit *types.CommitteeMember) (map[string]interface{}, error) {
		members := map[string]interface{}{
			"coinbase":      commit.Coinbase,
			"committeeBase": commit.CommitteeBase,
			"publickey":     commit.Publickey,
			"flag":          commit.Flag,
			"mType":         commit.MType,
		}
		return members, nil
	}
	switchInfos := make([]interface{}, len(b.SwitchInfos()))
	for i, member := range b.SwitchInfos() {
		if switchInfos[i], err = formatMembers(member); err != nil {
			return nil, err
		}
	}

	fields["switchInfos"] = switchInfos

	if inclTx {
		formatTx := func(tx *types.Transaction) (interface{}, error) {
			return tx.Hash(), nil
		}
		if fullTx {
			formatTx = func(tx *types.Transaction) (interface{}, error) {
				return newRPCTransactionFromBlockHash(b, tx.Hash()), nil
			}
		}
		txs := b.Transactions()
		transactions := make([]interface{}, len(txs))
		var err error
		for i, tx := range txs {
			if transactions[i], err = formatTx(tx); err != nil {
				return nil, err
			}
		}
		fields["transactions"] = transactions
	}

	return fields, nil
}

// rpcOutputBlock uses the generalized output filler.
func (s *PublicBlockChainAPI) rpcOutputBlock(b *types.Block, inclTx bool, fullTx bool) (map[string]interface{}, error) {
	fields, err := RPCMarshalBlock(b, inclTx, fullTx)
	if err != nil {
		return nil, err
	}
	// remove nonexistent field: totalDifficulty
	// fields["totalDifficulty"] = (*hexutil.Big)(s.b.GetTd(b.Hash()))
	return fields, err
}

// RPCMarshalSnailBlock converts the given snail block to the RPC output.
func RPCMarshalSnailBlock(b *types.SnailBlock, inclFruit bool) (map[string]interface{}, error) {
	head := b.Header() // copies the header once
	fields := map[string]interface{}{
		"number":     (*hexutil.Big)(head.Number),
		"hash":       b.Hash(),
		"parentHash": head.ParentHash,
		"fruitsHash": head.FruitsHash,
		"nonce":      head.Nonce,
		"mixHash":    head.MixDigest,
		"miner":      head.Coinbase,
		"difficulty": (*hexutil.Big)(head.Difficulty),
		"extraData":  hexutil.Bytes(head.Extra),
		"size":       hexutil.Uint64(b.Size()),
		"timestamp":  (*hexutil.Big)(head.Time),
	}

	fs := b.Fruits()
	if inclFruit {
		formatFruit := func(fruit *types.SnailBlock) (interface{}, error) {
			return map[string]interface{}{
				"number":     (*hexutil.Big)(fruit.Header().FastNumber),
				"hash":       fruit.Hash(),
				"nonce":      fruit.Header().Nonce,
				"miner":      fruit.Header().Coinbase,
				"difficulty": (*hexutil.Big)(fruit.Header().FruitDifficulty),
			}, nil
		}
		fruits := make([]interface{}, len(fs))
		var err error
		for i, f := range fs {
			if fruits[i], err = formatFruit(f); err != nil {
				return nil, err
			}
		}
		fields["fruits"] = fruits
	} /*else {
		fields["fruits"] = len(fs)
	}*/
	if len(fs) > 0 {
		fields["beginFruitNumber"] = (*hexutil.Big)(fs[0].FastNumber())
		fields["endFruitNumber"] = (*hexutil.Big)(fs[len(fs)-1].FastNumber())
	}

	return fields, nil
}

func RPCMarshalFruit(fruit *types.SnailBlock, fullSigns bool) (map[string]interface{}, error) {
	head := fruit.Header() // copies the header once
	fields := map[string]interface{}{
		"number":          (*hexutil.Big)(head.Number),
		"hash":            fruit.Hash(),
		"fastHash":        head.FastHash,
		"fastNumber":      (*hexutil.Big)(head.FastNumber),
		"nonce":           head.Nonce,
		"mixHash":         head.MixDigest,
		"miner":           head.Coinbase,
		"publicKey":       hexutil.Bytes(head.Publickey),
		"fruitDifficulty": (*hexutil.Big)(head.FruitDifficulty),
		"extraData":       hexutil.Bytes(head.Extra),
		"size":            hexutil.Uint64(fruit.Size()),
		"timestamp":       (*hexutil.Big)(head.Time),
		"PointerHash":     head.PointerHash,
		"pointerNumber":   (*hexutil.Big)(head.PointerNumber),
	}
	signs := fruit.Signs()
	if fullSigns {
		pbftSigns := make([]interface{}, len(signs))
		for i, sign := range signs {
			signInfo := map[string]interface{}{
				"fastHash":   sign.FastHash,
				"fastHeight": (*hexutil.Big)(sign.FastHeight),
				"result":     sign.Result,
				"sign":       hexutil.Bytes(sign.Sign),
			}
			pbftSigns[i] = signInfo
		}
		fields["signs"] = pbftSigns
	} else {
		fields["signs"] = len(signs)
	}
	return fields, nil
}

// rpcOutputSnailBlock uses the generalized output filler.
// TODO: maybe add argument/flag: fullFruit to return block with full fruit details
func (s *PublicBlockChainAPI) rpcOutputSnailBlock(b *types.SnailBlock, inclFruit bool) (map[string]interface{}, error) {
	fields, err := RPCMarshalSnailBlock(b, inclFruit)
	if err != nil {
		return nil, err
	}
	return fields, err
}

func (s *PublicBlockChainAPI) rpcOutputFruit(b *types.SnailBlock, fullSigns bool) (map[string]interface{}, error) {
	fields, err := RPCMarshalFruit(b, fullSigns)
	if err != nil {
		return nil, err
	}
	return fields, err
}

// RewardSnailBlock return the latest snail block rewarded.
func (s *PublicBlockChainAPI) RewardSnailBlock(ctx context.Context) (map[string]interface{}, error) {
	rew := s.b.GetReward(-1)
	if rew == nil {
		return nil, nil
	}
	block, err := s.b.GetSnailBlock(ctx, rew.SnailHash)

	if block != nil {
		return s.rpcOutputSnailBlock(block, true)
	}
	return nil, err
}

// GetRewardBlock return the fast block position where the given snail block is rewarded.
func (s *PublicBlockChainAPI) GetRewardBlock(ctx context.Context, blockNr rpc.BlockNumber) (map[string]interface{}, error) {
	rew := s.b.GetReward(blockNr.Int64())
	if rew == nil {
		return nil, nil
	}
	block, err := s.b.GetBlock(ctx, rew.FastHash)
	if block != nil {
		return s.rpcOutputBlock(block, true, false)
	}
	return nil, err
}

func (s *PublicBlockChainAPI) GetSnailRewardContent(blockNr rpc.BlockNumber) map[string]interface{} {
	snailRewardContent := s.b.GetSnailRewardContent(blockNr)
	return RPCMarshalRewardContent(snailRewardContent)
}

func RPCMarshalRewardContent(content *types.SnailRewardContenet) map[string]interface{} {
	if content == nil {
		return nil
	}
	fields := map[string]interface{}{
		"blockminer":      content.BlockMinerReward,
		"fruitminer":      content.FruitMinerReward,
		"committeeReward": content.CommitteeReward,
		"developerReward": content.FoundationReward,
	}
	/*log.Warn("api", "blockminer", content.BlockMinerReward)
	log.Warn("api", "committeReward", content.CommitteeReward)
	log.Warn("api", "fruitminer", len(content.FruitMinerReward))
	for _,reward :=range content.FruitMinerReward{
		for k,v :=range reward{
			log.Warn("api", hex.EncodeToString(k[:]), v)
		}
	}*/
	return fields
}

func (s *PublicBlockChainAPI) GetChainRewardContent(blockNr rpc.BlockNumber, addr common.Address) map[string]interface{} {
	content := s.b.GetChainRewardContent(blockNr)
	if content == nil {
		return nil
	}
	empty := common.Address{}
	if bytes.Equal(addr.Bytes(), empty.Bytes()) {
		fields := map[string]interface{}{
			"Number":          hexutil.Uint64(blockNr),
			"time":            hexutil.Uint64(content.St),
			"developerReward": content.Foundation,
			"blockminer":      content.CoinBase,
			"fruitminer":      content.FruitBase,
			"committeeReward": content.CommitteeBase,
		}
		return fields
	} else {
		fields := map[string]interface{}{
			"Number":        hexutil.Uint64(blockNr),
			"time":          hexutil.Uint64(content.St),
			"stakingReward": types.FetchOne(content.CommitteeBase, addr),
		}
		return fields
	}
}

// RPCTransaction represents a transaction that will serialize to the RPC representation of a transaction
type RPCTransaction struct {
	BlockHash        common.Hash     `json:"blockHash"`
	BlockNumber      *hexutil.Big    `json:"blockNumber"`
	From             common.Address  `json:"from"`
	Gas              hexutil.Uint64  `json:"gas"`
	GasPrice         *hexutil.Big    `json:"gasPrice"`
	Hash             common.Hash     `json:"hash"`
	Input            hexutil.Bytes   `json:"input"`
	Nonce            hexutil.Uint64  `json:"nonce"`
	To               *common.Address `json:"to"`
	TransactionIndex hexutil.Uint    `json:"transactionIndex"`
	Value            *hexutil.Big    `json:"value"`
	V                *hexutil.Big    `json:"v"`
	R                *hexutil.Big    `json:"r"`
	S                *hexutil.Big    `json:"s"`
	Payer            *common.Address `json:"payer"`
	Fee              *hexutil.Big    `json:"fee"`
	PV               *hexutil.Big    `json:"pv"`
	PR               *hexutil.Big    `json:"pr"`
	PS               *hexutil.Big    `json:"ps"`
}

// newRPCTransaction returns a transaction that will serialize to the RPC
// representation, with the given location metadata set (if available).
func newRPCTransaction(tx *types.Transaction, blockHash common.Hash, blockNumber uint64, index uint64) *RPCTransaction {
	var signer types.Signer = types.NewTIP1Signer(tx.ChainId())
	from, _ := types.Sender(signer, tx)
	v, r, s := tx.RawSignatureValues()

	result := &RPCTransaction{
		From:     from,
		Gas:      hexutil.Uint64(tx.Gas()),
		GasPrice: (*hexutil.Big)(tx.GasPrice()),
		Hash:     tx.Hash(),
		Input:    hexutil.Bytes(tx.Data()),
		Nonce:    hexutil.Uint64(tx.Nonce()),
		To:       tx.To(),
		Value:    (*hexutil.Big)(tx.Value()),
		V:        (*hexutil.Big)(v),
		R:        (*hexutil.Big)(r),
		S:        (*hexutil.Big)(s),
	}
	if tx.Payer() != nil {
		result.Payer = tx.Payer()
		pv, pr, ps := tx.TrueRawSignatureValues()
		result.PV = (*hexutil.Big)(pv)
		result.PR = (*hexutil.Big)(pr)
		result.PS = (*hexutil.Big)(ps)
	}
	if tx.Fee() != nil {
		result.Fee = (*hexutil.Big)(tx.Fee())
	}
	if blockHash != (common.Hash{}) {
		result.BlockHash = blockHash
		result.BlockNumber = (*hexutil.Big)(new(big.Int).SetUint64(blockNumber))
		result.TransactionIndex = hexutil.Uint(index)
	}
	return result
}

// newRPCPendingTransaction returns a pending transaction that will serialize to the RPC representation
func newRPCPendingTransaction(tx *types.Transaction) *RPCTransaction {
	return newRPCTransaction(tx, common.Hash{}, 0, 0)
}

// newRPCTransactionFromBlockIndex returns a transaction that will serialize to the RPC representation.
func newRPCTransactionFromBlockIndex(b *types.Block, index uint64) *RPCTransaction {
	txs := b.Transactions()
	if index >= uint64(len(txs)) {
		return nil
	}
	return newRPCTransaction(txs[index], b.Hash(), b.NumberU64(), index)
}

// newRPCFruitFromBlockIndex returns a fruit that will serialize to the RPC representation.
func (s *PublicBlockChainAPI) newRPCFruitFromBlockIndex(ctx context.Context, b *types.SnailBlock, index uint64, fullSigns bool) (map[string]interface{}, error) {
	fts := b.Fruits()
	if index >= uint64(len(fts)) {
		return nil, fmt.Errorf("the index you give (%d) is higher than len(fruits) (%d)", index, len(fts))
	}
	return s.GetFruitByHash(ctx, fts[index].FastHash(), fullSigns)
}

// newRPCRawTransactionFromBlockIndex returns the bytes of a transaction given a block and a transaction index.
func newRPCRawTransactionFromBlockIndex(b *types.Block, index uint64) hexutil.Bytes {
	txs := b.Transactions()
	if index >= uint64(len(txs)) {
		return nil
	}
	blob, _ := rlp.EncodeToBytes(txs[index])
	return blob
}

// newRPCTransactionFromBlockHash returns a transaction that will serialize to the RPC representation.
func newRPCTransactionFromBlockHash(b *types.Block, hash common.Hash) *RPCTransaction {
	for idx, tx := range b.Transactions() {
		if tx.Hash() == hash {
			return newRPCTransactionFromBlockIndex(b, uint64(idx))
		}
	}
	return nil
}

// PublicTransactionPoolAPI exposes methods for the RPC interface
type PublicTransactionPoolAPI struct {
	b         Backend
	nonceLock *AddrLocker
}

// NewPublicTransactionPoolAPI creates a new RPC service with methods specific for the transaction pool.
func NewPublicTransactionPoolAPI(b Backend, nonceLock *AddrLocker) *PublicTransactionPoolAPI {
	return &PublicTransactionPoolAPI{b, nonceLock}
}

// GetBlockTransactionCountByNumber returns the number of transactions in the block with the given block number.
func (s *PublicTransactionPoolAPI) GetBlockTransactionCountByNumber(ctx context.Context, blockNr rpc.BlockNumber) *hexutil.Uint {
	if block, _ := s.b.BlockByNumber(ctx, blockNr); block != nil {
		n := hexutil.Uint(len(block.Transactions()))
		return &n
	}
	return nil
}

// GetBlockTransactionCountByHash returns the number of transactions in the block with the given hash.
func (s *PublicTransactionPoolAPI) GetBlockTransactionCountByHash(ctx context.Context, blockHash common.Hash) *hexutil.Uint {
	if block, _ := s.b.GetBlock(ctx, blockHash); block != nil {
		n := hexutil.Uint(len(block.Transactions()))
		return &n
	}
	return nil
}

// GetBlockFruitCountByNumber returns the number of fruits in the block with the given block number.
func (s *PublicBlockChainAPI) GetBlockFruitCountByNumber(ctx context.Context, blockNr rpc.BlockNumber) *hexutil.Uint {
	if block, _ := s.b.SnailBlockByNumber(ctx, blockNr); block != nil {
		n := hexutil.Uint(len(block.Fruits()))
		return &n
	}
	return nil
}

// GetBlockFruitCountByHash returns the number of fruits in the block with the given hash.
func (s *PublicBlockChainAPI) GetBlockFruitCountByHash(ctx context.Context, blockHash common.Hash) *hexutil.Uint {
	if block, _ := s.b.GetSnailBlock(ctx, blockHash); block != nil {
		n := hexutil.Uint(len(block.Fruits()))
		return &n
	}
	return nil
}

// GetTransactionByBlockNumberAndIndex returns the transaction for the given block number and index.
func (s *PublicTransactionPoolAPI) GetTransactionByBlockNumberAndIndex(ctx context.Context, blockNr rpc.BlockNumber, index hexutil.Uint) *RPCTransaction {
	if block, _ := s.b.BlockByNumber(ctx, blockNr); block != nil {
		return newRPCTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

// GetTransactionByBlockHashAndIndex returns the transaction for the given block hash and index.
func (s *PublicTransactionPoolAPI) GetTransactionByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index uint64) *RPCTransaction {
	if block, _ := s.b.GetBlock(ctx, blockHash); block != nil {
		return newRPCTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

// GetFruitByBlockNumberAndIndex returns the fruit for the given block number and index.
func (s *PublicBlockChainAPI) GetFruitByBlockNumberAndIndex(ctx context.Context, blockNr rpc.BlockNumber, index hexutil.Uint, fullSigns bool) (map[string]interface{}, error) {
	if block, _ := s.b.SnailBlockByNumber(ctx, blockNr); block != nil {
		return s.newRPCFruitFromBlockIndex(ctx, block, uint64(index), fullSigns)
	}
	return nil, nil
}

// GetFruitByBlockHashAndIndex returns the fruit for the given block hash and index.
func (s *PublicBlockChainAPI) GetFruitByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint, fullSigns bool) (map[string]interface{}, error) {
	if block, _ := s.b.GetSnailBlock(ctx, blockHash); block != nil {
		return s.newRPCFruitFromBlockIndex(ctx, block, uint64(index), fullSigns)
	}
	return nil, nil
}

// GetRawTransactionByBlockNumberAndIndex returns the bytes of the transaction for the given block number and index.
func (s *PublicTransactionPoolAPI) GetRawTransactionByBlockNumberAndIndex(ctx context.Context, blockNr rpc.BlockNumber, index hexutil.Uint) hexutil.Bytes {
	if block, _ := s.b.BlockByNumber(ctx, blockNr); block != nil {
		return newRPCRawTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

// GetRawTransactionByBlockHashAndIndex returns the bytes of the transaction for the given block hash and index.
func (s *PublicTransactionPoolAPI) GetRawTransactionByBlockHashAndIndex(ctx context.Context, blockHash common.Hash, index hexutil.Uint) hexutil.Bytes {
	if block, _ := s.b.GetBlock(ctx, blockHash); block != nil {
		return newRPCRawTransactionFromBlockIndex(block, uint64(index))
	}
	return nil
}

// GetTransactionCount returns the number of transactions the given address has sent for the given block number or hash
func (s *PublicTransactionPoolAPI) GetTransactionCount(ctx context.Context, address common.Address, blockNrOrHash rpc.BlockNumberOrHash) (*hexutil.Uint64, error) {
	state, _, err := s.b.StateAndHeaderByNumberOrHash(ctx, blockNrOrHash)
	if state == nil || err != nil {
		return nil, err
	}
	nonce := state.GetNonce(address)
	return (*hexutil.Uint64)(&nonce), state.Error()
}

// GetTransactionByHash returns the transaction for the given hash
func (s *PublicTransactionPoolAPI) GetTransactionByHash(ctx context.Context, hash common.Hash) *RPCTransaction {
	// Try to return an already finalized transaction
	if tx, blockHash, blockNumber, index := rawdb.ReadTransaction(s.b.ChainDb(), hash); tx != nil {
		return newRPCTransaction(tx, blockHash, blockNumber, index)
	}
	// No finalized transaction, try to retrieve it from the pool
	if tx := s.b.GetPoolTransaction(hash); tx != nil {
		return newRPCPendingTransaction(tx)
	}
	// Transaction unknown, return as such
	return nil
}

// GetRawTransactionByHash returns the bytes of the transaction for the given hash.
func (s *PublicTransactionPoolAPI) GetRawTransactionByHash(ctx context.Context, hash common.Hash) (hexutil.Bytes, error) {
	var tx *types.Transaction

	// Retrieve a finalized transaction, or a pooled otherwise
	if tx, _, _, _ = rawdb.ReadTransaction(s.b.ChainDb(), hash); tx == nil {
		if tx = s.b.GetPoolTransaction(hash); tx == nil {
			// Transaction not found anywhere, abort
			return nil, nil
		}
	}
	// Serialize to RLP and return
	return rlp.EncodeToBytes(tx)
}

// GetTransactionReceipt returns the transaction receipt for the given transaction hash.
func (s *PublicTransactionPoolAPI) GetTransactionReceipt(ctx context.Context, hash common.Hash) (map[string]interface{}, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(s.b.ChainDb(), hash)
	if tx == nil {
		return nil, nil
	}
	receipts, err := s.b.GetReceipts(ctx, blockHash)
	if err != nil {
		return nil, err
	}
	if len(receipts) <= int(index) {
		return nil, nil
	}
	receipt := receipts[index]

	var signer types.Signer = types.NewTIP1Signer(tx.ChainId())
	from, _ := types.Sender(signer, tx)

	fields := map[string]interface{}{
		"blockHash":         blockHash,
		"blockNumber":       hexutil.Uint64(blockNumber),
		"transactionHash":   hash,
		"transactionIndex":  hexutil.Uint64(index),
		"from":              from,
		"to":                tx.To(),
		"gasUsed":           hexutil.Uint64(receipt.GasUsed),
		"cumulativeGasUsed": hexutil.Uint64(receipt.CumulativeGasUsed),
		"contractAddress":   nil,
		"logs":              receipt.Logs,
		"logsBloom":         receipt.Bloom,
		"status":            hexutil.Uint(receipt.Status),
	}

	// Assign receipt status or post state.
	if len(receipt.PostState) > 0 {
		fields["root"] = hexutil.Bytes(receipt.PostState)
	}
	if receipt.Logs == nil {
		fields["logs"] = [][]*types.Log{}
	}
	// If the ContractAddress is 20 0x0 bytes, assume it is not a contract creation
	if receipt.ContractAddress != (common.Address{}) {
		fields["contractAddress"] = receipt.ContractAddress
	}
	return fields, nil
}

// sign is a helper function that signs a transaction with the private key of the given address.
func (s *PublicTransactionPoolAPI) sign(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}
	// Request the wallet to sign the transaction
	return wallet.SignTx(account, tx, s.b.ChainConfig().ChainID)
}

func (s *PublicTransactionPoolAPI) sign_payment(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}
	// Request the wallet to sign the transaction
	return wallet.SignTx(account, tx, s.b.ChainConfig().ChainID)
}

// SendTxArgs represents the arguments to sumbit a new transaction into the transaction pool.
type SendTxArgs struct {
	Payment  common.Address  `json:"payment"`
	Fee      *hexutil.Big    `json:"fee"`
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Gas      *hexutil.Uint64 `json:"gas"`
	GasPrice *hexutil.Big    `json:"gasPrice"`
	Value    *hexutil.Big    `json:"value"`
	Nonce    *hexutil.Uint64 `json:"nonce"`
	// We accept "data" and "input" for backwards-compatibility reasons. "input" is the
	// newer name and should be preferred by clients.
	Data  *hexutil.Bytes `json:"data"`
	Input *hexutil.Bytes `json:"input"`
}

// setDefaults is a helper function that fills in default values for unspecified tx fields.
func (args *SendTxArgs) setDefaults(ctx context.Context, b Backend) error {
	if args.Gas == nil {
		args.Gas = new(hexutil.Uint64)
		*(*uint64)(args.Gas) = 90000
	}
	if args.GasPrice == nil {
		price, err := b.SuggestPrice(ctx)
		if err != nil {
			return err
		}
		args.GasPrice = (*hexutil.Big)(price)
	}
	if args.Value == nil {
		args.Value = new(hexutil.Big)
	}
	if args.Nonce == nil {
		nonce, err := b.GetPoolNonce(ctx, args.From)
		if err != nil {
			return err
		}
		args.Nonce = (*hexutil.Uint64)(&nonce)
	}
	if args.Data != nil && args.Input != nil && !bytes.Equal(*args.Data, *args.Input) {
		return errors.New(`Both "data" and "input" are set and not equal. Please use "input" to pass transaction call data.`)
	}
	if args.To == nil {
		// Contract creation
		var input []byte
		if args.Data != nil {
			input = *args.Data
		} else if args.Input != nil {
			input = *args.Input
		}
		if len(input) == 0 {
			return errors.New(`contract creation without any data provided`)
		}
	}
	return nil
}

func (args *SendTxArgs) toTransaction() *types.Transaction {
	var input []byte
	if args.Data != nil {
		input = *args.Data
	} else if args.Input != nil {
		input = *args.Input
	}
	if args.To == nil {
		return types.NewContractCreation_Payment(uint64(*args.Nonce), (*big.Int)(args.Value), (*big.Int)(args.Fee), uint64(*args.Gas), (*big.Int)(args.GasPrice), input, args.Payment)
	}
	return types.NewTransaction_Payment(uint64(*args.Nonce), *args.To, (*big.Int)(args.Value), (*big.Int)(args.Fee), uint64(*args.Gas), (*big.Int)(args.GasPrice), input, args.Payment)
}

func (args *SendTxArgs) toRawTransaction() *types.RawTransaction {
	var input []byte
	if args.Data != nil {
		input = *args.Data
	} else if args.Input != nil {
		input = *args.Input
	}
	if args.To == nil {
		return types.NewRawTransactionContract(uint64(*args.Nonce), (*big.Int)(args.Value), uint64(*args.Gas), (*big.Int)(args.GasPrice), input)
	}
	return types.NewRawTransaction(uint64(*args.Nonce), *args.To, (*big.Int)(args.Value), uint64(*args.Gas), (*big.Int)(args.GasPrice), input)
}

// submitTransaction is a helper function that submits tx to txPool and logs a message.
func submitTransaction(ctx context.Context, b Backend, tx *types.Transaction) (common.Hash, error) {
	LocalTxMetrics.Mark(1)
	if err := b.SendTx(ctx, tx); err != nil {
		return common.Hash{}, err
	}
	signer := types.MakeSigner(b.ChainConfig(), b.CurrentBlock().Number())
	//print message
	from, err := types.Sender(signer, tx)
	if err != nil {
		log.Error("submitTransaction error", "from", from.String())
		return common.Hash{}, err
	}
	payment, err := types.Payer(signer, tx)
	if err != nil {
		log.Error("submitTransaction signature error", "payemnt", payment.String())
		return common.Hash{}, err
	}
	//fmt.Printf("submitTransaction:from=%v,payemnt=%v\n", from.String(), payment.String())
	//end
	if tx.To() == nil {
		signer := types.MakeSigner(b.ChainConfig(), b.CurrentBlock().Number())
		from, err := types.Sender(signer, tx)
		if err != nil {
			return common.Hash{}, err
		}
		addr := crypto.CreateAddress(from, tx.Nonce())
		log.Info("Submitted contract creation", "fullhash", tx.Hash().Hex(), "contract", addr.Hex())
	} else {
		//log.Info("Submitted transaction", "fullhash", tx.Hash().Hex(), "recipient", tx.To())
	}
	return tx.Hash(), nil
}

func (s *PublicTransactionPoolAPI) signPayment(payment common.Address, tx *types.Transaction) (*types.Transaction, error) {
	if payment == params.EmptyAddress {
		return tx, nil
	}
	signed_payment, err := s.sign_payment(payment, tx)
	if err != nil {
		return nil, err
	}
	//fmt.Println("newTx:", newTx.Info())
	return signed_payment, nil
}

// SendTransaction creates a transaction for the given argument, sign it and submit it to the
// transaction pool.
func (s *PublicTransactionPoolAPI) SendTransaction(ctx context.Context, args SendTxArgs) (common.Hash, error) {
	log.Debug("SendTransaction", "args",
		fmt.Sprintf("API recieved  from=%v\n ,Payment=%v", args.From.String(), args.Payment.String()))
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: args.From}
	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return common.Hash{}, err
	}
	if args.Nonce == nil {
		// Hold the addresse's mutex around signing to prevent concurrent assignment of
		// the same nonce to multiple accounts.
		s.nonceLock.LockAddr(args.From)
		defer s.nonceLock.UnlockAddr(args.From)
	}

	// Set some sanity defaults and terminate on failure
	if err := args.setDefaults(ctx, s.b); err != nil {
		return common.Hash{}, err
	}
	// Assemble the transaction and sign with the wallet
	tx := args.toTransaction()
	//sign sender
	signed, err := wallet.SignTx(account, tx, s.b.ChainConfig().ChainID)
	if err != nil {
		return params.EmptyHash, err
	}
	//normal transaction execution
	if args.Payment == (common.Address{}) {
		return submitTransaction(ctx, s.b, signed)
	}
	//sign payment
	signed_payment, err := s.signPayment(args.Payment, signed)
	if err != nil {
		log.Error("signPayment error", "error", err)
		return params.EmptyHash, err
	}
	//fmt.Println("newTx2:", signedPaymentTx.Info())
	return submitTransaction(ctx, s.b, signed_payment)
}

// SendRawTransaction will add the signed transaction to the transaction pool.
// The sender is responsible for signing the transaction and using the correct nonce.
func (s *PublicTransactionPoolAPI) SendRawTransaction(ctx context.Context, encodedTx hexutil.Bytes) (common.Hash, error) {
	raw_tx := new(types.RawTransaction)
	if err := rlp.DecodeBytes(encodedTx, raw_tx); err != nil {
		log.Error("api method SendRawTransaction error", "error", err)
		return common.Hash{}, err
	}
	tx := raw_tx.ConvertTransaction()
	//log.Info("api method SendRawTransaction info", "tx.info", tx.Info())
	return submitTransaction(ctx, s.b, tx)
}

func (s *PublicTransactionPoolAPI) SendTrueRawTransaction(ctx context.Context, encodedTx hexutil.Bytes) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := rlp.DecodeBytes(encodedTx, tx); err != nil {
		log.Error("api method SendTrueRawTransaction error", "error", err)
		return common.Hash{}, err
	}
	//log.Info("api method SendTrueRawTransaction info", "tx.info", tx.Info())
	return submitTransaction(ctx, s.b, tx)
}

// Sign calculates an ECDSA signature for:
// keccack256("\x19True Signed Message:\n" + len(message) + message).
//
// Note, the produced signature conforms to the secp256k1 curve R, S and V values,
// where the V value will be 27 or 28 for legacy reasons.
//
// The account associated with addr must be unlocked.
//
// https://github.com/True/wiki/wiki/JSON-RPC#etrue_sign
func (s *PublicTransactionPoolAPI) Sign(addr common.Address, data hexutil.Bytes) (hexutil.Bytes, error) {
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}
	// Sign the requested hash with the wallet
	signature, err := wallet.SignText(account, signHash(data))
	if err == nil {
		signature[64] += 27 // Transform V from 0/1 to 27/28 according to the yellow paper
	}
	return signature, err
}

// SignTransactionResult represents a RLP encoded signed transaction.
type SignTransactionResult struct {
	Raw hexutil.Bytes      `json:"raw"`
	Tx  *types.Transaction `json:"tx"`
}

// SignTransaction will sign the given transaction with the from account.
// The node needs to have the private key of the account corresponding with
// the given from address and it needs to be unlocked.
func (s *PublicTransactionPoolAPI) SignTransaction(ctx context.Context, args SendTxArgs) (*SignTransactionResult, error) {
	if args.Gas == nil {
		return nil, fmt.Errorf("gas not specified")
	}
	if args.GasPrice == nil {
		return nil, fmt.Errorf("gasPrice not specified")
	}
	if args.Nonce == nil {
		return nil, fmt.Errorf("nonce not specified")
	}
	if err := args.setDefaults(ctx, s.b); err != nil {
		return nil, err
	}
	tx := args.toTransaction()
	//fmt.Println("api method signTransaction received payment", args.Payment.String())
	//sign from
	signed, err := s.sign(args.From, tx)
	if err != nil {
		return nil, err
	}
	if args.Payment == (common.Address{}) && args.Fee == nil { //normal ethereum transaction
		raw_tx_signed := signed.ConvertRawTransaction()
		if err != nil {
			return nil, err
		}
		data, err := rlp.EncodeToBytes(raw_tx_signed)
		if err != nil {
			return nil, err
		}
		return &SignTransactionResult{data, signed}, nil
	} else if args.Payment == (common.Address{}) { //pay is nil
		data, err := rlp.EncodeToBytes(signed)
		if err != nil {
			return nil, err
		}
		//fmt.Println("api method signTransaction signed", signed.Info())
		return &SignTransactionResult{data, signed}, nil
	}

	//sign payment
	signed_payment, err := s.signPayment(args.Payment, signed)
	if err != nil {
		log.Error("SignTransaction signPayment error", "error", err)
		return nil, err
	}

	data, err := rlp.EncodeToBytes(signed_payment)
	if err != nil {
		return nil, err
	}
	//fmt.Println("api method signTransaction signed", signed.Info())
	return &SignTransactionResult{data, signed_payment}, nil
}

// PendingTransactions returns the transactions that are in the transaction pool
// and have a from address that is one of the accounts this node manages.
func (s *PublicTransactionPoolAPI) PendingTransactions() ([]*RPCTransaction, error) {
	pending, err := s.b.GetPoolTransactions()
	if err != nil {
		return nil, err
	}
	accounts := make(map[common.Address]struct{})
	for _, wallet := range s.b.AccountManager().Wallets() {
		for _, account := range wallet.Accounts() {
			accounts[account.Address] = struct{}{}
		}
	}
	transactions := make([]*RPCTransaction, 0, len(pending))
	for _, tx := range pending {
		var signer types.Signer = types.NewTIP1Signer(tx.ChainId())
		from, _ := types.Sender(signer, tx)
		if _, exists := accounts[from]; exists {
			transactions = append(transactions, newRPCPendingTransaction(tx))
		}
	}
	return transactions, nil
}

// Resend accepts an existing transaction and a new gas price and limit. It will remove
// the given transaction from the pool and reinsert it with the new gas price and limit.
func (s *PublicTransactionPoolAPI) Resend(ctx context.Context, sendArgs SendTxArgs, gasPrice *hexutil.Big, gasLimit *hexutil.Uint64) (common.Hash, error) {
	if sendArgs.Nonce == nil {
		return common.Hash{}, fmt.Errorf("missing transaction nonce in transaction spec")
	}
	if err := sendArgs.setDefaults(ctx, s.b); err != nil {
		return common.Hash{}, err
	}
	if sendArgs.Payment != (common.Address{}) || sendArgs.Fee != nil {
		log.Error("tx has payment or fee cannot use Resend")
		return common.Hash{}, errors.New("tx has payment or fee cannot use Resend")
	}
	matchTx := sendArgs.toTransaction()
	pending, err := s.b.GetPoolTransactions()
	if err != nil {
		return common.Hash{}, err
	}

	for _, p := range pending {
		var signer types.Signer = types.NewTIP1Signer(p.ChainId())
		wantSigHash := signer.Hash(matchTx)

		if pFrom, err := types.Sender(signer, p); err == nil && pFrom == sendArgs.From && signer.Hash(p) == wantSigHash {
			/*if pPayment, err := types.Payer(signer, p); err != nil || pPayment != sendArgs.Payment {
				log.Error("pPayment error ", "from", sendArgs.From, "to", sendArgs.To, "payment", sendArgs.Payment)
				continue
			}*/
			// Match. Re-sign and send the transaction.
			if gasPrice != nil && (*big.Int)(gasPrice).Sign() != 0 {
				sendArgs.GasPrice = gasPrice
			}
			if gasLimit != nil && *gasLimit != 0 {
				sendArgs.Gas = gasLimit
			}
			signedTx, err := s.sign(sendArgs.From, sendArgs.toTransaction())
			if err != nil {
				return common.Hash{}, err
			}
			if err = s.b.SendTx(ctx, signedTx); err != nil {
				return common.Hash{}, err
			}
			return signedTx.Hash(), nil
		}
	}

	return common.Hash{}, fmt.Errorf("Transaction %#x not found", matchTx.Hash())
}

// PublicDebugAPI is the collection of True APIs exposed over the public
// debugging endpoint.
type PublicDebugAPI struct {
	b Backend
}

// NewPublicDebugAPI creates a new API definition for the public debug methods
// of the True service.
func NewPublicDebugAPI(b Backend) *PublicDebugAPI {
	return &PublicDebugAPI{b: b}
}

// GetBlockRlp retrieves the RLP encoded for of a single block.
func (api *PublicDebugAPI) GetBlockRlp(ctx context.Context, number uint64) (string, error) {
	block, _ := api.b.BlockByNumber(ctx, rpc.BlockNumber(number))
	if block == nil {
		return "", fmt.Errorf("block #%d not found", number)
	}
	encoded, err := rlp.EncodeToBytes(block)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", encoded), nil
}

// PrintBlock retrieves a block and returns its pretty printed form.
func (api *PublicDebugAPI) PrintBlock(ctx context.Context, number uint64) (string, error) {
	block, _ := api.b.BlockByNumber(ctx, rpc.BlockNumber(number))
	if block == nil {
		return "", fmt.Errorf("block #%d not found", number)
	}
	return spew.Sdump(block), nil
}

// SeedHash retrieves the seed hash of a block.
func (api *PublicDebugAPI) SeedHash(ctx context.Context, number uint64) (string, error) {
	block, _ := api.b.BlockByNumber(ctx, rpc.BlockNumber(number))
	if block == nil {
		return "", fmt.Errorf("block #%d not found", number)
	}
	return fmt.Sprintf("0x%x", ethash.SeedHash(number)), nil
}

// PrivateDebugAPI is the collection of True APIs exposed over the private
// debugging endpoint.
type PrivateDebugAPI struct {
	b Backend
}

// NewPrivateDebugAPI creates a new API definition for the private debug methods
// of the True service.
func NewPrivateDebugAPI(b Backend) *PrivateDebugAPI {
	return &PrivateDebugAPI{b: b}
}

// ChaindbProperty returns leveldb properties of the chain database.
func (api *PrivateDebugAPI) ChaindbProperty(property string) (string, error) {
	ldb, ok := api.b.ChainDb().(interface {
		LDB() *leveldb.DB
	})
	if !ok {
		return "", fmt.Errorf("chaindbProperty does not work for memory databases")
	}
	if property == "" {
		property = "leveldb.stats"
	} else if !strings.HasPrefix(property, "leveldb.") {
		property = "leveldb." + property
	}
	return ldb.LDB().GetProperty(property)
}

func (api *PrivateDebugAPI) ChaindbCompact() error {
	ldb, ok := api.b.ChainDb().(interface {
		LDB() *leveldb.DB
	})
	if !ok {
		return fmt.Errorf("chaindbCompact does not work for memory databases")
	}
	for b := byte(0); b < 255; b++ {
		log.Info("Compacting chain database", "range", fmt.Sprintf("0x%0.2X-0x%0.2X", b, b+1))
		err := ldb.LDB().CompactRange(util.Range{Start: []byte{b}, Limit: []byte{b + 1}})
		if err != nil {
			log.Error("Database compaction failed", "err", err)
			return err
		}
	}
	return nil
}

// SetHead rewinds the head of the blockchain to a previous block.
func (api *PrivateDebugAPI) SetHead(number hexutil.Uint64) {
	api.b.SetSnailHead(uint64(number))
}

// PublicNetAPI offers network related RPC methods
type PublicNetAPI struct {
	net            *p2p.Server
	networkVersion uint64
}

// NewPublicNetAPI creates a new net API instance.
func NewPublicNetAPI(net *p2p.Server, networkVersion uint64) *PublicNetAPI {
	return &PublicNetAPI{net, networkVersion}
}

// Listening returns an indication if the node is listening for network connections.
func (s *PublicNetAPI) Listening() bool {
	return true // always listening
}

// PeerCount returns the number of connected peers
func (s *PublicNetAPI) PeerCount() hexutil.Uint {
	return hexutil.Uint(s.net.PeerCount())
}

// Version returns the current True protocol version.
func (s *PublicNetAPI) Version() string {
	return fmt.Sprintf("%d", s.networkVersion)
}

// PublicImpawnAPI offers and API for the snail pool. It only operates on data that is non confidential.
type PublicImpawnAPI struct {
	b Backend
}

// NewPublicFruitPoolAPI creates a new snail pool service that gives information about the snail pool.
func NewPublicImpawnAPI(b Backend) *PublicImpawnAPI {
	return &PublicImpawnAPI{b}
}

// GetAllStakingAccount returns the pendingFruits contained within the snail pool.
func (s *PublicImpawnAPI) GetAllStakingAccount(ctx context.Context, blockNr rpc.BlockNumber) (map[string]interface{}, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	impawn := vm.NewImpawnImpl()
	err = impawn.Load(state, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	return impawn.GetAllStakingAccountRPC(uint64(blockNr)), nil
}

// GetStakingAsset returns the pendingFruits contained within the snail pool.
func (s *PublicImpawnAPI) GetStakingAsset(ctx context.Context, addr common.Address, blockNr rpc.BlockNumber) ([]vm.StakingAsset, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	impawn := vm.NewImpawnImpl()
	err = impawn.Load(state, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	return impawn.GetStakingAssetRPC(addr), nil
}

// GetLockedAsset returns the pendingFruits contained within the snail pool.
func (s *PublicImpawnAPI) GetLockedAsset(ctx context.Context, addr common.Address, blockNr rpc.BlockNumber) ([]vm.LockedAsset, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	impawn := vm.NewImpawnImpl()
	err = impawn.Load(state, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	return impawn.GetLockedAssetRPC(addr, uint64(blockNr)), nil
}

// GetAllCancelableAsset returns the pendingFruits contained within the snail pool.
func (s *PublicImpawnAPI) GetAllCancelableAsset(ctx context.Context, addr common.Address, blockNr rpc.BlockNumber) ([]vm.CancelableAsset, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	impawn := vm.NewImpawnImpl()
	err = impawn.Load(state, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	return impawn.GetAllCancelableAssetRPC(addr), nil
}

// GetStakingAccount returns the addr staking account.
func (s *PublicImpawnAPI) GetStakingAccount(ctx context.Context, addr common.Address, blockNr rpc.BlockNumber) (map[string]interface{}, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	impawn := vm.NewImpawnImpl()
	err = impawn.Load(state, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	return impawn.GetStakingAccountRPC(uint64(blockNr), addr), nil
}
func (s *PublicImpawnAPI) GetImpawnSummay(ctx context.Context, blockNr rpc.BlockNumber) (map[string]interface{}, error) {
	state, _, err := s.b.StateAndHeaderByNumber(ctx, blockNr)
	if state == nil || err != nil {
		return nil, err
	}
	impawn := vm.NewImpawnImpl()
	err = impawn.Load(state, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	return types.ToJSON(impawn.Summay()), nil
}
