package truechain

import (
	"bytes"
	"encoding/gob"
)

type CommitteeMember struct {
	Nodeid		string			// the pubkey of the node(nodeid)
	Addr		string
	Port		int
}

func (t *CommitteeMember) ToByte() ([]byte,error) {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(t)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}

func (t *CommitteeMember) FromByte(data []byte) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	to := CommitteeMember{}
	dec.Decode(&to)
	t.Nodeid = to.Nodeid
	t.Addr = to.Addr
	t.Port = to.Port
	return nil
}