package truescan

import (
	"encoding/hex"
	"fmt"

	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
)

type TrueScan struct {
	sub               Subscriber
	txsCh             chan core.NewTxsEvent
	txsSub            event.Subscription
	removeTxCh        chan core.RemoveTxEvent
	removeTxSub       event.Subscription
	receiptsCh        chan types.Receipts
	receiptsSub       event.Subscription
	chainHeadCh       chan core.ChainHeadEvent
	chainHeadSub      event.Subscription
	fruitsch          chan snailchain.NewFruitsEvent //fruit
	fruitsSub         event.Subscription             // for fruit pool
	snailChainHeadCh  chan snailchain.ChainHeadEvent
	snailChainHeadSub event.Subscription
	electionCh        chan core.ElectionEvent
	electionSub       event.Subscription
	stateChangeCh     chan core.StateChangeEvent
	stateChangeSub    event.Subscription
	redisClient       *RedisClient
	quit              chan struct{}
}

// New function return a TrueScan message processing client
func New(sub Subscriber) *TrueScan {
	ts := &TrueScan{
		sub:              sub,
		txsCh:            make(chan core.NewTxsEvent, txsChanSize),
		removeTxCh:       make(chan core.RemoveTxEvent, removeTxChanSize),
		receiptsCh:       make(chan types.Receipts, receiptsChanSize),
		chainHeadCh:      make(chan core.ChainHeadEvent, chainHeadChanSize),
		fruitsch:         make(chan snailchain.NewFruitsEvent, fruitChanSize),
		snailChainHeadCh: make(chan snailchain.ChainHeadEvent, snailChainHeadSize),
		electionCh:       make(chan core.ElectionEvent, electionChanSize),
		stateChangeCh:    make(chan core.StateChangeEvent, stateChangeChanSize),
		quit:             make(chan struct{}),
	}
	rc, err := NewRedisClient("39.105.126.32:6379", 1)
	if err != nil {
		return nil
	}
	ts.redisClient = rc
	return ts
}

// Start TrueScan message processing client
func (ts *TrueScan) Start() {
	ts.redisClient.Start()

	// broadcast transactions
	ts.txsSub = ts.sub.SubscribeNewTxsEvent(ts.txsCh)
	go ts.txHandleLoop()

	ts.removeTxSub = ts.sub.SubscribeRemoveTxEvent(ts.removeTxCh)
	go ts.removeTxHandleLoop()

	ts.receiptsSub = ts.sub.SubscribeReceiptsEvent(ts.receiptsCh)
	go ts.receiptsHandleLoop()

	// Subscribe events from blockchain
	ts.chainHeadSub = ts.sub.SubscribeChainHeadEvent(ts.chainHeadCh)
	go ts.fastChainHandleLoop()

	ts.fruitsSub = ts.sub.SubscribeNewFruitEvent(ts.fruitsch)
	go ts.fruitHandleLoop()

	ts.snailChainHeadSub = ts.sub.SubscribeSnailChainHeadEvent(ts.snailChainHeadCh)
	go ts.snailChainHandleLoop()

	ts.electionSub = ts.sub.SubscribeElectionEvent(ts.electionCh)
	go ts.electionHandleLoop()

	ts.stateChangeSub = ts.sub.SubscribeStateChangeEvent(ts.stateChangeCh)
	go ts.stateChangeHandleLoop()

	go ts.loop()
}

func (ts *TrueScan) txHandleLoop() error {
	for {
		select {
		case event := <-ts.txsCh:
			ts.handleTx(event.Txs)
		case <-ts.txsSub.Err():
			return errResp("tx terminated")
		}
	}
}

func (ts *TrueScan) handleTx(txs []*types.Transaction) {
	for _, tx := range txs {
		from, err := types.NewEIP155Signer(tx.ChainId()).Sender(tx)
		if err != nil {
			continue
		}
		var toHex string
		if to := tx.To(); to == nil {
			toHex = ""
		} else {
			toHex = to.String()
		}
		tm := &TransactionMsg{
			Nonce:    tx.Nonce(),
			Hash:     tx.Hash().String(),
			From:     from.String(),
			To:       toHex,
			Value:    "0x" + hex.EncodeToString(tx.Value().Bytes()),
			Gas:      tx.Gas(),
			GasPrice: "0x" + hex.EncodeToString(tx.GasPrice().Bytes()),
			Input:    "0x" + hex.EncodeToString(tx.Data()),
		}
		ts.redisClient.PendingTransaction(tm)
	}
}

func (ts *TrueScan) removeTxHandleLoop() error {
	for {
		select {
		case removeTxEvent := <-ts.removeTxCh:
			ts.handleRemoveTx(removeTxEvent)
		case <-ts.removeTxSub.Err():
			return errResp("removeTx terminated")
		}
	}
}

func (ts *TrueScan) handleRemoveTx(rte core.RemoveTxEvent) {
	rtm := &RemoveTxMsg{
		Hash: rte.Hash.String(),
	}
	ts.redisClient.RemoveTransaction(rtm)
}

func (ts *TrueScan) electionHandleLoop() error {
	for {
		select {
		case electionEvent := <-ts.electionCh:
			if electionEvent.Option == types.CommitteeStart {
				ts.handleElection(&electionEvent)
			}
		case <-ts.electionSub.Err():
			return errResp("election terminated")
		}
	}
}
func (ts *TrueScan) handleElection(ee *core.ElectionEvent) {
	bfn := ee.BeginFastNumber
	if bfn == nil {
		return
	}
	members := ee.CommitteeMembers
	mas := make([]string, len(members))
	for i, member := range members {
		mas[i] = member.Coinbase.String()
	}
	cvm := &ChangeViewMsg{
		ViewNumber:      ee.CommitteeID.Uint64(),
		Members:         mas,
		BeginFastNumber: bfn.Uint64(),
	}
	ts.redisClient.ChangeView(cvm)
}

func (ts *TrueScan) stateChangeHandleLoop() error {
	for {
		select {
		case stateChangeEvent := <-ts.stateChangeCh:
			ts.handleStateChange(stateChangeEvent)
		case <-ts.stateChangeSub.Err():
			return errResp("state terminated")
		}
	}
}

func (ts *TrueScan) handleStateChange(bsd core.StateChangeEvent) {
	balances := make([]*Account, len(bsd.Balances))
	rewards := make([]*Account, len(bsd.Rewards))
	for i, b := range bsd.Balances {
		balances[i] = &Account{
			Address: b.Address.String(),
			Value:   "0x" + hex.EncodeToString(b.Balance.Bytes()),
		}
	}
	for i, r := range bsd.Rewards {
		rewards[i] = &Account{
			Address: r.Address.String(),
			Value:   "0x" + hex.EncodeToString(r.Balance.Bytes()),
		}
	}
	scm := &StateChangeMsg{
		Balances: balances,
		Rewards:  rewards,
	}
	ts.redisClient.StateChange(scm)
}

func (ts *TrueScan) snailChainHandleLoop() error {
	for {
		select {
		case snailChainEvent := <-ts.snailChainHeadCh:
			ts.handleSnailChain(snailChainEvent.Block)
		case <-ts.fruitsSub.Err():
			return errResp("fruit terminated")
		}
	}
}

func (ts *TrueScan) handleSnailChain(block *types.SnailBlock) {
	sbm := &SnailBlockHeaderMsg{
		Number:     block.NumberU64(),
		Hash:       block.Hash().String(),
		ParentHash: block.ParentHash().String(),
		Nonce:      block.Nonce(),
		Miner:      block.Coinbase().String(),
		Difficulty: block.Difficulty().Uint64(),
		ExtraData:  "0x" + hex.EncodeToString(block.Extra()),
		Size:       block.Size().Int(),
		Timestamp:  block.Time().Uint64(),
	}
	fruits := block.Fruits()
	fms := make([]*FruitHeaderMsg, len(fruits))
	for i, fruit := range fruits {
		fm := &FruitHeaderMsg{
			Number:     fruit.FastNumber().Uint64(),
			Hash:       fruit.FruitsHash().String(),
			Nonce:      fruit.Nonce(),
			Miner:      fruit.Coinbase().String(),
			Difficulty: fruit.Difficulty().Uint64(),
		}
		fms[i] = fm
	}
	sbm.StartFruitNumber = fms[0].Number
	sbm.EndFruitNumber = fms[len(fms)-1].Number
	sbm.Fruits = fms
	ts.redisClient.NewSnailBlockHeader(sbm)
}

func (ts *TrueScan) fruitHandleLoop() error {
	for {
		select {
		case fruitsEvent := <-ts.fruitsch:
			ts.handleFruits(fruitsEvent.Fruits)
		case <-ts.fruitsSub.Err():
			return errResp("fruit terminated")
		}
	}
}

func (ts *TrueScan) handleFruits(fruits []*types.SnailBlock) {
	fmt.Println("-----------------")
}

func (ts *TrueScan) fastChainHandleLoop() error {
	for {
		select {
		case ev := <-ts.chainHeadCh:
			ts.handleFastChain(ev.Block)
		case <-ts.chainHeadSub.Err():
			return errResp("chain terminated")
		}
	}
}

func (ts *TrueScan) handleFastChain(block *types.Block) {
	fbm := &FastBlockHeaderMsg{
		Number:     block.NumberU64(),
		Hash:       block.Hash().String(),
		ParentHash: block.ParentHash().String(),
		ExtraData:  "0x" + hex.EncodeToString(block.Extra()),
		Size:       block.Size().Int(),
		GasLimit:   block.GasLimit(),
		GasUsed:    block.GasUsed(),
		Timestamp:  block.Time().Uint64(),
	}
	txs := block.Transactions()
	tms := make([]*TransactionMsg, len(txs))
	for i, tx := range txs {
		from, err := types.NewEIP155Signer(tx.ChainId()).Sender(tx)
		if err != nil {
			continue
		}
		var toHex string
		if to := tx.To(); to == nil {
			toHex = ""
		} else {
			toHex = to.String()
		}
		tm := &TransactionMsg{
			Nonce:    tx.Nonce(),
			Hash:     tx.Hash().String(),
			From:     from.String(),
			To:       toHex,
			Value:    "0x" + hex.EncodeToString(tx.Value().Bytes()),
			Gas:      tx.Gas(),
			GasPrice: "0x" + hex.EncodeToString(tx.GasPrice().Bytes()),
			Input:    "0x" + hex.EncodeToString(tx.Data()),
		}
		tms[i] = tm
	}
	fbm.Txs = tms
	ts.redisClient.NewFastBlockHeader(fbm)
}

func (ts *TrueScan) receiptsHandleLoop() error {
	for {
		select {
		case receipts := <-ts.receiptsCh:
			ts.handleReceipts(receipts)
		case <-ts.receiptsSub.Err():
			return errResp("receipts terminated")
		}
	}
}

func (ts *TrueScan) handleReceipts(receipts types.Receipts) {
	ts.redisClient.TransactionReceipts(receipts)
}

func (ts *TrueScan) loop() error {
	for {
		select {
		case <-ts.quit:
			// TrueScan terminating, abort all operations
			return errTerminated
		}
	}
}

// Stop TrueScan message processing client
func (ts *TrueScan) Stop() {
	ts.txsSub.Unsubscribe()
	ts.chainHeadSub.Unsubscribe()
	ts.fruitsSub.Unsubscribe()
	ts.snailChainHeadSub.Unsubscribe()
	ts.electionSub.Unsubscribe()
	close(ts.quit)
}
