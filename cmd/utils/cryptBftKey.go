package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh/terminal"
	"io/ioutil"
	"strings"
	"syscall"

	"github.com/ethereum/go-ethereum/crypto"
)

func pkcs5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func pkcs5UnPadding(origData []byte) ([]byte, error) {
	length := len(origData)
	unpadding := int(origData[length-1])
	if unpadding > length {
		return nil, errors.New("wrong data len")
	}
	return origData[:(length - unpadding)], nil
}

func aesEncrypt(origData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	origData = pkcs5Padding(origData, blockSize)
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	return crypted, nil
}

func aesDecrypt(crypted, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	return pkcs5UnPadding(origData)
}

func getpasswordFromScreen(create bool) string {
	var pass string
	if create {
		var p1, p2 string
		for len(p1) != 6 || (p1 != "" && 0 != strings.Compare(p1, p2)) {
			fmt.Println("please enter password for create bft private key:6 byte")
			p1byte, _ := terminal.ReadPassword(int(syscall.Stdin))
			p1 = string(p1byte)
			fmt.Println("please confirm password:")
			p2byte, _ := terminal.ReadPassword(int(syscall.Stdin))
			p2 = string(p2byte)
			//fmt.Scanln(&p2)
		}
		pass = p1
	} else {
		fmt.Println("please enter your password for bft private key:")
		p1byte, _ := terminal.ReadPassword(int(syscall.Stdin))
		pass = string(p1byte)
	}
	return pass
}
func getEncryptDataFromFile(file string) ([]byte, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return b, err
}

func LoadBftKey(file string) (*ecdsa.PrivateKey, error) {
	data, err := getEncryptDataFromFile(file)
	if err != nil {
		fmt.Println("Failed to load bft key:", err)
		return nil, err
	}
	pass := getpasswordFromScreen(false)
	password := make([]byte, 16)
	copy(password, []byte(pass))

	if key, err := aesDecrypt(data, password); err != nil {
		Fatalf("wrong password:%v", err)
		return nil, err
	} else {
		return crypto.ToECDSA(key)
	}
}

func GenEncryptBftKey() (*ecdsa.PrivateKey, []byte, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		fmt.Println("Failed to generate bft key:", err)
		return nil, nil, err
	}
	data := crypto.FromECDSA(key)
	pass := getpasswordFromScreen(true)
	password := make([]byte, 16)
	copy(password, []byte(pass))

	if enData, err := aesEncrypt(data, password); err != nil {
		fmt.Println("Failed to aesEncrypt bft key:", err)
		return nil, nil, err
	} else {
		return key, enData, nil
	}
}
