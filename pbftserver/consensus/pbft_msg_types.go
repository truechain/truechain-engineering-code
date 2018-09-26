package consensus

import "github.com/truechain/truechain-engineering-code/core/types"

type RequestMsg struct {
	Timestamp  int64  `json:"timestamp"`
	ClientID   string `json:"clientID"`
	Operation  string `json:"operation"`
	SequenceID int64  `json:"sequenceID"`
	Height     int64  `json:"Height"`
}

type ReplyMsg struct {
	ViewID    int64  `json:"viewID"`
	Timestamp int64  `json:"timestamp"`
	ClientID  string `json:"clientID"`
	NodeID    string `json:"nodeID"`
	Result    string `json:"result"`
	Height    int64  `json:"Height"`
}

type PrePrepareMsg struct {
	ViewID     int64       `json:"viewID"`
	SequenceID int64       `json:"sequenceID"`
	Digest     string      `json:"digest"`
	RequestMsg *RequestMsg `json:"requestMsg"`
	Height     int64       `json:"Height"`
}

type VoteMsg struct {
	ViewID     int64           `json:"viewID"`
	SequenceID int64           `json:"sequenceID"`
	Digest     string          `json:"digest"`
	NodeID     string          `json:"nodeID"`
	Pass       *SignedVoteMsg  `json:"Pass"`
	Height     int64           `json:"Height"`
	Signs      *types.PbftSign `json:"signs"`
	MsgType    `json:"msgType"`
}

type MsgType int

const (
	PrepareMsg MsgType = iota
	CommitMsg
)

type StorgePrepareMsg struct {
	ViewID     int64  `json:"viewID"`
	SequenceID int64  `json:"sequenceID"`
	Digest     string `json:"digest"`
	NodeID     string `json:"nodeID"`
	Height     int64  `json:"Height"`
	MsgType    `json:"msgType"`
}
