package consensus

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/pbftserver/lock"
	"sync"
	"time"
)

type State struct {
	ViewID         int64
	MsgLogs        *MsgLogs
	LastSequenceID int64
	CurrentStage   Stage
	FastStage      Stage
	BlockResults   *SignedVoteMsg
	Clear          bool
}

type MsgLogs struct {
	ReqMsg      *RequestMsg
	lockPrepare *sync.Mutex
	lockCommit  *sync.Mutex
	PrepareMsgs map[string]*VoteMsg
	CommitMsgs  map[string]*VoteMsg
}

type Stage int

const (
	Idle        Stage = iota // Node is created successfully, but the consensus process is not started yet.
	PrePrepared              // The ReqMsgs is processed successfully. The node is ready to head to the Prepare stage.
	Prepared                 // Same with `prepared` stage explained in the original paper.
	Committed                // Same with `committed-local` stage explained in the original paper.
)

func (m MsgLogs) SetPrepareMsg(key string, data *VoteMsg) {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	m.PrepareMsgs[key] = data
}

func (m MsgLogs) GetPrepareMsg(key string) (data *VoteMsg) {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	data, ok := m.PrepareMsgs[key]
	if ok {
		return
	} else {
		return nil
	}
}

func (m MsgLogs) GetPrepareOne() (data *VoteMsg) {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	for _, v := range m.PrepareMsgs {
		return v
	}
	return nil
}

func (m MsgLogs) GetPrepareCount() int {
	m.lockPrepare.Lock()
	defer m.lockPrepare.Unlock()
	return len(m.PrepareMsgs)
}

func (m MsgLogs) SetCommitMsgs(key string, data *VoteMsg) {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	m.CommitMsgs[key] = data
}

func (m MsgLogs) GetCommitMsgs(key string) (data *VoteMsg) {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	data, ok := m.CommitMsgs[key]
	if ok {
		return
	} else {
		return nil
	}
}

func (m MsgLogs) GetCommitPassCount() int {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	cnt := 0
	for _, v := range m.CommitMsgs {
		if v.Pass != nil && v.Pass.Result == types.VoteAgree {
			cnt += 1
		}
	}
	return cnt
}

func (m MsgLogs) GetCommitOne() (data *VoteMsg) {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	for _, v := range m.CommitMsgs {
		return v
	}
	return nil
}

func (m MsgLogs) GetCommitCount() int {
	m.lockCommit.Lock()
	defer m.lockCommit.Unlock()
	return len(m.CommitMsgs)
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
			PrepareMsgs: make(map[string]*VoteMsg),
			CommitMsgs:  make(map[string]*VoteMsg),
		},
		LastSequenceID: lastSequenceID,
		CurrentStage:   Idle,
		BlockResults:   nil,
	}
}

func (state *State) StartConsensus(request *RequestMsg) (*PrePrepareMsg, error) {
	// `sequenceID` will be the index of this message.
	sequenceID := time.Now().UnixNano()

	// Find the unique and largest number for the sequence ID
	if state.LastSequenceID != -1 {
		for state.LastSequenceID >= sequenceID {
			sequenceID += 1
		}
	}

	// Assign a new sequence ID to the request message object.
	request.SequenceID = sequenceID

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
		SequenceID: sequenceID,
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

func (state *State) Prepare(prepareMsg *VoteMsg, f int) (*VoteMsg, error) {
	lock.PSLog("Prepare in")
	if !state.verifyMsg(prepareMsg.ViewID, prepareMsg.SequenceID, prepareMsg.Digest) {
		return nil, errors.New("prepare message is corrupted")
	}

	// Append msg to its logs
	state.MsgLogs.SetPrepareMsg(prepareMsg.NodeID, prepareMsg)

	//lock.PSLog("Prepare PrepareMsgs cnt", len(state.MsgLogs.PrepareMsgs))
	// Print current voting status
	log.Info("Prepare ", "count", f)
	if state.prepared(f) {
		log.Info("Prepare ok", "count", f)
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

func (state *State) Commit(commitMsg *VoteMsg, f int) (*ReplyMsg, *RequestMsg, error) {

	if !state.verifyMsg(commitMsg.ViewID, commitMsg.SequenceID, commitMsg.Digest) {
		return nil, nil, errors.New("commit message is corrupted")
	}

	// Append msg to its logs
	state.MsgLogs.SetCommitMsgs(commitMsg.NodeID, commitMsg)
	// Print current voting status
	log.Info("Commit ", "count", f)
	if state.committed(f) {
		log.Info("Commit ok", "count", f)
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

func (state *State) verifyMsg(viewID int64, sequenceID int64, digestGot string) bool {
	// Wrong view. That is, wrong configurations of peers to start the consensus.
	if state.ViewID != viewID {
		return false
	}

	// Check if the Primary sent fault sequence number. => Faulty primary.
	// TODO: adopt upper/lower bound check.
	if state.LastSequenceID != -1 {
		if state.LastSequenceID >= sequenceID {
			return false
		}
	}

	digest, err := digest(state.MsgLogs.ReqMsg)
	if err != nil {
		lock.PSLog("[digest]", "error", err.Error())
		return false
	}

	// Check digest.
	if digestGot != digest {
		return false
	}

	return true
}

func (state *State) prepared(f int) bool {
	if state.MsgLogs.ReqMsg == nil {
		return false
	}
	if state.MsgLogs.GetPrepareCount() < 2*f {
		return false
	}

	return true
}

func (state *State) committed(f int) bool {
	lock.PSLog("committed in")
	if !state.prepared(f) {
		return false
	}
	lock.PSLog("committed prepared")
	if state.MsgLogs.GetCommitCount() < 2*f {
		return false
	}
	lock.PSLog("committed len(state.MsgLogs.CommitMsgs) >= 2*f")
	passCount := state.MsgLogs.GetCommitPassCount()
	lock.PSLog("committed", fmt.Sprintf("%+v", state.MsgLogs.CommitMsgs), passCount)
	return passCount >= 2*f
}

func digest(object interface{}) (string, error) {
	msg, err := json.Marshal(object)

	if err != nil {
		return "", err
	}

	return Hash(msg), nil
}
