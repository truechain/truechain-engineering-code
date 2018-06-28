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
    "math/big"
    "time"
    "net"

    "golang.org/x/net/context"
    "google.golang.org/grpc"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/core"
    "github.com/ethereum/go-ethereum/p2p"
)

type HybridConsensusHelp struct {
    tt          *TrueHybrid
    *BlockPool
    rw p2p.MsgReadWriter
}

const NewBftBlockMsg  = 0x11

func (s *HybridConsensusHelp) PutBlock(ctx context.Context, block *TruePbftBlock) (*CommonReply, error) {
    // do something
    s.AddBlock(block)
    p2p.Send(s.rw, NewBftBlockMsg, []interface{}{block})
    return &CommonReply{Message: "success "}, nil
}
func (s *HybridConsensusHelp) PutNewSignedCommittee(ctx context.Context, msg *SignCommittee) (*CommonReply, error) {
    cmm,err := s.getTrue().MakeNewCommittee(msg)
    if cmm == nil {
        return &CommonReply{Message: "fail "}, err
    }
    s.getTrue().UpdateLocalCommittee(cmm,true) 
    return &CommonReply{Message: "success "}, nil
}
func (s *HybridConsensusHelp) ViewChange(ctx context.Context, in *EmptyParam) (*CommonReply, error) {
    // do something
    m,err := s.getTrue().Vote(s.getTrue().GetCommitteeCount())
    if err != nil {
        return &CommonReply{Message: "fail "}, err
    }
    err = s.getTrue().MembersNodes(m)
    return &CommonReply{Message: "success "}, err
}
func (s *HybridConsensusHelp) setTrue(t *TrueHybrid) {
    s.tt = t
}
func (s *HybridConsensusHelp) getTrue() *TrueHybrid {
    return s.tt
}


type Config struct {
    ServerAddress           string          // local GRPC server address,ip:port 
    ClientAddress           string          // Pbft Node address,ip:port
    CmmCount                int             // amount of Pbft Committee Members 
    Sdmsize                 int             // amount of Pbft Standby Members
}

type TrueHybrid struct {
    Config

    quit        bool
    Cmm         *PbftCommittee              // Pbft Committee
    Cdm         *PbftCdCommittee            // Pbft candidate Member
    grpcServer  *grpc.Server
    p2pServer   *p2p.Server
    bc          *core.BlockChain
    Bp          *BlockPool
    CMSchache   []*PbftCommittee
    CDSchache   []*PbftCdCommittee
}

func New() *TrueHybrid {
    // init TrueHybrid object
    // read cfg
    cfg := Config {
        ServerAddress:          ":17546",
        ClientAddress:          "127.0.0.1:17545",
        CmmCount:               5,
        Sdmsize:                1000,
    }
    tc := &TrueHybrid{
        Config:             cfg,
        quit:               true,
        Cmm:                nil,
        Cdm:                nil,
        p2pServer:          nil,
        grpcServer:         nil,
    }
    tc.Cdm = &PbftCdCommittee{
        Cm:             make([]*CdMember,0,0),
        VCdCrypMsg:     make([]*cdEncryptionMsg,0,0),
        NCdCrypMsg:     make([]*cdEncryptionMsg,0,0),
    }
    tc.Bp = &BlockPool{
        blocks: 		make([]*TruePbftBlock,0,0),
        TrueTxsCh:      make( chan core.NewTxsEvent),
        th:				nil,
    }
    return tc
}
func (t *TrueHybrid) StartTrueChain(b *core.BlockChain) error {
    t.bc = b
    t.quit = false
    t.grpcServer = grpc.NewServer()
    go HybridConsensusHelpInit(t)
    go SyncWork(t)
    go TrueChainWork(t)
    return nil
}
func (t *TrueHybrid) StopTrueChain() {
    t.quit = true
    if t.grpcServer != nil {
        t.grpcServer.Stop()
    }
    time.Sleep(2*time.Second)
}
func (t *TrueHybrid) GetCommitteeCount() int {
    return t.CmmCount
}
func (t *TrueHybrid) GetSdmsize() int {
    return t.Sdmsize
}
func (t *TrueHybrid) GetCommitteeMembers() []string {
    cmm := t.Cmm.GetCmm()
    addrs := make([]string, len(cmm))
    for i, value := range cmm {
        addrs[i] = value.addr
    }
    return addrs
}
func HybridConsensusHelpInit(t *TrueHybrid) {
    lis, err := net.Listen("tcp", t.ServerAddress)
    if err != nil {
        //log.Fatalf("failed to listen: %v", err)
        return
    }
    rpcServer := HybridConsensusHelp{}
    rpcServer.setTrue(t)
    RegisterHybridConsensusHelpServer(t.GrpcServer(), &rpcServer)
    // Register reflection service on gRPC server.
    // reflection.Register(t.GrpcServer())
    if err := t.GrpcServer().Serve(lis); err != nil {
        //log.Fatalf("failed to serve: %v", err)
    }
}
func TrueChainWork(t *TrueHybrid) {
    t.StandbyWork()
}
func SyncWork(t *TrueHybrid) {
    for {
        if t.quit {
            break
        }
        t.SyncMainMembers()
        t.SyncStandbyMembers()
        for i:=0;i<30;i++ {
            if t.quit {
                return
            }
            time.Sleep(1*time.Second)
        }
    }
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
    _,lnodeid,priv := t.getNodeID()
    pbNodes := make([]*TruePbftNode,0,0)
    for _,v := range nodes {
        n := TruePbftNode{
            Addr:       v.addr,
            Pubkey:     v.Nodeid,
            Privkey:    "",
        }
        if lnodeid == v.Nodeid {
            n.Privkey = priv
        }
        pbNodes = append(pbNodes,&n)
    }
    _, err1 := c.MembersNodes(ctx, &Nodes{Nodes:pbNodes})
    if err1 != nil {
        return err1
    }
    return nil
}
func (t *TrueHybrid) SetTransactions(txs []*types.Transaction) error {
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.ClientAddress, grpc.WithInsecure())
    if err != nil {
        return err
    }
    defer conn.Close()

    c := NewPyHybConsensusClient(conn)
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    pbTxs := make([]*Transaction,0,0)
    for _,vv := range txs {
        to := make([]byte,0,0)
        if tt := vv.To(); tt != nil {
            to = tt.Bytes()
        }
        v,r,s := vv.RawSignatureValues()
        pbTxs = append(pbTxs,&Transaction{
            Data:       &TxData{
                AccountNonce:       vv.Nonce(),
                Price:              vv.GasPrice().Int64(),
                GasLimit:           new(big.Int).SetUint64(vv.Gas()).Int64(),
                Recipient:          to,
                Amount:             vv.Value().Int64(),
                Payload:            vv.Data(),
                V:                  v.Int64(),
                R:                  r.Int64(),
                S:                  s.Int64(),
            },
        })
    }
    _, err1 := c.SetTransactions(ctx, &Transactions{Txs:pbTxs})
    if err1 != nil {
        return err1
    }
    return nil
}
func (t *TrueHybrid) Start() error{
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.ClientAddress, grpc.WithInsecure())
    if err != nil {
        return err
    }
    defer conn.Close()
    _,_,priv := t.getNodeID()
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