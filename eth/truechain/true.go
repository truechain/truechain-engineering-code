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
	"github.com/ethereum/go-ethereum/common"
	"net"
    "math/big"
    "errors"
    
    "golang.org/x/net/context"
    "google.golang.org/grpc"
    "github.com/ethereum/go-ethereum/core/types"
    pb "github.com/ethereum/go-ethereum/eth/truechain"
	// "github.com/ethereum/go-ethereum/accounts"
	// "github.com/ethereum/go-ethereum/ethdb"
	// "github.com/ethereum/go-ethereum/event"
	// "github.com/ethereum/go-ethereum/p2p"
	// "github.com/ethereum/go-ethereum/rpc"
)

type HybridConsensusHelp struct {
}
func (s *HybridConsensusHelp) PutBlock(ctx context.Context, block *pb.TruePbftBlock) (*pb.CommonReply, error) {
    // do something
    return &pb.CommonReply{Message: "success "}, nil
}
func (s *HybridConsensusHelp) ViewChange(ctx context.Context, in *pb.EmptyParam) (*pb.CommonReply, error) {
    // do something
    return &pb.CommonReply{Message: "success "}, nil
}

type PyHybConsensus struct {
}

type TrueHybrid struct {
    address     string
    sdmsize     int
    sdm         []*StandbyInfo
    crpmsg      []*TrueCryptoMsg
}

func (t *TrueHybrid) HybridConsensusHelpInit() {
    port = 17546
    lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &HybridConsensusHelp{})
	// Register reflection service on gRPC server.
	// reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (t *TrueHybrid) MembersNodes(nodes []*TruePbftNode) error{
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        return err
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)
 
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    pbNodes := make([]*pb.TruePbftNode,0,0)
    for _,v := range nodes {
        pbNodes = append(pbNodes,&pb.TruePbftNode{
            Addr:       v.Addr,
            Pubkey:     v.pubkey,
            Privkey:    v.Privkey,
        })
    }
    _, err1 := c.MembersNodes(ctx, &pb.Nodes{Nodes:pbNodes})
    if err1 != nil {
        return err1
    }
    return nil
}
func (t *TrueHybrid) SetTransactions(txs []*types.Transaction){
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        return err
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)
 
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    pbTxs := make([]*pb.Transaction,0,0)
    for _,v := range txs {
        to := make([]byte,0,0)
        if t := v.To(); t != nil {
            to = t.Bytes()
        }
        v,r,s := v.RawSignatureValues()
        pbTxs = append(pbTxs,&pb.Transaction{
            Data:       &pb.TxData{
                AccountNonce:       v.Nonce(),
                Price:              v.GasPrice().Int64(),
                GasLimit:           v.Gas().Int64(),
                Recipient:          to,
                Amount:             v.Value().Int64(),
                Payload:            v.Data(),
                V:                  v.Int64(),
                R:                  r.Int64(),
                S:                  s.Int64(),
            },
        })
    }
    _, err1 := c.SetTransactions(ctx, &pb.Transactions{Txs:pbTxs})
    if err1 != nil {
        return err1
    }
    return nil     
}
func (t *TrueHybrid) Start() error{
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        return err
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    _, err1 := c.Start(ctx, &pb.EmptyParam{})
    if err1 != nil {
        return err1
    }
    return nil    
}
func (t *TrueHybrid) Stop() error{
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        return err
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)
 
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    _, err1 := c.Stop(ctx, &pb.EmptyParam{})
    if err1 != nil {
        return err1
    }   
    return nil
}