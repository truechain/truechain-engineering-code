package truechain

import (
	"math/big"
	"bytes"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/crypto"

	"encoding/gob"
)

type CdEncryptionMsg struct {
	Height		*big.Int
	Msg			[]byte
	Sig 		[]byte
	Use			bool
}

func (t *CdEncryptionMsg) GetUse() bool { return t.Use }
func (t *CdEncryptionMsg) SetUse(u bool) { t.Use = u }

//convert CdEncryptionMsg into CdMember
func (t *CdEncryptionMsg) convertMsgToCdMember() *CdMember {
	info := CdMember{Height:big.NewInt(0),}
	info.ConvertToCdMember(t.Msg)
	//fromByte(t.Msg,info)
	return &info
}
// convert CdMember into  CdEncryptionMsg
func ConvertCdMemberToMsg(n *CdMember,priv *ecdsa.PrivateKey) (*CdEncryptionMsg,error) {
	cmsg := CdEncryptionMsg{
		Height:		n.Height,
		Msg:		make([]byte,0,0),
		Sig:		make([]byte,0,0),
		Use:		false,
	}
	var err error
	cmsg.Msg,err = toByte(n)
	if err != nil {
		return nil,err
	}
	cmsg.Sig,err = crypto.Sign(cmsg.Msg[:32],priv)
	if err != nil {
		return nil,err
	}
	return &cmsg,nil
}
func (t *CdMember) ConvertToCdMember(data []byte) error {
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

func Removemgs(msg []*CdEncryptionMsg,i int) []*CdEncryptionMsg {
	return append(msg[:i], msg[i+1:]...)
}

//find the min height of CdEncryptionMsg  from  crpmsg
// use=true include msg which was used
func minMsg(msg []*CdEncryptionMsg,use bool) (*CdEncryptionMsg,int) {
	if len(msg) <= 0 {
		return nil,0
	}
	min := msg[0].Height
	pos := 0
	for ii,v := range msg {
		if use {
			//1 means min>v.Height
			if min.Cmp(v.Height) == 1 {
				min = v.Height
				pos = ii
			}
		} else {
			if msg[pos].GetUse() == true {
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
		return msg[pos],pos
	} else {
		if msg[pos].GetUse() {
			return nil,0
		} else {
			return msg[pos],pos
		}
	}
}