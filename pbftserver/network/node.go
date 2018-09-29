package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"github.com/truechain/truechain-engineering-code/pbftserver/lock"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
	"math/big"
	"sync"
	"time"
)

type Node struct {
	NodeID             string
	NodeTable          map[string]string // key=nodeID, value=url
	NTLock             sync.Mutex
	View               *View
	States             map[int64]*consensus.State
	CommittedMsgs      []*consensus.RequestMsg // kinda block.
	CommitWaitQueue    *prque.Prque
	MsgBuffer          *MsgBuffer
	MsgEntrance        chan interface{}
	MsgDelivery        chan interface{}
	MsgBackward        chan interface{}
	Alarm              chan bool
	FinishChan         chan int64
	Verify             consensus.ConsensusVerify
	Finish             consensus.ConsensusFinish
	ID                 *big.Int
	lock               sync.Mutex
	PrePareLock        sync.Mutex
	CommitLock         sync.Mutex
	CurrentHeight      int64
	RetryPrePrepareMsg map[int64]*consensus.PrePrepareMsg
	//stop               bool
}

type MsgBuffer struct {
	ReqMsgs        []*consensus.RequestMsg
	PrePrepareMsgs []*consensus.PrePrepareMsg
	PrepareMsgs    []*consensus.VoteMsg
	CommitMsgs     []*consensus.VoteMsg
}

type View struct {
	ID      int64
	Primary string
}

const (
	ResolvingTimeDuration = time.Second // 1 second.
	StateMax              = 1000        //max size for status
	StateClear            = 500
)

func NewNode(nodeID string, verify consensus.ConsensusVerify, finish consensus.ConsensusFinish,
	addrs []*types.CommitteeNode, id *big.Int) *Node {
	const viewID = 10000000000 // temporary.
	if len(addrs) <= 0 {
		return nil
	}
	primary := common.ToHex(addrs[0].Publickey)
	nodeTable := make(map[string]string)
	ID := id.Uint64()
	for _, v := range addrs {
		name := common.ToHex(v.Publickey)
		if ID%2 > 0 {
			nodeTable[name] = fmt.Sprintf("%s:%d", v.IP, v.Port)
		} else {
			nodeTable[name] = fmt.Sprintf("%s:%d", v.IP, v.Port2)
		}

	}
	node := &Node{
		// Hard-coded for test.
		NodeID:    nodeID,
		NodeTable: nodeTable,
		View: &View{
			ID:      viewID,
			Primary: primary,
		},
		Verify:        verify,
		Finish:        finish,
		ID:            id,
		States:        make(map[int64]*consensus.State),
		CommittedMsgs: make([]*consensus.RequestMsg, 0),
		//CommitWaitMsg: make(map[int64]*consensus.VoteMsg),
		CommitWaitQueue: prque.New(),
		MsgBuffer: &MsgBuffer{
			ReqMsgs:        make([]*consensus.RequestMsg, 0),
			PrePrepareMsgs: make([]*consensus.PrePrepareMsg, 0),
			PrepareMsgs:    make([]*consensus.VoteMsg, 0),
			CommitMsgs:     make([]*consensus.VoteMsg, 0),
		},
		MsgEntrance:        make(chan interface{}, 1024),
		MsgDelivery:        make(chan interface{}, 1024),
		MsgBackward:        make(chan interface{}, 1024),
		Alarm:              make(chan bool, 100),
		FinishChan:         make(chan int64, 100),
		RetryPrePrepareMsg: make(map[int64]*consensus.PrePrepareMsg),
	}

	// Start message dispatcher
	go node.dispatchMsg()

	// Start alarm trigger
	go node.alarmToDispatcher()

	// Start message resolver
	go node.resolveMsg()

	//start backward message dispatcher
	go node.dispatchMsgBackward()

	//start Process message commit wait
	go node.processCommitWaitMessageQueue()

	return node
}

func (node *Node) Broadcast(msg interface{}, path string) map[string]error {
	node.NTLock.Lock()
	defer node.NTLock.Unlock()
	errorMap := make(map[string]error)
	for nodeID, url := range node.NodeTable {
		if nodeID == node.NodeID {
			continue
		}

		if url == ":0" {
			continue
		}

		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			errorMap[nodeID] = err
			continue
		}
		lock.PSLog("Broadcast", url+path)
		go send(url+path, jsonMsg)

	}

	if len(errorMap) == 0 {
		return nil
	} else {
		return errorMap
	}
}

func (node *Node) BroadcastOne(msg interface{}, path string, node_id string) (err error) {
	node.NTLock.Lock()
	defer node.NTLock.Unlock()
	for nodeID, url := range node.NodeTable {
		if nodeID != node_id {
			continue
		}

		if url == ":0" {
			continue
		}

		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			break
		}
		lock.PSLog("Broadcast One", url+path)
		go send(url+path, jsonMsg)
	}
	return
}

func (node *Node) ClearStatus(height int64) {
	dHeight := height % StateMax
	dHeight = dHeight - StateClear
	if dHeight < 0 {
		dHeight += StateMax
	}
	delete(node.States, dHeight)
}

func (node *Node) PutStatus(height int64, state *consensus.State) {
	node.CurrentHeight = height
	node.lock.Lock()
	defer node.lock.Unlock()
	id := height % StateMax
	node.States[id] = state
	node.ClearStatus(height)
	//fmt.Println("[status]", "put", id)
}

func (node *Node) GetStatus(height int64) *consensus.State {
	node.lock.Lock()
	defer node.lock.Unlock()
	id := height % StateMax
	if state, ok := node.States[id]; ok {
		return state
	}
	return nil
}

func (node *Node) handleResult(msg *consensus.ReplyMsg) {
	var res uint = 0
	if msg.Result == "Executed" {
		res = 1
	}
	CurrentState := node.GetStatus(msg.Height)

	if msg.ViewID == CurrentState.ViewID {
		signs := CurrentState.MsgLogs.GetCommitMsgsSigns()
		//fmt.Println("----------------------------------------pbft signs len", len(signs))
		signs = append(signs, CurrentState.MySign)
		//fmt.Println("----------------------------------------pbft signs my", CurrentState.MySign)
		node.Verify.ReplyResult(CurrentState.MsgLogs.ReqMsg, signs, res)
	} else {
		// wrong state
	}
}
func (node *Node) ReplyResult(msgHeight int64) {
	go node.Finish.ConsensusFinish()
}
func (node *Node) Reply(msg *consensus.ReplyMsg) error {
	lock.PSLog("node Reply", msg.Height)
	node.handleResult(msg)
	go func() {
		node.FinishChan <- msg.Height
	}()
	return nil
}

// GetReq can be called when the node's CurrentState is nil.
// Consensus start procedure for the Primary.
func (node *Node) GetReq(reqMsg *consensus.RequestMsg) error {
	lock.PSLog("node GetReq", reqMsg.Height)

	// Create a new state for the new consensus.
	err := node.createStateForNewConsensus(reqMsg.Height)
	if err != nil {
		return err
	}

	// Start the consensus process.
	prePrepareMsg, err := node.GetStatus(reqMsg.Height).StartConsensus(reqMsg)
	if err != nil {
		return err
	}

	LogStage(fmt.Sprintf("Consensus Process (ViewID:%d)", node.GetStatus(reqMsg.Height).ViewID), false)

	if prePrepareMsg != nil {
		go node.delayPrePrepareMessage(prePrepareMsg)
	}

	return nil
}

//Delay detection retransmission prePrepareMessage
func (node *Node) delayPrePrepareMessage(prePrepareMsg *consensus.PrePrepareMsg) {
	if prePrepareMsg.Height == node.CurrentHeight {
		node.Broadcast(prePrepareMsg, "/preprepare")
		time.Sleep(time.Second * 60)
		if prePrepareMsg.Height == node.CurrentHeight {
			node.Verify.RepeatFetch(node.ID, prePrepareMsg.Height)
		}
	}
}

// GetPrePrepare can be called when the node's CurrentState is nil.
// Consensus start procedure for normal participants.
func (node *Node) GetPrePrepare(prePrepareMsg *consensus.PrePrepareMsg) error {
	//lock.PSLog("node GetPrePrepare", fmt.Sprintf("%+v", prePrepareMsg))
	// Create a new state for the new consensus.
	err := node.createStateForNewConsensus(prePrepareMsg.Height)
	if err != nil {
		lock.PSLog("node GetPrePrepare1", err.Error())
		return err
	}

	if !node.Verify.InsertBlock(prePrepareMsg) {
		lock.PSLog("[BlockInsertError]", node.NodeID, prePrepareMsg.Height)
	}

	prePareMsg, err := node.GetStatus(prePrepareMsg.Height).PrePrepare(prePrepareMsg)
	if err != nil {
		lock.PSLog("node GetPrePrepare2", err.Error())
		return err
	}

	//Add self

	status := node.GetStatus(prePrepareMsg.Height)
	if status.MsgLogs.GetPrepareMsg(node.NodeID) == nil {
		myPrepareMsg := prePareMsg
		myPrepareMsg.NodeID = node.NodeID
		status.MsgLogs.SetPrepareMsg(node.NodeID, myPrepareMsg)
	}

	//lock.PSLog("node GetPrePrepare", "Len", len(node.GetStatus(prePrepareMsg.Height).MsgLogs.PrepareMsgs),
	//	len(node.GetStatus(prePrepareMsg.Height).MsgLogs.CommitMsgs))
	if prePareMsg != nil {
		// Attach node ID to the message
		prePareMsg.NodeID = node.NodeID

		LogStage("Pre-prepare", true)
		msg := &consensus.StorgePrepareMsg{
			ViewID:     prePareMsg.ViewID,
			SequenceID: prePareMsg.SequenceID,
			Digest:     prePareMsg.Digest,
			NodeID:     prePareMsg.NodeID,
			Height:     prePrepareMsg.Height,
			MsgType:    prePareMsg.MsgType,
		}
		node.Broadcast(msg, "/prepare")
		LogStage("Prepare", false)
	}

	return nil
}

func (node *Node) GetPrepare(prepareMsg *consensus.VoteMsg) error {
	node.PrePareLock.Lock()
	defer node.PrePareLock.Unlock()
	lock.PSLog("node GetPrepare", prepareMsg.Height)
	node.NTLock.Lock()
	f := len(node.NodeTable) / 3
	node.NTLock.Unlock()
	CurrentState := node.GetStatus(prepareMsg.Height)

	if CurrentState == nil ||
		CurrentState.MsgLogs.ReqMsg.SequenceID != prepareMsg.SequenceID {
		return nil
	}

	commitMsg, err := CurrentState.Prepare(prepareMsg, f)
	if err != nil {
		return err
	}

	if commitMsg != nil {
		// Attach node ID to the message
		commitMsg.NodeID = node.NodeID

		if node.GetStatus(commitMsg.Height).CurrentStage == consensus.Prepared {
			commitMsg.Pass = node.Verify.SignMsg(CurrentState.MsgLogs.ReqMsg.Height, types.VoteAgree)
			node.GetStatus(commitMsg.Height).BlockResults = commitMsg.Pass
			commitMsg.Signs = node.GetStatus(commitMsg.Height).MySign
			node.BroadcastOne(commitMsg, "/commit", prepareMsg.NodeID)
			return nil
		}

		sign, res := node.Verify.CheckMsg(CurrentState.MsgLogs.ReqMsg)
		//fmt.Println("---------------------------------------1,sign == nil", sign == nil, "res=nil", res == nil)
		if res != nil && res == types.ErrHeightNotYet {
			lock.PSLog("CheckMsg Err ", types.ErrHeightNotYet.Error(), CurrentState.MsgLogs.ReqMsg.Height)
			//node.CommitWaitMsg[commitMsg.Height] = prepareMsg
			node.CommitWaitQueue.Push(prepareMsg, float32(-prepareMsg.Height))
		} else {
			var result uint = types.VoteAgreeAgainst
			if res == nil {
				result = types.VoteAgree
			}

			//fmt.Println("---------------------------------------,sign == nil", sign == nil, "res=nil", res == nil)
			CurrentState.MySign = sign
			lock.PSLog("CheckMsg Result ", result)
			commitMsg.Pass = node.Verify.SignMsg(CurrentState.MsgLogs.ReqMsg.Height, result)
			commitMsg.Signs = sign
			//save Pass
			node.GetStatus(commitMsg.Height).BlockResults = commitMsg.Pass

			node.GetStatus(commitMsg.Height).MsgLogs.SetCommitMsgs(node.NodeID, commitMsg)

			// Change the stage to prepared.
			node.GetStatus(commitMsg.Height).CurrentStage = consensus.Prepared
			LogStage("Prepare", true)
			node.Broadcast(commitMsg, "/commit")
			LogStage("Commit", false)
		}
	}
	return nil
}

func (node *Node) processCommitWaitMessageQueue() {
	for {
		//if node.Stop {
		//	return
		//}
		var msgSend = make([]*consensus.VoteMsg, 0)
		if !node.CommitWaitQueue.Empty() {
			msg := node.CommitWaitQueue.PopItem().(*consensus.VoteMsg)
			state := node.GetStatus(int64(msg.Height))
			if state != nil {
				if state.CurrentStage == consensus.Committed {
					msg := state.MsgLogs.GetCommitOne()
					if msg != nil {
						msgSend := &consensus.VoteMsg{
							NodeID:     node.NodeID,
							ViewID:     state.ViewID,
							SequenceID: msg.SequenceID,
							Digest:     msg.Digest,
							MsgType:    consensus.CommitMsg,
							Height:     msg.Height,
							Pass:       state.BlockResults,
						}
						if msgSend.Pass == nil {
							msgSend.Pass = node.Verify.SignMsg(state.MsgLogs.ReqMsg.Height, types.VoteAgree)
							state.BlockResults = msgSend.Pass
						}
						if msgSend.Signs == nil && state.MySign != nil {
							msgSend.Signs = state.MySign
						}
						node.BroadcastOne(msgSend, "/commit", msg.NodeID)
					}
				}
				msgSend = append(msgSend, msg)
				node.MsgDelivery <- msgSend
			}
		}
		time.Sleep(time.Millisecond * 300)
	}
}

func (node *Node) GetCommit(commitMsg *consensus.VoteMsg) error {
	node.CommitLock.Lock()
	defer node.CommitLock.Unlock()
	//lock.PSLog("node GetCommit in", fmt.Sprintf("%+v", commitMsg))
	node.NTLock.Lock()
	f := len(node.NodeTable) / 3
	node.NTLock.Unlock()
	state := node.GetStatus(commitMsg.Height)
	if state == nil {
		return nil
	}

	//fmt.Println("_-----------------------------------------------------sings", commitMsg.Signs)
	replyMsg, committedMsg, err := node.GetStatus(commitMsg.Height).Commit(commitMsg, f)

	lock.PSLog("[Committed return]", "commitMsg.Height", commitMsg.Height, "CurrentStage", state.CurrentStage)
	if state.CurrentStage == consensus.Committed {
		lock.PSLog("[Committed return true]", "commitMsg.Height", commitMsg.Height, "CurrentStage", state.CurrentStage)
		state.MsgLogs.SetCommitMsgs(commitMsg.NodeID, commitMsg)
		return nil
	}

	if err != nil {
		return err
	}
	if replyMsg != nil {
		if committedMsg == nil {
			return errors.New("committed message is nil, even though the reply message is not nil")
		}

		// Change the stage to prepared.
		state.CurrentStage = consensus.Committed

		// Attach node ID to the message
		replyMsg.NodeID = node.NodeID

		// Save the last version of committed messages to node.
		node.CommittedMsgs = append(node.CommittedMsgs, committedMsg)

		LogStage("Commit", true)
		node.Reply(replyMsg)
		LogStage("Reply", true)
	}
	return nil
}

func (node *Node) GetReply(msg *consensus.ReplyMsg) {
	log.Trace("GetReply", "info", fmt.Sprintf("Result: %s by %s\n", msg.Result, msg.NodeID))
}

func (node *Node) createStateForNewConsensus(height int64) error {
	// Check if there is an ongoing consensus process.

	lock.PSLog("[create]", "height", height)
	//if node.GetStatus(height) != nil  {
	//	return errors.New("another consensus is ongoing")
	//}

	// Get the last sequence ID
	var lastSequenceID int64
	if len(node.CommittedMsgs) == 0 {
		lastSequenceID = -1
	} else {
		lastSequenceID = node.CommittedMsgs[len(node.CommittedMsgs)-1].SequenceID
	}

	// Create a new state for this new consensus process in the Primary
	node.PutStatus(height, consensus.CreateState(node.View.ID, lastSequenceID))

	LogStage("Create the replica status", true)

	return nil
}

func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance:

			//lock.PSLog("node.MsgEntrance", msg)
			err := node.routeMsg(msg)
			if err != nil {
				log.Error("dispatchMsg", "error", err[0].Error())
				// TODO: send err to ErrorChannel
			}

		case <-node.Alarm:

			err := node.routeMsgWhenAlarmed()
			if err != nil {
				log.Error("dispatchMsg", "error", err[0].Error())
				// TODO: send err to ErrorChannel
			}

		case msgHeight := <-node.FinishChan:
			node.ReplyResult(msgHeight)

		}
	}
}

func (node *Node) routeMsg(msg interface{}) []error {
	switch msg.(type) {
	case *consensus.RequestMsg:
		//lock.PSLog("node routeMsg",msg.(*consensus.RequestMsg).Height, fmt.Sprintf("%+v", msg.(*consensus.RequestMsg)))
		lock.PSLog("node routeMsg", msg.(*consensus.RequestMsg).Height)
		CurrentStage := node.GetStatus(msg.(*consensus.RequestMsg).Height)
		if CurrentStage == nil || (CurrentStage.CurrentStage == consensus.Idle) {
			// Copy buffered messages first.
			msgs := make([]*consensus.RequestMsg, 0)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.RequestMsg))

			// Send messages.
			node.MsgDelivery <- msgs
		} else {
			node.MsgBuffer.ReqMsgs = append(node.MsgBuffer.ReqMsgs, msg.(*consensus.RequestMsg))
		}
	case *consensus.PrePrepareMsg:
		//lock.PSLog("node PrePrepareMsg", fmt.Sprintf("%+v", msg.(*consensus.PrePrepareMsg)))
		lock.PSLog("node routeMsg", msg.(*consensus.PrePrepareMsg).Height)
		CurrentStage := node.GetStatus(msg.(*consensus.PrePrepareMsg).Height)
		if CurrentStage == nil || (CurrentStage.CurrentStage == consensus.Idle) {

			// Copy buffered messages first.
			msgs := make([]*consensus.PrePrepareMsg, 0)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.PrePrepareMsg))

			// Send messages.
			node.MsgDelivery <- msgs
		} else {
			for _, v := range node.MsgBuffer.PrePrepareMsgs {
				if v == msg.(*consensus.PrePrepareMsg) {
					return nil
				}
			}
			node.MsgBuffer.PrePrepareMsgs = append(node.MsgBuffer.PrePrepareMsgs, msg.(*consensus.PrePrepareMsg))
		}
	case *consensus.VoteMsg:
		if msg.(*consensus.VoteMsg).MsgType == consensus.PrepareMsg {
			//lock.PSLog("node VoteMsg_PrepareMsg", fmt.Sprintf("%+v", msg.(*consensus.VoteMsg)))
			lock.PSLog("node VoteMsg_PrepareMsg", msg.(*consensus.VoteMsg).Height)
			CurrentStage := node.GetStatus(msg.(*consensus.VoteMsg).Height)
			if CurrentStage == nil || CurrentStage.CurrentStage != consensus.PrePrepared {
				lock.PSLog("PrepareMsg to PrepareMsgs")
				node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs, msg.(*consensus.VoteMsg))
			} else {
				// Copy buffered messages first.
				msgs := make([]*consensus.VoteMsg, 0)

				// Append a newly arrived message.
				msgs = append(msgs, msg.(*consensus.VoteMsg))

				// Send messages.
				lock.PSLog("PrepareMsg to MsgDelivery")
				node.MsgDelivery <- msgs
			}
		} else if msg.(*consensus.VoteMsg).MsgType == consensus.CommitMsg {
			//lock.PSLog("node VoteMsg_CommitMsg", fmt.Sprintf("%+v", msg.(*consensus.VoteMsg)))
			lock.PSLog("node VoteMsg_CommitMsg", msg.(*consensus.VoteMsg).Height)
			CurrentStage := node.GetStatus(msg.(*consensus.VoteMsg).Height)

			if CurrentStage == nil || CurrentStage.CurrentStage != consensus.Prepared {
				node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs, msg.(*consensus.VoteMsg))
			} else {
				// Copy buffered messages first.
				msgs := make([]*consensus.VoteMsg, 0)

				// Append a newly arrived message.
				msgs = append(msgs, msg.(*consensus.VoteMsg))

				// Send messages.
				node.MsgDelivery <- msgs
			}
		}
	}

	return nil
}

func (node *Node) dispatchMsgBackward() {
	for {
		//if node.Stop {
		//	return
		//}
		select {
		case msg := <-node.MsgBackward:
			err := node.routeMsgBackward(msg)
			if err != nil {
				log.Error("getPrepare", "error", err.Error())
				// TODO: send err to ErrorChannel
			}
		}
	}
}

func (node *Node) routeMsgBackward(msg interface{}) error {
	switch msg.(type) {
	case []*consensus.VoteMsg:
		for _, v := range msg.([]*consensus.VoteMsg) {
			state := node.GetStatus(v.Height)
			if v.MsgType == consensus.CommitMsg {
				msg := state.MsgLogs.GetCommitOne()
				if msg != nil {
					msgSend := &consensus.VoteMsg{
						NodeID:     node.NodeID,
						ViewID:     state.ViewID,
						SequenceID: msg.SequenceID,
						Digest:     msg.Digest,
						MsgType:    consensus.CommitMsg,
						Height:     msg.Height,
						Pass:       state.BlockResults,
					}
					if msgSend.Pass == nil {
						msgSend.Pass = node.Verify.SignMsg(state.MsgLogs.ReqMsg.Height, types.VoteAgree)
						state.BlockResults = msgSend.Pass
					}
					if msgSend.Signs == nil && state.MySign != nil {
						msgSend.Signs = state.MySign
					}
					node.BroadcastOne(msgSend, "/commit", msg.NodeID)
				}
			} else if v.MsgType == consensus.PrepareMsg {
				v1 := state.MsgLogs.GetPrepareMsg(node.NodeID)
				if v1 != nil {
					msg := &consensus.VoteMsg{
						NodeID:     node.NodeID,
						ViewID:     state.ViewID,
						SequenceID: v1.SequenceID,
						Digest:     v1.Digest,
						MsgType:    consensus.PrepareMsg,
						Height:     v1.Height,
					}
					node.BroadcastOne(msg, "/prepare", v.NodeID)
				}

				msg := state.MsgLogs.GetCommitOne()
				if msg != nil {
					msgSend := &consensus.VoteMsg{
						NodeID:     node.NodeID,
						ViewID:     state.ViewID,
						SequenceID: msg.SequenceID,
						Digest:     msg.Digest,
						MsgType:    consensus.CommitMsg,
						Height:     msg.Height,
						Pass:       state.BlockResults,
					}
					if msgSend.Pass == nil {
						msgSend.Pass = node.Verify.SignMsg(state.MsgLogs.ReqMsg.Height, types.VoteAgree)
						state.BlockResults = msgSend.Pass
					}
					if msgSend.Signs == nil && state.MySign != nil {
						msgSend.Signs = state.MySign
					}
					node.BroadcastOne(msgSend, "/commit", msg.NodeID)
				}
			}
		}
	default:
		//LogFmt()
	}
	return nil
}

func sendSameHightMessage(node *Node) {
	//fmt.Println("sendSameHightMessage", "flag", 1)
	msgVote := make([]*consensus.VoteMsg, 0)
	msgVoteBackward := make([]*consensus.VoteMsg, 0)
	for i := len(node.MsgBuffer.CommitMsgs) - 1; i >= 0; i-- {
		status := node.GetStatus(node.MsgBuffer.CommitMsgs[i].Height)
		if status != nil && status.CurrentStage == consensus.Prepared {
			msgVote = append(msgVote, node.MsgBuffer.CommitMsgs[i])
			node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs[:i], node.MsgBuffer.CommitMsgs[i+1:]...)
		}
		if status != nil && status.CurrentStage > consensus.Prepared {
			tmp := node.MsgBuffer.CommitMsgs[i]
			node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs[:i], node.MsgBuffer.CommitMsgs[i+1:]...)
			if status.MsgLogs.GetPrepareMsg(tmp.NodeID) == nil {
				status.MsgLogs.SetCommitMsgs(tmp.NodeID, tmp)
				msgVoteBackward = append(msgVoteBackward, tmp)
			}
		}
	}
	//fmt.Println("sendSameHightMessage", "flag", 2)
	if len(msgVoteBackward) > 0 {
		node.MsgBackward <- msgVoteBackward
	}
	//fmt.Println("sendSameHightMessage", "flag", 3)
	if len(msgVote) > 0 {
		node.MsgDelivery <- msgVote
	}
	msgVote = make([]*consensus.VoteMsg, 0)
	msgVoteBackward2 := make([]*consensus.VoteMsg, 0)
	for i := len(node.MsgBuffer.PrepareMsgs) - 1; i >= 0; i-- {
		lock.PSLog("PrepareMsgs in")
		status := node.GetStatus(node.MsgBuffer.PrepareMsgs[i].Height)
		if status != nil && status.CurrentStage == consensus.PrePrepared {
			msgVote = append(msgVote, node.MsgBuffer.PrepareMsgs[i])
			node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs[:i], node.MsgBuffer.PrepareMsgs[i+1:]...)
		}
		if status != nil && status.CurrentStage > consensus.PrePrepared {
			tmp := node.MsgBuffer.PrepareMsgs[i]
			node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs[:i], node.MsgBuffer.PrepareMsgs[i+1:]...)

			if status.MsgLogs.GetPrepareMsg(tmp.NodeID) == nil {
				status.MsgLogs.SetPrepareMsg(tmp.NodeID, tmp)
				msgVoteBackward2 = append(msgVoteBackward2, tmp)
			}
		}
	}
	//fmt.Println("sendSameHightMessage", "flag", 4)
	if len(msgVoteBackward2) > 0 {
		node.MsgBackward <- msgVoteBackward2

	}

	if len(msgVote) > 0 {
		node.MsgDelivery <- msgVote
		lock.PSLog("PrepareMsgs out MsgDelivery")
	}

	msgPrePrepare := make([]*consensus.PrePrepareMsg, 0)
	for i := len(node.MsgBuffer.PrePrepareMsgs) - 1; i >= 0; i-- {
		status := node.GetStatus(node.MsgBuffer.PrePrepareMsgs[i].Height)
		if status != nil && (status.CurrentStage == consensus.Idle || status.CurrentStage == consensus.PrePrepared) {
			msgPrePrepare = append(msgPrePrepare, node.MsgBuffer.PrePrepareMsgs[i])
			node.MsgBuffer.PrePrepareMsgs = append(node.MsgBuffer.PrePrepareMsgs[:i], node.MsgBuffer.PrePrepareMsgs[i+1:]...)
		}
	}
	//fmt.Println("sendSameHightMessage", "flag", 5)
	if len(msgPrePrepare) > 0 {
		node.MsgDelivery <- msgPrePrepare
	}

	msgRequest := make([]*consensus.RequestMsg, 0)
	for i := len(node.MsgBuffer.ReqMsgs) - 1; i >= 0; i-- {
		status := node.GetStatus(node.MsgBuffer.ReqMsgs[i].Height)
		if status != nil && status.CurrentStage == consensus.Idle {
			msgRequest = append(msgRequest, node.MsgBuffer.ReqMsgs[i])
			node.MsgBuffer.ReqMsgs = append(node.MsgBuffer.ReqMsgs[:i], node.MsgBuffer.ReqMsgs[i+1:]...)
		}
	}
	//fmt.Println("sendSameHightMessage", "flag", 6)
	if len(msgRequest) > 0 {
		node.MsgDelivery <- msgRequest
	}
	//fmt.Println("sendSameHightMessage", "flag", 7)
}

func (node *Node) routeMsgWhenAlarmed() []error {
	sendSameHightMessage(node)
	return nil
}

func (node *Node) resolveMsg() {
	for {
		// Get buffered messages from the dispatcher.
		//if node.Stop {
		//	return
		//}
		msgs := <-node.MsgDelivery
		switch msgs.(type) {
		case []*consensus.RequestMsg:
			errs := node.resolveRequestMsg(msgs.([]*consensus.RequestMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					fmt.Println(err)
				}
				// TODO: send err to ErrorChannel
			}
		case []*consensus.PrePrepareMsg:
			errs := node.resolvePrePrepareMsg(msgs.([]*consensus.PrePrepareMsg))
			if len(errs) != 0 {
				for _, err := range errs {
					log.Error("getPrepare", "error", err.Error())
				}
				// TODO: send err to ErrorChannel
			}
		case []*consensus.VoteMsg:
			voteMsgs := msgs.([]*consensus.VoteMsg)
			if len(voteMsgs) == 0 {
				break
			}

			if voteMsgs[0].MsgType == consensus.PrepareMsg {
				errs := node.resolvePrepareMsg(voteMsgs)
				if len(errs) != 0 {
					for _, err := range errs {
						fmt.Println(err)
					}
					// TODO: send err to ErrorChannel
				}
			} else if voteMsgs[0].MsgType == consensus.CommitMsg {
				errs := node.resolveCommitMsg(voteMsgs)
				if len(errs) != 0 {
					for _, err := range errs {
						log.Error("getPrepare", "error", err.Error())
					}
					// TODO: send err to ErrorChannel
				}
			}
		}
	}
}

func (node *Node) alarmToDispatcher() {
	for {
		//if node.Stop {
		//	return
		//}
		time.Sleep(ResolvingTimeDuration)
		node.Alarm <- true
	}
}

func (node *Node) resolveRequestMsg(msgs []*consensus.RequestMsg) []error {
	errs := make([]error, 0)
	// Resolve messages
	for _, reqMsg := range msgs {
		//lock.PSLog("node resolveRequestMsg", fmt.Sprintf("%+v", reqMsg))
		lock.PSLog("node resolveRequestMsg", reqMsg.Height)
		err := node.GetReq(reqMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (node *Node) resolvePrePrepareMsg(msgs []*consensus.PrePrepareMsg) []error {
	errs := make([]error, 0)
	// Resolve messages
	for _, prePrepareMsg := range msgs {
		//lock.PSLog("node resolvePrePrepareMsg", fmt.Sprintf("%+v", prePrepareMsg))
		lock.PSLog("node resolvePrePrepareMsg", prePrepareMsg.Height)
		err := node.GetPrePrepare(prePrepareMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (node *Node) resolvePrepareMsg(msgs []*consensus.VoteMsg) []error {
	errs := make([]error, 0)
	// Resolve messages
	for _, prepareMsg := range msgs {
		//lock.PSLog("node resolvePrepareMsg", fmt.Sprintf("%+v", prepareMsg))
		lock.PSLog("node resolvePrepareMsg", prepareMsg.Height)
		err := node.GetPrepare(prepareMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}
	return nil
}

func (node *Node) resolveCommitMsg(msgs []*consensus.VoteMsg) []error {
	errs := make([]error, 0)

	// Resolve messages
	for _, commitMsg := range msgs {
		lock.PSLog("node resolveCommitMsg", commitMsg.Height)
		err := node.GetCommit(commitMsg)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}
