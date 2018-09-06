package network

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"github.com/truechain/truechain-engineering-code/pbftserver/lock"
)

func LogMsg(msg interface{}) {
	switch msg.(type) {
	case *consensus.RequestMsg:
		reqMsg := msg.(*consensus.RequestMsg)
		lock.PSLog(fmt.Sprintf("[REQUEST] ClientID: %s, Timestamp: %d, Operation: %s\n", reqMsg.ClientID, reqMsg.Timestamp, reqMsg.Operation))
	case *consensus.PrePrepareMsg:
		prePrepareMsg := msg.(*consensus.PrePrepareMsg)
		lock.PSLog(fmt.Sprintf("[PREPREPARE] ClientID: %s, Operation: %s, SequenceID: %d\n", prePrepareMsg.RequestMsg.ClientID, prePrepareMsg.RequestMsg.Operation, prePrepareMsg.SequenceID))
	case *consensus.VoteMsg:
		voteMsg := msg.(*consensus.VoteMsg)
		if voteMsg.MsgType == consensus.PrepareMsg {
			lock.PSLog(fmt.Sprintf("[PREPARE] NodeID: %s\n", voteMsg.NodeID))
		} else if voteMsg.MsgType == consensus.CommitMsg {
			lock.PSLog(fmt.Sprintf("[COMMIT] NodeID: %s\n", voteMsg.NodeID))
		}
	}
}

func LogStage(stage string, isDone bool) {
	if isDone {
		lock.PSLog(fmt.Sprintf("[STAGE-DONE] %s\n", stage))
	} else {
		lock.PSLog(fmt.Sprintf("[STAGE-BEGIN] %s\n", stage))
	}
}
