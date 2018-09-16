package types

import (
	"bytes"
	"crypto/ecdsa"
	"sync"
	"fmt"
	"time"
	"errors"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	tcrypto "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/tendermint/tendermint/types"
)

const (
	stepNone      int8 = 0 // Used to distinguish the initial state
	stepPropose   int8 = 1
	stepPrevote   int8 = 2
	stepPrecommit int8 = 3
)

func voteToStep(vote *Vote) int8 {
	switch vote.Type {
	case types.VoteTypePrevote:
		return stepPrevote
	case types.VoteTypePrecommit:
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
	PrivKey     	tcrypto.PrivKey
	LastHeight    	int64          `json:"last_height"`
	LastRound     	int            `json:"last_round"`
	LastStep      	int8           `json:"last_step"`
	LastSignature 	[]byte         `json:"last_signature,omitempty"` // so we dont lose signatures XXX Why would we lose signatures?
	LastSignBytes 	help.HexBytes   `json:"last_signbytes,omitempty"` // so we dont lose signatures XXX Why would we lose signatures?

	mtx      	sync.Mutex
}

func NewPrivValidator(priv ecdsa.PrivateKey) PrivValidator {
	return &privValidator{
		PrivKey:	tcrypto.PrivKeyTrue(priv),
		LastStep:	stepNone,
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
func (Validator *privValidator) saveSigned(height int64, round int, step int8,
	signBytes []byte, sig []byte) {

	Validator.LastHeight = height
	Validator.LastRound = round
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
	sig, err := Validator.PrivKey.Sign(signBytes)
	if err != nil {
		return err
	}
	Validator.saveSigned(height, round, step, signBytes, sig)
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
	height, round, step := proposal.Height, proposal.Round, stepPropose
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
	sig, err := Validator.PrivKey.Sign(signBytes)
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
	sig, err := Validator.PrivKey.Sign(heartbeat.SignBytes(chainID))
	if err != nil {
		return err
	}
	heartbeat.Signature = sig
	return nil
}
// returns error if HRS regression or no LastSignBytes. returns true if HRS is unchanged
func (Validator *privValidator) checkHRS(height int64, round int, step int8) (bool, error) {
	if Validator.LastHeight > height {
		return false, errors.New("Height regression")
	}

	if Validator.LastHeight == height {
		if Validator.LastRound > round {
			return false, errors.New("Round regression")
		}

		if Validator.LastRound == round {
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
	if err := help.UnmarshalJSON(lastSignBytes, &lastVote); err != nil {
		panic(fmt.Sprintf("LastSignBytes cannot be unmarshalled into vote: %v", err))
	}
	if err := help.UnmarshalJSON(newSignBytes, &newVote); err != nil {
		panic(fmt.Sprintf("signBytes cannot be unmarshalled into vote: %v", err))
	}

	lastTime, err := time.Parse(types.TimeFormat, lastVote.Timestamp)
	if err != nil {
		panic(err)
	}

	// set the times to the same value and check equality
	now := CanonicalTime(time.Now())
	lastVote.Timestamp = now
	newVote.Timestamp = now
	lastVoteBytes, _ := help.MarshalJSON(lastVote)
	newVoteBytes, _ := help.MarshalJSON(newVote)

	return lastTime, bytes.Equal(newVoteBytes, lastVoteBytes)
}
// returns the timestamp from the lastSignBytes.
// returns true if the only difference in the proposals is their timestamp
func checkProposalsOnlyDifferByTimestamp(lastSignBytes, newSignBytes []byte) (time.Time, bool) {
	var lastProposal, newProposal types.CanonicalJSONProposal
	if err := help.UnmarshalJSON(lastSignBytes, &lastProposal); err != nil {
		panic(fmt.Sprintf("LastSignBytes cannot be unmarshalled into proposal: %v", err))
	}
	if err := help.UnmarshalJSON(newSignBytes, &newProposal); err != nil {
		panic(fmt.Sprintf("signBytes cannot be unmarshalled into proposal: %v", err))
	}

	lastTime, err := time.Parse(types.TimeFormat, lastProposal.Timestamp)
	if err != nil {
		panic(err)
	}

	// set the times to the same value and check equality
	now := CanonicalTime(time.Now())
	lastProposal.Timestamp = now
	newProposal.Timestamp = now
	lastProposalBytes, _ := help.MarshalJSON(lastProposal)
	newProposalBytes, _ := help.MarshalJSON(newProposal)

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

	GetLastBlockHeight() int64
	GetChainID() string
	LoadSeenCommit(height int64) *Commit
	MakeBlock() (*ctypes.Block,*PartSet)
	ValidateBlock(block *ctypes.Block) error 
	ConsensusCommit(block *ctypes.Block) error 

	GetAddress() help.Address 
	GetPubKey() tcrypto.PubKey

	SignVote(chainID string, vote *Vote) error
	SignProposal(chainID string, proposal *Proposal) error
	SignHeartbeat(chainID string, heartbeat *Heartbeat) error
}

type stateAgent struct {
	Validators         *ValidatorSet
	PrivValidator      *ecdsa.PrivateKey
}

func NewStateAgent() *stateAgent {
	return nil
}
func makeValidators() *ValidatorSet{
	return nil
}
