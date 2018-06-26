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
	"time"
	"strconv"
	"crypto/ecdsa"
    "math/big"
    "errors"
    
    //"github.com/ethereum/go-ethereum/core/types"
	//"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core"
)
func (t *TrueHybrid) GetCryMsg() []*TrueCryptoMsg {
	return t.crpmsg
}
// all functions of sdm not thread-safe
func (t *TrueHybrid) add(msg *TrueCryptoMsg) error {
	node := msg.ToStandbyInfo()
	if node == nil {
		return errors.New("Wrong CrytoMsg")
	}
	// verfiy and add 
    if len(t.sdm) >= t.Sdmsize {
		t.sdm = append(t.sdm[:0],t.sdm[1:]...)
	} 	
	t.sdm = append(t.sdm,node)
	return nil
}
func (t *TrueHybrid) findMsg(h *big.Int) *TrueCryptoMsg {
	for _,v := range t.crpmsg {
		if v.Height.Cmp(h) == 0 {
			return v
		}
	}
	return nil
}

func (t *TrueHybrid) AddMsg(msg *TrueCryptoMsg) {
	m,_ := minMsg(t.GetCryMsg(),true)
	if m.Height.Cmp(msg.Height) <= 0 || existMsg(msg,t.crpmsg){
		return 
	}
	// verify the msg when the block is on
	res := verityMsg(msg,t.bc)
	if res == 1 {
		t.crpmsg = append(t.crpmsg,msg)
	} else if res == 0 {
		t.crptmp = append(t.crptmp,msg)
	}
}
func (t *TrueHybrid) Vote(num int) ([]*CommitteeMember,error) {
	vv := make([]*CommitteeMember,0,0)
	i := 0
	for _,v := range t.sdm {
		if i >= num {
			break
		} else {
			vv = append(vv,&CommitteeMember{
				Nodeid:		v.Nodeid,
				addr:		v.addr,			
				port:		v.port,			
			})
			i++
		}
	}
	return vv,nil
}
// check the crypmsg when blockchain has the block
func (t *TrueHybrid) checkTmpMsg() {
	for {
		if len(t.crptmp) <= 0 {
			break
		}
		msg,pos := minMsg(t.crptmp,true)
		res := verityMsg(msg,t.bc)
		if res == 1 {
			t.crpmsg = append(t.crpmsg,msg)
			t.removemgs(t.crptmp,pos)
		} else {
			break
		}	
	}
	return
}
// crpmsg was be check and insert to the standbyqueue
// when the blockchain has the block.
func (t *TrueHybrid) insertToSDM() error {
	m,_ := minMsg(t.crpmsg,false)
	if m == nil {
		return errors.New("no minMsg,msglen=" + strconv.Itoa(len(t.crpmsg)))
	}
	msgHeight := m.Height
	cur := big.NewInt(t.bc.CurrentHeader().Number.Int64())	
	if cur.Abs(msgHeight).Cmp(big.NewInt(12)) >= 0 {
		res := verityMsg(m,t.bc)
		if res == 1 {
			t.add(m)
		}
		m.SetUse(true)
		t.removeUnuseMsg(m.Height)
	}
	return nil
}

// remove the msg that has same height and it was used
func (t *TrueHybrid) removeUnuseMsg(num *big.Int) {
	pos := make([]int,0,0)
	for i,v := range t.crpmsg {
		if v.Height.Cmp(num) == 0 {
			if !v.Use() {
				pos = append(pos,i)
			}
		}
	}
	for _,i := range pos {
		t.removemgs(t.crpmsg,i)
	}
}
func (t *TrueHybrid) removemgs(crpmsg []*TrueCryptoMsg,i int) []*TrueCryptoMsg {
    return append(crpmsg[:i], crpmsg[i+1:]...)
}
// use=true include msg which was used 
func minMsg(crpmsg []*TrueCryptoMsg,use bool) (*TrueCryptoMsg,int) {
	if len(crpmsg) <= 0 {
		return nil,0
	} 
	min := crpmsg[0].Height
	pos := 0
	for ii,v := range crpmsg {
		if use {
			if min.Cmp(v.Height) == -1 {
				min = v.Height
				pos = ii
			}
		} else {
			if crpmsg[pos].Use() == true {
				min = v.Height
				pos = ii
			}
			if min.Cmp(v.Height) == -1 {
				min = v.Height
				pos = ii
			}
		}
	}
	if use {
		return crpmsg[pos],pos
	} else {
		if crpmsg[pos].Use() {
			return nil,0
		} else {
			return crpmsg[pos],pos
		}
	}
}
func existMsg(msg *TrueCryptoMsg,msgs []*TrueCryptoMsg) bool {
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
func (t *TrueHybrid) StandbyWork() error {
	for {
		if t.quit { break }
		
		t.insertToSDM()
		t.checkTmpMsg()
		for i:=0;i<5;i++ {
			if t.quit { return nil }
			time.Sleep(1 * time.Second)
		}
	}
	return nil
}
// after success pow,send the node by p2p
func MakeSignedStandbyNode(n *StandbyInfo,priv *ecdsa.PrivateKey) (*TrueCryptoMsg,error) {
	cmsg := TrueCryptoMsg{
		Height:		n.Height,
		Msg:		make([]byte,0,0),
		Sig:		make([]byte,0,0),
		use:		false,
	}
	var err error 
	cmsg.Msg,err = n.ToByte()
	if err != nil {
		return nil,err
	}
	cmsg.Sig,err = crypto.Sign(cmsg.Msg,priv)
	if err != nil {
		return nil,err
	}
	return &cmsg,nil
}
// 0 -- not ready; 1 -- success; -1 -- fail
func verityMsg(msg *TrueCryptoMsg,bc *core.BlockChain) int {
	if msg.Sig == nil || msg.Msg == nil || msg.Height.Cmp(zero) <= 0{
		return -1
	}
	// find the coinbase address from the heigth
	header := bc.GetHeaderByNumber(msg.Height.Uint64())
	if header == nil {
		return 0
	}
	coinbase := header.Coinbase.String()

	pub,err := crypto.SigToPub(msg.Msg,msg.Sig)
	if err != nil {
		return -1
	}
	addr := crypto.PubkeyToAddress(*pub).String()
	if addr == coinbase {
		return 1
	}
	return -1
}
func (t *TrueHybrid) SyncStandbyMembers() {
	// sync crypmsg 
	for _,v := range t.crpmsg {
		_,err := v.ToByte()
		if err != nil {
			// send data 
		}
	}
	// sync tmpcrypmsg???
}