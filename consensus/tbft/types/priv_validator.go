package types

import (
	"bytes"
	//"fmt"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
)

// PrivValidator defines the functionality of a local Tendermint validator
// that signs votes, proposals, and heartbeats, and never double signs.
type PrivValidator interface {
	GetAddress() help.Address // redundant since .PubKey().Address()
	GetPubKey() crypto.PubKey

	SignVote(chainID string, vote *Vote) error
	SignProposal(chainID string, proposal *Proposal) error
	SignHeartbeat(chainID string, heartbeat *Heartbeat) error
}

//----------------------------------------
// Misc.

type PrivValidatorsByAddress []PrivValidator

func (pvs PrivValidatorsByAddress) Len() int {
	return len(pvs)
}

func (pvs PrivValidatorsByAddress) Less(i, j int) bool {
	return bytes.Compare(pvs[i].GetAddress(), pvs[j].GetAddress()) == -1
}

func (pvs PrivValidatorsByAddress) Swap(i, j int) {
	it := pvs[i]
	pvs[i] = pvs[j]
	pvs[j] = it
}

//----------------------------------------
type StateAgent interface {
	GetValidator() *ValidatorSet 
	GetLastValidator() *ValidatorSet

	GetLastBlockHeight() int64
	GetChainID() string
	LoadSeenCommit(height int64) *Commit
	MakeBlock() (*ctypes.Block,*PartSet)
	ValidateBlock(block *ctypes.Block) error 
	ConsensusCommit(block *ctypes.Block) error 
}
