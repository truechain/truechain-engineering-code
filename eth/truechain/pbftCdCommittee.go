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
	"strconv"
	"math/big"
	"sort"
    "errors"
    
    //"github.com/ethereum/go-ethereum/core/types"
	//"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core"
)

const (
	NeedVerified_BlockNum int64 =12
)
var (
	zero = big.NewInt(0)
)

type CdMember struct {
	Nodeid		string			// the pubkey of the node(nodeid)
	Coinbase	string			// the bonus address of miner
	Addr		string
	Port		int
	Height		*big.Int		// block Height who pow success
	Comfire		bool			// the state of the block comfire,default greater 12 like eth
}

type PbftCdCommittee struct {
	Cm 				[]*CdMember				// confirmed member info
	VCdCrypMsg	 	[]*CdEncryptionMsg		// verified	candidate Member message(authenticated msg by block comfirm)
	NCdCrypMsg		[]*CdEncryptionMsg		// new candidate Member message(unauthenticated msg by block comfirm)
}

func (pcc *PbftCdCommittee) addMsgToNCdCrypMsg(msg *CdEncryptionMsg){
	if msg == nil {
		return
	}
	pcc.NCdCrypMsg = append(pcc.NCdCrypMsg,msg)
	if len(pcc.NCdCrypMsg) > 1000 {
		pcc.NCdCrypMsg = Removemgs(pcc.NCdCrypMsg, 0)
	}
}


func (pcd *PbftCdCommittee) addMsgToCm(msg *CdEncryptionMsg,pccSize int) error {
	// CdEncryptionMsg convert into CdMember
	cdMember := msg.convertMsgToCdMember()
	if cdMember == nil {
		return errors.New("Wrong CrytoMsg")
	}
	if len(pcd.Cm)>=pccSize  {
		pcd.Cm = pcd.Cm[len(pcd.Cm)-pccSize+1:]
	}
	pcd.Cm = append(pcd.Cm,cdMember)
	return nil
}

//find Specify height of CdEncryptionMsg from
// VCdCrypMsg(Verified	Candidate CdEncryptionMsg)
func (pcc *PbftCdCommittee) findMsg(height *big.Int) *CdEncryptionMsg {
	for _,v := range pcc.VCdCrypMsg {
		if v.Height.Cmp(height) == 0 {
			return v
		}
	}
	return nil
}

// check the crypmsg whether  blockchain has the block
// if yes do some work
func (pcc *PbftCdCommittee) checkTmpMsg(bc *core.BlockChain) {
	for {
		if len(pcc.NCdCrypMsg) <= 0 {
			break
		}
		msg,pos := minMsg(pcc.NCdCrypMsg,true)
		res := verityMsg(msg,bc)
		// 1 represent  the miner block of msg has been put into blockchain
		if res == 1 {
			pcc.VCdCrypMsg = append(pcc.VCdCrypMsg,msg)//add the msg into t.Cdm.VCdCrypMsg
			pcc.NCdCrypMsg = Removemgs(pcc.NCdCrypMsg,pos)//remove the msg from t.Cdm.NCdCrypMsg
		} else {
			break
		}
	}
}

// crpmsg was be check and insert to the standbyqueue
// when the blockchain has the block. which through 12 block verified
func (pcc *PbftCdCommittee) insertToSDM(bc *core.BlockChain,sdmsize int) error {
	m,_ := minMsg(pcc.VCdCrypMsg,false)
	if m == nil {
		return errors.New("no minMsg,msglen=" + strconv.Itoa(len(pcc.VCdCrypMsg)))
	}
	cur := big.NewInt(bc.CurrentHeader().Number.Int64())
	if cur.Abs(m.Height).Cmp(big.NewInt(NeedVerified_BlockNum)) >= 0 {
		//remove the CdEncryptionMsg from VCdCrypMsg
		pcc.removeUnuseMsg(m.Height)

		m.SetUse(true)
		res := verityMsg(m,bc)
		if res == 1 {
			pcc.addMsgToCm(m,sdmsize)// add the CdEncryptionMsg into standbyqueue
		}
	}
	return nil
}

// remove the msg from VCdCrypMsg that has same height and it was used
func (pcc *PbftCdCommittee) removeUnuseMsg(num *big.Int) {
	pos := make([]int,0,0)
	for i,v := range pcc.VCdCrypMsg {
		if v.Height.Cmp(num) == 0 && !v.GetUse(){
			pos = append(pos,i)
		}
	}
	for _,i := range pos {
		pcc.VCdCrypMsg =Removemgs(pcc.VCdCrypMsg,i)
	}
}

//find the cmm from the cdm
func (pcc *PbftCdCommittee) matchCommitteeMembers(comm []*CommitteeMember) []int {
	pos := make([]int,0,0)

	for _,v := range comm {
		//find index from t.Cdm by Nodeid
		i := pcc.posFromCm(v.Nodeid)
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
func (pcc *PbftCdCommittee) posFromCm(nid string) int {
	for i,v := range pcc.Cm {
		if v.Nodeid == nid {
			return i
		}
	}
	return -1
}

//vote PbftCommittee from PbftCdCommittee
func (pcd *PbftCdCommittee) voteFromCd(num int) ([]*CommitteeMember,error) {
	vv := make([]*CommitteeMember,0,0)
	i := 0
	for _,v := range pcd.Cm {
		if i >= num {
			break
		} else {
			vv = append(vv,&CommitteeMember{
				Nodeid:		v.Nodeid,
				Addr:		v.Addr,			
				Port:		uint32(v.Port),			
			})
			i++
		}
	}
	return vv,nil
}

func (pcd *PbftCdCommittee) syncStandbyMembers() {
	// sync crypmsg
	CdsCh <-pcd.VCdCrypMsg
}

//handle the msg of miner  CdEncryptionMsg
func (pcc *PbftCdCommittee) handleReceiveSdmMsg(msg *CdEncryptionMsg,bc *core.BlockChain) {
	if msg == nil {
		return
	}
	m,_ := minMsg(pcc.VCdCrypMsg,true)
	if m != nil {
		if m.Height.Cmp(msg.Height) <= 0 {
			return 
		}
	}
	if existMsg(msg,pcc.VCdCrypMsg){
		return 
	}
	// verify the msg when the block is on
	res := verityMsg(msg,bc)
	if res == 1 {
		pcc.VCdCrypMsg = append(pcc.VCdCrypMsg,msg)//?
	} else if res == 0 {
		pcc.NCdCrypMsg = append(pcc.NCdCrypMsg,msg)
		if len(pcc.NCdCrypMsg ) > 1000 {
			pcc.NCdCrypMsg = Removemgs(pcc.NCdCrypMsg, 0)
		}
	}
}

//remove the cmm from the cdm
func (pcc *PbftCdCommittee) handleRemoveFromCommittee(cmm *PbftCommittee){
	// match the committee number 
	// simple remove(one by one)....
	pos := pcc.matchCommitteeMembers(cmm.GetCmm())
	if pos != nil {
		for i := len(pos) -1; i > -1; i-- {
			pcc.Cm = append(pcc.Cm[:pos[i]], pcc.Cm[pos[i]+1:]...)//?
		}
		// update the committee number
	} else {
		// the sdm was dirty,must be update
	}
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