package truescan

import (
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
)

type TrueScan struct {
	sub Subscriber

	txsCh  chan core.NewTxsEvent
	txsSub event.Subscription

	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription

	//fruit
	fruitsch  chan snailchain.NewFruitsEvent
	fruitsSub event.Subscription // for fruit pool

	snailChainHeadCh  chan snailchain.ChainHeadEvent
	snailChainHeadSub event.Subscription

	quit chan struct{}
}

func New(s *Subscriber) *TrueScan {
	return &TrueScan{
		txsCh:            make(chan core.NewTxsEvent, txChanSize),
		chainHeadCh:      make(chan core.ChainHeadEvent, chainHeadChanSize),
		fruitsch:         make(chan snailchain.NewFruitsEvent, fruitChanSize),
		snailChainHeadCh: make(chan snailchain.ChainHeadEvent, snailChainHeadSize),
		quit:             make(chan struct{}),
	}
}

func (ts *TrueScan) Start(maxPeers int) {
	// broadcast transactions
	ts.txsSub = ts.sub.SubscribeNewTxsEvent(ts.txsCh)
	go ts.txHandleLoop()
	// Subscribe events from blockchain
	ts.chainHeadSub = ts.sub.SubscribeChainHeadEvent(ts.chainHeadCh)
	go ts.fastChainHandleLoop()

	ts.fruitsSub = ts.sub.SubscribeNewFruitEvent(ts.fruitsch)
	go ts.fruitHandleLoop()

	ts.snailChainHeadSub = ts.sub.SubscribeSnailChainHeadEvent(ts.snailChainHeadCh)
	go ts.snailChainHandleLoop()

	go ts.loop()
}

func (ts *TrueScan) snailChainHandleLoop() error {
	for {
		select {
		case snailChainEvent := <-ts.snailChainHeadCh:
			ts.handleSnailChain(snailChainEvent.Block)

			// Err() channel will be closed when unsubscribing.
		case <-ts.fruitsSub.Err():
			return errResp("fruit terminated")
		}
	}
}

func (ts *TrueScan) handleSnailChain(block *types.SnailBlock) {
}

func (ts *TrueScan) fruitHandleLoop() error {
	for {
		select {
		case fruitsEvent := <-ts.fruitsch:
			ts.handleFruits(fruitsEvent.Fruits)

			// Err() channel will be closed when unsubscribing.
		case <-ts.fruitsSub.Err():
			return errResp("fruit terminated")
		}
	}
}

func (ts *TrueScan) handleFruits(fruits []*types.SnailBlock) {
}

func (ts *TrueScan) fastChainHandleLoop() error {
	for {
		select {
		// Handle ChainHeadEvent
		case ev := <-ts.chainHeadCh:
			ts.handleFastChain(ev.Block)

			// Be unsubscribed due to system stopped
		case <-ts.chainHeadSub.Err():
			return errResp("chain terminated")
		}
	}
}

func (ts *TrueScan) handleFastChain(txs *types.Block) {
}

func (ts *TrueScan) txHandleLoop() error {
	for {
		select {
		case event := <-ts.txsCh:
			ts.handleTx(event.Txs)

			// Err() channel will be closed when unsubscribing.
		case <-ts.txsSub.Err():
			return errResp("tx terminated")
		}
	}
}

func (ts *TrueScan) handleTx(txs []*types.Transaction) {
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

func (ts *TrueScan) Stop() {
	ts.txsSub.Unsubscribe()
	ts.chainHeadSub.Unsubscribe()
	ts.fruitsSub.Unsubscribe()
	close(ts.quit)
}
