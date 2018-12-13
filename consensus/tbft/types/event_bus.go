package types

import (
	//"context"
	//"fmt"
	"errors"

	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/event"
)

const defaultCapacity = 0

// type EventBusSubscriber interface {
// 	Subscribe(ctx context.Context, subscriber string, query tmpubsub.Query, out chan<- interface{}) error
// 	Unsubscribe(ctx context.Context, subscriber string, query tmpubsub.Query) error
// 	UnsubscribeAll(ctx context.Context, subscriber string) error
// }

// EventBus is a common bus for all events going through the system. All calls
// are proxied to underlying pubsub server. All events must be published using
// EventBus to ensure correct data types.
type EventBus struct {
	help.BaseService
	scope event.SubscriptionScope
	subs  map[string]*event.Feed
}

// NewEventBus returns a new event bus.
func NewEventBus() *EventBus {
	b := &EventBus{subs: make(map[string]*event.Feed)}
	b.BaseService = *help.NewBaseService("EventBus", b)
	return b
}

// func (b *EventBus) SetLogger(l log.Logger) {
// 	b.BaseService.SetLogger(l)
// 	b.pubsub.SetLogger(l.With("module", "pubsub"))
// }

// OnStart  eventBus start
func (b *EventBus) OnStart() error {
	return nil
}

//OnStop eventBus stop
func (b *EventBus) OnStop() {
	b.scope.Close()
}

//Subscribe chan<- interface{}
func (b *EventBus) Subscribe(key string, out chan<- interface{}) event.Subscription {
	if _, ok := b.subs[key]; !ok {
		var feed event.Feed
		s := b.scope.Track(feed.Subscribe(out))
		b.subs[key] = &feed
		return s
	}
	return nil
}

// func (b *EventBus) Unsubscribe(ctx context.Context, subscriber string, query tmpubsub.Query) error {

// 	return b.pubsub.Unsubscribe(ctx, subscriber, query)
// }

// func (b *EventBus) UnsubscribeAll(ctx context.Context, subscriber string) error {
// 	return b.pubsub.UnsubscribeAll(ctx, subscriber)
// }

// func (b *EventBus) Publish(eventType string, eventData TMEventData) error {
// 	// no explicit deadline for publishing events
// 	ctx := context.Background()
// 	b.pubsub.PublishWithTags(ctx, eventData, tmpubsub.NewTagMap(map[string]string{EventTypeKey: eventType}))
// 	return nil
// }

//--- block, tx, and vote events

// func (b *EventBus) PublishEventNewBlock(event EventDataNewBlock) error {
// 	if v,ok := b.subs[EventNewBlock];ok {
// 		v.Send(event)
// 		return nil
// 	}
// 	return errors.New(EventMsgNotFound)
// }

// func (b *EventBus) PublishEventNewBlockHeader(event EventDataNewBlockHeader) error {
// 	if v,ok := b.subs[EventNewBlockHeader];ok {
// 		v.Send(event)
// 		return nil
// 	}
// 	return errors.New(EventMsgNotFound)
// }

//PublishEventVote send event data common EventVote
func (b *EventBus) PublishEventVote(event EventDataVote) error {
	if v, ok := b.subs[EventVote]; ok {
		v.Send(event)
		return nil
	}
	return errors.New(EventMsgNotFound)
}

// PublishEventTx publishes tx event with tags from Result. Note it will add
// predefined tags (EventTypeKey, TxHashKey). Existing tags with the same names
// will be overwritten.
// func (b *EventBus) PublishEventTx(event EventDataTx) error {
// 	// no explicit deadline for publishing events
// 	ctx := context.Background()

// 	tags := make(map[string]string)

// 	// validate and fill tags from tx result
// 	for _, tag := range event.Result.Tags {
// 		// basic validation
// 		if len(tag.Key) == 0 {
// 			b.Logger.Info("Got tag with an empty key (skipping)", "tag", tag, "tx", event.Tx)
// 			continue
// 		}
// 		tags[string(tag.Key)] = string(tag.Value)
// 	}

// 	// add predefined tags
// 	logIfTagExists(EventTypeKey, tags, b.Logger)
// 	tags[EventTypeKey] = EventTx

// 	logIfTagExists(TxHashKey, tags, b.Logger)
// 	tags[TxHashKey] = fmt.Sprintf("%X", event.Tx.Hash())

// 	logIfTagExists(TxHeightKey, tags, b.Logger)
// 	tags[TxHeightKey] = fmt.Sprintf("%d", event.Height)

// 	b.pubsub.PublishWithTags(ctx, event, tmpubsub.NewTagMap(tags))
// 	return nil
// }

//PublishEventProposalHeartbeat send event data common EventProposalHeartbeat
func (b *EventBus) PublishEventProposalHeartbeat(event EventDataProposalHeartbeat) error {
	if v, ok := b.subs[EventProposalHeartbeat]; ok {
		v.Send(event)
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//--- EventDataRoundState events

//PublishEventNewRoundStep send event data common EventNewRoundStep
func (b *EventBus) PublishEventNewRoundStep(event EventDataRoundState) error {
	if v, ok := b.subs[EventNewRoundStep]; ok {
		v.Send(EventDataCommon{
			Key:  EventNewRoundStep,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventTimeoutPropose send event data common EventTimeoutPropose
func (b *EventBus) PublishEventTimeoutPropose(event EventDataRoundState) error {
	if v, ok := b.subs[EventTimeoutPropose]; ok {
		v.Send(EventDataCommon{
			Key:  EventTimeoutPropose,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventTimeoutWait send event data common EventTimeoutWait
func (b *EventBus) PublishEventTimeoutWait(event EventDataRoundState) error {
	if v, ok := b.subs[EventTimeoutWait]; ok {
		v.Send(EventDataCommon{
			Key:  EventTimeoutWait,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventNewRound send event data common EventNewRound
func (b *EventBus) PublishEventNewRound(event EventDataRoundState) error {
	if v, ok := b.subs[EventNewRound]; ok {
		v.Send(EventDataCommon{
			Key:  EventNewRound,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventCompleteProposal send event data common EventCompleteProposal
func (b *EventBus) PublishEventCompleteProposal(event EventDataRoundState) error {
	if v, ok := b.subs[EventCompleteProposal]; ok {
		v.Send(EventDataCommon{
			Key:  EventCompleteProposal,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventPolka send event data common EventPolka
func (b *EventBus) PublishEventPolka(event EventDataRoundState) error {
	if v, ok := b.subs[EventPolka]; ok {
		v.Send(EventDataCommon{
			Key:  EventPolka,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventUnlock send event data common unlock
func (b *EventBus) PublishEventUnlock(event EventDataRoundState) error {
	if v, ok := b.subs[EventUnlock]; ok {
		v.Send(EventDataCommon{
			Key:  EventUnlock,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventRelock send event data common relock
func (b *EventBus) PublishEventRelock(event EventDataRoundState) error {
	if v, ok := b.subs[EventRelock]; ok {
		v.Send(EventDataCommon{
			Key:  EventRelock,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

//PublishEventLock send event data common
func (b *EventBus) PublishEventLock(event EventDataRoundState) error {
	if v, ok := b.subs[EventLock]; ok {
		v.Send(EventDataCommon{
			Key:  EventLock,
			Data: event,
		})
		return nil
	}
	return errors.New(EventMsgNotFound)
}

func logIfTagExists(tag string, tags map[string]string, logger log.Logger) {
	if value, ok := tags[tag]; ok {
		logger.Error("Found predefined tag (value will be overwritten)", "tag", tag, "value", value)
	}
}
