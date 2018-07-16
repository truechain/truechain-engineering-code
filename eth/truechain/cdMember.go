package truechain

import (
	"math/big"
	"bytes"
	"encoding/gob"
)

type CdMember struct {
	Nodeid		string			// the pubkey of the node(nodeid)
	Coinbase	string			// the bonus address of miner
	Addr		string
	Port		int
	Height		*big.Int		// block Height who pow success
	Comfire		bool			// the state of the block comfire,default greater 12 like eth
}

func (t *CdMember) FromByte(data []byte) error {
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
func (t *CdMember) ToByte() ([]byte,error) {
	// return common.FromHex(t.ToMsg())
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(t)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}
