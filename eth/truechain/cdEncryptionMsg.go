package truechain

import (
	"math/big"
	"bytes"
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
func (t *CdEncryptionMsg) ToStandbyInfo() *CdMember {
	info := CdMember{Height:big.NewInt(0),}
	fromByte(t.Msg,info)
	return &info
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