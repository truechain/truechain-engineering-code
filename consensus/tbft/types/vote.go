package types

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"time"
)

var (
	ErrVoteUnexpectedStep            = errors.New("Unexpected step")
	ErrVoteInvalidValidatorIndex     = errors.New("Invalid validator index")
	ErrVoteInvalidValidatorAddress   = errors.New("Invalid validator address")
	ErrVoteInvalidSignature          = errors.New("Invalid signature")
	ErrVoteInvalidBlockHash          = errors.New("Invalid block hash")
	ErrVoteNonDeterministicSignature = errors.New("Non-deterministic signature")
	ErrVoteConflictingVotes          = errors.New("Conflicting votes from validator")
	ErrVoteNil                       = errors.New("Nil vote")
)

// Types of votes
// TODO Make a new type "VoteType"
const (
	VoteTypePrevote   = byte(0x01)
	VoteTypePrecommit = byte(0x02)
)

func IsVoteTypeValid(type_ byte) bool {
	switch type_ {
	case VoteTypePrevote:
		return true
	case VoteTypePrecommit:
		return true
	default:
		return false
	}
}

// Represents a prevote, precommit, or commit vote from validators for consensus.
type Vote struct {
	ValidatorAddress help.Address `json:"validator_address"`
	ValidatorIndex   uint         `json:"validator_index"`
	Height           uint64       `json:"height"`
	Round            uint         `json:"round"`
	Result			 uint		  `json:"result"`	
	Timestamp        time.Time    `json:"timestamp"`
	Type             byte         `json:"type"`
	BlockID          BlockID      `json:"block_id"` // zero if vote is nil.
	Signature        []byte       `json:"signature"`
	ResultSign       []byte       `json:"reuslt_signature"`
}

func (vote *Vote) SignBytes(chainID string) []byte {
	bz, err := cdc.MarshalJSON(CanonicalVote(chainID, vote))
	if err != nil {
		panic(err)
	}
	signBytes := help.RlpHash([]interface{}{bz,})
	return signBytes[:]
}

func (vote *Vote) Copy() *Vote {
	voteCopy := *vote
	return &voteCopy
}

func (vote *Vote) String() string {
	if vote == nil {
		return "nil-Vote"
	}
	var typeString string
	switch vote.Type {
	case VoteTypePrevote:
		typeString = "Prevote"
	case VoteTypePrecommit:
		typeString = "Precommit"
	default:
		help.PanicSanity("Unknown vote type")
	}

	return fmt.Sprintf("Vote{%v:%X %v/%02d/%d/%v(%v) H:%X S1:%X S2:%X @ %s}",
		vote.ValidatorIndex, help.Fingerprint(vote.ValidatorAddress),
		vote.Height, vote.Round, vote.Result,vote.Type, typeString,
		help.Fingerprint(vote.BlockID.Hash),
		help.Fingerprint(vote.Signature),
		help.Fingerprint(vote.ResultSign),
		CanonicalTime(vote.Timestamp))
}

func (vote *Vote) Verify(chainID string, pubKey crypto.PubKey) error {
	if !bytes.Equal(pubKey.Address(), vote.ValidatorAddress) {
		return ErrVoteInvalidValidatorAddress
	}

	if !pubKey.VerifyBytes(vote.SignBytes(chainID), vote.Signature) {
		return ErrVoteInvalidSignature
	}
	return nil
}
