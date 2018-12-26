package types

// import (
// 	"fmt"
// )

// Reserved event types
const (
	EventCompleteProposal  = "CompleteProposal"
	EventLock              = "Lock"
	EventNewBlock          = "NewBlock"
	EventNewBlockHeader    = "NewBlockHeader"
	EventNewRound          = "NewRound"
	EventNewRoundStep      = "NewRoundStep"
	EventPolka             = "Polka"
	EventRelock            = "Relock"
	EventTimeoutPropose    = "TimeoutPropose"
	EventTimeoutWait       = "TimeoutWait"
	EventTx                = "Tx"
	EventUnlock            = "Unlock"
	EventVote              = "Vote"
	EventMsgNotFound       = "MessageUnsubscribe"
)

//EventDataRoundState NOTE: This goes into the replay WAL
type EventDataRoundState struct {
	Height uint64 `json:"height"`
	Round  uint   `json:"round"`
	Step   string `json:"step"`

	// private, not exposed to websockets
	RoundState interface{} `json:"-"`
}

//EventDataCommon struct
type EventDataCommon struct {
	Key  string              `json:"key"`
	Data EventDataRoundState `json:"data"`
}

//EventDataVote event vote
type EventDataVote struct {
	Vote *Vote
}

//EventDataProposalHeartbeat event heartbeat
type EventDataProposalHeartbeat struct {
	Heartbeat *Heartbeat
}
