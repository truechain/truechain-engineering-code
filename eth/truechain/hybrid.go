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
	"bytes"
	"encoding/gob"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
	// "github.com/ethereum/go-ethereum/ethdb"
	// "github.com/ethereum/go-ethereum/event"
	// "github.com/ethereum/go-ethereum/p2p"
	// "github.com/ethereum/go-ethereum/rpc"
)


type TruePbftNode struct {
	Addr 		string 		// node ip like 127.0.0.1,the port use default
	Pubkey  	string		// 
	Privkey		string  	//
}
type TruePbftBlockHeader struct {
	Number      *big.Int       // block Height out of pbft 
	GasLimit    *big.Int       // gaslimit in block include bonus tx
	GasUsed     *big.Int       // gasused in block
	Time        *big.Int       // generate time
}

type TruePbftBlock struct {
	header       *TruePbftBlockHeader
	Transactions []*types.Transaction		// raw tx（include bonus tx）
	sig		     []*string					// sign with all members
}

type StandbyInfo struct {
	nodeid		string			// the pubkey of the node(nodeid)
	coinbase	string			// the bonus address of miner
	addr		string 			
	port		int
	Height		*big.Int		// block Height who pow success 
	comfire		bool			// the state of the block comfire,default greater 12 like eth
}
type CommitteeMember struct {
	nodeid		string			// the pubkey of the node(nodeid)
	addr		string 			
	port		int
}
type TrueCryptoMsg struct {
	Height		*big.Int
	Msg			[]byte
	Sig 		[]byte
	use			bool
}

func (t *TrueCryptoMsg) Use() bool { return t.use }
func (t *TrueCryptoMsg) SetUse(u bool) { t.use = u }
func (t *TrueCryptoMsg) ToStandbyInfo() *StandbyInfo {
	info := struct StandbyInfo{Height:big.NewInt(0)}
	info.FromByte(t.Msg)
	return &info
}
func (t *StandbyInfo) FromByte(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	to := struct StandbyInfo{
		Height:		big.NewInt(0),
		comfire:	false,
	}
	dec.Decode(to)
	t.nodeid = to.nodeid
	t.coinbase = to.coinbase
	t.addr = to.addr
	t.port = to.port
	t.Height = to.Height
	t.comfire = to.comfire
	return nil
}
func (t *StandbyInfo) ToByte() ([]byte,error) {
	// return common.FromHex(t.ToMsg())
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(t)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}
func (t *TrueCryptoMsg) FromByte(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	to := struct TrueCryptoMsg{
		Height:		big.NewInt(0),
		Msg:		make([]byte,0,0),
		Sig:		make([]byte,0,0),
		use:		false,
	}
	dec.Decode(to)
	t.Height = to.Height
	t.Msg = to.Msg
	t.Sig = to.Sig
	t.use = to.use
	return nil
} 
func (t *TrueCryptoMsg) ToByte() ([]byte,error) {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(t)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
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