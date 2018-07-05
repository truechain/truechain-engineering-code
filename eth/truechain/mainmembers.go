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
	"time"
	"math/big"
	"encoding/hex"
	"crypto/ecdsa"
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
	// send by p2p network
	t.CMScache = append(t.CMScache,t.Cmm)
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
		err,r := verifyMembersSign(verifier,msg[:],common.FromHex(s))
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
func verifyMembersSign(cc []*CommitteeMember,msg,sig []byte) (error,int) {
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
func (t *TrueHybrid) InPbftCommittee(cmms []*CommitteeMember) bool {
	_,nodeid,_ := t.getNodeID()
	if cmms == nil {
		cmm := t.Cmm.GetCmm()
		for _,v := range cmm {
			if nodeid == v.Nodeid {
				return true
			}
		}
	} else {
		for _,v :=range cmms {
			if nodeid == v.Nodeid {
				return true
			}
		}
	}
	return false
}
// receive the sync message 
func (t *TrueHybrid) ReceiveCommittee(committee *PbftCommittee,from string) {
	// sync all current main committee
	// remove the standby members
	bstart := false
	if t.Cmm == nil {
		bstart = t.InPbftCommittee(nil)
		t.UpdateLocalCommittee(committee,false)	
	} else {
		if t.Cmm.No + 1 == committee.No {
			// new committee message 
			if t.verifyCommitteeMsg(committee) {
				bstart = t.InPbftCommittee(nil)
				t.UpdateLocalCommittee(committee,false)	
			}
		} else if t.Cmm.No == committee.No {
			if !t.sameCommittee(committee) {
				if t.VerifyCommitteeFromSdm(committee) {
					bstart = t.InPbftCommittee(nil)
					t.UpdateLocalCommittee(committee,false)				
				}
			} 
		} else if committee.No - t.Cmm.No > 1 {
			// the local was older, then can't verify the committee message
			// simple handle
			if t.VerifyCommitteeFromSdm(committee) {
				bstart = t.InPbftCommittee(nil)
				t.UpdateLocalCommittee(committee,false)		
			}
		}
	}
	if bstart {
		t.Start()
	}
}
// verify the new committee members message when committee replacement
func (t *TrueHybrid) verifyCommitteeMsg(cmm *PbftCommittee) bool {
	keys := make(map[checkPair]bool)
	msg := cmm.GetHash()
	oldCmm := t.Cmm
	sigs := cmm.GetSig()

	for i,s := range sigs {
		err,r := verifyMembersSign(oldCmm.GetCmm(),msg,common.FromHex(s))
		if err != nil {
			keys[checkPair{left:i,right:r}] = true
		} else {
			return false
		}
	}
	all := len(oldCmm.GetCmm())
	if len(keys) >= (all/3)*2{
		return true
	} 
	return false
}
func (t *TrueHybrid) sameCommittee(cmm *PbftCommittee) bool {
	if t.Cmm.No == cmm.No {
		if common.ToHex(t.Cmm.GetHash()) == common.ToHex(cmm.GetHash()) {
			return true
		}
	}
	return false
}
func (t *TrueHybrid) MakeNewCommittee(msg *SignCommittee) (*PbftCommittee,error) {
	m,err := t.Vote(t.GetCommitteeCount())
    if err != nil {
        return nil,err
	}

	curNo := 1
	if t.Cmm != nil {
		curNo = t.Cmm.No + 1
	}
	cmm := PbftCommittee{
		No:				curNo,
		ct:				time.Now(),
		lastt:			t.Cmm.ct,
		count:			len(m),
		lcount:			t.Cmm.count,
		cmm:			m,
		lcmm:			t.Cmm.cmm,
		sig:			msg.GetSigs(),
	}
	hash := common.ToHex(cmm.GetHash())
	if hash != msg.GetMsg() {
		return nil,errors.New("hash member was not equal")
	}
	return &cmm,nil
}
func (t *TrueHybrid) UpdateLocalCommittee(cmm *PbftCommittee,sync bool) {
	t.Cmm = cmm
	t.RemoveFromCommittee(cmm)
	if sync {
		t.SyncMainMembers()
	}
}
func (t *TrueHybrid) VerifyCommitteeFromSdm(cmm *PbftCommittee) bool {
	// committee members come from sdm
	// simple verify  
	oPos := t.matchCommitteeMembers(cmm.GetlCmm())
	if oPos == nil {
		return false
	}
	nPos := t.matchCommitteeMembers(cmm.GetCmm())
	if nPos == nil {
		return false
	}
	if nPos[0] > oPos[len(oPos)-1] {
		return true
	}
	return false
}
func (t *TrueHybrid) getNodeID() (string,string,string) {
	server := t.P2PServer() 
	ip := server.NodeInfo().IP
	priv := hex.EncodeToString(crypto.FromECDSA(server.PrivateKey))
	pub := hex.EncodeToString(crypto.FromECDSAPub(
		&ecdsa.PublicKey{
			Curve: 	server.PrivateKey.PublicKey.Curve, 
			X: 		big.NewInt(server.PrivateKey.PublicKey.X.Int64()), 
			Y: 		big.NewInt(server.PrivateKey.PublicKey.Y.Int64())}))
	return ip,pub,priv
}
func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
