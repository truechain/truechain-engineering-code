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
	"time"
)

// candidate Member
type CdMember struct {
	Nodeid		string			// the pubkey of the node(nodeid)
	coinbase	string			// the bonus address of miner
	addr		string 			
	port		int
	Height		*big.Int		// block Height who pow success 
	comfire		bool			// the state of the block comfire,default greater 12 like eth
}
type cdEncryptionMsg struct {
	Height		*big.Int
	Msg			[]byte
	Sig 		[]byte
	use			bool
}
type PbftCdCommittee struct {
	Cm 				[]*CdMember				// confirmed member info 
	VCdCrypMsg	 	[]*cdEncryptionMsg		// verified	candidate Member message(authenticated msg by block comfirm)
	NCdCrypMsg		[]*cdEncryptionMsg		// new candidate Member message(unauthenticated msg by block comfirm)
}

type CommitteeMember struct {
	Nodeid		string			// the pubkey of the node(nodeid)
	addr		string 			
	port		int
}
type PbftCommittee struct {
	No			int				// Committee number
	ct 			time.Time		// current Committee voted time		
	lastt		time.Time		// last Committee voted time
	count		int				// current Committee member Count
	lcount		int				// last Committee member Count
	cmm 		[]*CommitteeMember
	lcmm		[]*CommitteeMember
	sig 		[]string
}

var (
	zero = big.NewInt(0)
)

func (t *cdEncryptionMsg) Use() bool { return t.use }
func (t *cdEncryptionMsg) SetUse(u bool) { t.use = u }
func (t *cdEncryptionMsg) ToStandbyInfo() *CdMember {
	info := CdMember{Height:big.NewInt(0),}
	info.FromByte(t.Msg)
	return &info
}
func (t *cdEncryptionMsg) FromByte(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	to :=  cdEncryptionMsg{
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
func (t *cdEncryptionMsg) ToByte() ([]byte,error) {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(t)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}
func (t *CdMember) FromByte(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	to :=  CdMember{
		Height:		big.NewInt(0),
		comfire:	false,
	}
	dec.Decode(to)
	t.Nodeid = to.Nodeid
	t.coinbase = to.coinbase
	t.addr = to.addr
	t.port = to.port
	t.Height = to.Height
	t.comfire = to.comfire
	return nil
}
func (t *CdMember) ToByte() ([]byte,error) {
	// return common.FromHex(t.ToMsg())
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(t)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}
func (t *CommitteeMember) ToByte() ([]byte,error) {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(t)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}
func (t *CommitteeMember) FromByte(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	to := CommitteeMember{}
	dec.Decode(to)
	t.Nodeid = to.Nodeid
	t.addr = to.addr
	t.port = to.port
	return nil
}

func (t *PbftCommittee) GetCmm() []*CommitteeMember {
	return t.cmm
}
func (t *PbftCommittee) SetCmm(cmm []*CommitteeMember) {
	t.cmm = cmm
}
func (t *PbftCommittee) GetlCmm() []*CommitteeMember {
	return t.lcmm
}
func (t *PbftCommittee) SetlCmm(lcmm []*CommitteeMember) {
	t.lcmm = lcmm
}
func (t *PbftCommittee) GetSig() []string {
	return t.sig
}
func (t *PbftCommittee) SetSig(sig []string) {
	t.sig = sig
}
func (t *PbftCommittee) GetHash() []byte {
	tmp := struct {
		msg1	[]*CommitteeMember
		msg2	[]*CommitteeMember
	}{
		msg1:	t.GetCmm(),
		msg2:	t.GetlCmm(),
	}
	msg := rlpHash(tmp)
	return msg
}

func toByte(e interface{}) ([]byte,error) {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(e)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}
func fromByte(data []byte,to interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	dec.Decode(to)
	return nil
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

