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
	"strconv"
	"crypto/ecdsa"
	"math/big"
	"sort"
    "errors"
    
    //"github.com/ethereum/go-ethereum/core/types"
	//"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core"
)

const NeedVerified_BlockNum int64 =12

func (t *TrueHybrid) ReceiveSdmMsg(msg *CdEncryptionMsg) {
	t.cdRecv <- msg
}
func (t *TrueHybrid) Vote(num int) ([]*CommitteeMember,error) {
	t.vote <- num
	select {
	case res := <-t.voteRes:
		return res.cmm,res.err
	case <-t.quit:
	}
	return nil,errors.New("vote failed")
}
func (t *TrueHybrid) RemoveFromCommittee(cmm *PbftCommittee) {
	t.removeCd <- cmm
}
///////////////////////////////////////////////////////////////
func (t *TrueHybrid) add(msg *CdEncryptionMsg) error {
	// CdEncryptionMsg convert into CdMember
	node := msg.ToStandbyInfo()
	if node == nil {
		return errors.New("Wrong CrytoMsg")
	}
	// verfiy and add
	if len(t.Cdm.Cm)>=t.Sdmsize  {
		//t.Cdm.Cm = append(t.Cdm.Cm[:0],t.Cdm.Cm[1:]...)
		t.Cdm.Cm = t.Cdm.Cm[t.Sdmsize-len(t.Cdm.Cm)+1:]
	}
	t.Cdm.Cm = append(t.Cdm.Cm,node)
	return nil
}

//find Specify height of CdEncryptionMsg from VCdCrypMsg(Verified	Candidate CdEncryptionMsg)
func (t *TrueHybrid) findMsg(height *big.Int) *CdEncryptionMsg {
	for _,v := range t.Cdm.VCdCrypMsg {
		if v.Height.Cmp(height) == 0 {
			return v
		}
	}
	return nil
}
// check the crypmsg whether  blockchain has the block
// if yes do some work
func (t *TrueHybrid) checkTmpMsg() {
	for {
		if len(t.Cdm.NCdCrypMsg) <= 0 {
			break
		}
		msg,pos := minMsg(t.Cdm.NCdCrypMsg,true)
		// 1 represent  the miner block of msg has been put into blockchain
		res := verityMsg(msg,t.bc)
		if res == 1 {
			//add the msg into t.Cdm.VCdCrypMsg
			t.Cdm.VCdCrypMsg = append(t.Cdm.VCdCrypMsg,msg)
			//remove the msg from t.Cdm.NCdCrypMsg
			t.Cdm.NCdCrypMsg = t.removemgs(t.Cdm.NCdCrypMsg,pos)
		} else {
			break
		}	
	}
	return
}
// crpmsg was be check and insert to the standbyqueue
// when the blockchain has the block. which through 12 block verified
func (t *TrueHybrid) insertToSDM() error {
	m,_ := minMsg(t.Cdm.VCdCrypMsg,false)
	if m == nil {
		return errors.New("no minMsg,msglen=" + strconv.Itoa(len(t.Cdm.VCdCrypMsg)))
	}
	cur := big.NewInt(t.bc.CurrentHeader().Number.Int64())
	if cur.Abs(m.Height).Cmp(big.NewInt(NeedVerified_BlockNum)) >= 0 {
		//remove the CdEncryptionMsg from VCdCrypMsg
		t.removeUnuseMsg(m.Height)
		m.SetUse(true)
		res := verityMsg(m,t.bc)
		if res == 1 {
			// add the CdEncryptionMsg into standbyqueue
			t.add(m)
		}
	}
	return nil
}
// remove the msg that has same height and it was used
func (t *TrueHybrid) removeUnuseMsg(num *big.Int) {
	pos := make([]int,0,0)
	for i,v := range t.Cdm.VCdCrypMsg {
		if v.Height.Cmp(num) == 0 && !v.GetUse(){
			pos = append(pos,i)
		}
	}
	for _,i := range pos {
		t.Cdm.VCdCrypMsg = t.removemgs(t.Cdm.VCdCrypMsg,i)
	}
}
func (t *TrueHybrid) removemgs(msg []*CdEncryptionMsg,i int) []*CdEncryptionMsg {
    return append(msg[:i], msg[i+1:]...)
}

func (t *TrueHybrid) matchCommitteeMembers(comm []*CommitteeMember) []int {
	pos := make([]int,0,0)

	for _,v := range comm {
		//find index from t.Cdm by Nodeid
		i := t.posFromCm(v.Nodeid)
		if i != -1 {
			pos = append(pos,i)
		}
	}
	sort.Ints(pos[:])
	c1 := len(comm)
	c2 := len(pos)
	if c1 != c2 || c1 != (pos[c2-1]-pos[0]+1) {//?
		return nil
	}
	return pos
}
//find  Nodeid from standbyqueue by Nodeid
func (t *TrueHybrid) posFromCm(nid string) int {
	for i,v := range t.Cdm.Cm {
		if v.Nodeid == nid {
			return i
		}
	}
	return -1
}
func (t *TrueHybrid) worker() {
	for {
		select{
		case <-t.cdCheck.C://30*time.Second
			t.insertToSDM()
			t.checkTmpMsg()
		case <-t.cdSync.C://10*time.Second
			t.syncStandbyMembers()
			t.SyncMainMembers()
		case n:=<-t.vote://Vote(
			res,err := t.voteFromCd(n)
			t.voteRes<-&VoteResult{
				err:	err,
				cmm:	res,
			}
		case msg:=<-t.cdRecv:
			t.handleReceiveSdmMsg(msg)
		case cmm :=<-t.removeCd:
			t.handleRemoveFromCommittee(cmm)
		case <-t.quit:
			return
		}
	}
}
//vote PbftCommittee from PbftCdCommittee
func (t *TrueHybrid) voteFromCd(num int) ([]*CommitteeMember,error) {
	vv := make([]*CommitteeMember,0,0)
	i := 0
	for _,v := range t.Cdm.Cm {
		if i >= num {
			break
		} else {
			vv = append(vv,&CommitteeMember{
				Nodeid:		v.Nodeid,
				Addr:		v.Addr,			
				Port:		v.Port,			
			})
			i++
		}
	}
	return vv,nil
}
func (t *TrueHybrid) syncStandbyMembers() {
	// sync crypmsg
	CdsCh <-t.Cdm.VCdCrypMsg
}
//handle the msg of miner  CdEncryptionMsg
func (t *TrueHybrid) handleReceiveSdmMsg(msg *CdEncryptionMsg) {
	if msg == nil {
		return
	}
	m,_ := minMsg(t.Cdm.VCdCrypMsg,true)
	if m != nil {
		if m.Height.Cmp(msg.Height) <= 0 {
			return 
		}
	}
	if existMsg(msg,t.Cdm.VCdCrypMsg){
		return 
	}
	// verify the msg when the block is on
	res := verityMsg(msg,t.bc)
	if res == 1 {
		t.Cdm.VCdCrypMsg = append(t.Cdm.VCdCrypMsg,msg)//？
	} else if res == 0 {
		t.Cdm.NCdCrypMsg = append(t.Cdm.NCdCrypMsg,msg)
		if len(t.Cdm.NCdCrypMsg ) > 1000 {
			t.Cdm.NCdCrypMsg = t.removemgs(t.Cdm.NCdCrypMsg, 0)
		}
	}
}
//remove the cmm from the cdm
func (t *TrueHybrid) handleRemoveFromCommittee(cmm *PbftCommittee){
	// match the committee number 
	// simple remove(one by one)....
	pos := t.matchCommitteeMembers(cmm.GetCmm())
	if pos != nil {
		for i := len(pos) -1; i > -1; i-- {
			t.Cdm.Cm = append(t.Cdm.Cm[:pos[i]], t.Cdm.Cm[pos[i]+1:]...)//?
		}
		// update the committee number
	} else {
		// the sdm was dirty,must be update
	}
}

////////////////////////////////////////////////////////////////////////
//find the min height of CdEncryptionMsg  from  crpmsg
// use=true include msg which was used
func minMsg(crpmsg []*CdEncryptionMsg,use bool) (*CdEncryptionMsg,int) {
	if len(crpmsg) <= 0 {
		return nil,0
	}
	min := crpmsg[0].Height
	pos := 0
	for ii,v := range crpmsg {
		if use {
			//1 means min>v.Height
			if min.Cmp(v.Height) == 1 {
				min = v.Height
				pos = ii
			}
		} else {
			if crpmsg[pos].GetUse() == true {
				min = v.Height
				pos = ii
			}
			if min.Cmp(v.Height) == 1 {
				min = v.Height
				pos = ii
			}
		}
	}
	if use {
		return crpmsg[pos],pos
	} else {
		if crpmsg[pos].GetUse() {
			return nil,0
		} else {
			return crpmsg[pos],pos
		}
	}
}
//See if msg is in the msgs set
func existMsg(msg *CdEncryptionMsg,msgs []*CdEncryptionMsg) bool {
	for _,v := range msgs {
		if v.Height.Cmp(msg.Height) != 0{
			continue
		}
		if len(msg.Msg) != len(v.Msg) || len(msg.Sig) != len(v.Sig) {
			continue
		}
		if bytes.Compare(msg.Msg,v.Msg) == 0 && bytes.Compare(msg.Sig,v.Sig) == 0{
			return true
		}
	}
	return false
}
// convert CdMember into  CdEncryptionMsg
func MakeSignedStandbyNode(n *CdMember,priv *ecdsa.PrivateKey) (*CdEncryptionMsg,error) {
	cmsg := CdEncryptionMsg{
		Height:		n.Height,
		Msg:		make([]byte,0,0),
		Sig:		make([]byte,0,0),
		Use:		false,
	}
	var err error
	cmsg.Msg,err = n.ToByte()
	if err != nil {
		return nil,err
	}
	cmsg.Sig,err = crypto.Sign(cmsg.Msg[:32],priv)
	if err != nil {
		return nil,err
	}
	return &cmsg,nil
}
// verify whether  BlockChain include the CdEncryptionMsg
// 0 -- not ready; 1 -- success; -1 -- fail
func verityMsg(msg *CdEncryptionMsg,bc *core.BlockChain) int {
	if msg.Sig == nil || msg.Msg == nil || msg.Height.Cmp(zero) <= 0 {
		return -1
	}
	if bc == nil {
		return 0
	}
	// find the coinbase address from the heigth
	header := bc.GetHeaderByNumber(msg.Height.Uint64())
	if header == nil {
		return 0
	}
	coinbase := header.Coinbase.String()
	pub,err := crypto.SigToPub(msg.Msg[:32],msg.Sig)
	if err != nil {
		return -1
	}
	addr := crypto.PubkeyToAddress(*pub).String()
	if addr == coinbase {
		return 1
	}
	return -1
}
