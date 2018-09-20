package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"github.com/truechain/truechain-engineering-code/pbftserver/lock"
	"math/big"
	"net/http"
	"time"
)

type Server struct {
	url        string
	Node       *Node
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
	//server := &Server{ID: id, help: help}
	server := &Server{ID: new(big.Int).Set(id), help: help}
	node := NewNode(nodeID, verify, server, addrs, id)
	server.Node = node
	server.url = node.NodeTable[nodeID]
	server.server = &http.Server{
		Addr: server.url,
	}
	server.ActionChan = make(chan *consensus.ActionIn)
	server.setRoute()
	return server
}
func (server *Server) Start(work func(cid *big.Int, acChan <-chan *consensus.ActionIn)) {
	go server.startHttpServer()
	go work(server.ID, server.ActionChan)
}
func (server *Server) startHttpServer() {
	if err := server.server.ListenAndServe(); err != nil {
		fmt.Println(err)
		return
	}
}
func (server *Server) Stop() {
	if server.server != nil {
		server.server.Close()
	}
	ac := &consensus.ActionIn{
		AC:     consensus.ActionFinish,
		ID:     new(big.Int).Set(common.Big0),
		Height: new(big.Int).Set(common.Big0),
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
	lock.PSLog("getReq in")
	var msg consensus.RequestMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	lock.PSLog("getReq msg:", fmt.Sprintf("%+v", msg))
	server.Node.MsgEntrance <- &msg
}

func (server *Server) getPrePrepare(writer http.ResponseWriter, request *http.Request) {
	lock.PSLog("getPrePrepare in")
	var msg consensus.PrePrepareMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	lock.PSLog("getPrePrepare msg:", fmt.Sprintf("%+v", msg))
	server.Node.MsgEntrance <- &msg
}

func (server *Server) getPrepare(writer http.ResponseWriter, request *http.Request) {
	lock.PSLog("getPrepare in")
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
	msg.Height = tmp.Height
	lock.PSLog("getPrepare msg:", fmt.Sprintf("%+v", msg))
	server.Node.MsgEntrance <- &msg
}

func (server *Server) getCommit(writer http.ResponseWriter, request *http.Request) {
	lock.PSLog("getCommit in")
	var msg consensus.VoteMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	lock.PSLog("getCommit msg:", fmt.Sprintf("%+v", msg))
	server.Node.MsgEntrance <- &msg
}

func (server *Server) getReply(writer http.ResponseWriter, request *http.Request) {
	lock.PSLog("getReply in")
	var msg consensus.ReplyMsg
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		fmt.Println(err)
		return
	}
	lock.PSLog("getReply msg:", fmt.Sprintf("%+v", msg))
	server.Node.GetReply(&msg)
}

func (server *Server) PutRequest(msg *consensus.RequestMsg) {
	lock.PSLog("PutRequest in", fmt.Sprintf("%+v", msg))
	server.Node.MsgEntrance <- msg
	height := big.NewInt(msg.Height)
	ac := &consensus.ActionIn{
		AC:     consensus.ActionBroadcast,
		ID:     new(big.Int).Set(server.ID),
		Height: height,
	}
	go func() {
		server.ActionChan <- ac
	}()
}
func (server *Server) ConsensusFinish() {
	// start to fetch
	lock.PSLog("ConsensusFinish in")
	ac := &consensus.ActionIn{
		AC:     consensus.ActionFecth,
		ID:     new(big.Int).Set(server.ID),
		Height: new(big.Int).Set(common.Big0),
	}
	server.ActionChan <- ac
}

func send(url string, msg []byte) {
	buff := bytes.NewBuffer(msg)
	for i := 0; i < 3; i++ {
		_, e := http.Post("http://"+url, "application/json", buff)
		if e != nil {
			lock.PSLog("[POSTERROR]", e.Error())
			time.Sleep(time.Second * 1)
		} else {
			return
		}
	}
}
