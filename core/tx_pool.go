package core


import (
	"math/big"
	"sync"
	"time"
)

// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
// TxPool 包含了当前知的交易， 当前网络接收到交易，或者本地提交的交易会加入到TxPool。
// 当他们已经被添加到区块链的时候被移除。
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
// TxPool分为可执行的交易(可以应用到当前的状态)和未来的交易。 交易在这两种状态之间转换，
type TxPool struct {
	config       TxPoolConfig
	chainconfig  *params.ChainConfig
	chain        blockChain
	gasPrice     *big.Int             //最低的GasPrice限制
	txFeed       event.Feed	          //通过txFeed来订阅TxPool的消息
	scope        event.SubscriptionScope
	chainHeadCh  chan ChainHeadEvent  // 订阅了区块头的消息，当有了新的区块头生成的时候会在这里收到通知
	chainHeadSub event.Subscription   // 区块头消息的订阅器。
	signer       types.Signer		  // 封装了事务签名处理。
	mu           sync.RWMutex

	currentState  *state.StateDB      // Current state in the blockchain head
	pendingState  *state.ManagedState // Pending state tracking virtual nonces
	currentMaxGas *big.Int            // Current gas limit for transaction caps 目前交易上限的GasLimit

	locals  *accountSet // Set of local transaction to exepmt from evicion rules  本地交易免除驱逐规则
	journal *txJournal  // Journal of local transaction to back up to disk 本地交易会写入磁盘

	pending map[common.Address]*txList         // All currently processable transactions 所有当前可以处理的交易
	queue   map[common.Address]*txList         // Queued but non-processable transactions 当前还不能处理的交易
	beats   map[common.Address]time.Time       // Last heartbeat from each known account 每一个已知账号的最后一次心跳信息的时间
	all     map[common.Hash]*types.Transaction // All transactions to allow lookups 可以查找到所有交易
	priced  *txPricedList                      // All transactions sorted by price 按照价格排序的交易

	wg sync.WaitGroup // for shutdown sync

	homestead bool  // 家园版本
}
