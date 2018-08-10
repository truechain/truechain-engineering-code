package network

import (
	"math/big"
	"net/http"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"encoding/json"
	"fmt"
	"bytes"
)

const (
	Agree 			int = iota
	Against 	
	ActionFecth 	
	ActionBroadcast 
)

type ActionIn struct {
	ac 		int
	id 		*big.Int
}
var ActionChan chan *ActionIn = make(chan *ActionIn)

type SignedVoteMsg struct{
	FastHeight 	*big.Int
	Result     	uint        // 0--agree,1--against
	Sign       	[]byte      // sign for fastblock height + hash + result + Pk
}

type ConsensusHelp interface {
	GetRequest(id *big.Int) (*consensus.RequestMsg,error)
	CheckMsg(msg *consensus.RequestMsg) (bool)
	ReplyResult(msg *consensus.RequestMsg,res uint) (bool)
	SignMsg(msg *consensus.RequestMsg) (*SignedVoteMsg)	
	Broadcast(height *big.Int)
}

type Server struct {
	url string
	node *Node
	ID	*big.Int
	help ConsensusHelp
}

func NewServer(nodeID string,id *big.Int,help ConsensusHelp) *Server {
	node := NewNode(nodeID)
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
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

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
	server.node.GetReply(&msg)
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
	ActionChan <- &ActionIn{1,server.ID}
}
func (server *Server) PutRequest(msg *consensus.RequestMsg) {
	server.node.MsgEntrance <- msg
}

func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	http.Post("http://" + url, "application/json", buff)
}