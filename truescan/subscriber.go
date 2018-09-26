package truescan

import (
	"errors"
	"fmt"

	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/event"
)

const (
	// txsChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	addTxChanSize       = 4096
	removeTxChanSize    = 2048
	fastBlockChanSize   = 2048
	snailChainHeadSize  = 2048
	electionChanSize    = 2048
	stateChangeChanSize = 2048
)

var (
	errTerminated = errors.New("terminated")
)

func errResp(format string) error {
	return fmt.Errorf("%v", format)
}

// Subscriber contains all the events that the redis service needs to subscribe to.
type Subscriber interface {
	// SubscribeAddTxEvent should return an event subscription of
	// AddTxEvent and send events to the given channel.
	SubscribeAddTxEvent(chan<- core.AddTxEvent) event.Subscription
	// SubscribeRemoveTxEvent should return an event subscription of
	// RemoveTxEvent and send events to the given channel.
	SubscribeRemoveTxEvent(chan<- core.RemoveTxEvent) event.Subscription

	SubscribeFastBlock(chan<- core.FastBlockEvent) event.Subscription

	// SubscribeSnailChainHeadEvent should return an event subscription of
	// ChainHeadEvent and send events to the given channel.
	SubscribeSnailChainHeadEvent(chan<- snailchain.ChainHeadEvent) event.Subscription

	SubscribeElectionEvent(chan<- core.ElectionEvent) event.Subscription

	SubscribeStateChangeEvent(chan<- core.StateChangeEvent) event.Subscription
}
