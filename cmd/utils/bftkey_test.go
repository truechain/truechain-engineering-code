package utils

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestEnter(t *testing.T) {
	var data []byte

	if key, err := crypto.GenerateKey(); err != nil {
		fmt.Println("Failed to generate bft key:", err)
		return
	} else {
		data = crypto.FromECDSA(key)
	}

	fmt.Println("orig key:", hexutil.Encode(data))
	password := make([]byte, 16)
	pass1 := []byte("123456")
	copy(password, pass1)

	enData, err := aesEncrypt(data, password)
	if err != nil {
		fmt.Println("aesEncrypt failed,err:", err)
	} else {
		fmt.Println("enData key:", hexutil.Encode(enData))
	}
	// decrypt
	if data2, err := aesDecrypt(enData, password); err != nil {
		fmt.Println("aesDecrypt failed,err:", err)
	} else {
		fmt.Println("aesDecrypt key:", hexutil.Encode(data2))
	}
	fmt.Println("finish")
}
