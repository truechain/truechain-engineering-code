package types

import (
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"time"
)

// Canonical json is amino's json for structs with fields in alphabetical order

// TimeFormat is used for generating the sigs
const TimeFormat = time.RFC3339Nano

type CanonicalJSONBlockID struct {
	Hash        help.HexBytes              `json:"hash,omitempty"`
	PartsHeader CanonicalJSONPartSetHeader `json:"parts,omitempty"`
}

type CanonicalJSONPartSetHeader struct {
	Hash  help.HexBytes `json:"hash,omitempty"`
	Total uint          `json:"total,omitempty"`
}

type CanonicalJSONProposal struct {
	ChainID          string                     `json:"@chain_id"`
	Type             string                     `json:"@type"`
	BlockPartsHeader CanonicalJSONPartSetHeader `json:"block_parts_header"`
	Height           uint64                     `json:"height"`
	POLBlockID       CanonicalJSONBlockID       `json:"pol_block_id"`
	POLRound         uint                       `json:"pol_round"`
	Round            uint                       `json:"round"`
	Timestamp        string                     `json:"timestamp"`
}

type CanonicalJSONVote struct {
	ChainID   string               `json:"@chain_id"`
	Type      string               `json:"@type"`
	BlockID   CanonicalJSONBlockID `json:"block_id"`
	Height    uint64               `json:"height"`
	Round     uint                 `json:"round"`
	Timestamp string               `json:"timestamp"`
	VoteType  byte                 `json:"type"`
	// Result	  uint				   `json:"result"`	
	// ResSign	  []byte			   `json:"result_sign"`
}

type CanonicalJSONHeartbeat struct {
	ChainID          string       `json:"@chain_id"`
	Type             string       `json:"@type"`
	Height           uint64       `json:"height"`
	Round            uint         `json:"round"`
	Sequence         uint         `json:"sequence"`
	ValidatorAddress help.Address `json:"validator_address"`
	ValidatorIndex   uint         `json:"validator_index"`
}

//-----------------------------------
// Canonicalize the structs

func CanonicalBlockID(blockID BlockID) CanonicalJSONBlockID {
	return CanonicalJSONBlockID{
		Hash:        blockID.Hash,
		PartsHeader: CanonicalPartSetHeader(blockID.PartsHeader),
	}
}

func CanonicalPartSetHeader(psh PartSetHeader) CanonicalJSONPartSetHeader {
	return CanonicalJSONPartSetHeader{
		psh.Hash,
		psh.Total,
	}
}

func CanonicalProposal(chainID string, proposal *Proposal) CanonicalJSONProposal {
	return CanonicalJSONProposal{
		ChainID:          chainID,
		Type:             "proposal",
		BlockPartsHeader: CanonicalPartSetHeader(proposal.BlockPartsHeader),
		Height:           proposal.Height,
		Timestamp:        CanonicalTime(proposal.Timestamp),
		POLBlockID:       CanonicalBlockID(proposal.POLBlockID),
		POLRound:         proposal.POLRound,
		Round:            proposal.Round,
	}
}

func CanonicalVote(chainID string, vote *Vote) CanonicalJSONVote {
	return CanonicalJSONVote{
		ChainID:   chainID,
		Type:      "vote",
		BlockID:   CanonicalBlockID(vote.BlockID),
		Height:    vote.Height,
		Round:     vote.Round,
		Timestamp: CanonicalTime(vote.Timestamp),
		VoteType:  vote.Type,
		// Result:	   vote.Result,
		// ResSign:   vote.ResultSign,
	}
}

func CanonicalTime(t time.Time) string {
	// Note that sending time over amino resets it to
	// local time, we need to force UTC here, so the
	// signatures match
	return t.UTC().Format(TimeFormat)
}
