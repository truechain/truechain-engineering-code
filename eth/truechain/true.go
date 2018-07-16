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
    "net"
    "golang.org/x/net/context"
    "github.com/ethereum/go-ethereum/common"
)

const NewBftBlockMsg  = 0x11
var (
    BlockCh = make(chan *TruePbftBlock)
    CdsCh = make(chan []*CdEncryptionMsg)
    CmsCh = make(chan *PbftCommittee)
)

type HybridConsensusHelp struct {
    tt          *TrueHybrid
    *BlockPool
}

func (s *HybridConsensusHelp) setTrue(t *TrueHybrid) {
    s.tt = t
}
func (s *HybridConsensusHelp) getTrue() *TrueHybrid {
    return s.tt
}

func (s *HybridConsensusHelp) PutBlock(ctx context.Context, block *TruePbftBlock) (*CommonReply, error) {
    // do something
    s.tt.AddBlock(block)
    BlockCh <- block
    return &CommonReply{Message: "success "}, nil
}
func (s *HybridConsensusHelp) PutNewSignedCommittee(ctx context.Context, msg *SignCommittee) (*CommonReply, error) {
    err := s.tt.UpdateCommitteeFromPBFTMsg(msg)
    if err != nil {
        return &CommonReply{Message: "fail "}, err
    }
    return &CommonReply{Message: "success "}, err
}
func (s *HybridConsensusHelp) ViewChange(ctx context.Context, in *EmptyParam) (*CommonReply, error) {
    // do something
    m,err := s.tt.Vote(s.tt.GetCommitteeCount())
    if err != nil {
        return &CommonReply{Message: "fail "}, err
    }
    err = s.tt.MembersNodes(m)
    // control py-pbft directy provisional
    if s.tt.InPbftCommittee(m) {
        s.tt.Start()
    } else {
        s.tt.Stop()
    }
    return &CommonReply{Message: "success "}, err
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

func RlpHash(x interface{}) (h common.Hash) {
    return rlpHash(x)
}

func (t *TrueHybrid) SetCommitteeCount(c int) {
	t.setCommitteeCount(c)
}

