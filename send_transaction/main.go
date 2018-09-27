package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/rpc"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"
)

//send complete
var Count int64 = 0

//Transaction from to account id
var from, to, frequency = 0, 1, 1

//Two transmission intervals
var interval = time.Millisecond * 0

//get all account
var account []string

// get par
func main() {
	if len(os.Args) < 4 {
		fmt.Printf("invalid args : %s [count] [frequency] [interval] [from] [to]  [\"ip:port\"]\n", os.Args[0])
		return
	}

	count, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Println("count err")
		return
	}

	frequency, err = strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Println("frequency err")
		return
	}

	intervalCount, err := strconv.Atoi(os.Args[3])
	if err != nil {
		fmt.Println("interval err")
		return
	} else {
		interval = time.Millisecond * time.Duration(intervalCount)
	}

	from, err = strconv.Atoi(os.Args[4])
	if err != nil {
		fmt.Println("from err default 0")
	}

	to, err = strconv.Atoi(os.Args[5])
	if err != nil {
		fmt.Println("from err default 1")
	}

	ip := "127.0.0.1:8888"
	if len(os.Args) == 7 {
		ip = os.Args[6]
	}

	send(count, ip)

}

//send transaction init
func send(count int, ip string) {
	//dial etrue
	client, err := rpc.Dial("http://" + ip)
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
	fmt.Println("account:", account)

	// get balance
	var result string = ""
	err = client.Call(&result, "etrue_getBalance", account[from], "latest")
	if err != nil {
		fmt.Println("etrue_getBalance Error:", err)
		return
	} else {

		bl, _ := new(big.Int).SetString(result, 10)
		fmt.Println("etrue_getBalance Ok:", bl)
	}

	//unlock account
	var reBool bool
	err = client.Call(&reBool, "personal_unlockAccount", account[from], "admin", 90000)
	if err != nil {
		fmt.Println("personal_unlockAccount Error:", err.Error())
		return
	} else {
		fmt.Println("personal_unlockAccount Ok", reBool)
	}

	// send
	waitMain := &sync.WaitGroup{}
	for {
		waitMain.Add(1)
		go sendTransactions(client, account, count, waitMain)
		frequency -= 1
		if frequency <= 0 {
			break
		}
		time.Sleep(interval)
		// get balance
		err = client.Call(&result, "etrue_getBalance", account[from], "latest")
		if err != nil {
			fmt.Println("etrue_getBalance Error:", err)
			return
		} else {
			bl, _ := new(big.Int).SetString(result, 10)
			fmt.Println("etrue_getBalance Ok:", bl)
		}
	}
	waitMain.Wait()
}

//send count transaction
func sendTransactions(client *rpc.Client, account []string, count int, wait *sync.WaitGroup) {
	defer wait.Done()
	waitGroup := &sync.WaitGroup{}

	//发送交易
	for a := 0; a < count; a++ {
		waitGroup.Add(1)
		go sendTransaction(client, account, waitGroup)
	}
	fmt.Println("Send in go Complete", count)
	waitGroup.Wait()
	fmt.Println("Complete", Count)
}

//send one transaction
func sendTransaction(client *rpc.Client, account []string, wait *sync.WaitGroup) {
	defer wait.Done()
	map_data := make(map[string]interface{})
	map_data["from"] = account[from]
	map_data["to"] = account[to]
	map_data["value"] = "0x2100"
	var result string
	client.Call(&result, "etrue_sendTransaction", map_data)
	if result != "" {
		Count += 1
	}
}
