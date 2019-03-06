package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/rpc"
	"math/big"
	"strings"
	"time"
)

var (
	//transaction type  false normal tx; true  other tx
	txType = true
	//get all account
	account []string
	//uint
	ether int64 = 1000000000000000000
)

// get par
//	./main 10 1000000 1000 0 0 8888 1
func main() {
	fmt.Println("go into main")
	ip := "127.0.0.1:8888"

	send(ip)
}

//send transaction init
func send(ip string) {
	//dial etrue
	client, err := rpc.Dial("http://" + ip)
	defer client.Close()
	if err != nil {
		fmt.Println("Dail:", ip, err.Error())
		return
	}
	err = client.Call(&account, "etrue_accounts")
	if err != nil {
		fmt.Println("etrue_accounts Error", err.Error())
		return
	}
	if len(account) == 0 {
		fmt.Println("no account")
		return
	}

	if txType {
		getBalance(client, 3)
		fmt.Println("go into sendTrueRawTransaction")
		fmt.Println("========tx1 start==========")
		sendTrueRawTransaction(client, account[0], account[1], "", "0xde0b6b3a7640000", "0x0")
		getBalance(client, 3)
		fmt.Println("========tx2 start==========")
		sendTrueRawTransaction(client, account[0], account[2], account[1], "0xde0b6b3a7640000", "0x0")
		getBalance(client, 3)
		fmt.Println("========tx3 start==========")
		sendTrueRawTransaction(client, account[0], account[2], account[1], "0xde0b6b3a7640000", "0xde0b6b3a7640000")
		getBalance(client, 3)
		fmt.Println("========tx4 start==========")
		sendTrueRawTransaction(client, account[0], account[2], "", "0xde0b6b3a7640000", "0xde0b6b3a7640000")
		getBalance(client, 3)
		return
	}

}

func getBalance(client *rpc.Client, index int) {
	time.Sleep(time.Second * 1)
	var result string
	for i := 0; i < index; i++ {
		err := client.Call(&result, "etrue_getBalance", account[i], "latest")
		if err != nil {
			log.Error("getBalance", "i", i, "error", err)
		}
		fmt.Println("account[", i, "](", account[i], ")= ", getBalanceValue(result).Uint64())
	}
}

func sendTrueRawTransaction(client *rpc.Client, from string, to string, payment string, value string, fee string) (string, error) {
	mapData := make(map[string]interface{})
	mapData["from"] = from
	mapData["to"] = to
	if payment != "" {
		mapData["payment"] = payment
	}
	mapData["value"] = value
	mapData["fee"] = fee

	var reBool bool
	err := client.Call(&reBool, "personal_unlockAccount", from, "admin", 9000000)
	if payment != "" {
		err = client.Call(&reBool, "personal_unlockAccount", payment, "admin", 9000000)
	}
	if err != nil {
		log.Error("personal_unlockAccount", "err", err)
	}
	var result string
	err = client.Call(&result, "etrue_sendTransaction", mapData)
	if err != nil {
		log.Error("etrue_sendTransaction", "err", err)
	}
	return result, err
}

func getBalanceValue(hex string) *big.Int {
	if strings.HasPrefix(hex, "0x") {
		hex = strings.TrimPrefix(hex, "0x")
	}
	value, _ := new(big.Int).SetString(hex, 16)
	//balance := new(big.Int).Set(value)
	//fmt.Println("etrue_getBalance Ok:", balance.Div(balance, big.NewInt(ether)), " value ", value, " hex ", hex)
	return value
}
