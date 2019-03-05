package types

import (
	"encoding/json"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/core/types"
	"time"
)

//-----------------------------------------------------------------------------
// RoundStepType enum type

// RoundStepType enumerates the state of the consensus state machine
type RoundStepType uint8 // These must be numeric, ordered.

// RoundStepType
const (
	RoundStepNewHeight     = RoundStepType(0x01) // Wait til CommitTime + timeoutCommit
	RoundStepNewRound      = RoundStepType(0x02) // Setup new round and go to RoundStepPropose
	RoundStepPropose       = RoundStepType(0x03) // Did propose, gossip proposal
	RoundStepPrevote       = RoundStepType(0x04) // Did prevote, gossip prevotes
	RoundStepPrevoteWait   = RoundStepType(0x05) // Did receive any +2/3 prevotes, start timeout
	RoundStepPrecommit     = RoundStepType(0x06) // Did precommit, gossip precommits
	RoundStepPrecommitWait = RoundStepType(0x07) // Did receive any +2/3 precommits, start timeout
	RoundStepCommit        = RoundStepType(0x08) // Entered commit state machine
	// NOTE: RoundStepNewHeight acts as RoundStepCommitWait.
	RoundStepBlockSync = RoundStepType(0xee) // Entered commit state machine
)

// String returns a string
func (rs RoundStepType) String() string {
	switch rs {
	case RoundStepNewHeight:
		return "RoundStepNewHeight"
	case RoundStepNewRound:
		return "RoundStepNewRound"
	case RoundStepPropose:
		return "RoundStepPropose"
	case RoundStepPrevote:
		return "RoundStepPrevote"
	case RoundStepPrevoteWait:
		return "RoundStepPrevoteWait"
	case RoundStepPrecommit:
		return "RoundStepPrecommit"
	case RoundStepPrecommitWait:
		return "RoundStepPrecommitWait"
	case RoundStepCommit:
		return "RoundStepCommit"
	case RoundStepBlockSync:
		return "RoundStepBlockSync"
	default:
		return "RoundStepUnknown" // Cannot panic.
	}
}

//-----------------------------------------------------------------------------

// RoundState defines the internal consensus state.
// NOTE: Not thread safe. Should only be manipulated by functions downstream
// of the cs.receiveRoutine
type RoundState struct {
	Height             uint64         `json:"height"` // Height we are working on
	Round              uint           `json:"round"`
	Step               RoundStepType  `json:"step"`
	StartTime          time.Time      `json:"start_time"`
	CommitTime         time.Time      `json:"commit_time"` // Subjective time when +2/3 precommits for Block at Round were found
	Validators         *ValidatorSet  `json:"validators"`
	Proposal           *Proposal      `json:"proposal"`
	ProposalBlock      *types.Block   `json:"proposal_block"`
	ProposalBlockParts *PartSet       `json:"proposal_block_parts"`
	LockedRound        uint           `json:"locked_round"`
	LockedBlock        *types.Block   `json:"locked_block"`
	LockedBlockParts   *PartSet       `json:"locked_block_parts"`
	ValidRound         uint           `json:"valid_round"`       // Last known round with POL for non-nil valid block.
	ValidBlock         *types.Block   `json:"valid_block"`       // Last known block of POL mentioned above.
	ValidBlockParts    *PartSet       `json:"valid_block_parts"` // Last known block parts of POL metnioned above.
	Votes              *HeightVoteSet `json:"votes"`
	CommitRound        uint           `json:"commit_round"` //
	LastCommit         *VoteSet       `json:"last_commit"`  // Last precommits at Height-1
}

//RoundStateSimple  Compressed version of the RoundState for use in RPC
type RoundStateSimple struct {
	HeightRoundStep   string          `json:"height/round/step"`
	StartTime         time.Time       `json:"start_time"`
	ProposalBlockHash help.HexBytes   `json:"proposal_block_hash"`
	LockedBlockHash   help.HexBytes   `json:"locked_block_hash"`
	ValidBlockHash    help.HexBytes   `json:"valid_block_hash"`
	Votes             json.RawMessage `json:"height_vote_set"`
}

// RoundStateSimple Compress the RoundState to RoundStateSimple
func (rs *RoundState) RoundStateSimple() RoundStateSimple {
	votesJSON, err := rs.Votes.MarshalJSON()
	if err != nil {
		panic(err)
	}

	tmpPro := rs.ProposalBlock.Hash()
	tmpLock := rs.LockedBlock.Hash()
	tmpValid := rs.ValidBlock.Hash()
	return RoundStateSimple{
		HeightRoundStep:   fmt.Sprintf("%d/%d/%d", rs.Height, rs.Round, rs.Step),
		StartTime:         rs.StartTime,
		ProposalBlockHash: tmpPro[:],
		LockedBlockHash:   tmpLock[:],
		ValidBlockHash:    tmpValid[:],
		Votes:             votesJSON,
	}
}

// RoundStateEvent returns the H/R/S of the RoundState as an event.
func (rs *RoundState) RoundStateEvent() EventDataRoundState {
	// XXX: copy the RoundState
	// if we want to avoid this, we may need synchronous events after all
	rsCopy := *rs
	edrs := EventDataRoundState{
		Height:     rs.Height,
		Round:      uint(rs.Round),
		Step:       rs.Step.String(),
		RoundState: &rsCopy,
	}
	return edrs
}

// String returns a string
func (rs *RoundState) String() string {
	return rs.StringIndented("")
}

// StringIndented returns a string
func (rs *RoundState) StringIndented(indent string) string {
	return fmt.Sprintf(`RoundState{
%s  H:%v R:%v S:%v
%s  StartTime:     %v
%s  CommitTime:    %v
%s  Validators:    %v
%s  Proposal:      %v
%s  ProposalBlock: %v %v
%s  LockedRound:   %v
%s  LockedBlock:   %v %v
%s  ValidRound:   %v
%s  ValidBlock:   %v %v
%s  Votes:         %v
%s  LastCommit:    %v
%s}`,
		indent, rs.Height, rs.Round, rs.Step,
		indent, rs.StartTime,
		indent, rs.CommitTime,
		indent, rs.Validators.StringIndented(indent+"  "),
		indent, rs.Proposal,
		indent, rs.ProposalBlockParts.StringShort(), rs.ProposalBlock.Hash(),
		indent, rs.LockedRound,
		indent, rs.LockedBlockParts.StringShort(), rs.LockedBlock.Hash(),
		indent, rs.ValidRound,
		indent, rs.ValidBlockParts.StringShort(), rs.ValidBlock.Hash(),
		indent, rs.Votes.StringIndented(indent+"  "),
		indent, rs.LastCommit.StringShort(),
		indent)
}

// StringShort returns a string
func (rs *RoundState) StringShort() string {
	return fmt.Sprintf(`RoundState{H:%v R:%v S:%v ST:%v}`,
		rs.Height, rs.Round, rs.Step, rs.StartTime)
}
