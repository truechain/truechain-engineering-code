package truescan

import (
	"errors"
	"fmt"

	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
)

const (
	// txsChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txsChanSize         = 4096
	removeTxChanSize    = 64
	receiptsChanSize    = 16
	chainHeadChanSize   = 16
	fruitChanSize       = 256
	snailChainHeadSize  = 64
	electionChanSize    = 64
	stateChangeChanSize = 64
)

var (
	errTerminated = errors.New("terminated")
)

func errResp(format string) error {
	return fmt.Errorf("%v", format)
}

// Subscriber contains all the events that the redis service needs to subscribe to.
type Subscriber interface {
	// SubscribeNewTxsEvent should return an event subscription of
	// NewTxsEvent and send events to the given channel.
	SubscribeNewTxsEvent(chan<- core.NewTxsEvent) event.Subscription
	SubscribeRemoveTxEvent(chan<- core.RemoveTxEvent) event.Subscription

	// SubscribeChainHeadEvent should return an event subscription of
	// ChainHeadEvent and send events to the given channel.
	SubscribeChainHeadEvent(chan<- core.ChainHeadEvent) event.Subscription

	// SubscribeNewFruitEvent should return an event subscription of
	// NewFruitsEvent and send events to the given channel.
	SubscribeNewFruitEvent(chan<- snailchain.NewFruitsEvent) event.Subscription

	// SubscribeSnailChainHeadEvent should return an event subscription of
	// ChainHeadEvent and send events to the given channel.
	SubscribeSnailChainHeadEvent(chan<- snailchain.ChainHeadEvent) event.Subscription

	SubscribeElectionEvent(chan<- core.ElectionEvent) event.Subscription

	SubscribeStateChangeEvent(chan<- core.StateChangeEvent) event.Subscription

	SubscribeReceiptsEvent(chan<- types.Receipts) event.Subscription
}
