package truechain

import (
	"io/ioutil"
	"encoding/json"
	"bytes"
	"encoding/gob"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
)

func ReadCfg(filename string) (map[string]interface{}, error){
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil,err
	}
	result := make(map[string]interface{})
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil,err
	}
	return result,nil
}

func toByte(e interface{}) ([]byte,error) {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(e)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}

func fromByte(data []byte,to interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	dec.Decode(to)
	return nil
}

func ToByte(e interface{}) ([]byte,error) {
	return toByte(e)
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func RlpHash(x interface{}) (h common.Hash) {
	return rlpHash(x)
}

func (t *TrueHybrid) SetCommitteeCount(c int) {
	t.setCommitteeCount(c)
}

