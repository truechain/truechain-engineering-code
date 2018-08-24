package consensus

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
)

const (
	Agree int = iota
	Against
	ActionFecth
	ActionBroadcast
	ActionFinish
)

type ActionIn struct {
	AC     int
	ID     *big.Int
	Height *big.Int
}

// var ActionChan chan *ActionIn = make(chan *ActionIn)

type SignedVoteMsg struct {
	FastHeight *big.Int
	Result     uint   // 0--agree,1--against
	Sign       []byte // sign for fastblock height + hash + result + Pk
}

type ConsensusHelp interface {
	GetRequest(id *big.Int) (*RequestMsg, error)
	Broadcast(height *big.Int)
}
type ConsensusVerify interface {
	SignMsg(h int64, res uint) *SignedVoteMsg
	CheckMsg(msg *RequestMsg) bool
	ReplyResult(msg *RequestMsg, res uint) bool
	InsertBlock(msg *PrePrepareMsg) bool
}
type ConsensusFinish interface {
	ConsensusFinish()
}

func Hash(content []byte) string {
	h := sha256.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}
