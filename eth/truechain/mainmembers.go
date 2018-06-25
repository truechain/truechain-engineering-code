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
	"encoding/hex"
	"crypto/ecdsa"
   	// "math/big"
	"errors"
	// "bytes"
    
	//"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	// "github.com/ethereum/go-ethereum/p2p"
	// "github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
)

type checkPair struct {
	left	int
	right 	int
}

// all function was not tread-safe
func (t *TrueHybrid) SyncMainMembers() {
	// sync current CommitteeMember 
	// buf := bytes.NewBuffer(nil)
	// for _,v := range t.curCmm {
	// 	data,err := v.ToByte()
	// 	if err != nil {
	// 		// fmt.Println("ToByte error=",err)
	// 		return 
	// 	} else {
	// 		buf.Write(data)
	// 	}
	// }
	// send by p2p network
}
// verify the block which from pbft Committee
func (t *TrueHybrid) CheckBlock(block *TruePbftBlock) error {
	// check with current committee
	all := len(block.Sigs)
	if all == 0 {
		return errors.New("empty...")
	}
	err,useold := checkPbftBlock(t.Cmm.GetCmm(),block)
	if useold {
		erro,_ := checkPbftBlock(t.Cmm.GetlCmm(),block)
		return erro
	} 
	return err
}
// return true means maybe old committee check
func checkPbftBlock(verifier []*CommitteeMember,block *TruePbftBlock) (error,bool) {
	sCount := len(block.Sigs)
	vCount := len(verifier)
	if sCount != vCount {
		return errors.New("not all members sign"),true
	}
	keys := make(map[checkPair]bool)
	msg := rlpHash(block.Txs)
	for i,s := range block.Sigs {
		err,r := verifyMember(verifier,msg,common.FromHex(s))
		if err != nil {
			keys[checkPair{left:i,right:r}] = true
		} else {
			return err,true
		}
	}
	if vCount == len(keys) {
		return nil,false
	} else {
		return errors.New("not all members sign"),true
	}
}
func verifyMember(cc []*CommitteeMember,msg,sig []byte) (error,int) {
	for i,v := range cc {
		pub,err := crypto.SigToPub(crypto.Keccak256(msg),sig)
		if err != nil {
			return err,0
		}
		if v.Nodeid == hex.EncodeToString(crypto.FromECDSAPub(pub)) {
			return nil,i
		}
	} 
	return errors.New("has no one sign..."),0
}
func (t *TrueHybrid) InPbftCommittee() bool {
	_,nodeid,_ := t.getNodeID()
	cmm := t.Cmm.GetCmm()
	for _,v := range cmm {
		if nodeid == v.Nodeid {
			return true
		}
	}
	
	return false
}
// receive the sync message 
func (t *TrueHybrid) ReceiveCommittee(committee *PbftCommittee,from string) {
	// sync all current main committee
	bstart := false
	if t.Cmm == nil {
		t.Cmm = committee
		bstart = t.InPbftCommittee()
	} else {
		// do nothing temporarily
		// remove the standby members
		if t.Cmm.No + 1 == committee.No {
			t.Cmm = committee
			bstart = t.InPbftCommittee()
		}
	}
	if bstart {
		t.Start()
	}
}


func (t *TrueHybrid) getNodeID() (string,string,string) {
	server := t.P2PServer()  // tmp
	ip := server.NodeInfo().IP
	priv := hex.EncodeToString(crypto.FromECDSA(server.PrivateKey))
	pub := hex.EncodeToString(crypto.FromECDSAPub(
		&ecdsa.PublicKey{
			Curve: 	server.PrivateKey.PublicKey.Curve, 
			X: 		big.NewInt(server.PrivateKey.PublicKey.X.Int64()), 
			Y: 		big.NewInt(server.PrivateKey.PublicKey.Y.Int64())}))
	return ip,pub,priv
}
func rlpHash(x interface{}) (h []byte) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
