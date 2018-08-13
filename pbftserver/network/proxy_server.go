package network

import (
	"math/big"
	"net/http"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
	"encoding/json"
	"fmt"
	"bytes"
)


type Server struct {
	url string
	node *Node
	ID	*big.Int
	help consensus.ConsensusHelp
}

func NewServer(nodeID string,id *big.Int,help consensus.ConsensusHelp,
	verify consensus.ConsensusVerify,addrs []*types.CommitteeNode) *Server {
	if len(addrs) <= 0{
		return nil
	}
	node := NewNode(nodeID,verify,addrs)
	server := &Server{node.NodeTable[nodeID], node,id,help}

	server.setRoute()

	return server
}

func (server *Server) Start() {
	fmt.Printf("Server will be started at %s...\n", server.url)
	if err := http.ListenAndServe(server.url, nil); err != nil {
		fmt.Println(err)
		return
	}
}
func (server *Server) Stop(){
	// do nothing
}
func (server *Server) setRoute() {
	http.HandleFunc("/req", server.getReq)
	http.HandleFunc("/preprepare", server.getPrePrepare)
	http.HandleFunc("/prepare", server.getPrepare)
	http.HandleFunc("/commit", server.getCommit)
	http.HandleFunc("/reply", server.getReply)
}

func (server *Server) getReq(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.RequestMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getPrePrepare(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.PrePrepareMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getPrepare(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.VoteMsg
	var tmp consensus.StorgePrepareMsg
	err := json.NewDecoder(request.Body).Decode(&tmp)
	if err != nil {
		fmt.Println(err)
		return
	}
	msg.Digest,msg.NodeID,msg.ViewID = tmp.Digest,tmp.NodeID,tmp.ViewID
	msg.SequenceID,msg.MsgType = tmp.SequenceID,tmp.MsgType
	msg.Pass = nil
	server.node.MsgEntrance <- &msg
}

func (server *Server) getCommit(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.VoteMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
}

func (server *Server) getReply(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.ReplyMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	// server.node.GetReply(&msg)
	server.handleResult(&msg)
}
func (server *Server) handleResult(msg *consensus.ReplyMsg) {
	var res uint = 0
	if msg.NodeID == "Executed" {
		res = 1
	}
	if msg.ViewID == server.node.CurrentState.ViewID {
		server.help.ReplyResult(server.node.CurrentState.MsgLogs.ReqMsg,res)
	} else {
		// wrong state
	}
	height := big.NewInt(msg.Height)
	ac := &consensus.ActionIn{
		AC:		consensus.ActionBroadcast,
		ID:		server.ID,
		Height:	height,
	}
	consensus.ActionChan <- ac
}
func (server *Server) PutRequest(msg *consensus.RequestMsg) {
	server.node.MsgEntrance <- msg
}

func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	http.Post("http://" + url, "application/json", buff)
}