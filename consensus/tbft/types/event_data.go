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
	EventProposalHeartbeat = "ProposalHeartbeat"
	EventMsgNotFound	   = "MessageUnsubscribe"
)

// NOTE: This goes into the replay WAL
type EventDataRoundState struct {
	Height int64  				`json:"height"`
	Round  int    				`json:"round"`
	Step   string 				`json:"step"`

	// private, not exposed to websockets
	RoundState interface{} 		`json:"-"`
}
type EventDataCommon struct {
	Key 	string					 `json:"key"`
	Data	EventDataRoundState		 `json:"data"`
}

type EventDataVote struct {
	Vote *Vote
}

type EventDataProposalHeartbeat struct {
	Heartbeat *Heartbeat
}