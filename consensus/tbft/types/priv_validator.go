package types

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"fmt"
	tcrypto "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
	"sync"
	"time"
)

const (
	stepNone           uint8 = 0 // Used to distinguish the initial state
	stepPropose        uint8 = 1
	stepPrevote        uint8 = 2
	stepPrecommit      uint8 = 3
	BlockPartSizeBytes uint  = 65536 // 64kB,
)

func voteToStep(vote *Vote) uint8 {
	switch vote.Type {
	case VoteTypePrevote:
		return stepPrevote
	case VoteTypePrecommit:
		return stepPrecommit
	default:
		help.PanicSanity("Unknown vote type")
		return 0
	}
}

// PrivValidator defines the functionality of a local Tendermint validator
// that signs votes, proposals, and heartbeats, and never double signs.
type PrivValidator interface {
	GetAddress() help.Address // redundant since .PubKey().Address()
	GetPubKey() tcrypto.PubKey

	SignVote(chainID string, vote *Vote) error
	SignProposal(chainID string, proposal *Proposal) error
	SignHeartbeat(chainID string, heartbeat *Heartbeat) error
}

type privValidator struct {
	PrivKey       tcrypto.PrivKey
	LastHeight    uint64        `json:"last_height"`
	LastRound     uint          `json:"last_round"`
	LastStep      uint8         `json:"last_step"`
	LastSignature []byte        `json:"last_signature,omitempty"` // so we dont lose signatures XXX Why would we lose signatures?
	LastSignBytes help.HexBytes `json:"last_signbytes,omitempty"` // so we dont lose signatures XXX Why would we lose signatures?

	mtx sync.Mutex
}

func NewPrivValidator(priv ecdsa.PrivateKey) PrivValidator {
	return &privValidator{
		PrivKey:  tcrypto.PrivKeyTrue(priv),
		LastStep: stepNone,
	}
}

func (Validator *privValidator) Reset() {
	var sig []byte
	Validator.LastHeight = 0
	Validator.LastRound = 0
	Validator.LastStep = 0
	Validator.LastSignature = sig
	Validator.LastSignBytes = nil
}

// Persist height/round/step and signature
func (Validator *privValidator) saveSigned(height uint64, round int, step uint8,
	signBytes []byte, sig []byte) {

	Validator.LastHeight = height
	Validator.LastRound = uint(round)
	Validator.LastStep = step
	Validator.LastSignature = sig
	Validator.LastSignBytes = signBytes
}

func (Validator *privValidator) GetAddress() help.Address {
	return Validator.PrivKey.PubKey().Address()
}
func (Validator *privValidator) GetPubKey() tcrypto.PubKey {
	return Validator.PrivKey.PubKey()
}

// SignVote signs a canonical representation of the vote, along with the
// chainID. Implements PrivValidator.
func (Validator *privValidator) SignVote(chainID string, vote *Vote) error {
	Validator.mtx.Lock()
	defer Validator.mtx.Unlock()
	if err := Validator.signVote(chainID, vote); err != nil {
		return errors.New(fmt.Sprintf("Error signing vote: %v", err))
	}
	return nil
}

// signVote checks if the vote is good to sign and sets the vote signature.
// It may need to set the timestamp as well if the vote is otherwise the same as
// a previously signed vote (ie. we crashed after signing but before the vote hit the WAL).
func (Validator *privValidator) signVote(chainID string, vote *Vote) error {
	height, round, step := vote.Height, vote.Round, voteToStep(vote)
	signBytes := vote.SignBytes(chainID)

	sameHRS, err := Validator.checkHRS(height, int(round), step)
	if err != nil {
		return err
	}

	// We might crash before writing to the wal,
	// causing us to try to re-sign for the same HRS.
	// If signbytes are the same, use the last signature.
	// If they only differ by timestamp, use last timestamp and signature
	// Otherwise, return error
	if sameHRS {
		if bytes.Equal(signBytes, Validator.LastSignBytes) {
			vote.Signature = Validator.LastSignature
		} else if timestamp, ok := checkVotesOnlyDifferByTimestamp(Validator.LastSignBytes, signBytes); ok {
			vote.Timestamp = timestamp
			vote.Signature = Validator.LastSignature
		} else {
			err = fmt.Errorf("Conflicting data")
		}
		return err
	}

	// It passed the checks. Sign the vote
	hash := help.RlpHash([]interface{}{signBytes,})
	sig, err := Validator.PrivKey.Sign(hash[:])
	if err != nil {
		return err
	}
	Validator.saveSigned(height, int(round), step, signBytes, sig)
	vote.Signature = sig
	return nil
}

// SignProposal signs a canonical representation of the proposal, along with
// the chainID. Implements PrivValidator.
func (Validator *privValidator) SignProposal(chainID string, proposal *Proposal) error {
	Validator.mtx.Lock()
	defer Validator.mtx.Unlock()
	if err := Validator.signProposal(chainID, proposal); err != nil {
		return fmt.Errorf("Error signing proposal: %v", err)
	}
	return nil
}

// signProposal checks if the proposal is good to sign and sets the proposal signature.
// It may need to set the timestamp as well if the proposal is otherwise the same as
// a previously signed proposal ie. we crashed after signing but before the proposal hit the WAL).
func (Validator *privValidator) signProposal(chainID string, proposal *Proposal) error {
	height, round, step := proposal.Height, int(proposal.Round), stepPropose
	signBytes := proposal.SignBytes(chainID)

	sameHRS, err := Validator.checkHRS(height, round, step)
	if err != nil {
		return err
	}

	// We might crash before writing to the wal,
	// causing us to try to re-sign for the same HRS.
	// If signbytes are the same, use the last signature.
	// If they only differ by timestamp, use last timestamp and signature
	// Otherwise, return error
	if sameHRS {
		if bytes.Equal(signBytes, Validator.LastSignBytes) {
			proposal.Signature = Validator.LastSignature
		} else if timestamp, ok := checkProposalsOnlyDifferByTimestamp(Validator.LastSignBytes, signBytes); ok {
			proposal.Timestamp = timestamp
			proposal.Signature = Validator.LastSignature
		} else {
			err = fmt.Errorf("Conflicting data")
		}
		return err
	}
	
	// It passed the checks. Sign the proposal
	hash := help.RlpHash([]interface{}{signBytes,})
	sig, err := Validator.PrivKey.Sign(hash[:])
	if err != nil {
		return err
	}
	Validator.saveSigned(height, round, step, signBytes, sig)
	proposal.Signature = sig
	return nil
}
func (Validator *privValidator) SignHeartbeat(chainID string, heartbeat *Heartbeat) error {
	Validator.mtx.Lock()
	defer Validator.mtx.Unlock()
	hash := help.RlpHash([]interface{}{heartbeat.SignBytes(chainID),})
	sig, err := Validator.PrivKey.Sign(hash[:])
	if err != nil {
		return err
	}
	heartbeat.Signature = sig
	return nil
}

// returns error if HRS regression or no LastSignBytes. returns true if HRS is unchanged
func (Validator *privValidator) checkHRS(height uint64, round int, step uint8) (bool, error) {
	if Validator.LastHeight > height {
		return false, errors.New("Height regression")
	}

	if Validator.LastHeight == height {
		if int(Validator.LastRound) > round {
			return false, errors.New("Round regression")
		}

		if int(Validator.LastRound) == round {
			if Validator.LastStep > step {
				return false, errors.New("Step regression")
			} else if Validator.LastStep == step {
				if Validator.LastSignBytes != nil {
					if Validator.LastSignature == nil {
						panic("Validator: LastSignature is nil but LastSignBytes is not!")
					}
					return true, nil
				}
				return false, errors.New("No LastSignature found")
			}
		}
	}
	return false, nil
}

// returns the timestamp from the lastSignBytes.
// returns true if the only difference in the votes is their timestamp.
func checkVotesOnlyDifferByTimestamp(lastSignBytes, newSignBytes []byte) (time.Time, bool) {
	var lastVote, newVote CanonicalJSONVote
	if err := cdc.UnmarshalJSON(lastSignBytes, &lastVote); err != nil {
		panic(fmt.Sprintf("LastSignBytes cannot be unmarshalled into vote: %v", err))
	}
	if err := cdc.UnmarshalJSON(newSignBytes, &newVote); err != nil {
		panic(fmt.Sprintf("signBytes cannot be unmarshalled into vote: %v", err))
	}

	lastTime, err := time.Parse(TimeFormat, lastVote.Timestamp)
	if err != nil {
		panic(err)
	}

	// set the times to the same value and check equality
	now := CanonicalTime(time.Now())
	lastVote.Timestamp = now
	newVote.Timestamp = now
	lastVoteBytes, _ := cdc.MarshalJSON(lastVote)
	newVoteBytes, _ := cdc.MarshalJSON(newVote)

	return lastTime, bytes.Equal(newVoteBytes, lastVoteBytes)
}

// returns the timestamp from the lastSignBytes.
// returns true if the only difference in the proposals is their timestamp
func checkProposalsOnlyDifferByTimestamp(lastSignBytes, newSignBytes []byte) (time.Time, bool) {
	var lastProposal, newProposal CanonicalJSONProposal
	if err := cdc.UnmarshalJSON(lastSignBytes, &lastProposal); err != nil {
		panic(fmt.Sprintf("LastSignBytes cannot be unmarshalled into proposal: %v", err))
	}
	if err := cdc.UnmarshalJSON(newSignBytes, &newProposal); err != nil {
		panic(fmt.Sprintf("signBytes cannot be unmarshalled into proposal: %v", err))
	}

	lastTime, err := time.Parse(TimeFormat, lastProposal.Timestamp)
	if err != nil {
		panic(err)
	}

	// set the times to the same value and check equality
	now := CanonicalTime(time.Now())
	lastProposal.Timestamp = now
	newProposal.Timestamp = now
	lastProposalBytes, _ := cdc.MarshalJSON(lastProposal)
	newProposalBytes, _ := cdc.MarshalJSON(newProposal)

	return lastTime, bytes.Equal(newProposalBytes, lastProposalBytes)
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
// StateAgent implements PrivValidator
type StateAgent interface {
	GetValidator() *ValidatorSet
	GetLastValidator() *ValidatorSet

	GetLastBlockHeight() uint64
	GetChainID() string
	UpdateHeight(height uint64)
	MakeBlock() (*ctypes.Block, *PartSet)
	ValidateBlock(block *ctypes.Block) error
	ConsensusCommit(block *ctypes.Block) error

	GetAddress() help.Address
	GetPubKey() tcrypto.PubKey
	SignVote(chainID string, vote *Vote) error
	SignProposal(chainID string, proposal *Proposal) error
	SignHeartbeat(chainID string, heartbeat *Heartbeat) error
}

type stateAgent struct {
	Priv       *privValidator
	Agent      ctypes.PbftAgentProxy
	Validators *ValidatorSet
	ChainID    string
	Height     uint64
}

func NewStateAgent(agent ctypes.PbftAgentProxy, chainID string,
	vals *ValidatorSet, height uint64) *stateAgent {
	return &stateAgent{
		Agent:      agent,
		ChainID:    chainID,
		Validators: vals,
		Height:     height,
	}
}

func MakePartSet(partSize uint, block *ctypes.Block) *PartSet {
	// We prefix the byte length, so that unmarshaling
	// can easily happen via a reader.
	bz, err := cdc.MarshalBinary(block)
	if err != nil {
		panic(err)
	}
	return NewPartSetFromData(bz, partSize)
}
func (state *stateAgent) GetChainID() string {
	return state.ChainID
}
func (state *stateAgent) SetPrivValidator(priv *privValidator) {
	state.Priv = priv
}
func (state *stateAgent) MakeBlock() (*ctypes.Block, *PartSet) {
	block, err := state.Agent.FetchFastBlock()
	if err != nil {
		return nil, nil
	}
	return block, MakePartSet(BlockPartSizeBytes, block)
}
func (state *stateAgent) ConsensusCommit(block *ctypes.Block) error {
	if block == nil {
		return errors.New("error param")
	}
	err := state.Agent.BroadcastConsensus(block)
	if err != nil {
		return err
	}
	return nil
}
func (state *stateAgent) ValidateBlock(block *ctypes.Block) error {
	if block == nil {
		return errors.New("block not have")
	}
	err := state.Agent.VerifyFastBlock(block)
	if err != nil {
		return err
	}
	return nil
}
func (state *stateAgent) GetValidator() *ValidatorSet {
	return state.Validators
}
func (state *stateAgent) GetLastValidator() *ValidatorSet {
	return state.Validators
}
func (state *stateAgent) GetLastBlockHeight() uint64 {
	return state.Height
}
func (state *stateAgent) UpdateHeight(height uint64) {
	if height > state.Height {
		state.Height = height
	}
}
func (state *stateAgent) GetAddress() help.Address {
	return state.Priv.GetAddress()
}
func (state *stateAgent) GetPubKey() tcrypto.PubKey {
	return state.Priv.GetPubKey()
}
func (state *stateAgent) SignVote(chainID string, vote *Vote) error {
	return state.Priv.SignVote(chainID, vote)
}
func (state *stateAgent) SignProposal(chainID string, proposal *Proposal) error {
	return state.Priv.signProposal(chainID, proposal)
}
func (state *stateAgent) SignHeartbeat(chainID string, heartbeat *Heartbeat) error {
	return state.Priv.SignHeartbeat(chainID, heartbeat)
}
func (state *stateAgent) Broadcast(height *big.Int) {
	// if fb := ss.getBlock(height.Uint64()); fb != nil {
	// 	state.Agent.BroadcastFastBlock(fb)
	// }
}
