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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/bloombits"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
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

// TRUEAPIBackend implements ethapi.Backend for full nodes
type TrueAPIBackend struct {
	etrue *Truechain
	gpo   *gasprice.Oracle
	*truescan.TrueScan
}

func NewTrueAPIBackend(etrue *Truechain) *TrueAPIBackend {
	apiBackend := &TrueAPIBackend{
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

// ChainConfig returns the active chain configuration.
func (b *TrueAPIBackend) ChainConfig() *params.ChainConfig {
	return b.etrue.chainConfig
}

func (b *TrueAPIBackend) CurrentBlock() *types.Block {
	return b.etrue.blockchain.CurrentBlock()
}

func (b *TrueAPIBackend) SetHead(number uint64) {
	b.etrue.protocolManager.downloader.Cancel()
	b.etrue.blockchain.SetHead(number)
}

func (b *TrueAPIBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error) {
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

func (b *TrueAPIBackend) SnailHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.SnailHeader, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.etrue.miner.PendingSnailBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.etrue.snailblockchain.CurrentBlock().Header(), nil
	}
	return b.etrue.snailblockchain.GetHeaderByNumber(uint64(blockNr)), nil
}

func (b *TrueAPIBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error) {
	// Only snailchain has miner, also return current block here for fastchain
	if blockNr == rpc.PendingBlockNumber {
		block := b.etrue.blockchain.CurrentBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.etrue.blockchain.CurrentBlock(), nil
	}
	return b.etrue.blockchain.GetBlockByNumber(uint64(blockNr)), nil
}

func (b *TrueAPIBackend) SnailBlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.SnailBlock, error) {
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

func (b *TrueAPIBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		state, _ := b.etrue.blockchain.State()
		block := b.etrue.blockchain.CurrentBlock()
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

func (b *TrueAPIBackend) GetBlock(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.etrue.blockchain.GetBlockByHash(hash), nil
}

func (b *TrueAPIBackend) GetSnailBlock(ctx context.Context, hash common.Hash) (*types.SnailBlock, error) {
	return b.etrue.snailblockchain.GetBlockByHash(hash), nil
}

func (b *TrueAPIBackend) GetFruit(ctx context.Context, fastblockHash common.Hash) (*types.SnailBlock, error) {
	return b.etrue.snailblockchain.GetFruit(fastblockHash), nil
}

func (b *TrueAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	if number := rawdb.ReadHeaderNumber(b.etrue.chainDb, hash); number != nil {
		return rawdb.ReadReceipts(b.etrue.chainDb, hash, *number), nil
	}
	return nil, nil
}

func (b *TrueAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
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

func (b *TrueAPIBackend) GetTd(blockHash common.Hash) *big.Int {
	return b.etrue.blockchain.GetTdByHash(blockHash)
}

func (b *TrueAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error) {
	state.SetBalance(msg.From(), math.MaxBig256)
	vmError := func() error { return nil }

	context := core.NewEVMContext(msg, header, b.etrue.BlockChain())
	return vm.NewEVM(context, state, b.etrue.chainConfig, vmCfg), vmError, nil
}

func (b *TrueAPIBackend) SubscribeRemovedLogsEvent(ch chan<- types.RemovedLogsEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *TrueAPIBackend) SubscribeChainEvent(ch chan<- types.ChainFastEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainEvent(ch)
}

func (b *TrueAPIBackend) SubscribeChainHeadEvent(ch chan<- types.ChainFastHeadEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *TrueAPIBackend) SubscribeChainSideEvent(ch chan<- types.ChainFastSideEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *TrueAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.etrue.BlockChain().SubscribeLogsEvent(ch)
}

func (b *TrueAPIBackend) GetReward(number int64) *types.BlockReward {
	if number < 0 {
		return b.etrue.blockchain.CurrentReward()
	}
	return b.etrue.blockchain.GetFastHeightBySnailHeight(uint64(number))
}

func (b *TrueAPIBackend) GetCommittee(number rpc.BlockNumber) (map[string]interface{}, error) {
	return b.etrue.election.GetComitteeById(big.NewInt(number.Int64())), nil
}

func (b *TrueAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.etrue.txPool.AddLocal(signedTx)
}

func (b *TrueAPIBackend) GetPoolTransactions() (types.Transactions, error) {
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

func (b *TrueAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.etrue.txPool.Get(hash)
}

func (b *TrueAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.etrue.txPool.State().GetNonce(addr), nil
}

func (b *TrueAPIBackend) Stats() (pending int, queued int) {
	return b.etrue.txPool.Stats()
}

func (b *TrueAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.etrue.TxPool().Content()
}

// SubscribeNewTxsEvent registers a subscription of NewTxsEvent.
func (b *TrueAPIBackend) SubscribeNewTxsEvent(ch chan<- types.NewTxsEvent) event.Subscription {
	return b.etrue.TxPool().SubscribeNewTxsEvent(ch)
}

// SubscribeAddTxEvent registers a subscription of AddTxEvent.
func (b *TrueAPIBackend) SubscribeAddTxEvent(ch chan<- types.AddTxEvent) event.Subscription {
	return b.etrue.TxPool().SubscribeAddTxEvent(ch)
}

// SubscribeRemoveTxEvent registers a subscription of RemoveTxEvent.
func (b *TrueAPIBackend) SubscribeRemoveTxEvent(ch chan<- types.RemoveTxEvent) event.Subscription {
	return b.etrue.TxPool().SubscribeRemoveTxEvent(ch)
}

func (b *TrueAPIBackend) Downloader() *downloader.Downloader {
	return b.etrue.Downloader()
}

func (b *TrueAPIBackend) ProtocolVersion() int {
	return b.etrue.EthVersion()
}

func (b *TrueAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *TrueAPIBackend) ChainDb() ethdb.Database {
	return b.etrue.ChainDb()
}

func (b *TrueAPIBackend) EventMux() *event.TypeMux {
	return b.etrue.EventMux()
}

func (b *TrueAPIBackend) AccountManager() *accounts.Manager {
	return b.etrue.AccountManager()
}

func (b *TrueAPIBackend) SnailPoolContent() []*types.SnailBlock {
	return b.etrue.SnailPool().Content()
}

func (b *TrueAPIBackend) SnailPoolInspect() []*types.SnailBlock {
	return b.etrue.SnailPool().Inspect()
}

func (b *TrueAPIBackend) SnailPoolStats() (pending int, unVerified int) {
	return b.etrue.SnailPool().Stats()
}

func (b *TrueAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.etrue.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *TrueAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.etrue.bloomRequests)
	}
}

func (b *TrueAPIBackend) SubscribeFastBlock(ch chan<- types.FastBlockEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeFastBlock(ch)
}

// SubscribeSnailChainHeadEvent registers a subscription of ChainHeadEvent.
func (b *TrueAPIBackend) SubscribeSnailChainHeadEvent(ch chan<- types.ChainSnailHeadEvent) event.Subscription {
	return b.etrue.SnailBlockChain().SubscribeChainHeadEvent(ch)
}

func (b *TrueAPIBackend) SubscribeSnailChainEvent(ch chan<- types.ChainSnailEvent) event.Subscription {
	return b.etrue.SnailBlockChain().SubscribeChainEvent(ch)
}
func (b *TrueAPIBackend) SubscribeSnailChainSideEvent(ch chan<- types.ChainSnailSideEvent) event.Subscription {
	return b.etrue.SnailBlockChain().SubscribeChainSideEvent(ch)
}

// SubscribeElectionEvent registers a subscription of ElectionEvent.
func (b *TrueAPIBackend) SubscribeElectionEvent(ch chan<- types.ElectionEvent) event.Subscription {
	return b.etrue.election.SubscribeElectionEvent(ch)
}

// SubscribeStateChangeEvent registers a subscription of StateChangeEvent.
func (b *TrueAPIBackend) SubscribeStateChangeEvent(ch chan<- types.StateChangeEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeStateChangeEvent(ch)
}

// SubscribeRewardsEvent registers a subscription of RewardsEvent.
func (b *TrueAPIBackend) SubscribeRewardsEvent(ch chan<- types.RewardsEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeRewardsEvent(ch)
}
