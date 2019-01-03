package types

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	tcrypto "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
	"sync"
	"time"
)

const (
	stepNone      uint8 = 0 // Used to distinguish the initial state
	stepPropose   uint8 = 1
	stepPrevote   uint8 = 2
	stepPrecommit uint8 = 3
	//BlockPartSizeBytes is part's size
	BlockPartSizeBytes uint = 65536 // 64kB,
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

// PrivValidator defines the functionality of a local TrueChain validator
// that signs votes, proposals, and heartbeats, and never double signs.
type PrivValidator interface {
	GetAddress() help.Address // redundant since .PubKey().Address()
	GetPubKey() tcrypto.PubKey

	SignVote(chainID string, vote *Vote) error
	SignProposal(chainID string, proposal *Proposal) error
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

//KeepBlockSign is block's sign
type KeepBlockSign struct {
	Result uint
	Sign   []byte
	Hash   common.Hash
}

//NewPrivValidator return new private Validator
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
		return fmt.Errorf("Error signing vote: %v", err)
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
	sig, err := Validator.PrivKey.Sign(signBytes)
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
	sig, err := Validator.PrivKey.Sign(signBytes)
	if err != nil {
		return err
	}
	Validator.saveSigned(height, round, step, signBytes, sig)
	proposal.Signature = sig
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

//PrivValidatorsByAddress is private validators for address
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
	UpdateValidator(vset *ValidatorSet) error
	GetLastValidator() *ValidatorSet

	GetLastBlockHeight() uint64
	SetEndHeight(h uint64)
	GetChainID() string
	MakeBlock(v *SwitchValidator) (*ctypes.Block, error)
	MakePartSet(partSize uint, block *ctypes.Block) (*PartSet, error)
	ValidateBlock(block *ctypes.Block, result bool) (*KeepBlockSign, error)
	ConsensusCommit(block *ctypes.Block) error

	GetAddress() help.Address
	GetPubKey() tcrypto.PubKey
	SignVote(chainID string, vote *Vote) error
	SignProposal(chainID string, proposal *Proposal) error
	PrivReset()
}

//StateAgentImpl agent state struct
type StateAgentImpl struct {
	Priv        *privValidator
	Agent       ctypes.PbftAgentProxy
	Validators  *ValidatorSet
	ChainID     string
	LastHeight  uint64
	StartHeight uint64
	EndHeight   uint64
	CID         uint64
}

//NewStateAgent return new agent state
func NewStateAgent(agent ctypes.PbftAgentProxy, chainID string,
	vals *ValidatorSet, height, cid uint64) *StateAgentImpl {
	lh := agent.GetCurrentHeight()
	return &StateAgentImpl{
		Agent:       agent,
		ChainID:     chainID,
		Validators:  vals,
		StartHeight: height,
		EndHeight:   0, // defualt 0,mean not work
		LastHeight:  lh.Uint64(),
		CID:         cid,
	}
}

//MakePartSet  block to partset
func MakePartSet(partSize uint, block *ctypes.Block) (*PartSet, error) {
	// We prefix the byte length, so that unmarshaling
	// can easily happen via a reader.
	bzs, err := rlp.EncodeToBytes(block)
	if err != nil {
		panic(err)
	}
	bz, err := cdc.MarshalBinary(bzs)
	if err != nil {
		return nil, err
	}
	return NewPartSetFromData(bz, partSize), nil
}

//MakeBlockFromPartSet partSet to block
func MakeBlockFromPartSet(reader *PartSet) (*ctypes.Block, error) {
	if reader.IsComplete() {
		maxsize := int64(MaxBlockBytes)
		bytes := make([]byte, maxsize, maxsize)
		_, err := cdc.UnmarshalBinaryReader(reader.GetReader(), &bytes, maxsize)
		if err != nil {
			return nil, err
		}
		var block ctypes.Block
		if err = rlp.DecodeBytes(bytes, &block); err != nil {
			return nil, err
		}
		return &block, nil
	}
	return nil, errors.New("not complete")
}

//PrivReset reset PrivValidator
func (state *StateAgentImpl) PrivReset() {
	state.Priv.Reset()
}

//SetEndHeight set now committee fast block height for end
func (state *StateAgentImpl) SetEndHeight(h uint64) {
	state.EndHeight = h
}

//GetChainID is get state'chainID
func (state *StateAgentImpl) GetChainID() string {
	return state.ChainID
}

//SetPrivValidator set state a new PrivValidator
func (state *StateAgentImpl) SetPrivValidator(priv PrivValidator) {
	pp := (priv).(*privValidator)
	state.Priv = pp
}
//UpdateValidator set new Validators when committee member was changed
func (state *StateAgentImpl) UpdateValidator(vset *ValidatorSet) error {
	state.Validators = vset
	return nil
}
//MakePartSet make block to part for partSiae
func (state *StateAgentImpl) MakePartSet(partSize uint, block *ctypes.Block) (*PartSet, error) {
	return MakePartSet(partSize, block)
}

//MakeBlock from agent FetchFastBlock
func (state *StateAgentImpl) MakeBlock(v *SwitchValidator) (*ctypes.Block, error) {
	committeeID := new(big.Int).SetUint64(state.CID)
	var info *ctypes.SwitchInfos
	if v != nil {
		info = v.Infos
		log.Info("MakeBlock", "val", info.Vals)
	}
	watch := newInWatch(3, "FetchFastBlock")
	block, err := state.Agent.FetchFastBlock(committeeID, info)
	if err != nil {
		return nil, err
	}
	if state.EndHeight > 0 && block.NumberU64() > state.EndHeight {
		return nil, fmt.Errorf("over height range,cur=%v,end=%v", block.NumberU64(), state.EndHeight)
	}
	if state.StartHeight > block.NumberU64() {
		return nil, fmt.Errorf("no more height,cur=%v,start=%v", block.NumberU64(), state.StartHeight)
	}
	watch.EndWatch()
	watch.Finish(block.NumberU64())
	return block, err
}

//ConsensusCommit is BroadcastConsensus block to agent
func (state *StateAgentImpl) ConsensusCommit(block *ctypes.Block) error {
	if block == nil {
		return errors.New("error param")
	}
	watch := newInWatch(3, "BroadcastConsensus")
	err := state.Agent.BroadcastConsensus(block)
	watch.EndWatch()
	watch.Finish(block.NumberU64())
	if err != nil {
		return err
	}
	return nil
}

//ValidateBlock get a verify block if nil return new
func (state *StateAgentImpl) ValidateBlock(block *ctypes.Block, result bool) (*KeepBlockSign, error) {
	if block == nil {
		return nil, errors.New("block not have")
	}
	if state.EndHeight > 0 && block.NumberU64() > state.EndHeight {
		return nil, fmt.Errorf("over height range,cur=%v,end=%v", block.NumberU64(), state.EndHeight)
	}
	watch := newInWatch(3, "VerifyFastBlock")
	sign, err := state.Agent.VerifyFastBlock(block, result)
	log.Info("VerifyFastBlockResult", "sign", sign, "result", sign.Result, "err", err)
	watch.EndWatch()
	watch.Finish(block.NumberU64())
	if sign != nil {
		return &KeepBlockSign{
			Result: sign.Result,
			Sign:   sign.Sign,
			Hash:   sign.FastHash,
		}, err
	}
	return nil, err
}

//GetValidator is get state's Validators
func (state *StateAgentImpl) GetValidator() *ValidatorSet {
	return state.Validators
}

//GetLastValidator is get state's Validators
func (state *StateAgentImpl) GetLastValidator() *ValidatorSet {
	return state.Validators
}

//GetLastBlockHeight is get fast block height for agent
func (state *StateAgentImpl) GetLastBlockHeight() uint64 {
	lh := state.Agent.GetCurrentHeight()
	state.LastHeight = lh.Uint64()
	return state.LastHeight
}

//GetAddress get priv_validator's address
func (state *StateAgentImpl) GetAddress() help.Address {
	return state.Priv.GetAddress()
}

//GetPubKey get priv_validator's public key
func (state *StateAgentImpl) GetPubKey() tcrypto.PubKey {
	return state.Priv.GetPubKey()
}

//SignVote sign of vote
func (state *StateAgentImpl) SignVote(chainID string, vote *Vote) error {
	return state.Priv.SignVote(chainID, vote)
}

//SignProposal sign of proposal msg
func (state *StateAgentImpl) SignProposal(chainID string, proposal *Proposal) error {
	return state.Priv.signProposal(chainID, proposal)
}

//Broadcast is agent Broadcast block
func (state *StateAgentImpl) Broadcast(height *big.Int) {
	// if fb := ss.getBlock(height.Uint64()); fb != nil {
	// 	state.Agent.BroadcastFastBlock(fb)
	// }
}

type inWatch struct {
	begin  time.Time
	end    time.Time
	expect float64
	str    string
}

func newInWatch(e float64, s string) *inWatch {
	return &inWatch{
		begin:  time.Now(),
		end:    time.Now(),
		expect: e,
		str:    s,
	}
}
func (in *inWatch) EndWatch() {
	in.end = time.Now()
}
func (in *inWatch) Finish(comment interface{}) {
	if d := in.end.Sub(in.begin); d.Seconds() > in.expect {
		log.Warn(in.str, "not expecting time", d.Seconds(), "comment", comment)
	}
}
