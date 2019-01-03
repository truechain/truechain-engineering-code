package consensus

import (
	"encoding/json"
	"errors"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/pbftserver/lock"
	"sync"
)

type State struct {
	ViewID         int64
	MsgLogs        *MsgLogs
	LastSequenceID int64
	CurrentStage   Stage
	FastStage      Stage
	BlockResults   *SignedVoteMsg
	Clear          bool
	MySign         *types.PbftSign
}

type MsgLogs struct {
	ReqMsg      *RequestMsg
	lockPrepare *sync.Mutex
	lockCommit  *sync.Mutex
	prepareMsgs map[string]*VoteMsg
	commitMsgs  map[string]*VoteMsg
}

type Stage int

const (
	Idle        Stage = iota // Node is created successfully, but the consensus process is not started yet.
	PrePrepared              // The ReqMsgs is processed successfully. The node is ready to head to the Prepare stage.
	Prepared                 // Same with `prepared` stage explained in the original paper.
	Committed                // Same with `committed-local` stage explained in the original paper.
)

func (m *MsgLogs) GetPrepareMessages() map[string]*VoteMsg {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	return m.prepareMsgs
}

func (m *MsgLogs) Clear() {
	m = &MsgLogs{
		ReqMsg:      nil,
		prepareMsgs: make(map[string]*VoteMsg),
		commitMsgs:  make(map[string]*VoteMsg),
		lockPrepare: new(sync.Mutex),
		lockCommit:  new(sync.Mutex),
	}
}

func (m *MsgLogs) GetCommitMessages() map[string]*VoteMsg {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	return m.commitMsgs
}

func (m *MsgLogs) SetPrepareMsg(key string, data *VoteMsg) {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	m.prepareMsgs[key] = data
}

func (m *MsgLogs) GetPrepareMsg(key string) (data *VoteMsg) {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	data, ok := m.prepareMsgs[key]
	if ok {
		return
	} else {
		return nil
	}
}

func (m *MsgLogs) GetPrepareOne() *VoteMsg {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	for _, v := range m.prepareMsgs {
		return v
	}
	return nil
}

func (m *MsgLogs) GetPrepareCount() int {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	return len(m.prepareMsgs)
}

func (m *MsgLogs) SetCommitMsgs(key string, data *VoteMsg) {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	m.commitMsgs[key] = data
}

func (m *MsgLogs) GetCommitMsgs(key string) (data *VoteMsg) {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	data, ok := m.commitMsgs[key]
	if ok {
		return
	} else {
		return nil
	}
}

func (m *MsgLogs) GetCommitMsgsSigns() []*types.PbftSign {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	signs := make([]*types.PbftSign, 0)
	for _, v := range m.commitMsgs {
		signs = append(signs, v.Signs)
	}
	return signs
}

func (m *MsgLogs) GetCommitPassCount() int {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	cnt := 0
	for _, v := range m.commitMsgs {
		if v.Pass != nil && v.Pass.Result == types.VoteAgree {
			cnt += 1
		}
	}
	return cnt
}

func (m *MsgLogs) GetCommitOne() *VoteMsg {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	for _, v := range m.commitMsgs {
		return v
	}
	return nil
}

func (m *MsgLogs) GetCommitCount() int {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	return len(m.commitMsgs)
}

// f: # of Byzantine faulty node
// f = (n­1) / 3
// n = 4, in this case.
// const f = 1

// lastSequenceID will be -1 if there is no last sequence ID.
func CreateState(viewID int64, lastSequenceID int64) *State {
	return &State{
		ViewID: viewID,
		MsgLogs: &MsgLogs{
			ReqMsg:      nil,
			prepareMsgs: make(map[string]*VoteMsg),
			commitMsgs:  make(map[string]*VoteMsg),
			lockPrepare: new(sync.Mutex),
			lockCommit:  new(sync.Mutex),
		},
		LastSequenceID: lastSequenceID,
		CurrentStage:   Idle,
		BlockResults:   nil,
	}
}

func (state *State) StartConsensus(request *RequestMsg) (*PrePrepareMsg, error) {
	// `sequenceID` will be the index of this message.
	//sequenceID := request.Height

	// Find the unique and largest number for the sequence ID
	//if state.LastSequenceID != -1 {
	//	for state.LastSequenceID >= sequenceID {
	//		sequenceID += 1
	//	}
	//}

	// Assign a new sequence ID to the request message object.
	request.SequenceID = request.Height

	// Save ReqMsgs to its logs.
	state.MsgLogs.ReqMsg = request

	// Get the digest of the request message
	digest, err := digest(request)
	if err != nil {
		return nil, err
	}

	// Change the stage to pre-prepared.
	state.CurrentStage = PrePrepared

	return &PrePrepareMsg{
		ViewID:     state.ViewID,
		SequenceID: request.Height,
		Digest:     digest,
		RequestMsg: request,
		Height:     request.Height,
	}, nil
}

func (state *State) PrePrepare(prePrepareMsg *PrePrepareMsg) (*VoteMsg, error) {
	// Get ReqMsgs and save it to its logs like the primary.
	state.MsgLogs.ReqMsg = prePrepareMsg.RequestMsg

	// Verify if v, n(a.k.a. sequenceID), d are correct.
	if !state.verifyMsg(prePrepareMsg.ViewID, prePrepareMsg.SequenceID, prePrepareMsg.Digest) {
		return nil, errors.New("pre-prepare message is corrupted")
	}
	// Change the stage to pre-prepared.
	state.CurrentStage = PrePrepared

	return &VoteMsg{
		ViewID:     state.ViewID,
		SequenceID: prePrepareMsg.SequenceID,
		Digest:     prePrepareMsg.Digest,
		MsgType:    PrepareMsg,
		Height:     prePrepareMsg.Height,
	}, nil
}

func (state *State) Prepare(prepareMsg *VoteMsg, f float64) (*VoteMsg, error) {
	//lock.PSLog("Prepare in")
	if !state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, prepareMsg.Digest) {
		return nil, errors.New("prepare message is corrupted")
	}

	// Append msg to its logs
	state.MsgLogs.SetPrepareMsg(prepareMsg.NodeID, prepareMsg)

	//lock.PSLog("Prepare PrepareMsgs cnt", len(state.MsgLogs.PrepareMsgs))
	// Print current voting status
	//log.Debug("Prepare ", "count", f)
	if state.prepared(f) {
		//log.Debug("Prepare ok", "count", f)
		//// Change the stage to prepared.
		//state.CurrentStage = Prepared

		return &VoteMsg{
			ViewID:     state.ViewID,
			SequenceID: prepareMsg.SequenceID,
			Digest:     prepareMsg.Digest,
			MsgType:    CommitMsg,
			Height:     prepareMsg.Height,
		}, nil
		lock.PSLog("Prepare", "end", f, "Return")
	}

	lock.PSLog("Prepare", "end", f, "notReturn")
	return nil, nil
}

func (state *State) Commit(commitMsg *VoteMsg, f float64) (*ReplyMsg, *RequestMsg, error) {

	if !state.verifyMsg(commitMsg.ViewID, commitMsg.SequenceID, commitMsg.Digest) {
		return nil, nil, errors.New("commit message is corrupted")
	}

	// Append msg to its logs
	state.MsgLogs.SetCommitMsgs(commitMsg.NodeID, commitMsg)

	// Print current voting status
	//log.Debug("Commit ", "count", f)
	if state.committed(f) {
		//log.Debug("Commit ok", "count", f)
		// This node executes the requested operation locally and gets the result.
		result := "Executed"

		return &ReplyMsg{
			ViewID:    state.ViewID,
			Timestamp: state.MsgLogs.ReqMsg.Timestamp,
			ClientID:  state.MsgLogs.ReqMsg.ClientID,
			Result:    result,
			Height:    state.MsgLogs.ReqMsg.Height,
		}, state.MsgLogs.ReqMsg, nil
	}
	return nil, nil, nil
}
func (state *State) VerifyMsg(viewID int64, sequenceID int64, digestGot string) bool {
	return state.verifyMsg(viewID, sequenceID, digestGot)
}

func (state *State) verifyMsg(viewID int64, sequenceID int64, digestGot string) bool {
	// Wrong view. That is, wrong configurations of peers to start the consensus.
	if state.ViewID != viewID {
		return false
	}

	// Check if the Primary sent fault sequence number. => Faulty primary.
	// TODO: adopt upper/lower bound check.
	//if state.LastSequenceID != -1 {
	if state.LastSequenceID > sequenceID {
		return false
	}
	//}
	digest, err := digest(state.MsgLogs.ReqMsg)
	if err != nil {
		lock.PSLog("[digest]", "error", err.Error())
		return false
	}

	// Check digest.
	if digestGot != digest {
		log.Error("verifyMsg messgae fail", digestGot, digest)
		return false
	}

	return true
}

func (state *State) prepared(f float64) bool {
	if state.MsgLogs.ReqMsg == nil {
		return false
	}
	if state.MsgLogs.GetPrepareCount() < int(2*f) {
		return false
	}

	return true
}

func (state *State) committed(f float64) bool {
	lock.PSLog("committed in")
	if !state.prepared(f) {
		return false
	}
	lock.PSLog("committed prepared")
	if state.MsgLogs.GetCommitCount() <= int(2*f) {
		return false
	}
	lock.PSLog("committed len(state.MsgLogs.CommitMsgs) >= 2*f")
	passCount := state.MsgLogs.GetCommitPassCount()
	//lock.PSLog("committed", fmt.Sprintf("%+v", state.MsgLogs.CommitMsgs), passCount)
	return passCount > int(2*f)
}

func digest(object interface{}) (string, error) {
	msg, err := json.Marshal(object)

	if err != nil {
		return "", err
	}

	return Hash(msg), nil
}
