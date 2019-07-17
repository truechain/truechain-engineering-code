// Copyright 2016 The go-ethereum Authors
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

// Package etrueclient provides a client for the Ethereum RPC API.
package etrueclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/rpc"
)

// Client defines typed wrappers for the Ethereum RPC API.
type Client struct {
	c *rpc.Client
}

// Dial connects a client to the given URL.
func Dial(rawurl string) (*Client, error) {
	return DialContext(context.Background(), rawurl)
}

func DialContext(ctx context.Context, rawurl string) (*Client, error) {
	c, err := rpc.DialContext(ctx, rawurl)
	if err != nil {
		return nil, err
	}
	return NewClient(c), nil
}

// NewClient creates a client that uses the given RPC client.
func NewClient(c *rpc.Client) *Client {
	return &Client{c}
}

func (ec *Client) Close() {
	ec.c.Close()
}

// SnailBlockchain Access
// ChainId retrieves the current chain ID for transaction replay protection.
func (ec *Client) ChainID(ctx context.Context) (*big.Int, error) {
	var result hexutil.Big
	err := ec.c.CallContext(ctx, &result, "eth_chainId")
	if err != nil {
		return nil, err
	}
	return (*big.Int)(&result), err
}

// SnailBlockByHash returns the given full block.
//
// Note that loading full blocks requires two requests.
func (ec *Client) SnailBlockByHash(ctx context.Context, hash Hash) (*rpcSnailBlock, error) {
	return ec.getSnailBlock(ctx, "etrue_getSnailBlockByHash", hash, true)
}

// SnailBlockByNumber returns a block from the current canonical chain. If number is nil, the
// latest known block is returned.
//
// Note that loading full blocks requires two requests.
func (ec *Client) SnailBlockByNumber(ctx context.Context, number *big.Int) (*rpcSnailBlock, error) {
	return ec.getSnailBlock(ctx, "etrue_getSnailBlockByNumber", toBlockNumArg(number), true)
}

type rpcSnailBlock struct {
	Hash             common.Hash      `json:"hash"`
	Number           *hexutil.Big     `json:"number"`
	ParentHash       common.Hash      `json:"parentHash"`
	FruitsHash       common.Hash      `json:"fruitsHash"`
	Nonce            types.BlockNonce `json:"nonce"`
	MixHash          common.Hash      `json:"mixHash"`
	Miner            common.Address   `json:"miner"`
	Difficulty       *hexutil.Big     `json:"difficulty"`
	ExtraData        hexutil.Bytes    `json:"extraData"`
	Size             hexutil.Uint64   `json:"size"`
	Timestamp        *hexutil.Big     `json:"timestamp"`
	Fruits           interface{}      `json:"fruits"`
	BeginFruitNumber *hexutil.Big     `json:"beginFruitNumber"`
	EndFruitNumber   *hexutil.Big     `json:"endFruitNumber"`
}

func (ec *Client) getSnailBlock(ctx context.Context, method string, args ...interface{}) (*rpcSnailBlock, error) {
	var raw json.RawMessage
	err := ec.c.CallContext(ctx, &raw, method, args...)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, truechain.NotFound
	}
	// Decode snail block
	var block rpcSnailBlock
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, err
	}

	return &block, nil
}

// FruitByHash returns the given fruit.
//
// Note that loading full blocks requires three requests.
func (ec *Client) FruitByHash(ctx context.Context, hash Hash, fullSigns bool) (*rpcFruit, error) {
	return ec.getFruit(ctx, "etrue_getFruitByHash", hash, fullSigns)
}

// FruitByNumber returns a block from the current canonical chain. If number is nil, the
// latest known fruit is returned.
//
// Note that loading full blocks requires three requests.
func (ec *Client) FruitByNumber(ctx context.Context, number *big.Int, fullSigns bool) (*rpcFruit, error) {
	return ec.getFruit(ctx, "etrue_getFruitByNumber", toBlockNumArg(number), fullSigns)
}

type rpcFruit struct {
	Hash            common.Hash      `json:"hash"`
	Number          *hexutil.Big     `json:"number"`
	FastHash        common.Hash      `json:"fastHash"`
	FastNumber      *hexutil.Big     `json:"fastNumber"`
	Nonce           types.BlockNonce `json:"nonce"`
	MixHash         common.Hash      `json:"mixHash"`
	Miner           common.Address   `json:"miner"`
	FruitDifficulty *hexutil.Big     `json:"fruitDifficulty"`
	ExtraData       hexutil.Bytes    `json:"extraData"`
	Size            hexutil.Uint64   `json:"size"`
	Timestamp       *hexutil.Big     `json:"timestamp"`
	PointerHash     common.Hash      `json:"pointerHash"`
	PointerNumber   *hexutil.Big     `json:"pointerNumber"`
	Signs           interface{}      `json:"signs"`
}

func (ec *Client) getFruit(ctx context.Context, method string, args ...interface{}) (*rpcFruit, error) {
	var raw json.RawMessage
	err := ec.c.CallContext(ctx, &raw, method, args...)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, truechain.NotFound
	}
	// Decode fruit
	var block rpcFruit
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, err
	}

	return &block, nil
}

type rpcSnailHeader struct {
	Hash       common.Hash      `json:"hash"`
	Number     *hexutil.Big     `json:"number"`
	ParentHash common.Hash      `json:"parentHash"`
	FruitsHash common.Hash      `json:"fruitsHash"`
	Nonce      types.BlockNonce `json:"nonce"`
	MixHash    common.Hash      `json:"mixHash"`
	Miner      common.Address   `json:"miner"`
	Difficulty *hexutil.Big     `json:"difficulty"`
	ExtraData  hexutil.Bytes    `json:"extraData"`
	Size       hexutil.Uint64   `json:"size"`
	Timestamp  *hexutil.Big     `json:"timestamp"`
}

// SnailHeaderByHash returns the block header with the given hash.
func (ec *Client) SnailHeaderByHash(ctx context.Context, hash Hash) (*rpcSnailHeader, error) {
	var head *rpcSnailHeader
	err := ec.c.CallContext(ctx, &head, "etrue_getSnailBlockByHash", hash, false)
	if err == nil && head == nil {
		err = truechain.NotFound
	}
	return head, err
}

// SnailHeaderByNumber returns a block header from the current canonical chain. If number is
// nil, the latest known header is returned.
func (ec *Client) SnailHeaderByNumber(ctx context.Context, number *big.Int) (*rpcSnailHeader, error) {
	var head *rpcSnailHeader
	err := ec.c.CallContext(ctx, &head, "etrue_getSnailBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		err = truechain.NotFound
	}
	return head, err
}

// Blockchain Access

// BlockByHash returns the given full block.
//
// Note that loading full blocks requires two requests. Use HeaderByHash
// if you don't need all transactions or uncle headers.
func (ec *Client) BlockByHash(ctx context.Context, hash Hash) (*types.Block, error) {
	return ec.getBlock(ctx, "etrue_getBlockByHash", hash, true)
}

// BlockByNumber returns a block from the current canonical chain. If number is nil, the
// latest known block is returned.
//
// Note that loading full blocks requires two requests. Use HeaderByNumber
// if you don't need all transactions or uncle headers.
func (ec *Client) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return ec.getBlock(ctx, "etrue_getBlockByNumber", toBlockNumArg(number), true)
}

type rpcBlock struct {
	Hash         common.Hash              `json:"hash"`
	Transactions []rpcTransaction         `json:"transactions"`
	SwitchInfos  []*types.CommitteeMember `json:"switchInfos"`
	Signs        []*types.PbftSign        `json:"signs"`
}

func (ec *Client) getBlock(ctx context.Context, method string, args ...interface{}) (*types.Block, error) {
	var raw json.RawMessage
	err := ec.c.CallContext(ctx, &raw, method, args...)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, truechain.NotFound
	}
	// Decode header and transactions.
	var head *types.Header
	var body rpcBlock
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	// Quick-verify transaction and uncle lists. This mostly helps with debugging the server.
	/*
		if head.UncleHash == types.EmptyUncleHash && len(body.UncleHashes) > 0 {
			return nil, fmt.Errorf("server returned non-empty uncle list but block header indicates no uncles")
		}
		if head.UncleHash != types.EmptyUncleHash && len(body.UncleHashes) == 0 {
			return nil, fmt.Errorf("server returned empty uncle list but block header indicates uncles")
		}
	*/
	if head.TxHash == types.EmptyRootHash && len(body.Transactions) > 0 {
		return nil, fmt.Errorf("server returned non-empty transaction list but block header indicates no transactions")
	}
	if head.TxHash != types.EmptyRootHash && len(body.Transactions) == 0 {
		return nil, fmt.Errorf("server returned empty transaction list but block header indicates transactions")
	}
	// Fill the sender cache of transactions in the block.
	txs := make([]*types.Transaction, len(body.Transactions))
	for i, tx := range body.Transactions {
		if tx.From != nil {
			setSenderFromServer(tx.tx, *tx.From, body.Hash)
		}
		txs[i] = tx.tx
	}
	return types.NewBlockWithHeader(head).WithBody(txs, body.Signs, body.SwitchInfos), nil
}

// HeaderByHash returns the block header with the given hash.
func (ec *Client) HeaderByHash(ctx context.Context, hash Hash) (*types.Header, error) {
	var head *types.Header
	err := ec.c.CallContext(ctx, &head, "etrue_getBlockByHash", hash, false)
	if err == nil && head == nil {
		err = truechain.NotFound
	}
	return head, err
}

// HeaderByNumber returns a block header from the current canonical chain. If number is
// nil, the latest known header is returned.
func (ec *Client) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	var head *types.Header
	err := ec.c.CallContext(ctx, &head, "etrue_getBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		err = truechain.NotFound
	}
	return head, err
}

type rpcTransaction struct {
	tx *types.Transaction
	txExtraInfo
}

type txExtraInfo struct {
	BlockNumber *string         `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash    `json:"blockHash,omitempty"`
	From        *common.Address `json:"from,omitempty"`
}

func (tx *rpcTransaction) UnmarshalJSON(msg []byte) error {
	if err := json.Unmarshal(msg, &tx.tx); err != nil {
		return err
	}
	return json.Unmarshal(msg, &tx.txExtraInfo)
}

// TransactionByHash returns the transaction with the given hash.
func (ec *Client) TransactionByHash(ctx context.Context, hash Hash) (tx *types.Transaction, isPending bool, err error) {
	var json *rpcTransaction
	err = ec.c.CallContext(ctx, &json, "etrue_getTransactionByHash", hash)
	if err != nil {
		return nil, false, err
	} else if json == nil {
		return nil, false, truechain.NotFound
	} else if _, r, _ := json.tx.RawSignatureValues(); r == nil {
		return nil, false, fmt.Errorf("server returned transaction without signature")
	}
	if json.From != nil && json.BlockHash != nil {
		setSenderFromServer(json.tx, *json.From, *json.BlockHash)
	}
	return json.tx, json.BlockNumber == nil, nil
}

// TransactionSender returns the sender address of the given transaction. The transaction
// must be known to the remote node and included in the blockchain at the given block and
// index. The sender is the one derived by the protocol at the time of inclusion.
//
// There is a fast-path for transactions retrieved by TransactionByHash and
// TransactionInBlock. Getting their sender address can be done without an RPC interaction.
func (ec *Client) TransactionSender(ctx context.Context, tx *types.Transaction, block Hash, index uint) (common.Address, error) {
	byteHash := block.Bytes()
	hash := common.BytesToHash(byteHash)
	// Try to load the address from the cache.
	sender, err := types.Sender(&senderFromServer{blockhash: hash}, tx)
	if err == nil {
		return sender, nil
	}
	var meta struct {
		Hash common.Hash
		From common.Address
	}
	if err = ec.c.CallContext(ctx, &meta, "etrue_getTransactionByBlockHashAndIndex", block, hexutil.Uint64(index)); err != nil {
		return common.Address{}, err
	}
	if meta.Hash == (common.Hash{}) || meta.Hash != tx.Hash() {
		return common.Address{}, errors.New("wrong inclusion block/index")
	}
	return meta.From, nil
}

// TransactionCount returns the total number of transactions in the given block.
func (ec *Client) TransactionCount(ctx context.Context, blockHash Hash) (uint, error) {
	var num hexutil.Uint
	err := ec.c.CallContext(ctx, &num, "etrue_getBlockTransactionCountByHash", blockHash)
	return uint(num), err
}

// FruitCount returns the total number of fruits in the given block.
func (ec *Client) FruitCount(ctx context.Context, blockHash Hash) (uint, error) {
	var num hexutil.Uint
	err := ec.c.CallContext(ctx, &num, "etrue_getBlockFruitCountByHash", blockHash)
	return uint(num), err
}

// TransactionInBlock returns a single transaction at index in the given block.
func (ec *Client) TransactionInBlock(ctx context.Context, blockHash Hash, index uint) (*types.Transaction, error) {
	var json *rpcTransaction
	err := ec.c.CallContext(ctx, &json, "etrue_getTransactionByBlockHashAndIndex", blockHash, hexutil.Uint64(index))
	if err == nil {
		if json == nil {
			return nil, truechain.NotFound
		} else if _, r, _ := json.tx.RawSignatureValues(); r == nil {
			return nil, fmt.Errorf("server returned transaction without signature")
		}
	}
	if json.From != nil && json.BlockHash != nil {
		setSenderFromServer(json.tx, *json.From, *json.BlockHash)
	}
	return json.tx, err
}

// FruitInBlockByHash returns a single fruit at index in the given block.
func (ec *Client) FruitInBlockByHash(ctx context.Context, snailBlockHash Hash, index uint, fullSigns bool) (*rpcFruit, error) {
	var json *rpcFruit
	err := ec.c.CallContext(ctx, &json, "etrue_getFruitByBlockHashAndIndex", snailBlockHash, hexutil.Uint64(index), fullSigns)
	return json, err
}

// FruitInBlockByNumber returns a single fruit at index in the given block.
func (ec *Client) FruitInBlockByNumber(ctx context.Context, snailBlockNumber *big.Int, index uint, fullSigns bool) (*rpcFruit, error) {
	var json *rpcFruit
	err := ec.c.CallContext(ctx, &json, "etrue_getFruitByBlockNumberAndIndex", toBlockNumArg(snailBlockNumber), hexutil.Uint64(index), fullSigns)
	return json, err
}

// TransactionReceipt returns the receipt of a transaction by transaction hash.
// Note that the receipt is not available for pending transactions.
func (ec *Client) TransactionReceipt(ctx context.Context, txHash Hash) (*types.Receipt, error) {
	var r *types.Receipt
	err := ec.c.CallContext(ctx, &r, "etrue_getTransactionReceipt", txHash)
	if err == nil {
		if r == nil {
			return nil, truechain.NotFound
		}
	}
	return r, err
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	return hexutil.EncodeBig(number)
}

type rpcProgress struct {
	StartingFastBlock uint64 // Snail Block number where sync began
	CurrentFastBlock  uint64 // Current block number where sync is at
	HighestFastBlock  uint64 // Highest alleged block number in the chain

	StartingSnailBlock hexutil.Uint64
	CurrentSnailBlock  hexutil.Uint64
	HighestSnailBlock  hexutil.Uint64
	PulledStates       hexutil.Uint64
	KnownStates        hexutil.Uint64
}

// SyncProgress retrieves the current progress of the sync algorithm. If there's
// no sync currently running, it returns nil.
func (ec *Client) SyncProgress(ctx context.Context) (*truechain.SyncProgress, error) {
	var raw json.RawMessage
	if err := ec.c.CallContext(ctx, &raw, "etrue_syncing"); err != nil {
		return nil, err
	}
	// Handle the possible response types
	var syncing bool
	if err := json.Unmarshal(raw, &syncing); err == nil {
		return nil, nil // Not syncing (always false)
	}
	var progress *rpcProgress
	if err := json.Unmarshal(raw, &progress); err != nil {
		return nil, err
	}
	return &truechain.SyncProgress{
		StartingFastBlock: uint64(progress.StartingFastBlock),
		CurrentFastBlock:  uint64(progress.CurrentFastBlock),
		HighestFastBlock:  uint64(progress.HighestFastBlock),

		StartingSnailBlock: uint64(progress.StartingSnailBlock),
		CurrentSnailBlock:  uint64(progress.CurrentSnailBlock),
		HighestSnailBlock:  uint64(progress.HighestSnailBlock),
		PulledStates:       uint64(progress.PulledStates),
		KnownStates:        uint64(progress.KnownStates),
	}, nil
}

// SubscribeNewHead subscribes to notifications about the current blockchain head
// on the given channel.
func (ec *Client) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (truechain.Subscription, error) {
	return ec.c.EthSubscribe(ctx, ch, "newHeads")
}

// State Access

// NetworkID returns the network ID (also known as the chain ID) for this chain.
func (ec *Client) NetworkID(ctx context.Context) (*big.Int, error) {
	version := new(big.Int)
	var ver string
	if err := ec.c.CallContext(ctx, &ver, "net_version"); err != nil {
		return nil, err
	}
	if _, ok := version.SetString(ver, 10); !ok {
		return nil, fmt.Errorf("invalid net_version result %q", ver)
	}
	return version, nil
}

// BalanceAt returns the wei balance of the given account.
// The block number can be nil, in which case the balance is taken from the latest known block.
func (ec *Client) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	var result hexutil.Big
	err := ec.c.CallContext(ctx, &result, "etrue_getBalance", account, toBlockNumArg(blockNumber))
	return (*big.Int)(&result), err
}

// GetBalanceAtBlockNumber returns the wei balance of the given account.
// The block number can be nil, in which case the balance is taken from the latest known block.
func (ec *Client) GetBalanceAtBlockNumber(ctx context.Context, account Address, blockNumber *big.Int) (*big.Int, error) {
	var result hexutil.Big
	err := ec.c.CallContext(ctx, &result, "etrue_getBalance", account, toBlockNumArg(blockNumber))
	return (*big.Int)(&result), err
}

// StorageAt returns the value of key in the contract storage of the given account.
// The block number can be nil, in which case the value is taken from the latest known block.
func (ec *Client) StorageAt(ctx context.Context, account Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	var result hexutil.Bytes
	err := ec.c.CallContext(ctx, &result, "etrue_getStorageAt", account, key, toBlockNumArg(blockNumber))
	return result, err
}

// CodeAt returns the contract code of the given account.
// The block number can be nil, in which case the code is taken from the latest known block.
func (ec *Client) CodeAt(ctx context.Context, account Address, blockNumber *big.Int) ([]byte, error) {
	var result hexutil.Bytes
	err := ec.c.CallContext(ctx, &result, "etrue_getCode", account, toBlockNumArg(blockNumber))
	return result, err
}

// NonceAt returns the account nonce of the given account.
// The block number can be nil, in which case the nonce is taken from the latest known block.
func (ec *Client) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	var result hexutil.Uint64
	err := ec.c.CallContext(ctx, &result, "etrue_getTransactionCount", account, toBlockNumArg(blockNumber))
	return uint64(result), err
}

// GetNonceAtBlockNumber returns the account nonce of the given account.
// The block number can be nil, in which case the nonce is taken from the latest known block.
func (ec *Client) GetNonceAtBlockNumber(ctx context.Context, account Address, blockNumber *big.Int) (uint64, error) {
	var result hexutil.Uint64
	err := ec.c.CallContext(ctx, &result, "etrue_getTransactionCount", account, toBlockNumArg(blockNumber))
	return uint64(result), err
}

// Filters

// FilterLogs executes a filter query.
func (ec *Client) FilterLogs(ctx context.Context, q truechain.FilterQuery) ([]types.Log, error) {
	var result []types.Log
	err := ec.c.CallContext(ctx, &result, "etrue_getLogs", toFilterArg(q))
	return result, err
}

// SubscribeFilterLogs subscribes to the results of a streaming filter query.
func (ec *Client) SubscribeFilterLogs(ctx context.Context, q truechain.FilterQuery, ch chan<- types.Log) (truechain.Subscription, error) {
	return ec.c.EthSubscribe(ctx, ch, "logs", toFilterArg(q))
}

func toFilterArg(q truechain.FilterQuery) interface{} {
	arg := map[string]interface{}{
		"fromBlock": toBlockNumArg(q.FromBlock),
		"toBlock":   toBlockNumArg(q.ToBlock),
		"address":   q.Addresses,
		"topics":    q.Topics,
	}
	if q.FromBlock == nil {
		arg["fromBlock"] = "0x0"
	}
	return arg
}

// Pending State

// PendingBalanceAt returns the wei balance of the given account in the pending state.
func (ec *Client) PendingBalanceAt(ctx context.Context, account Address) (*big.Int, error) {
	var result hexutil.Big
	err := ec.c.CallContext(ctx, &result, "etrue_getBalance", account, "pending")
	return (*big.Int)(&result), err
}

// PendingStorageAt returns the value of key in the contract storage of the given account in the pending state.
func (ec *Client) PendingStorageAt(ctx context.Context, account Address, key Hash) ([]byte, error) {
	var result hexutil.Bytes
	err := ec.c.CallContext(ctx, &result, "etrue_getStorageAt", account, key, "pending")
	return result, err
}

// PendingCodeAt returns the contract code of the given account in the pending state.
func (ec *Client) PendingCodeAt(ctx context.Context, account Address) ([]byte, error) {
	var result hexutil.Bytes
	err := ec.c.CallContext(ctx, &result, "etrue_getCode", account, "pending")
	return result, err
}

// PendingNonceAt returns the account nonce of the given account in the pending state.
// This is the nonce that should be used for the next transaction.
func (ec *Client) PendingNonceAt(ctx context.Context, account Address) (uint64, error) {
	var result hexutil.Uint64
	err := ec.c.CallContext(ctx, &result, "etrue_getTransactionCount", account, "pending")
	return uint64(result), err
}

// PendingTransactionCount returns the total number of transactions in the pending state.
func (ec *Client) PendingTransactionCount(ctx context.Context) (uint, error) {
	var num hexutil.Uint
	err := ec.c.CallContext(ctx, &num, "etrue_getBlockTransactionCountByNumber", "pending")
	return uint(num), err
}

// PendingFruitCount returns the total number of fruits in the pending state.
func (ec *Client) PendingFruitCount(ctx context.Context) (uint, error) {
	var num hexutil.Uint
	err := ec.c.CallContext(ctx, &num, "etrue_getBlockFruitCountByNumber", "pending")
	return uint(num), err
}

// TODO: SubscribePendingTransactions (needs server side)

// Contract Calling

// CallContract executes a message call transaction, which is directly executed in the VM
// of the node, but never mined into the blockchain.
//
// blockNumber selects the block height at which the call runs. It can be nil, in which
// case the code is taken from the latest known block. Note that state from very old
// blocks might not be available.
func (ec *Client) CallContract(ctx context.Context, msg truechain.CallMsg, blockNumber *big.Int) ([]byte, error) {
	var hex hexutil.Bytes
	err := ec.c.CallContext(ctx, &hex, "etrue_call", toCallArg(msg), toBlockNumArg(blockNumber))
	if err != nil {
		return nil, err
	}
	return hex, nil
}

// PendingCallContract executes a message call transaction using the EVM.
// The state seen by the contract call is the pending state.
func (ec *Client) PendingCallContract(ctx context.Context, msg truechain.CallMsg) ([]byte, error) {
	var hex hexutil.Bytes
	err := ec.c.CallContext(ctx, &hex, "etrue_call", toCallArg(msg), "pending")
	if err != nil {
		return nil, err
	}
	return hex, nil
}

// SuggestGasPrice retrieves the currently suggested gas price to allow a timely
// execution of a transaction.
func (ec *Client) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	var hex hexutil.Big
	if err := ec.c.CallContext(ctx, &hex, "etrue_gasPrice"); err != nil {
		return nil, err
	}
	return (*big.Int)(&hex), nil
}

// EstimateGas tries to estimate the gas needed to execute a specific transaction based on
// the current pending state of the backend blockchain. There is no guarantee that this is
// the true gas limit requirement as other transactions may be added or removed by miners,
// but it should provide a basis for setting a reasonable default.
func (ec *Client) EstimateGas(ctx context.Context, msg truechain.CallMsg) (uint64, error) {
	var hex hexutil.Uint64
	err := ec.c.CallContext(ctx, &hex, "etrue_estimateGas", toCallArg(msg))
	if err != nil {
		return 0, err
	}
	return uint64(hex), nil
}

// SendTransaction injects a signed transaction into the pending pool for execution.
//
// If the transaction was a contract creation use the TransactionReceipt method to get the
// contract address after the transaction has been mined.
func (ec *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	data, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return err
	}
	return ec.c.CallContext(ctx, nil, "etrue_sendRawTransaction", common.ToHex(data))
}

// SendPayTransaction injects a signed transaction(both sender and payer) into the pending pool for execution.
//
// If the transaction was a contract creation use the TransactionReceipt method to get the
// contract address after the transaction has been mined.
func (ec *Client) SendPayTransaction(ctx context.Context, tx *types.Transaction) error {
	data, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return err
	}
	return ec.c.CallContext(ctx, nil, "etrue_sendTrueRawTransaction", common.ToHex(data))
}

func toCallArg(msg truechain.CallMsg) interface{} {
	arg := map[string]interface{}{
		"from": msg.From,
		"to":   msg.To,
	}
	if len(msg.Data) > 0 {
		arg["data"] = hexutil.Bytes(msg.Data)
	}
	if msg.Value != nil {
		arg["value"] = (*hexutil.Big)(msg.Value)
	}
	if msg.Gas != 0 {
		arg["gas"] = hexutil.Uint64(msg.Gas)
	}
	if msg.GasPrice != nil {
		arg["gasPrice"] = (*hexutil.Big)(msg.GasPrice)
	}
	if msg.Fee != nil {
		arg["fee"] = (*hexutil.Big)(msg.Fee)
	}
	return arg
}

//etrue_protocolVersion
func (ec *Client) GetProtocolVersion(ctx context.Context) (string, error) {
	var result string
	err := ec.c.CallContext(ctx, &result, "etrue_protocolVersion", nil)
	if err != nil {
		return result, err
	}
	return result, nil
}

//etrue_coinbase
func (ec *Client) Coinbase(ctx context.Context) (string, error) {
	var result string
	err := ec.c.CallContext(ctx, &result, "etrue_coinbase", nil)
	if err != nil {
		return result, err
	}
	return result, nil
}

//etrue_mining
func (ec *Client) IsMining(ctx context.Context) (bool, error) {
	var result bool
	err := ec.c.CallContext(ctx, &result, "etrue_mining", nil)
	if err != nil {
		return result, err
	}
	return result, nil
}

//personal_listAccounts
func (ec *Client) ListAccounts(ctx context.Context) ([]common.Address, error) {
	var result []common.Address
	err := ec.c.CallContext(ctx, &result, "personal_listAccounts", nil)
	if err != nil {
		return result, err
	}
	return result, nil
}
