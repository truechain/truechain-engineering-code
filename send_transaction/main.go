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

//Send transmission full sleep intervals
var sleep = time.Millisecond * 10000

//get all account
var account []string

//time format
var termTimeFormat = "[01-02|15:04:05.000]"

//the pre count
var preCount int64 = 0

//the pre account
var preAccount = ""

//flag sleep
var bSleep = false

// get par
func main() {
	if len(os.Args) < 7 {
		fmt.Printf("invalid args : %s [count] [frequency] [interval] [sleep] [from] [to]  [\"ip:port\"]\n", os.Args[0])
		return
	}

	count, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Println(getTime(), "count err")
		return
	}

	frequency, err = strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Println(getTime(), "frequency err")
		return
	}

	intervalCount, err := strconv.Atoi(os.Args[3])
	if err != nil {
		fmt.Println(getTime(), "interval err")
		return
	} else {
		interval = time.Millisecond * time.Duration(intervalCount)
	}

	sleepCnt, err := strconv.Atoi(os.Args[4])
	if err != nil {
		fmt.Println(getTime(), "sleep err default 10000")
		return
	} else {
		sleep = time.Millisecond * time.Duration(sleepCnt)
	}

	from, err = strconv.Atoi(os.Args[5])
	if err != nil {
		fmt.Println(getTime(), "from err default 0")
	}

	to, err = strconv.Atoi(os.Args[6])
	if err != nil {
		fmt.Println(getTime(), "from err default 1")
	}

	ip := "127.0.0.1:8888"
	if len(os.Args) == 8 {
		ip = os.Args[7]
	}

	send(count, ip)

}

//get time
func getTime() string {
	return time.Now().Format(termTimeFormat)
}

//send transaction init
func send(count int, ip string) {
	//dial etrue
	client, err := rpc.Dial("http://" + ip)
	if err != nil {
		fmt.Println(getTime(), "Dail:", ip, err.Error())
		return
	}

	err = client.Call(&account, "etrue_accounts")
	if err != nil {
		fmt.Println(getTime(), "etrue_accounts Error", err.Error())
		return
	}
	if len(account) == 0 {
		fmt.Println(getTime(), "no account")
		return
	}
	fmt.Println(getTime(), "account:", account)

	// get balance
	var result string = ""
	err = client.Call(&result, "etrue_getBalance", account[from], "latest")
	if err != nil {
		fmt.Println(getTime(), "etrue_getBalance Error:", err)
		return
	} else {

		bl, _ := new(big.Int).SetString(result, 10)
		fmt.Println(getTime(), "etrue_getBalance Ok:", bl, result)
	}

	//unlock account
	var reBool bool
	err = client.Call(&reBool, "personal_unlockAccount", account[from], "admin", 90000)
	if err != nil {
		fmt.Println(getTime(), "personal_unlockAccount Error:", err.Error())
		return
	} else {
		fmt.Println(getTime(), "personal_unlockAccount Ok", reBool)
	}

	// send
	waitMain := &sync.WaitGroup{}
	for {
		if bSleep {
			bSleep = false
			time.Sleep(sleep)
		}
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
			fmt.Println(getTime(), "etrue_getBalance Error:", err)
			//return
		} else {
			bl, _ := new(big.Int).SetString(result, 10)
			fmt.Println(getTime(), "etrue_getBalance Ok:", bl, result)

			if preAccount == result {
				bSleep = true
				fmt.Println(getTime(), "Account not dec sleep")
			} else {
				preAccount = result
			}
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
	fmt.Println(getTime(), "Send in go Complete", count)
	waitGroup.Wait()
	fmt.Println(getTime(), "Complete", Count)
	if Count > preCount {
		preCount = Count
	} else {
		fmt.Println(getTime(), "tx full sleep")
		bSleep = true
	}
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
