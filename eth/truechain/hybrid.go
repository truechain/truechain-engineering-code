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
	Coinbase	string			// the bonus address of miner
	Addr		string 			
	Port		int
	Height		*big.Int		// block Height who pow success 
	Comfire		bool			// the state of the block comfire,default greater 12 like eth
}
type CdEncryptionMsg struct {
	Height		*big.Int
	Msg			[]byte
	Sig 		[]byte
	Use			bool
}
type PbftCdCommittee struct {
	Cm 				[]*CdMember				// confirmed member info 
	VCdCrypMsg	 	[]*CdEncryptionMsg		// verified	candidate Member message(authenticated msg by block comfirm)
	NCdCrypMsg		[]*CdEncryptionMsg		// new candidate Member message(unauthenticated msg by block comfirm)
}

type CommitteeMember struct {
	Nodeid		string			// the pubkey of the node(nodeid)
	Addr		string 			
	Port		int
}
type PbftCommittee struct {
	No			int				// Committee number
	Ct 			time.Time		// current Committee voted time		
	Lastt		time.Time		// last Committee voted time
	Count		int				// current Committee member Count
	Lcount		int				// last Committee member Count
	Comm 		[]*CommitteeMember
	Lcomm		[]*CommitteeMember
	Sig 		[]string
}

var (
	zero = big.NewInt(0)
)

func (t *CdEncryptionMsg) GetUse() bool { return t.Use }
func (t *CdEncryptionMsg) SetUse(u bool) { t.Use = u }
func (t *CdEncryptionMsg) ToStandbyInfo() *CdMember {
	info := CdMember{Height:big.NewInt(0),}
	info.FromByte(t.Msg)
	return &info
}
func (t *CdEncryptionMsg) FromByte(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	to :=  CdEncryptionMsg{}
	dec.Decode(&to)
	t.Height = to.Height
	t.Msg = to.Msg
	t.Sig = to.Sig
	t.Use = to.Use
	return nil
} 
func (t *CdEncryptionMsg) ToByte() ([]byte,error) {
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
	to :=  CdMember{}
	dec.Decode(&to)
	t.Nodeid = to.Nodeid
	t.Coinbase = to.Coinbase
	t.Addr = to.Addr
	t.Port = to.Port
	t.Height = to.Height
	t.Comfire = to.Comfire
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
	dec.Decode(&to)
	t.Nodeid = to.Nodeid
	t.Addr = to.Addr
	t.Port = to.Port
	return nil
}

func (t *PbftCommittee) GetCmm() []*CommitteeMember {
	return t.Comm
}
func (t *PbftCommittee) SetCmm(cmm []*CommitteeMember) {
	t.Comm = cmm
}
func (t *PbftCommittee) GetlCmm() []*CommitteeMember {
	return t.Lcomm
}
func (t *PbftCommittee) SetlCmm(lcmm []*CommitteeMember) {
	t.Lcomm = lcmm
}
func (t *PbftCommittee) GetSig() []string {
	return t.Sig
}
func (t *PbftCommittee) SetSig(sig []string) {
	t.Sig = sig
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
	return msg[:]
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

