package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"math/big"
	"net/http"
)

type Server struct {
	url        string
	node       *Node
	ID         *big.Int
	help       consensus.ConsensusHelp
	server     *http.Server
	ActionChan chan *consensus.ActionIn
}

func NewServer(nodeID string, id *big.Int, help consensus.ConsensusHelp,
	verify consensus.ConsensusVerify, addrs []*types.CommitteeNode) *Server {
	if len(addrs) <= 0 {
		return nil
	}
	node := NewNode(nodeID, verify, addrs)

	server := &Server{url: node.NodeTable[nodeID], node: node, ID: id, help: help}
	server.server = &http.Server{
		Addr: server.url,
	}
	server.ActionChan = make(chan *consensus.ActionIn)
	server.setRoute()
	return server
}
func (server *Server) Start(work func(acChan <-chan *consensus.ActionIn)) {
	go server.startHttpServer()
	go work(server.ActionChan)
}
func (server *Server) startHttpServer() {
	if err := server.server.ListenAndServe(); err != nil {
		fmt.Println(err)
		return
	}
}
func (server *Server) Stop() {
	// do nothing
	if server.server != nil {
		server.server.Close()
	}
	ac := &consensus.ActionIn{
		AC:     consensus.ActionFinish,
		ID:     common.Big0,
		Height: common.Big0,
	}
	server.ActionChan <- ac
}
func (server *Server) setRoute() {
	mux := http.NewServeMux()
	mux.HandleFunc("/req", server.getReq)
	mux.HandleFunc("/preprepare", server.getPrePrepare)
	mux.HandleFunc("/prepare", server.getPrepare)
	mux.HandleFunc("/commit", server.getCommit)
	mux.HandleFunc("/reply", server.getReply)
	server.server.Handler = mux
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
	msg.Digest, msg.NodeID, msg.ViewID = tmp.Digest, tmp.NodeID, tmp.ViewID
	msg.SequenceID, msg.MsgType = tmp.SequenceID, tmp.MsgType
	msg.Pass = nil
	server.node.MsgEntrance <- &msg
}

func (server *Server) getCommit(writer http.ResponseWriter, request *http.Request) {
	//fmt.Println("proxy commit start")
	var msg consensus.VoteMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	server.node.MsgEntrance <- &msg
	//fmt.Println("proxy commit end")
}

func (server *Server) getReply(writer http.ResponseWriter, request *http.Request) {
	var msg consensus.ReplyMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	server.node.GetReply(&msg)
}

func (server *Server) PutRequest(msg *consensus.RequestMsg) {
	// server.node.Broadcast(msg, "/req")
	server.node.MsgEntrance <- msg
	height := big.NewInt(msg.Height)
	ac := &consensus.ActionIn{
		AC:     consensus.ActionBroadcast,
		ID:     server.ID,
		Height: height,
	}
	go func() {
		server.ActionChan <- ac
	}()
}

func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	go http.Post("http://"+url, "application/json", buff)
}
