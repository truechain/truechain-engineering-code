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

package etrue

import (
	"context"
	"math/big"

	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/math"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/bloombits"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/etrue/gasprice"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
	"github.com/truechain/truechain-engineering-code/truescan"
)

// EthAPIBackend implements ethapi.Backend for full nodes
type EthAPIBackend struct {
	etrue *Truechain
	gpo   *gasprice.Oracle
	*truescan.TrueScan
}

// ChainConfig returns the active chain configuration.
func (b *EthAPIBackend) ChainConfig() *params.ChainConfig {
	return b.etrue.chainConfig
}

func (b *EthAPIBackend) CurrentBlock() *types.Block {
	return b.etrue.blockchain.CurrentBlock()
}

func (b *EthAPIBackend) SetHead(number uint64) {
	b.etrue.protocolManager.downloader.Cancel()
	b.etrue.blockchain.SetHead(number)
}

func (b *EthAPIBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.etrue.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.etrue.blockchain.CurrentBlock().Header(), nil
	}
	return b.etrue.blockchain.GetHeaderByNumber(uint64(blockNr)), nil
}

func (b *EthAPIBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.etrue.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.etrue.blockchain.CurrentBlock(), nil
	}
	return b.etrue.blockchain.GetBlockByNumber(uint64(blockNr)), nil
}

func (b *EthAPIBackend) SnailBlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.SnailBlock, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.etrue.miner.PendingSnailBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.etrue.snailblockchain.CurrentBlock(), nil
	}
	return b.etrue.snailblockchain.GetBlockByNumber(uint64(blockNr)), nil
}

func (b *EthAPIBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block, state := b.etrue.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, blockNr)
	if header == nil || err != nil {
		return nil, nil, err
	}
	stateDb, err := b.etrue.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *EthAPIBackend) GetBlock(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.etrue.blockchain.GetBlockByHash(hash), nil
}

func (b *EthAPIBackend) GetSnailBlock(ctx context.Context, hash common.Hash) (*types.SnailBlock, error) {
	return b.etrue.snailblockchain.GetBlockByHash(hash), nil
}

func (b *EthAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	if number := rawdb.ReadHeaderNumber(b.etrue.chainDb, hash); number != nil {
		return rawdb.ReadReceipts(b.etrue.chainDb, hash, *number), nil
	}
	return nil, nil
}

func (b *EthAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	number := rawdb.ReadHeaderNumber(b.etrue.chainDb, hash)
	if number == nil {
		return nil, nil
	}
	receipts := rawdb.ReadReceipts(b.etrue.chainDb, hash, *number)
	if receipts == nil {
		return nil, nil
	}
	logs := make([][]*types.Log, len(receipts))
	for i, receipt := range receipts {
		logs[i] = receipt.Logs
	}
	return logs, nil
}

func (b *EthAPIBackend) GetTd(blockHash common.Hash) *big.Int {
	return b.etrue.blockchain.GetTdByHash(blockHash)
}

func (b *EthAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error) {
	state.SetBalance(msg.From(), math.MaxBig256)
	vmError := func() error { return nil }

	context := core.NewEVMContext(msg, header, b.etrue.BlockChain())
	return vm.NewEVM(context, state, b.etrue.chainConfig, vmCfg), vmError, nil
}

func (b *EthAPIBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *EthAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.etrue.BlockChain().SubscribeLogsEvent(ch)
}

func (b *EthAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.etrue.txPool.AddLocal(signedTx)
}

func (b *EthAPIBackend) GetPoolTransactions() (types.Transactions, error) {
	pending, err := b.etrue.txPool.Pending()
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *EthAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.etrue.txPool.Get(hash)
}

func (b *EthAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.etrue.txPool.State().GetNonce(addr), nil
}

func (b *EthAPIBackend) Stats() (pending int, queued int) {
	return b.etrue.txPool.Stats()
}

func (b *EthAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.etrue.TxPool().Content()
}

// SubscribeNewTxsEvent registers a subscription of NewTxsEvent.
func (b *EthAPIBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.etrue.TxPool().SubscribeNewTxsEvent(ch)
}

// SubscribeAddTxEvent registers a subscription of AddTxEvent.
func (b *EthAPIBackend) SubscribeAddTxEvent(ch chan<- core.AddTxEvent) event.Subscription {
	return b.etrue.TxPool().SubscribeAddTxEvent(ch)
}

// SubscribeRemoveTxEvent registers a subscription of RemoveTxEvent.
func (b *EthAPIBackend) SubscribeRemoveTxEvent(ch chan<- core.RemoveTxEvent) event.Subscription {
	return b.etrue.TxPool().SubscribeRemoveTxEvent(ch)
}

func (b *EthAPIBackend) Downloader() *downloader.Downloader {
	return b.etrue.Downloader()
}

func (b *EthAPIBackend) ProtocolVersion() int {
	return b.etrue.EthVersion()
}

func (b *EthAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *EthAPIBackend) ChainDb() ethdb.Database {
	return b.etrue.ChainDb()
}

func (b *EthAPIBackend) EventMux() *event.TypeMux {
	return b.etrue.EventMux()
}

func (b *EthAPIBackend) AccountManager() *accounts.Manager {
	return b.etrue.AccountManager()
}

func (b *EthAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.etrue.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *EthAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.etrue.bloomRequests)
	}
}

func NewEthAPIBackend(etrue *Truechain) *EthAPIBackend {
	apiBackend := &EthAPIBackend{
		etrue: etrue,
		gpo:   nil,
	}
	apiBackend.TrueScan = truescan.New(apiBackend, &truescan.Config{
		RedisHost: etrue.config.RedisHost,
		RedisPort: etrue.config.RedisPort,
		ChannelID: etrue.config.ChannelID,
		Password:  etrue.config.Password,
	})
	return apiBackend
}

func (b *EthAPIBackend) SubscribeFastBlock(ch chan<- core.FastBlockEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeFastBlock(ch)
}

// SubscribeSnailChainHeadEvent registers a subscription of ChainHeadEvent.
func (b *EthAPIBackend) SubscribeSnailChainHeadEvent(ch chan<- snailchain.ChainHeadEvent) event.Subscription {
	return b.etrue.SnailBlockChain().SubscribeChainHeadEvent(ch)
}

func (b *EthAPIBackend) SubscribeSnailChainEvent(ch chan<- snailchain.ChainEvent) event.Subscription {
	return b.etrue.SnailBlockChain().SubscribeChainEvent(ch)
}
func (b *EthAPIBackend) SubscribeSnailChainSideEvent(ch chan<- snailchain.ChainSideEvent) event.Subscription {
	return b.etrue.SnailBlockChain().SubscribeChainSideEvent(ch)
}

// SubscribeElectionEvent registers a subscription of ElectionEvent.
func (b *EthAPIBackend) SubscribeElectionEvent(ch chan<- core.ElectionEvent) event.Subscription {
	return b.etrue.election.SubscribeElectionEvent(ch)
}

// SubscribeStateChangeEvent registers a subscription of StateChangeEvent.
func (b *EthAPIBackend) SubscribeStateChangeEvent(ch chan<- core.StateChangeEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeStateChangeEvent(ch)
}

// SubscribeRewardsEvent registers a subscription of RewardsEvent.
func (b *EthAPIBackend) SubscribeRewardsEvent(ch chan<- core.RewardsEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeRewardsEvent(ch)
}
