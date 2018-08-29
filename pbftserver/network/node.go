package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"math/big"
	"sync"
	"time"
)

type Node struct {
	NodeID        string
	NodeTable     map[string]string // key=nodeID, value=url
	View          *View
	States        map[int64]*consensus.State
	CommittedMsgs []*consensus.RequestMsg // kinda block.
	CommitWaitMsg map[int64]*consensus.VoteMsg
	MsgBuffer     *MsgBuffer
	MsgEntrance   chan interface{}
	MsgDelivery   chan interface{}
	MsgBackward   chan interface{}
	Alarm         chan bool
	FinishChan    chan int64
	Verify        consensus.ConsensusVerify
	Finish        consensus.ConsensusFinish
	ID            *big.Int
	lock          sync.Mutex
	Count         int64
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
)

func NewNode(nodeID string, verify consensus.ConsensusVerify, finish consensus.ConsensusFinish,
	addrs []*types.CommitteeNode, id *big.Int) *Node {
	const viewID = 10000000000 // temporary.
	if len(addrs) <= 0 {
		return nil
	}
	primary := common.ToHex(addrs[0].Publickey)
	nodeTable := make(map[string]string)
	for _, v := range addrs {
		name := common.ToHex(v.Publickey)
		nodeTable[name] = fmt.Sprintf("%s:%d", v.IP, v.Port)
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
		CommitWaitMsg: make(map[int64]*consensus.VoteMsg),
		MsgBuffer: &MsgBuffer{
			ReqMsgs:        make([]*consensus.RequestMsg, 0),
			PrePrepareMsgs: make([]*consensus.PrePrepareMsg, 0),
			PrepareMsgs:    make([]*consensus.VoteMsg, 0),
			CommitMsgs:     make([]*consensus.VoteMsg, 0),
		},
		MsgEntrance: make(chan interface{}),
		MsgDelivery: make(chan interface{}),
		MsgBackward: make(chan interface{}),
		Alarm:       make(chan bool),
		FinishChan:  make(chan int64),
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
	go node.processCommitWaitMessage()

	return node
}

func (node *Node) Broadcast(msg interface{}, path string) map[string]error {
	errorMap := make(map[string]error)
	for nodeID, url := range node.NodeTable {
		if nodeID == node.NodeID {
			continue
		}

		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			errorMap[nodeID] = err
			continue
		}
		PSLog("Broadcast", url+path, string(jsonMsg))
		go send(url+path, jsonMsg)

	}

	if len(errorMap) == 0 {
		return nil
	} else {
		return errorMap
	}
}

func (node *Node) BroadcastOne(msg interface{}, path string, node_id string) (err error) {
	for nodeID, url := range node.NodeTable {
		if nodeID != node_id {
			continue
		}

		jsonMsg, err := json.Marshal(msg)
		if err != nil {
			break
		}
		PSLog("Broadcast One", url+path, string(jsonMsg))
		go send(url+path, jsonMsg)
	}
	return
}

func (node *Node) PutStatus(height int64, state *consensus.State) {
	node.lock.Lock()
	defer node.lock.Unlock()
	id := height % StateMax
	node.States[id] = state
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
		node.Verify.ReplyResult(CurrentState.MsgLogs.ReqMsg, res)
	} else {
		// wrong state
	}
}
func (node *Node) ReplyResult(msgHeight int64) {
	//CurrentState := node.GetStatus(msgHeight)
	//if CurrentState.CurrentStage == consensus.Committed {
	//
	//	node.SetCurrentState(nil)
	//	node.MsgBuffer.ReqMsgs = make([]*consensus.RequestMsg, 0)
	//	node.MsgBuffer.PrePrepareMsgs = make([]*consensus.PrePrepareMsg, 0)
	//	node.MsgBuffer.PrepareMsgs = make([]*consensus.VoteMsg, 0)
	//	node.MsgBuffer.CommitMsgs = make([]*consensus.VoteMsg, 0)
	//}
	go node.Finish.ConsensusFinish()
}
func (node *Node) Reply(msg *consensus.ReplyMsg) error {
	PSLog("node Reply", fmt.Sprintf("%+v", msg))
	node.handleResult(msg)
	go func() {
		//fmt.Println("[Chan]", "FinishChan", "In")
		node.FinishChan <- msg.Height
		//fmt.Println("[Chan]", "FinishChan", "Out")
		//node.ReplyResult(msg.Height)
	}()
	return nil
}

// GetReq can be called when the node's CurrentState is nil.
// Consensus start procedure for the Primary.
func (node *Node) GetReq(reqMsg *consensus.RequestMsg) error {
	PSLog("node GetReq", fmt.Sprintf("%+v", reqMsg))

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

	// Send getPrePrepare message
	if prePrepareMsg != nil {
		node.Broadcast(prePrepareMsg, "/preprepare")
		LogStage("Pre-prepare", true)
	}

	return nil
}

// GetPrePrepare can be called when the node's CurrentState is nil.
// Consensus start procedure for normal participants.
func (node *Node) GetPrePrepare(prePrepareMsg *consensus.PrePrepareMsg) error {
	PSLog("node GetPrePrepare", fmt.Sprintf("%+v", prePrepareMsg))
	// Create a new state for the new consensus.
	err := node.createStateForNewConsensus(prePrepareMsg.Height)
	if err != nil {
		return err
	}

	if !node.Verify.InsertBlock(prePrepareMsg) {
		fmt.Println("[BlockInsertError]", node.NodeID, prePrepareMsg.Height)
	}

	prePareMsg, err := node.GetStatus(prePrepareMsg.Height).PrePrepare(prePrepareMsg)
	if err != nil {
		return err
	}

	//Add self
	if _, ok := node.GetStatus(prePrepareMsg.Height).MsgLogs.PrepareMsgs[node.NodeID]; !ok {
		myPrepareMsg := prePareMsg
		node.GetStatus(prePrepareMsg.Height).MsgLogs.PrepareMsgs[node.NodeID] = myPrepareMsg
	}
	PSLog("node GetPrePrepare", "Len", len(node.GetStatus(prePrepareMsg.Height).MsgLogs.PrepareMsgs),
		len(node.GetStatus(prePrepareMsg.Height).MsgLogs.CommitMsgs))
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
	PSLog("node GetPrepare", fmt.Sprintf("%+v", prepareMsg))
	f := len(node.NodeTable) / 3

	CurrentState := node.GetStatus(prepareMsg.Height)

	if CurrentState == nil ||
		CurrentState.MsgLogs.ReqMsg.SequenceID != prepareMsg.SequenceID {
		return nil
	}
	commitMsg, err := CurrentState.Prepare(prepareMsg, f)
	if err != nil {
		return err
	}

	PSLog("node GetPrepare", "Len", len(CurrentState.MsgLogs.PrepareMsgs), len(CurrentState.MsgLogs.CommitMsgs))
	if commitMsg != nil {
		// Attach node ID to the message
		commitMsg.NodeID = node.NodeID

		res := node.Verify.CheckMsg(CurrentState.MsgLogs.ReqMsg)

		if res != nil && res == types.ErrHeightNotYet {
			PSLog("CheckMsg Err ", types.ErrHeightNotYet.Error())
			node.CommitWaitMsg[commitMsg.Height] = commitMsg
		} else {
			var result uint = 0
			if res != nil {
				result = 1
			}
			PSLog("CheckMsg Result ", result)
			commitMsg.Pass = node.Verify.SignMsg(CurrentState.MsgLogs.ReqMsg.Height, result)
			LogStage("Prepare", true)
			node.Broadcast(commitMsg, "/commit")
			LogStage("Commit", false)
		}
	}
	return nil
}

func (node *Node) processCommitWaitMessage() {
	for {
		for k, v := range node.CommitWaitMsg {
			state := node.GetStatus(v.Height)
			if state == nil {
				return
			}

			if state.CurrentStage == consensus.Committed {
				delete(node.CommitWaitMsg, k)
				return
			}

			res := node.Verify.CheckMsg(state.MsgLogs.ReqMsg)

			if res != nil && res != types.ErrHeightNotYet {
				var result uint = 0
				if res != nil {
					result = 1
				}
				v.Pass = node.Verify.SignMsg(state.MsgLogs.ReqMsg.Height, result)
				LogStage("Prepare", true)
				node.Broadcast(v, "/commit")
				LogStage("Commit", false)

				delete(node.CommitWaitMsg, k)
			}
		}
		time.Sleep(time.Second * 5)
	}
}

func (node *Node) GetCommit(commitMsg *consensus.VoteMsg) error {
	PSLog("node GetCommit", fmt.Sprintf("%+v", commitMsg))
	f := len(node.NodeTable) / 3
	if node.GetStatus(commitMsg.Height) == nil {
		return nil
	}
	replyMsg, committedMsg, err := node.GetStatus(commitMsg.Height).Commit(commitMsg, f)

	if err != nil {
		return err
	}
	if replyMsg != nil {
		if committedMsg == nil {
			return errors.New("committed message is nil, even though the reply message is not nil")
		}

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
	fmt.Printf("Result: %s by %s\n", msg.Result, msg.NodeID)
}

func (node *Node) createStateForNewConsensus(height int64) error {
	// Check if there is an ongoing consensus process.

	PSLog("[create]", "height", height)
	if node.GetStatus(height) != nil && node.GetStatus(height).CurrentStage != consensus.Committed {
		return errors.New("another consensus is ongoing")
	}

	// Get the last sequence ID
	var lastSequenceID int64
	if len(node.CommittedMsgs) == 0 {
		lastSequenceID = -1
	} else {
		lastSequenceID = node.CommittedMsgs[len(node.CommittedMsgs)-1].SequenceID
	}

	// Create a new state for this new consensus process in the Primary
	fmt.Println("[create]", node.NodeID, lastSequenceID, height)

	node.PutStatus(height, consensus.CreateState(node.View.ID, lastSequenceID))

	LogStage("Create the replica status", true)

	return nil
}

func (node *Node) dispatchMsg() {
	for {
		select {
		case msg := <-node.MsgEntrance:
			err := node.routeMsg(msg)
			if err != nil {
				fmt.Println(err)
				// TODO: send err to ErrorChannel
			}
		case <-node.Alarm:
			err := node.routeMsgWhenAlarmed()
			if err != nil {
				fmt.Println(err)
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
		PSLog("node routeMsg", fmt.Sprintf("%+v", msg.(*consensus.RequestMsg)))
		CurrentStage := node.GetStatus(msg.(*consensus.RequestMsg).Height)
		if CurrentStage == nil || (CurrentStage.CurrentStage == consensus.Idle) {
			// Copy buffered messages first.
			msgs := make([]*consensus.RequestMsg, 0)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.RequestMsg))

			//// Empty the buffer.
			//node.MsgBuffer.ReqMsgs = make([]*consensus.RequestMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		} else {
			node.MsgBuffer.ReqMsgs = append(node.MsgBuffer.ReqMsgs, msg.(*consensus.RequestMsg))
		}
	case *consensus.PrePrepareMsg:
		PSLog("node PrePrepareMsg", fmt.Sprintf("%+v", msg.(*consensus.PrePrepareMsg)))
		CurrentStage := node.GetStatus(msg.(*consensus.PrePrepareMsg).Height)
		if CurrentStage == nil || (CurrentStage.CurrentStage == consensus.Idle) {

			// Copy buffered messages first.
			msgs := make([]*consensus.PrePrepareMsg, 0)
			//copy(msgs, node.MsgBuffer.PrePrepareMsgs)

			// Append a newly arrived message.
			msgs = append(msgs, msg.(*consensus.PrePrepareMsg))

			// Empty the buffer.
			//node.MsgBuffer.PrePrepareMsgs = make([]*consensus.PrePrepareMsg, 0)

			// Send messages.
			node.MsgDelivery <- msgs
		} else {
			node.MsgBuffer.PrePrepareMsgs = append(node.MsgBuffer.PrePrepareMsgs, msg.(*consensus.PrePrepareMsg))
		}
	case *consensus.VoteMsg:
		if msg.(*consensus.VoteMsg).MsgType == consensus.PrepareMsg {
			PSLog("node VoteMsg_PrepareMsg", fmt.Sprintf("%+v", msg.(*consensus.VoteMsg)))
			CurrentStage := node.GetStatus(msg.(*consensus.VoteMsg).Height)
			if CurrentStage == nil || CurrentStage.CurrentStage != consensus.PrePrepared {
				node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs, msg.(*consensus.VoteMsg))
			} else {
				// Copy buffered messages first.
				msgs := make([]*consensus.VoteMsg, 0)
				//copy(msgs, node.MsgBuffer.PrepareMsgs)

				// Append a newly arrived message.
				msgs = append(msgs, msg.(*consensus.VoteMsg))

				// Empty the buffer.
				//node.MsgBuffer.PrepareMsgs = make([]*consensus.VoteMsg, 0)

				// Send messages.
				node.MsgDelivery <- msgs
			}
		} else if msg.(*consensus.VoteMsg).MsgType == consensus.CommitMsg {
			PSLog("node VoteMsg_CommitMsg", fmt.Sprintf("%+v", msg.(*consensus.VoteMsg)))
			CurrentStage := node.GetStatus(msg.(*consensus.VoteMsg).Height)

			if CurrentStage == nil || CurrentStage.CurrentStage != consensus.Prepared {
				node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs, msg.(*consensus.VoteMsg))
			} else {
				// Copy buffered messages first.
				msgs := make([]*consensus.VoteMsg, 0)
				//copy(msgs, node.MsgBuffer.CommitMsgs)

				// Append a newly arrived message.
				msgs = append(msgs, msg.(*consensus.VoteMsg))

				// Empty the buffer.
				//node.MsgBuffer.CommitMsgs = make([]*consensus.VoteMsg, 0)

				// Send messages.
				node.MsgDelivery <- msgs
			}
		}
	}

	return nil
}

func (node *Node) dispatchMsgBackward() {
	for {
		select {
		case msg := <-node.MsgBackward:
			err := node.routeMsgBackward(msg)
			if err != nil {
				fmt.Println(err)
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
				if v, ok := state.MsgLogs.CommitMsgs[node.NodeID]; ok {
					msg := &consensus.VoteMsg{
						ViewID:     state.ViewID,
						SequenceID: v.SequenceID,
						Digest:     v.Digest,
						MsgType:    consensus.PrepareMsg,
						Height:     v.Height,
					}
					node.BroadcastOne(msg, "/commit", v.NodeID)
				}
				//for _, msg := range state.MsgLogs.CommitMsgs {
				//	if msg.NodeID == v.NodeID{
				//		return nil
				//	}
				//	var msgSend *consensus.VoteMsg = msg
				//	msgSend.NodeID = node.NodeID
				//	node.BroadcastOne(msgSend, "/commit", v.NodeID)
				//}
			} else if v.MsgType == consensus.PrepareMsg {
				if v, ok := state.MsgLogs.PrepareMsgs[node.NodeID]; ok {
					msg := &consensus.VoteMsg{
						ViewID:     state.ViewID,
						SequenceID: v.SequenceID,
						Digest:     v.Digest,
						MsgType:    consensus.PrepareMsg,
						Height:     v.Height,
					}
					node.BroadcastOne(msg, "/prepare", v.NodeID)
				}
				if v, ok := state.MsgLogs.CommitMsgs[node.NodeID]; ok {
					msg := &consensus.VoteMsg{
						ViewID:     state.ViewID,
						SequenceID: v.SequenceID,
						Digest:     v.Digest,
						MsgType:    consensus.CommitMsg,
						Height:     v.Height,
					}
					node.BroadcastOne(msg, "/commit", v.NodeID)
				}

				//for _, msg := range state.MsgLogs.CommitMsgs {
				//	if msg.NodeID == node.NodeID{
				//		return nil
				//	}
				//	var msgSend *consensus.VoteMsg = msg
				//	msgSend.NodeID = node.NodeID
				//	node.BroadcastOne(msgSend, "/commit", v.NodeID)
				//}
				//
				//for _, msg := range state.MsgLogs.PrepareMsgs {
				//	if msg.NodeID == v.NodeID{
				//		return nil
				//	}
				//	var msgSend *consensus.VoteMsg = msg
				//	msgSend.NodeID = node.NodeID
				//	node.BroadcastOne(msgSend, "/prepare", v.NodeID)
				//}

			}

		}
	default:
		//LogFmt()
	}
	return nil
}

func sendSameHightMessage(node *Node) {
	if (len(node.MsgBuffer.ReqMsgs) +
		len(node.MsgBuffer.PrePrepareMsgs) +
		len(node.MsgBuffer.PrepareMsgs) +
		len(node.MsgBuffer.CommitMsgs)) > 0 {
		fmt.Printf("")
	}

	msgVote := make([]*consensus.VoteMsg, 0)

	for i := len(node.MsgBuffer.CommitMsgs) - 1; i >= 0; i-- {
		status := node.GetStatus(node.MsgBuffer.CommitMsgs[i].Height)
		if status != nil && status.CurrentStage == consensus.Prepared {
			msgVote = append(msgVote, node.MsgBuffer.CommitMsgs[i])
			node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs[:i], node.MsgBuffer.CommitMsgs[i+1:]...)
		}
		if status != nil && status.CurrentStage > consensus.Prepared {
			msgVoteBackward := make([]*consensus.VoteMsg, 0)
			msgVoteBackward = append(msgVoteBackward, node.MsgBuffer.CommitMsgs[i])
			node.MsgBuffer.CommitMsgs = append(node.MsgBuffer.CommitMsgs[:i], node.MsgBuffer.CommitMsgs[i+1:]...)
			if _, ok := status.MsgLogs.CommitMsgs[msgVoteBackward[0].NodeID]; !ok {
				status.MsgLogs.CommitMsgs[msgVoteBackward[0].NodeID] = msgVoteBackward[0]
				node.MsgBackward <- msgVoteBackward
			}
		}
	}

	for i := len(node.MsgBuffer.PrepareMsgs) - 1; i >= 0; i-- {
		status := node.GetStatus(node.MsgBuffer.PrepareMsgs[i].Height)
		if status != nil && status.CurrentStage == consensus.PrePrepared {
			msgVote = append(msgVote, node.MsgBuffer.PrepareMsgs[i])
			node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs[:i], node.MsgBuffer.PrepareMsgs[i+1:]...)
		}
		if status != nil && status.CurrentStage > consensus.PrePrepared {
			msgVoteBackward := make([]*consensus.VoteMsg, 0)
			msgVoteBackward = append(msgVoteBackward, node.MsgBuffer.PrepareMsgs[i])
			node.MsgBuffer.PrepareMsgs = append(node.MsgBuffer.PrepareMsgs[:i], node.MsgBuffer.PrepareMsgs[i+1:]...)
			if _, ok := status.MsgLogs.PrepareMsgs[msgVoteBackward[0].NodeID]; !ok {
				status.MsgLogs.PrepareMsgs[msgVoteBackward[0].NodeID] = msgVoteBackward[0]
				node.MsgBackward <- msgVoteBackward
			}
		}
	}
	if len(msgVote) > 0 {
		node.MsgDelivery <- msgVote
	}

	msgPrePrepare := make([]*consensus.PrePrepareMsg, 0)
	for i := len(node.MsgBuffer.PrePrepareMsgs) - 1; i >= 0; i-- {
		status := node.GetStatus(node.MsgBuffer.PrePrepareMsgs[i].Height)
		if status != nil && status.CurrentStage == consensus.Idle {
			msgPrePrepare = append(msgPrePrepare, node.MsgBuffer.PrePrepareMsgs[i])
			node.MsgBuffer.PrePrepareMsgs = append(node.MsgBuffer.PrePrepareMsgs[:i], node.MsgBuffer.PrePrepareMsgs[i+1:]...)
			//} else if status.CurrentStage > consensus.Idle {
			//	msg := make([]*consensus.PrePrepareMsg, 0)
			//	msg = append(msg, node.MsgBuffer.PrePrepareMsgs[i])
			//	node.MsgBuffer.PrePrepareMsgs = append(node.MsgBuffer.PrePrepareMsgs[:i], node.MsgBuffer.PrePrepareMsgs[i+1:]...)
			//	node.MsgBackward <- msg
		}
	}
	if len(msgPrePrepare) > 0 {
		node.MsgDelivery <- msgPrePrepare
	}

	msgRequest := make([]*consensus.RequestMsg, 0)
	for i := len(node.MsgBuffer.ReqMsgs) - 1; i >= 0; i-- {
		status := node.GetStatus(node.MsgBuffer.ReqMsgs[i].Height)
		if status != nil && status.CurrentStage == consensus.Idle {
			msgRequest = append(msgRequest, node.MsgBuffer.ReqMsgs[i])
			node.MsgBuffer.ReqMsgs = append(node.MsgBuffer.ReqMsgs[:i], node.MsgBuffer.ReqMsgs[i+1:]...)
			//} else if status.CurrentStage > consensus.Idle {
			//	msg := make([]*consensus.RequestMsg, 0)
			//	msg = append(msg, node.MsgBuffer.ReqMsgs[i])
			//	node.MsgBuffer.ReqMsgs = append(node.MsgBuffer.ReqMsgs[:i], node.MsgBuffer.ReqMsgs[i+1:]...)
			//	node.MsgBackward <- msg
		}
	}
	if len(msgRequest) > 0 {
		node.MsgDelivery <- msgRequest
	}
}

func (node *Node) routeMsgWhenAlarmed() []error {
	sendSameHightMessage(node)
	//if node.CurrentState == nil {
	//	// Check ReqMsgs, send them.
	//	if len(node.MsgBuffer.ReqMsgs) != 0 {
	//		msgs := make([]*consensus.RequestMsg, len(node.MsgBuffer.ReqMsgs))
	//		copy(msgs, node.MsgBuffer.ReqMsgs)
	//
	//		node.MsgDelivery <- msgs
	//	}
	//
	//	// Check PrePrepareMsgs, send them.
	//	if len(node.MsgBuffer.PrePrepareMsgs) != 0 {
	//		msgs := make([]*consensus.PrePrepareMsg, len(node.MsgBuffer.PrePrepareMsgs))
	//		copy(msgs, node.MsgBuffer.PrePrepareMsgs)
	//
	//		node.MsgDelivery <- msgs
	//	}
	//} else {
	//	switch node.CurrentState.CurrentStage {
	//	case consensus.PrePrepared:
	//		// Check PrepareMsgs, send them.
	//		if len(node.MsgBuffer.PrepareMsgs) != 0 {
	//			msgs := make([]*consensus.VoteMsg, len(node.MsgBuffer.PrepareMsgs))
	//			copy(msgs, node.MsgBuffer.PrepareMsgs)
	//
	//			node.MsgDelivery <- msgs
	//		}
	//	case consensus.Prepared:
	//		// Check CommitMsgs, send them.
	//		if len(node.MsgBuffer.CommitMsgs) != 0 {
	//			msgs := make([]*consensus.VoteMsg, len(node.MsgBuffer.CommitMsgs))
	//			copy(msgs, node.MsgBuffer.CommitMsgs)
	//
	//			node.MsgDelivery <- msgs
	//		}
	//	}
	//}

	return nil
}

func (node *Node) resolveMsg() {
	for {
		// Get buffered messages from the dispatcher.
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
					fmt.Println(err)
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
						fmt.Println(err)
					}
					// TODO: send err to ErrorChannel
				}
			}
		}
	}
}

func (node *Node) alarmToDispatcher() {
	for {
		node.Count += 1
		time.Sleep(ResolvingTimeDuration)
		//fmt.Println("[Chan]", "FinishChan", "In")
		node.Alarm <- true
		//fmt.Println("[Chan]", "FinishChan", "Out")
	}
}

func (node *Node) resolveRequestMsg(msgs []*consensus.RequestMsg) []error {
	errs := make([]error, 0)
	// Resolve messages
	for _, reqMsg := range msgs {
		//if node.CurrentState.MsgLogs.ReqMsg.Height != reqMsg.Height{
		//	LogFmt("[ErrorHeight]","State",node.CurrentState.MsgLogs.ReqMsg.Height,"msgHeight",reqMsg.Height)
		//	return nil
		//}
		PSLog("node resolveRequestMsg", fmt.Sprintf("%+v", reqMsg))
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
		//if node.CurrentState.MsgLogs.ReqMsg.Height != prePrepareMsg.Height{
		//	LogFmt("[ErrorHeight]","State",node.CurrentState.MsgLogs.ReqMsg.Height,"msgHeight",prePrepareMsg.Height)
		//	return nil
		//}
		PSLog("node resolvePrePrepareMsg", fmt.Sprintf("%+v", prePrepareMsg))
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
		//if node.CurrentState.MsgLogs.ReqMsg.Height != prepareMsg.Height{
		//	LogFmt("[ErrorHeight]","State",node.CurrentState.MsgLogs.ReqMsg.Height,"msgHeight",prepareMsg.Height)
		//	return nil
		//}
		PSLog("node resolvePrepareMsg", fmt.Sprintf("%+v", prepareMsg))
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
		//if node.CurrentState.MsgLogs.ReqMsg.Height != commitMsg.Height{
		//	LogFmt("[ErrorHeight]","State",node.CurrentState.MsgLogs.ReqMsg.Height,"msgHeight",commitMsg.Height)
		//	return nil
		//}
		PSLog("node resolveCommitMsg", fmt.Sprintf("%+v", commitMsg))
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
