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
	"sync"
)

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












type checkPair struct {
	left	int
	right 	int
}

// all function was not tread-safe
func (cmm *PbftCommittee) SyncMainMembers(mutex *sync.Mutex) {
	// send by p2p network
	mutex.Lock()
	defer mutex.Unlock()
	if cmm == nil {
		return
	}
	// don't consider the channel cost time
	CmsCh <- cmm
	return
}
// verify the block which from pbft Committee
func (t *TrueHybrid) CheckBlock(block *TruePbftBlock) error {
	// check with current committee
	all := len(block.Sigs)
	if all == 0 {
		return errors.New("empty...")
	}
	curCmm := t.getCurrentCmm()
	err,useold := checkPbftBlock(curCmm,block)
	if useold {
		lCmm := t.getLastCmm()
		erro,_ := checkPbftBlock(lCmm,block)
		return erro
	} 
	return err
}
// return true means maybe old committee check
func checkPbftBlock(verifier []*CommitteeMember,block *TruePbftBlock) (error,bool) {
	if verifier == nil {
		return errors.New("no committee"),true
	}
	sCount := len(block.Sigs)
	vCount := len(verifier)
	if sCount != vCount {
		return errors.New("not all members sign"),true
	}
	keys := make(map[checkPair]bool)
	msg := rlpHash(block.Txs)
	for i,s := range block.Sigs {
		err,r := verifyBlockMembersSign(verifier,msg[:],common.FromHex(s))
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
func verifyBlockMembersSign(cc []*CommitteeMember,msg,sig []byte) (error,int) {
	for i,v := range cc {
		// pub,err := crypto.SigToPub(crypto.Keccak256(msg),sig)
		pub,err := crypto.SigToPub(msg,sig)
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
	_,nodeid,_ := t.GetNodeID()
	if cmms == nil {
		cmm := t.getCurrentCmm()
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
	cmm := t.getCmm()
	if cmm == nil {
		bstart = t.InPbftCommittee(nil)
		t.UpdateLocalCommittee(committee,false)	
	} else {
		if cmm.No + 1 == committee.No {
			// new committee message 
			if t.verifyCommitteeMsg(committee,committee.GetHash()) {
				bstart = t.InPbftCommittee(nil)
				t.UpdateLocalCommittee(committee,false)	
			}
		} else if cmm.No == committee.No {
			if !t.sameCommittee(committee) {
				if t.Cdm.VerifyCommitteeFromSdm(committee) {
					bstart = t.InPbftCommittee(nil)
					t.UpdateLocalCommittee(committee,false)				
				}
			} 
		} else if committee.No - cmm.No > 1 {
			// the local was older, then can't verify the committee message
			// simple handle
			if t.Cdm.VerifyCommitteeFromSdm(committee) {
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
func (t *TrueHybrid) verifyCommitteeMsg(cmm *PbftCommittee,hash []byte) bool {
	keys := make(map[checkPair]bool)
	// msg := cmm.GetHash()
	oldCmm := t.getCmm()
	sigs := cmm.GetSig()

	for i,s := range sigs {
		err,r := verifyBlockMembersSign(oldCmm.GetCmm(),hash,common.FromHex(s))
		if err == nil {
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
	curCmm := t.getCmm()
	if curCmm.No == cmm.No {
		if common.ToHex(curCmm.GetHash()) == common.ToHex(cmm.GetHash()) {
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
	now := time.Now()
	cmm := PbftCommittee{
		No:				1,
		Ct:				now,
		Lastt:			now,
		Count:			len(m),
		Lcount:			0,
		Comm:			m,
		Lcomm:			nil,
		Sig:			msg.GetSigs(),
	}
	hash := common.ToHex(cmm.GetHash())
	if hash != msg.GetMsg() {
		return nil,errors.New("hash member was not equal")
	}
	return &cmm,nil
}
func (t *TrueHybrid) UpdateLocalCommittee(cmm *PbftCommittee,sync bool) {
	if cmm == nil {
		return
	}
	{
		t.CmmLock.Lock()
		defer t.CmmLock.Unlock()
		t.Cmm = cmm
	}
	t.RemoveFromCommittee(cmm)
	if sync {
		t.Cmm.SyncMainMembers(t.CmmLock)
	}
}
func (t *TrueHybrid) UpdateCommitteeFromPBFTMsg(msg *SignCommittee) error {
	cur := t.getCmm()
	if cur == nil {
		cmm,err := t.MakeNewCommittee(msg)
		if err == nil {
			t.UpdateLocalCommittee(cmm,true)
		}
		return err
	} else {
		// verify the msg from pbft 
		m,err := t.Vote(t.GetCommitteeCount())
		if err != nil {
			return err
		}
		newNo := cur.No + 1
		cmm := PbftCommittee{
			No:				newNo,
			Ct:				time.Now(),
			Lastt:			cur.Ct,
			Count:			len(m),
			Lcount:			cur.Count,
			Comm:			m,
			Lcomm:			cur.GetCmm(),
			Sig:			msg.GetSigs(),
		}
		hash := common.ToHex(cmm.GetHash())
		if hash != msg.GetMsg() {
			return errors.New("hash member was not equal")
		}
		if !t.verifyCommitteeMsg(&cmm,cmm.GetHash()) {
			return errors.New("verify the sign from committee members was failed")
		}
		t.setCurrentCmm(cmm.GetCmm())
		curCmm := t.getCmm()
		curCmm.No = newNo
		curCmm.Ct = cmm.Ct
		curCmm.Lastt = cur.Lastt
		curCmm.Count = cur.Count
		curCmm.Lcount = cur.Lcount
		curCmm.Sig = cur.Sig
	}
	return nil
}
func (pcc *PbftCdCommittee) VerifyCommitteeFromSdm(cmm *PbftCommittee) bool {
	// committee members come from sdm
	// simple verify  
	oPos := pcc.matchCommitteeMembers(cmm.GetlCmm())
	if oPos == nil {
		return false
	}
	nPos := pcc.matchCommitteeMembers(cmm.GetCmm())
	if nPos == nil {
		return false
	}
	if nPos[0] > oPos[len(oPos)-1] {
		return true
	}
	return false
}
func (t *TrueHybrid) GetNodeID() (string,string,string) {
	server := t.P2PServer() 
	ip := server.NodeInfo().IP
	priv := hex.EncodeToString(crypto.FromECDSA(server.PrivateKey))
	pub := hex.EncodeToString(crypto.FromECDSAPub(
		&ecdsa.PublicKey{
			Curve: 	server.PrivateKey.Curve,
			X: 		new(big.Int).Set(server.PrivateKey.X),
			Y: 		new(big.Int).Set(server.PrivateKey.Y)}))
	return ip,pub,priv
}

func (t *TrueHybrid) getCmm() *PbftCommittee {
	t.CmmLock.Lock()
	defer t.CmmLock.Unlock()
	return t.Cmm
}

func (t *TrueHybrid) getCurrentCmm() []*CommitteeMember {
	t.CmmLock.Lock()
	defer t.CmmLock.Unlock()
	if t.Cmm != nil {
		return t.Cmm.GetCmm()
	}
	return nil
}
func (t *TrueHybrid) getLastCmm() []*CommitteeMember {
	t.CmmLock.Lock()
	defer t.CmmLock.Unlock()
	if t.Cmm != nil {
		return t.Cmm.GetlCmm()
	}
	return nil
}
func (t *TrueHybrid) setCurrentCmm(cc []*CommitteeMember) {
	if cc == nil {
		return
	}
	t.CmmLock.Lock()
	defer t.CmmLock.Unlock()
	if t.Cmm != nil {
		t.Cmm.SetlCmm(t.Cmm.GetCmm())
		t.Cmm.SetCmm(cc)
	}
}
func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

