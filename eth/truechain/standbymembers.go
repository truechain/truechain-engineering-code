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
	"crypto/ecdsa"
	"net"
    "math/big"
    "errors"
    
    "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core"
)

// all functions of sdm not thread-safe
func (t *TrueHybrid) add(msg *TrueCryptoMsg) error {
	node := msg.ToStandbyInfo()
	if node == nil {
		return errors.New("Wrong CrytoMsg")
	}
	// verfiy and add 
    if len(t.sdm) >= t.sdmsize {
		t.sdm = append(t.sdm[:0],t.sdm[1:])
	} 	
	t.sdm = append(t.sdm,node)
	return nil
}
func (t *TrueHybrid) findMsg(h *big.Int) *TrueCryptoMsg {
	for _,v := range t.crpmsg {
		if v.heigth.Cmp(h) == 0 {
			return v
		}		
	}
	return nil
}

func (t *TrueHybrid) AddMsg(msg *TrueCryptoMsg,bc *core.BlockChain) {
	// verify the msg when the block is on
	res := verityMsg(msg,bc)
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
				nodeid:		v.nodeid,
				addr:		v.addr,			
				port:		v.port,			
			})
		}
	}
	return vv,nil
}
// check the crypmsg when blockchain has the block
func (t *TrueHybrid) VerifyCheck(bc *core.BlockChain) error {
	// sheight := t.sdm[len(t.sdm)-1].height
	// cur := bc.CurrentHeader().Number()	
	// if cur.Abs(sheight).Cmp(big.NewInt(12)) >= 0 {
	// 	msg := findMsg(sheight.Add(sheight,big.NewInt(1)))
	// 	if msg != nil {
	// 		res := verityMsg(msg,bc)
	// 		if res == 1 {
	// 			add(msg)
	// 		}
	// 	}
	// }
	msg := t.crptmp[0]
	res := verityMsg(msg,bc)
	if res == 1 {
		t.crpmsg = append(t.crpmsg,msg)
		t.crptmp = t.crptmp[1:]
	}
	return nil
}
// crpmsg was be check and insert to the standbyqueue
// when the blockchain has the block
func (t *TrueHybrid) insertToSDM(bc *core.BlockChain) error {
	m := t.minMsg(false)
	if m == nil {
		return errors.New("no minMsg,msglen=",strconv.Atoi(len(t.crpmsg)))
	}
	msgHeight := m.Height
	cur := big.NewInt(bc.CurrentHeader().Number().Int64())	
	if cur.Abs(msgHeight).Cmp(big.NewInt(12)) >= 0 {
		res := verityMsg(m,bc)
		if res == 1 {
			add(m)
		}
		m.SetUse(true)
	}
	return nil
}
// remove the msg that has same height and it was used
func (t *TrueHybrid) removeUnuseMsg(num *big.Int) {

}
// use=true include msg which was used 
func (t *TrueHybrid) minMsg(use bool) *TrueCryptoMsg {
	if len(t.crpmsg) <= 0 {
		return nil
	} 
	min := t.crpmsg[0].Height
	pos := 0
	for ii,v := range t.crpmsg {
		if use {
			if min.Cmp(v.Height) == -1 {
				min = v.Height
				pos = ii
			}
		} else {
			if t.crpmsg[pos].Use() == true {
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
		return t.crpmsg[pos]
	} else {
		if t.crpmsg[pos].Use() {
			return nil
		} else {
			return t.crpmsg[pos]
		}
	}
}
func (t *TrueHybrid) standbyWork(bc *core.BlockChain) error {
	t.insertToSDM()
	return nil
}
// after success pow,send the node by p2p
func MakeSignedStandbyNode(n *StandbyInfo,priv *ecdsa.PrivateKey) (*TrueCryptoMsg,error) {
	cmsg := struct TrueCryptoMsg{
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
	return cmsg,nil
}
// 0 -- not ready; 1 -- success; -1 -- fail
func verityMsg(msg *TrueCryptoMsg,bc *core.BlockChain) int {
	// find the coinbase address from the heigth
	header := bc.GetHeaderByNumber(msg.heigth)
	if header == nil {
		return 0
	}
	coinbase := header.Coinbase.String()

	pub,err := crypto.SigToPub(msg.Msg,msg.Sig)
	if err != nil {
		return -1
	}
	addr := crypto.PubkeyToAddress(pub).String()
	if addr == coinbase {
		return 1
	}
	return -1
}