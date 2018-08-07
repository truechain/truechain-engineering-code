/*
Copyright (c) 2018 TrueChain Foundation
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package truechain

import (
	"golang.org/x/net/context"
	"math/big"
	"encoding/hex"
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/core/types"
	"sync"
	"google.golang.org/grpc"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/core"
	"time"
	"errors"
)

const(
	fileName="./config.json"
)



// the lock will be change to rwlock
type TrueHybrid struct {
	Config
	TrueChannel
	Cmm         *PbftCommittee              // Pbft Committee
	CmmLock     *sync.Mutex
	Cdm         *PbftCdCommittee            // Pbft candidate Member
	grpcServer  *grpc.Server
	p2pServer   *p2p.Server
	bc          *core.BlockChain
	Bp          *BlockPool
}

type Config struct {
	ServerAddress           string          // local GRPC server address,ip:port
	ClientAddress           string          // Pbft Node address,ip:port
	CmmCount                int             // amount of Pbft Committee Members
	Sdmsize                 int             // amount of Pbft Standby Members
}
type TrueChannel struct {
	quit        chan bool
	vote        chan int
	voteRes     chan *VoteResult
	cdRecv      chan *CdEncryptionMsg
	removeCd    chan *PbftCommittee
	cdCheck     *time.Ticker
	cdSync      *time.Ticker
}
type VoteResult struct {
	err 		error
	cmm 		[]*CommitteeMember
}

func NewTrueHybrid() *TrueHybrid {
	// read cfg
	config := Config {
		ServerAddress:          ":17546",
		ClientAddress:         "129.168.190.1:17545", // "127.0.0.1:17545",
		CmmCount:               5,
		Sdmsize:                1000,
	}
	channel := TrueChannel{
		quit:           make(chan bool),
		vote:           make(chan int),
		voteRes:        make(chan *VoteResult),
		cdRecv:         make(chan *CdEncryptionMsg,5),
		removeCd:       make(chan *PbftCommittee),
		cdCheck:        time.NewTicker(30*time.Second),
		cdSync:         time.NewTicker(10*time.Second),
	}
	t := &TrueHybrid{
		Config:             config,
		TrueChannel:        channel,
		Cmm:                nil,
		CmmLock:            new(sync.Mutex),
		Cdm:                nil,
		p2pServer:          nil,
		grpcServer:         nil,
	}
	t.Cdm = &PbftCdCommittee{
		Cm:             make([]*CdMember,0,0),
		VCdCrypMsg:     make([]*CdEncryptionMsg,0,0),
		NCdCrypMsg:     make([]*CdEncryptionMsg,0,0),
	}
	t.Bp = &BlockPool{
		blocks: 		make([]*TruePbftBlock,0,0),
		TrueTxsCh:      make( chan core.NewTxsEvent),
		th:				nil,
	}
	return t
}

//entrance
func (t *TrueHybrid) StartTrueChain(b *core.BlockChain) error {
	t.bc = b
	t.grpcServer = grpc.NewServer()
	// generate PbftCommittee(t.Cmm)
	CreateCommittee(t)//t.Cmm
	if GetFirstStart() {
		ctm := GetPbftNodesFromCfg()
		if ctm != nil {
			t.MembersNodes(ctm)
			//CreateCommittee(t)
			t.Start()
		}
	}
	go HybridConsensusHelpInit(t)
	time.Sleep(1*time.Second)
	go TrueChainWork(t)
	return nil
}

func TrueChainWork(t *TrueHybrid) {
	t.worker()
}

func (t *TrueHybrid) worker() {
	cdm :=t.Cdm
	cmm :=t.Cmm
	for {
		select{
		case <-t.TrueChannel.cdCheck.C://30*time.Second
			cdm.insertToSDM(t.bc,t.Sdmsize)
			cdm.checkTmpMsg(t.bc)
		case <-t.TrueChannel.cdSync.C://10*time.Second
			//put pcd.VCdCrypMsg and pbftCommittee into channel
			cdm.syncStandbyMembers()
			cmm.SyncMainMembers(t.CmmLock)
		case n	:=	<-t.vote://Vote(
			res,err := cdm.voteFromCd(n)
			t.voteRes<-&VoteResult{
				err:	err,
				cmm:	res,
			}
		case msg:=<-t.TrueChannel.cdRecv:
			cdm.handleReceiveSdmMsg(msg,t.bc)
		case cmm :=<-t.TrueChannel.removeCd:
			cdm.handleRemoveFromCommittee(cmm)
		case <-t.quit:
			return
		}
	}
}


func (t *TrueHybrid) StopTrueChain() {
	t.quit <- true
	if t.grpcServer != nil {
		t.grpcServer.Stop()
	}
	time.Sleep(2*time.Second)
}

//get property of "enabled" from file of config.json
func GetFirstStart() bool{
	result,err := ReadCfg(fileName)
	if err != nil {
		return false
	}
	en := result["enabled"].(bool)
	return en
}

func (t *TrueHybrid) ReceiveSdmMsg(msg *CdEncryptionMsg) {
	t.TrueChannel.cdRecv <- msg
}

func (t *TrueHybrid) Vote(num int) ([]*CommitteeMember,error) {
	t.vote <- num
	select {
	case res := <-t.voteRes:
		return res.cmm,res.err
	case <-t.quit:
	}
	return nil,errors.New("vote failed")
}

func (t *TrueHybrid) RemoveFromCommittee(cmm *PbftCommittee) {
	t.TrueChannel.removeCd <- cmm
}

//implement interface begin
func (t *TrueHybrid) Start() error{
	// Set up a connection to the server.
	conn, err := grpc.Dial(t.ClientAddress, grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()
	_,_,priv := t.GetNodeID()
	c := NewPyHybConsensusClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err1 := c.Start(ctx, &BftPrivateKey{Pkey:priv})
	if err1 != nil {
		return err1
	}
	return nil
}
func (t *TrueHybrid) Stop() error{
	// Set up a connection to the server.
	conn, err := grpc.Dial(t.ClientAddress, grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()
	c := NewPyHybConsensusClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err1 := c.Stop(ctx, &EmptyParam{})
	if err1 != nil {
		return err1
	}
	return nil
}

func (t *TrueHybrid) MembersNodes(nodes []*CommitteeMember) error{
	// Set up a connection to the server.
	conn, err := grpc.Dial(t.ClientAddress, grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()
	c := NewPyHybConsensusClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// _,lnodeid,priv := t.getNodeID()
	pbNodes := make([]*TruePbftNode,0,0)
	for _,node := range nodes {
		pbNode := &TruePbftNode{
			Addr:       node.Addr,
			Nodeid:     node.Nodeid,
			Port:       node.Port,
		}
		pbNodes = append(pbNodes,pbNode)
	}

	_, err1 := c.MembersNodes(ctx, &Nodes{Nodes:pbNodes})
	if err1 != nil {
		return err1
	}
	return nil
}
func (t *TrueHybrid) SetTransactions(th *TrueHybrid,txs []*types.Transaction) {
	// Set up a connection to the server.

	//go ConvTransaction(th,txs)

	//test
	//conn, err := grpc.Dial(t.ClientAddress, grpc.WithInsecure())
	//if err != nil {
	//  return err
	//}
	//defer conn.Close()
	//
	//c := NewPyHybConsensusClient(conn)
	//ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	//defer cancel()
	//
	//pbTxs := make([]*Transaction,0,0)
	//for _,vv := range txs {
	//  to := make([]byte,0,0)
	//  if tt := vv.To(); tt != nil {
	//      to = tt.Bytes()
	//  }
	//  v,r,s := vv.RawSignatureValues()
	//  pbTxs = append(pbTxs,&Transaction{
	//      Data:       &TxData{
	//          AccountNonce:       vv.Nonce(),
	//          Price:              vv.GasPrice().Int64(),
	//          GasLimit:           new(big.Int).SetUint64(vv.Gas()).Int64(),
	//          Recipient:          to,
	//          Amount:             vv.Value().Int64(),
	//          Payload:            vv.Data(),
	//          V:                  v.Int64(),
	//          R:                  r.Int64(),
	//          S:                  s.Int64(),
	//      },
	//  })
	//}
	//_, err1 := c.SetTransactions(ctx, &Transactions{Txs:pbTxs})
	//if err1 != nil {
	//  return err1
	//}
	//return nil
}
//implement interface end

//add block to BlockPool
func (t *TrueHybrid) AddBlock(block *TruePbftBlock) {
	//Judging committee members
	if(t.CheckBlock(block)==nil){
		t.Bp.blocks = append(t.Bp.blocks, block)
	}
	//self.blocks = append(self.blocks, block)
	t.GetBp().JoinEth()
}

//get IP 、publicKey and privateKey from P2PServer
func (t *TrueHybrid) GetNodeID() (string,string,string) {
	server := t.P2PServer()
	ip := server.NodeInfo().IP
	priv := hex.EncodeToString(crypto.FromECDSA(server.PrivateKey))
	pub := hex.EncodeToString(crypto.FromECDSAPub(
		&ecdsa.PublicKey{
			Curve: 	server.PrivateKey.Curve,
			X: 		new(big.Int).Set(server.PrivateKey.X),
			Y: 		new(big.Int).Set(server.PrivateKey.Y)}))
	return ip,pub,priv
}


/////get and set method
func (t *TrueHybrid) setCommitteeCount(c int)  {
	t.CmmCount = c
}
func (t *TrueHybrid) GetCommitteeCount() int {
	return t.CmmCount
}
func (t *TrueHybrid) GetSdmsize() int {
	return t.Sdmsize
}

func (t *TrueHybrid) P2PServer() *p2p.Server {
	return t.p2pServer
}
func (t *TrueHybrid) SetP2PServer(s *p2p.Server) {
	t.p2pServer = s
}
func (t *TrueHybrid) GrpcServer() *grpc.Server {
	return t.grpcServer
}
func (t *TrueHybrid) GetBp() *BlockPool {
	return t.Bp
}

// type HybridConsensus interface {
// 	// main chain set node to the py-pbft
// 	MembersNodes(nodes []*TruePbftNode) error
// 	// main chain set node to the py-pbft
// 	SetTransactions(txs []*types.Transaction) error

// 	PutBlock(block *TruePbftBlock)  error

// 	ViewChange() error

// 	Start() error

// 	Stop() error
// 	// tx validation in py-pbft, Temporary slightly
// }

