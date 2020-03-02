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
	"fmt"
	"github.com/truechain/truechain-engineering-code/log"
	"math/big"

	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/math"
	"github.com/truechain/truechain-engineering-code/consensus"
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

// TRUEAPIBackend implements ethapi.Backend for full nodes
type TrueAPIBackend struct {
	etrue *Truechain
	gpo   *gasprice.Oracle
}

// ChainConfig returns the active chain configuration.
func (b *TrueAPIBackend) ChainConfig() *params.ChainConfig {
	return b.etrue.chainConfig
}

// CurrentBlock return the fast chain current Block
func (b *TrueAPIBackend) CurrentBlock() *types.Block {
	return b.etrue.blockchain.CurrentBlock()
}

// CurrentSnailBlock return the Snail chain current Block
func (b *TrueAPIBackend) CurrentSnailBlock() *types.SnailBlock {
	return b.etrue.snailblockchain.CurrentBlock()
}

// SetHead Set the newest position of Fast Chain, that will reset the fast blockchain comment
func (b *TrueAPIBackend) SetHead(number uint64) {
	b.etrue.protocolManager.downloader.Cancel()
	b.etrue.blockchain.SetHead(number)
}

// SetSnailHead Set the newest position of snail chain
func (b *TrueAPIBackend) SetSnailHead(number uint64) {
	b.etrue.protocolManager.downloader.Cancel()
	b.etrue.snailblockchain.SetHead(number)
}

// HeaderByNumber returns Header of fast chain by the number
// rpc.PendingBlockNumber == "pending"; rpc.LatestBlockNumber == "latest" ; rpc.LatestBlockNumber == "earliest"
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

// HeaderByHash returns header of fast chain by the hash
func (b *TrueAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.etrue.blockchain.GetHeaderByHash(hash), nil
}

// SnailHeaderByNumber returns Header of snail chain by the number
// rpc.PendingBlockNumber == "pending"; rpc.LatestBlockNumber == "latest" ; rpc.LatestBlockNumber == "earliest"
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

// BlockByNumber returns block of fast chain by the number
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

// SnailBlockByNumber returns block of snial chain by the number
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

// StateAndHeaderByNumber returns the state of block by the number
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

// GetBlock returns the block by the block's hash
func (b *TrueAPIBackend) GetBlock(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.etrue.blockchain.GetBlockByHash(hash), nil
}

// GetSnailBlock returns the snail block by the block's hash
func (b *TrueAPIBackend) GetSnailBlock(ctx context.Context, hash common.Hash) (*types.SnailBlock, error) {
	return b.etrue.snailblockchain.GetBlockByHash(hash), nil
}

// GetFruit returns the fruit by the block's hash
func (b *TrueAPIBackend) GetFruit(ctx context.Context, fastblockHash common.Hash) (*types.SnailBlock, error) {
	return b.etrue.snailblockchain.GetFruit(fastblockHash), nil
}

// GetReceipts returns the Receipt details by txhash
func (b *TrueAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	if number := rawdb.ReadHeaderNumber(b.etrue.chainDb, hash); number != nil {
		return rawdb.ReadReceipts(b.etrue.chainDb, hash, *number), nil
	}
	return nil, nil
}

// GetLogs returns the logs by txhash
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

// GetTd returns the total diffcult with block height by blockhash
func (b *TrueAPIBackend) GetTd(blockHash common.Hash) *big.Int {
	return b.etrue.snailblockchain.GetTdByHash(blockHash)
}

// GetEVM returns the EVM
func (b *TrueAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error) {
	state.SetBalance(msg.From(), math.MaxBig256)
	vmError := func() error { return nil }

	context := core.NewEVMContext(msg, header, b.etrue.BlockChain(), nil, nil)
	return vm.NewEVM(context, state, b.etrue.chainConfig, vmCfg), vmError, nil
}

// SubscribeRemovedLogsEvent registers a subscription of RemovedLogsEvent in fast blockchain
func (b *TrueAPIBackend) SubscribeRemovedLogsEvent(ch chan<- types.RemovedLogsEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeRemovedLogsEvent(ch)
}

// SubscribeChainEvent registers a subscription of chainEvnet in fast blockchain
func (b *TrueAPIBackend) SubscribeChainEvent(ch chan<- types.FastChainEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainEvent(ch)
}

// SubscribeChainHeadEvent registers a subscription of chainHeadEvnet in fast blockchain
func (b *TrueAPIBackend) SubscribeChainHeadEvent(ch chan<- types.FastChainHeadEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainHeadEvent(ch)
}

// SubscribeChainSideEvent registers a subscription of chainSideEvnet in fast blockchain,deprecated
func (b *TrueAPIBackend) SubscribeChainSideEvent(ch chan<- types.FastChainSideEvent) event.Subscription {
	return b.etrue.BlockChain().SubscribeChainSideEvent(ch)
}

// SubscribeLogsEvent registers a subscription of log in fast blockchain
func (b *TrueAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.etrue.BlockChain().SubscribeLogsEvent(ch)
}

// GetReward returns the Reward info by number in fastchain
func (b *TrueAPIBackend) GetReward(number int64) *types.BlockReward {
	if number < 0 {
		return b.etrue.blockchain.CurrentReward()
	}
	return b.etrue.blockchain.GetBlockReward(uint64(number))
}

// GetSnailRewardContent returns the Reward content by number in Snailchain
func (b *TrueAPIBackend) GetSnailRewardContent(snailNumber rpc.BlockNumber) *types.SnailRewardContenet {
	return b.etrue.agent.GetSnailRewardContent(uint64(snailNumber))
}
func (b *TrueAPIBackend) GetRecentChainRewardContent(blockNr rpc.BlockNumber) *types.TimedChainReward {
	snailHeight := uint64(blockNr)
	return consensus.CR.GetChainReward(snailHeight)
}
func (b *TrueAPIBackend) GetChainRewardContent(blockNr rpc.BlockNumber) *types.ChainReward {
	sheight := uint64(blockNr)
	return b.etrue.snailblockchain.GetRewardInfos(sheight)
}

// GetCommittee returns the Committee info by committee number

func (b *TrueAPIBackend) GetStateChangeByFastNumber(ctx context.Context,
	fastNumber rpc.BlockNumber) *types.BalanceChange {
	fmt.Println("go into fastNumber")
	header, err := b.HeaderByNumber(ctx, fastNumber)
	if header == nil || err != nil {
		return nil
	}
	stateDb, err := b.etrue.BlockChain().StateAt(header.Root)
	//var addrWithBalance = stateDb.Balances() //map[common.Address]*big.Int
	var addrWithBalance = stateDb.Balances() //map[common.Address]*big.Int
	return &types.BalanceChange{addrWithBalance}
}

func (b *TrueAPIBackend) GetBalanceChangeBySnailNumber(
	snailNumber rpc.BlockNumber) *types.BalanceChange {
	fmt.Println("go into snailumber")
	var sBlock = b.etrue.SnailBlockChain().GetBlockByNumber(uint64(snailNumber))
	state, _ := b.etrue.BlockChain().State()
	var (
		addrWithBalance          = make(map[common.Address]*big.Int)
		committeeAddrWithBalance = make(map[common.Address]*big.Int)
		blockFruits              = sBlock.Body().Fruits
		blockFruitsLen           = big.NewInt(int64(len(blockFruits)))
	)
	if blockFruitsLen.Uint64() == 0 {
		return nil
	}
	//snailBlock miner's award
	var balance = state.GetBalance(sBlock.Coinbase())
	addrWithBalance[sBlock.Coinbase()] = balance

	for _, fruit := range blockFruits {
		if addrWithBalance[fruit.Coinbase()] == nil {
			addrWithBalance[fruit.Coinbase()] = state.GetBalance(fruit.Coinbase())
		}
		var committeeMembers = b.etrue.election.GetCommittee(fruit.FastNumber())

		for _, cm := range committeeMembers {
			if committeeAddrWithBalance[cm.Coinbase] == nil {
				committeeAddrWithBalance[cm.Coinbase] = state.GetBalance(cm.Coinbase)
			}
		}
	}
	log.Error("committeeMembers info", "committeeAddrWithBalance.length", len(committeeAddrWithBalance))
	for addr, balance := range committeeAddrWithBalance {
		if addrWithBalance[addr] == nil {
			addrWithBalance[addr] = balance
		}
	}
	return &types.BalanceChange{addrWithBalance}
}

func (b *TrueAPIBackend) GetCommittee(number rpc.BlockNumber) (map[string]interface{}, error) {
	if number == rpc.LatestBlockNumber {
		return b.etrue.election.GetCommitteeById(new(big.Int).SetUint64(b.etrue.agent.CommitteeNumber())), nil
	}
	return b.etrue.election.GetCommitteeById(big.NewInt(number.Int64())), nil
}

func (b *TrueAPIBackend) GetCurrentCommitteeNumber() *big.Int {
	return b.etrue.election.GetCurrentCommitteeNumber()
}

// SendTx returns nil by success to add local txpool
func (b *TrueAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.etrue.txPool.AddLocal(signedTx)
}

// GetPoolTransactions returns Transactions by pending state in txpool
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

// GetPoolTransaction returns Transaction by txHash in txpool
func (b *TrueAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.etrue.txPool.Get(hash)
}

// GetPoolNonce returns user nonce by user address in txpool
func (b *TrueAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.etrue.txPool.State().GetNonce(addr), nil
}

// Stats returns the count tx in txpool
func (b *TrueAPIBackend) Stats() (pending int, queued int) {
	return b.etrue.txPool.Stats()
}

func (b *TrueAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.etrue.TxPool().Content()
}

// SubscribeNewTxsEvent returns the subscript event of new tx
func (b *TrueAPIBackend) SubscribeNewTxsEvent(ch chan<- types.NewTxsEvent) event.Subscription {
	return b.etrue.TxPool().SubscribeNewTxsEvent(ch)
}

// Downloader returns the fast downloader
func (b *TrueAPIBackend) Downloader() *downloader.Downloader {
	return b.etrue.Downloader()
}

// ProtocolVersion returns the version of protocol
func (b *TrueAPIBackend) ProtocolVersion() int {
	return b.etrue.EthVersion()
}

// SuggestPrice returns tht suggest gas price
func (b *TrueAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

// ChainDb returns tht database of fastchain
func (b *TrueAPIBackend) ChainDb() etruedb.Database {
	return b.etrue.ChainDb()
}

// EventMux returns Event locker
func (b *TrueAPIBackend) EventMux() *event.TypeMux {
	return b.etrue.EventMux()
}

// AccountManager returns Account Manager
func (b *TrueAPIBackend) AccountManager() *accounts.Manager {
	return b.etrue.AccountManager()
}

// SnailPoolContent returns snail pool content
func (b *TrueAPIBackend) SnailPoolContent() []*types.SnailBlock {
	return b.etrue.SnailPool().Content()
}

// SnailPoolInspect returns snail pool Inspect
func (b *TrueAPIBackend) SnailPoolInspect() []*types.SnailBlock {
	return b.etrue.SnailPool().Inspect()
}

// SnailPoolStats returns snail pool Stats
func (b *TrueAPIBackend) SnailPoolStats() (pending int, unVerified int) {
	return b.etrue.SnailPool().Stats()
}

// BloomStatus returns Bloom Status
func (b *TrueAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.etrue.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

// ServiceFilter make the Filter for the truechian
func (b *TrueAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.etrue.bloomRequests)
	}
}
