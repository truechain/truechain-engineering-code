package truechain

import (
	"io/ioutil"
	"encoding/json"
	"bytes"
	"encoding/gob"
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

