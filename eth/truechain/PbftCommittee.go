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
	"encoding/hex"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"sync"
	"log"
)

type CommitteeMember struct {
	Nodeid		string			// the pubkey of the node(nodeid)
	Addr		string
	Port		uint32
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

type checkPair struct {
	left	int
	right 	int
}

func CreateCommittee(t *TrueHybrid) {
	curCmm := make([]*CommitteeMember,0,0)
	curCmmCount := 1
	_,pubKey,privateKey := t.GetNodeID()
	cc := CommitteeMember{
		Addr:			"127.0.0.1", //"192.168.190.1",
		Port:			16745,
		Nodeid:			pubKey,
	}
	curCmm = append(curCmm,&cc)

	cmm := &PbftCommittee{
		No:				1,
		Ct:				time.Now(),
		Lastt:			time.Now(),
		Count:			curCmmCount,
		Lcount:			0,
		Comm:			curCmm,
		Lcomm:			nil,
		Sig:			make([]string,0,0),
	}
	//obtain Sig
	sig := cmm.Sig
	bp,_ := hex.DecodeString(privateKey)
	priv,_ := crypto.ToECDSA(bp)
	k,err:= crypto.Sign(cmm.GetHash(),priv)
	if err != nil{
		log.Panic(err)
	}
	sig = append(sig,common.ToHex(k))
	cmm.Sig = sig
	t.Cmm = cmm
}

func GetPbftNodesFromCfg() []*CommitteeMember {
	// filename will be change
	filename := "./config.json"
	result,err := ReadCfg(filename)
	if err != nil {
		return nil
	}
	dbs := result["database"].([]interface{})
	cm := make([]*CommitteeMember ,0,0)

	for _, v := range dbs {
		vv := v.(map[string]interface{})
		nid := vv["Nodeid"].(string)
		addr := vv["Addr"].(string)
		port := vv["Port"].(float64)
		cm = append(cm,&CommitteeMember{
			Nodeid:         nid,
			Addr:           addr,
			Port:           uint32(port),
		})
	}
	return  cm
}

//put PbftCommittee into channel
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

// verify the block which generate from pbft Committee
func (t *TrueHybrid) CheckBlock(block *TruePbftBlock) error {
	// check with current committee
	if len(block.Sigs) == 0 {
		return errors.New("empty...")
	}
	curCmm := t.getCurrentCmm()
	err,useold := checkPbftBlock(curCmm,block)//verified pass return nil,false
	if useold {//??
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
	return errors.New("has no one sign"),0
}

// verify the current TrueHybrid in cmms
func (t *TrueHybrid) InPbftCommittee(cmms []*CommitteeMember) bool {
	_,nodeid,_ := t.GetNodeID()
	if cmms == nil {
		cmms = t.getCurrentCmm()
	}
	for _,v := range cmms {
		if nodeid == v.Nodeid {
			return true
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
//convert SignCommittee into PbftCommittee
func (t *TrueHybrid) MakeNewCommittee(msg *SignCommittee) (*PbftCommittee,error) {
	// elect new []*CommitteeMember
	ctms,err := t.Vote(t.Config.CmmCount)
    if err != nil {
        return nil,err
	}
	now := time.Now()
	cmm := PbftCommittee{
		No:				1,
		Ct:				now,
		Lastt:			now,
		Count:			len(ctms),
		Lcount:			0,
		Comm:			ctms,
		Lcomm:			nil,
		Sig:			msg.GetSigs(),
	}
	hash := common.ToHex(cmm.GetHash())
	if hash != msg.GetMsg() {
		return nil,errors.New("hash member was not equal")
	}
	return &cmm,nil
}

/**
receive new pbftCommittee
remove the old  pbftCommittee from pbftcdCommittee
 */
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
		cmm,err := t.MakeNewCommittee(msg)//pbftCommittee
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
		t.Cmm.setCurrentCmm(cmm.GetCmm(),t.CmmLock)
		//?
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

func (t *TrueHybrid) GetCommitteeMembers() []string {
	cmm := t.getCurrentCmm()
	addrs := make([]string, len(cmm))
	for i, value := range cmm {
		addrs[i] = value.Addr
	}
	return addrs
}

/////get and set method /////
func (t *TrueHybrid) getCmm() *PbftCommittee {
	t.CmmLock.Lock()
	defer t.CmmLock.Unlock()
	return t.Cmm
}

func (t *TrueHybrid) getCurrentCmm() []*CommitteeMember {
	t.CmmLock.Lock()
	defer t.CmmLock.Unlock()
	if t.Cmm != nil {
		return t.Cmm.Comm
	}
	return nil
}
func (t *TrueHybrid) getLastCmm() []*CommitteeMember {
	t.CmmLock.Lock()
	defer t.CmmLock.Unlock()
	if t.Cmm != nil {
		return t.Cmm.Lcomm
	}
	return nil
}
func (pcm *PbftCommittee) setCurrentCmm(cc []*CommitteeMember,mutex *sync.Mutex) {
	if cc == nil {
		return
	}
	mutex.Lock()
	defer mutex.Unlock()
	if pcm != nil {
		pcm.SetlCmm(pcm.GetCmm())
		pcm.SetCmm(cc)
	}
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