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
    "math/big"
    
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
    return &pb.CommonReply{Message: "Hello " + in.Name}, nil
}
func (s *HybridConsensusHelp) ViewChange(ctx context.Context, in *pb.EmptyParam) (*pb.CommonReply, error) {
    // do something
    return &pb.CommonReply{Message: "Hello " + in.Name}, nil
}

type PyHybConsensus struct {
}

type TrueChain struct {
    address    string
}

func (t *TrueChain) HybridConsensusHelpInit() {
    port = 17546
    lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &HybridConsensusHelp{})
	// Register reflection service on gRPC server.
	reflection.Register(s)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func (t *TrueChain) MembersNodes(nodes []*TruePbftNode) error{
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("did not connect: %v", err)
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)
 
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    r, err := c.MembersNodes(ctx, &pb.Nodes{Nodes:nodes})
    if err != nil {
        log.Fatalf("could not greet: %v", err)
    }
    log.Printf("Greeting: %s", r.Message)
}
func (t *TrueChain) SetTransactions(txs []*types.Transaction){
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("did not connect: %v", err)
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)
 
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    r, err := c.SetTransactions(ctx, &pb.Transactions{Txs:txs})
    if err != nil {
        log.Fatalf("could not greet: %v", err)
    }
    log.Printf("Greeting: %s", r.Message)     
}
func (t *TrueChain) Start() error{
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("did not connect: %v", err)
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    r, err := c.Start(ctx, &pb.EmptyParam{})
    if err != nil {
        log.Fatalf("could not greet: %v", err)
    }
    log.Printf("Greeting: %s", r.Message)    
}
func (t *TrueChain) Stop() error{
    // Set up a connection to the server.
    conn, err := grpc.Dial(t.address, grpc.WithInsecure())
    if err != nil {
        log.Fatalf("did not connect: %v", err)
    }
    defer conn.Close()   
    c := pb.NewPyHybConsensusClient(conn)
 
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    r, err := c.Stop(ctx, &pb.EmptyParam{})
    if err != nil {
        log.Fatalf("could not greet: %v", err)
    }
    log.Printf("Greeting: %s", r.Message)     
}