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
    "io"
    "github.com/truechain/truechain-engineering-code/rlp"
)

const (
    NewBftBlockMsg  = 0x11
)

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

// "external" TxData encoding. used for eth protocol, etc.
type extTxData struct {
    AccountNonce         uint64
    Price                uint64
    GasLimit             uint64
    Recipient            []byte
    Amount               uint64
    Payload              []byte
    V                    uint64
    R                    uint64
    S                    uint64
    Hash                 []byte
}

// EncodeRLP implements rlp.Encoder
func (tx *Transaction) EncodeRLP(w io.Writer) error {
    return rlp.Encode(w, extTxData{
        AccountNonce:   tx.Data.AccountNonce,
        Price:       tx.Data.Price,
        GasLimit:    tx.Data.GasLimit,
        Recipient:  tx.Data.Recipient,
        Amount:     tx.Data.Amount,
        Payload:    tx.Data.Payload,
        V:      tx.Data.V,
        R:      tx.Data.R,
        S:      tx.Data.S,
        Hash:   tx.Data.Hash,
    })
}

func (tx *Transaction) DecodeRLP(s *rlp.Stream) error {
    var eb extTxData
    if err := s.Decode(&eb); err != nil {
        return err
    }
    tx.Data.AccountNonce, tx.Data.Price, tx.Data.GasLimit = eb.AccountNonce, eb.Price, eb.GasLimit
    tx.Data.Recipient = eb.Recipient
    tx.Data.Amount = eb.Amount
    tx.Data.Payload = eb.Payload
    tx.Data.V, tx.Data.R, tx.Data.S, tx.Data.Hash = eb.V, eb.R, eb.V, eb.Hash
    return nil
}

// EncodeRLP implements rlp.Encoder
func (txs *Transactions) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, txs.Txs)
}

func (txs *Transactions) DecodeRLP(s *rlp.Stream) error {
	err := s.Decode(txs.Txs)
	return err
}

// "external" header encoding. used for eth protocol, etc.
type extheader struct {
	Number               uint64
	GasLimit             uint64
	GasUsed              uint64
	Time                 uint64
}

// EncodeRLP implements rlp.Encoder
func (header *TruePbftBlockHeader) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extheader{
		Number:   header.Number,
		GasLimit: header.GasLimit,
		GasUsed:  header.GasUsed,
		Time:     header.Time,
	})
}

func (header *TruePbftBlockHeader) DecodeRLP(s *rlp.Stream) error {
	var eh extheader
	if err := s.Decode(&eh); err != nil {
		return err
	}
	header.Number = eh.Number
	header.GasLimit = eh.GasLimit
	header.GasUsed = eh.GasUsed
	header.Time = eh.Time
	return nil
}

// "external" block encoding. used for eth protocol, etc.
type extblock struct {
	Header               *TruePbftBlockHeader
	Txs                  *Transactions
	Sigs                 []string
}

func (block *TruePbftBlock) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header:   block.Header,
		Txs: block.Txs,
		Sigs:  block.Sigs,
	})
}

func (block *TruePbftBlock) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	if err := s.Decode(&eb); err != nil {
		return err
	}
	block.Header = eb.Header
	block.Txs = eb.Txs
	block.Sigs = eb.Sigs
	return nil
}