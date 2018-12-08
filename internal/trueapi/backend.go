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

// Package ethapi implements the general True API functions.
package trueapi

import (
	"context"
	"math/big"

	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
)

// Backend interface provides the common API services (that are provided by
// both full and light clients) with access to necessary functions.
type Backend interface {
	// General True API
	Downloader() *downloader.Downloader
	ProtocolVersion() int
	SuggestPrice(ctx context.Context) (*big.Int, error)
	ChainDb() ethdb.Database
	EventMux() *event.TypeMux
	AccountManager() *accounts.Manager

	// BlockChain API
	SetHead(number uint64)
	HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error)
	SnailHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.SnailHeader, error)
	BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error)
	SnailBlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.SnailBlock, error)
	GetFruit(ctx context.Context, fastblockHash common.Hash) (*types.SnailBlock, error)
	StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error)
	GetBlock(ctx context.Context, blockHash common.Hash) (*types.Block, error)
	GetSnailBlock(ctx context.Context, blockHash common.Hash) (*types.SnailBlock, error)
	GetReceipts(ctx context.Context, blockHash common.Hash) (types.Receipts, error)
	GetTd(blockHash common.Hash) *big.Int
	GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error)
	SubscribeChainEvent(ch chan<- types.ChainFastEvent) event.Subscription
	SubscribeChainHeadEvent(ch chan<- types.ChainFastHeadEvent) event.Subscription
	SubscribeChainSideEvent(ch chan<- types.ChainFastSideEvent) event.Subscription
	GetReward(number int64) *types.BlockReward
	GetCommittee(id rpc.BlockNumber) (map[string]interface{}, error)

	// TxPool API
	SendTx(ctx context.Context, signedTx *types.Transaction) error
	GetPoolTransactions() (types.Transactions, error)
	GetPoolTransaction(txHash common.Hash) *types.Transaction
	GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error)
	Stats() (pending int, queued int)
	TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions)
	SubscribeNewTxsEvent(chan<- types.NewTxsEvent) event.Subscription

	ChainConfig() *params.ChainConfig
	CurrentBlock() *types.Block

	// SnailPool API
	SnailPoolContent() []*types.SnailBlock
	SnailPoolInspect() []*types.SnailBlock
	SnailPoolStats() (pending int, unVerified int)
}

func GetAPIs(apiBackend Backend) []rpc.API {
	nonceLock := new(AddrLocker)
	var apis []rpc.API
	namespaces := []string{"etrue", "eth"}
	for _, name := range namespaces {
		apis = append(apis, []rpc.API{
			{
				Namespace: name,
				Version:   "1.0",
				Service:   NewPublicTrueAPI(apiBackend),
				Public:    true,
			}, {
				Namespace: name,
				Version:   "1.0",
				Service:   NewPublicBlockChainAPI(apiBackend),
				Public:    true,
			}, {
				Namespace: name,
				Version:   "1.0",
				Service:   NewPublicTransactionPoolAPI(apiBackend, nonceLock),
				Public:    true,
			}, {
				Namespace: name,
				Version:   "1.0",
				Service:   NewPublicAccountAPI(apiBackend.AccountManager()),
				Public:    true,
			},
		}...)
	}
	apis = append(apis, []rpc.API{
		{
			Namespace: "txpool",
			Version:   "1.0",
			Service:   NewPublicTxPoolAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "fruitpool",
			Version:   "1.0",
			Service:   NewPublicFruitPoolAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(apiBackend),
		}, {
			Namespace: "personal",
			Version:   "1.0",
			Service:   NewPrivateAccountAPI(apiBackend, nonceLock),
			Public:    false,
		},
	}...)
	return apis
}
