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

package les

import (
	"context"
	"github.com/truechain/truechain-engineering-code/light/fast"
	"math/big"

	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/math"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/bloombits"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/etrue/gasprice"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
)

type LesApiBackend struct {
	extRPCEnabled bool
	etrue         *LightEtrue
	gpo           *gasprice.Oracle
}

func (b *LesApiBackend) ChainConfig() *params.ChainConfig {
	return b.etrue.chainConfig
}

func (b *LesApiBackend) CurrentBlock() *types.Block {
	return types.NewBlockWithHeader(b.etrue.BlockChain().CurrentHeader())
}

func (b *LesApiBackend) CurrentSnailBlock() *types.SnailBlock {
	return nil
}

func (b *LesApiBackend) SetHead(number uint64) {
	b.etrue.protocolManager.downloader.Cancel()
	b.etrue.fblockchain.SetHead(number)
}

func (b *LesApiBackend) SetSnailHead(number uint64) {
	b.etrue.protocolManager.downloader.Cancel()
	b.etrue.blockchain.SetHead(number)
}

func (b *LesApiBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error) {
	if blockNr == rpc.LatestBlockNumber || blockNr == rpc.PendingBlockNumber {
		return b.etrue.fblockchain.CurrentHeader(), nil
	}

	return b.etrue.fblockchain.GetHeaderByNumberOdr(ctx, uint64(blockNr))
}
func (b *LesApiBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.etrue.fblockchain.GetHeaderByHash(hash), nil
}

func (b *LesApiBackend) SnailHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.SnailHeader, error) {
	if blockNr == rpc.LatestBlockNumber || blockNr == rpc.PendingBlockNumber {
		return b.etrue.blockchain.CurrentHeader(), nil
	}
	return b.etrue.blockchain.GetHeaderByNumberOdr(ctx, uint64(blockNr))
}

func (b *LesApiBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error) {
	header, err := b.HeaderByNumber(ctx, blockNr)
	if header == nil || err != nil {
		return nil, err
	}
	return b.GetBlock(ctx, header.Hash())
}

func (b *LesApiBackend) SnailBlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.SnailBlock, error) {
	header, err := b.SnailHeaderByNumber(ctx, blockNr)
	if header == nil || err != nil {
		return nil, err
	}
	return b.GetSnailBlock(ctx, header.Hash())
}

func (b *LesApiBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	header, err := b.HeaderByNumber(ctx, blockNr)
	if header == nil || err != nil {
		return nil, nil, err
	}
	return fast.NewState(ctx, header, b.etrue.odr), header, nil
}

func (b *LesApiBackend) GetBlock(ctx context.Context, blockHash common.Hash) (*types.Block, error) {
	return b.etrue.fblockchain.GetBlockByHash(ctx, blockHash)
}

func (b *LesApiBackend) GetFruit(ctx context.Context, fastblockHash common.Hash) (*types.SnailBlock, error) {
	return b.etrue.blockchain.GetFruit(ctx, fastblockHash)
}

func (b *LesApiBackend) GetSnailBlock(ctx context.Context, blockHash common.Hash) (*types.SnailBlock, error) {
	return b.etrue.blockchain.GetBlockByHash(ctx, blockHash)
}

func (b *LesApiBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	if number := rawdb.ReadHeaderNumber(b.etrue.chainDb, hash); number != nil {
		return fast.GetBlockReceipts(ctx, b.etrue.odr, hash, *number)
	}
	return nil, nil
}

func (b *LesApiBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	if number := rawdb.ReadHeaderNumber(b.etrue.chainDb, hash); number != nil {
		return fast.GetBlockLogs(ctx, b.etrue.odr, hash, *number)
	}
	return nil, nil
}

func (b *LesApiBackend) GetTd(hash common.Hash) *big.Int {
	return b.etrue.blockchain.GetTdByHash(hash)
}

func (b *LesApiBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error) {
	state.SetBalance(msg.From(), math.MaxBig256)
	context := core.NewEVMContext(msg, header, b.etrue.fblockchain, nil, nil)
	return vm.NewEVM(context, state, b.etrue.chainConfig, vmCfg), state.Error, nil
}

func (b *LesApiBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.etrue.txPool.Add(ctx, signedTx)
}

func (b *LesApiBackend) RemoveTx(txHash common.Hash) {
	b.etrue.txPool.RemoveTx(txHash)
}

func (b *LesApiBackend) GetPoolTransactions() (types.Transactions, error) {
	return b.etrue.txPool.GetTransactions()
}

func (b *LesApiBackend) GetPoolTransaction(txHash common.Hash) *types.Transaction {
	return b.etrue.txPool.GetTransaction(txHash)
}

func (b *LesApiBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	return fast.GetTransaction(ctx, b.etrue.odr, txHash)
}

func (b *LesApiBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.etrue.txPool.GetNonce(ctx, addr)
}

func (b *LesApiBackend) Stats() (pending int, queued int) {
	return b.etrue.txPool.Stats(), 0
}

func (b *LesApiBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.etrue.txPool.Content()
}

func (b *LesApiBackend) SubscribeNewTxsEvent(ch chan<- types.NewTxsEvent) event.Subscription {
	return b.etrue.txPool.SubscribeNewTxsEvent(ch)
}

func (b *LesApiBackend) SubscribeChainEvent(ch chan<- types.FastChainEvent) event.Subscription {
	return b.etrue.fblockchain.SubscribeChainEvent(ch)
}

func (b *LesApiBackend) SubscribeChainHeadEvent(ch chan<- types.FastChainHeadEvent) event.Subscription {
	return b.etrue.fblockchain.SubscribeChainHeadEvent(ch)
}

func (b *LesApiBackend) SubscribeChainSideEvent(ch chan<- types.FastChainSideEvent) event.Subscription {
	return b.etrue.fblockchain.SubscribeChainSideEvent(ch)
}

func (b *LesApiBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.etrue.fblockchain.SubscribeLogsEvent(ch)
}

func (b *LesApiBackend) SubscribeRemovedLogsEvent(ch chan<- types.RemovedLogsEvent) event.Subscription {
	return b.etrue.fblockchain.SubscribeRemovedLogsEvent(ch)
}

func (b *LesApiBackend) GetReward(number int64) *types.BlockReward {
	//if number < 0 {
	//	return b.etrue.blockchain.CurrentReward()
	//}

	//return b.etrue.blockchain.GetFastHeightBySnailHeight(uint64(number))
	return nil
}

func (b *LesApiBackend) GetCommittee(number rpc.BlockNumber) (map[string]interface{}, error) {
	return nil, nil
}

func (b *LesApiBackend) GetBalanceChangeBySnailNumber(snailNumber rpc.BlockNumber) *types.BalanceChange {
	return nil
}

func (b *LesApiBackend) GetStateChangeByFastNumber(ctx context.Context, fastNumber rpc.BlockNumber) *types.BalanceChange {
	return nil
}

func (b *LesApiBackend) GetSnailRewardContent(number rpc.BlockNumber) *types.SnailRewardContenet {
	return nil
}
func (b *LesApiBackend) GetChainRewardContent(blockNr rpc.BlockNumber) *types.TimedChainReward {
	return nil
}

func (b *LesApiBackend) SnailPoolContent() []*types.SnailBlock {
	return nil
}

func (b *LesApiBackend) SnailPoolInspect() []*types.SnailBlock {
	return nil
}

func (b *LesApiBackend) SnailPoolStats() (pending int, unVerified int) {
	return 0, 0
}

func (b *LesApiBackend) Downloader() *downloader.Downloader {
	return b.etrue.Downloader()
}

func (b *LesApiBackend) ProtocolVersion() int {
	return b.etrue.LesVersion() + 10000
}

func (b *LesApiBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *LesApiBackend) ChainDb() etruedb.Database {
	return b.etrue.chainDb
}

func (b *LesApiBackend) EventMux() *event.TypeMux {
	return b.etrue.eventMux
}

func (b *LesApiBackend) AccountManager() *accounts.Manager {
	return b.etrue.accountManager
}

func (b *LesApiBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *LesApiBackend) BloomStatus() (uint64, uint64) {
	if b.etrue.bloomIndexer == nil {
		return 0, 0
	}
	sections, _, _ := b.etrue.bloomIndexer.Sections()
	return params.BloomBitsBlocksClient, sections
}

func (b *LesApiBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.etrue.bloomRequests)
	}
}
