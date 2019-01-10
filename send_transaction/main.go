package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/truechain/truechain-engineering-code/rpc"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//Count send complete
var Count int64

//Transaction from to account id
var from, to, frequency = 0, 0, 1

//Two transmission intervals
var interval = time.Millisecond * 0

//get all account
var account []string

// The message state
var msg = make(chan bool)

// Restart the number
var num int

// SLEEPTIME The interval between reconnections
const SLEEPTIME = 120

// SLEEPTX The interval between send son address
const SLEEPTX = 30

// get par
func main() {
	if len(os.Args) < 4 {
		fmt.Printf("invalid args : %s [count] [frequency] [interval] [from] [to] [\"ip:port\"]\n", os.Args[0])
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
	}

	interval = time.Millisecond * time.Duration(intervalCount)

	from, err = strconv.Atoi(os.Args[4])
	if err != nil {
		fmt.Println("from err default 0")
	}

	to, err = strconv.Atoi(os.Args[5])

	if err != nil {
		fmt.Println("to err 0：Local address 1: Generate address")
	}

	ip := "127.0.0.1:8888"
	if len(os.Args) == 7 {
		ip = os.Args[6]
	}
	fmt.Println("==========================")

	go send(count, ip)

	for {
		if !<-msg {
			fmt.Println("======================send Transaction restart=========================")
			num++
			time.Sleep(time.Second * SLEEPTIME)
			go send(count, ip)
		} else {
			fmt.Println("=======================send Transaction end=========================")
			break
		}
	}
	fmt.Println("send Transaction num is:", num)
}

//send transaction init
func send(count int, ip string) {
	//dial etrue
	client, err := rpc.Dial("http://" + ip)

	defer client.Close()

	if err != nil {
		fmt.Println("Dail:", ip, err.Error())
		msg <- false
		return
	}

	err = client.Call(&account, "etrue_accounts")
	if err != nil {
		fmt.Println("etrue_accounts Error", err.Error())
		msg <- false
		return
	}
	if len(account) == 0 {
		fmt.Println("no account")
		return
	}
	fmt.Println("account:", account)

	for i := len(account); i < count; i++ {
		//new account
		var address string
		err = client.Call(&address, "personal_newAccount", "admin")
		if err != nil {
			fmt.Println("personal_newAccount Error:", err.Error())
			msg <- false
			return
		}
		account = append(account, address)
		fmt.Println("personal_newAccount ", i, " accounts ", " Ok ", len(account))
	}
	fmt.Println("personal_newAccount success ", len(account))

	// get balance
	var result string
	err = client.Call(&result, "etrue_getBalance", account[from], "latest")
	if err != nil {
		fmt.Println("etrue_getBalance Error:", err)
		msg <- false
		return
	}

	if strings.HasPrefix(result, "0x") {
		result = strings.TrimPrefix(result, "0x")
	}
	fmt.Println("etrue_getBalance Ok:", result)

	//main unlock account
	_, err = unlockAccount(client, account[from], "admin", 90000, "main")
	if err != nil {
		fmt.Println("personal_unlockAccount Error:", err.Error())
		msg <- false
		return
	}

	address, _ := new(big.Int).SetString(result, 16)
	value := "0x" + address.Div(address, big.NewInt(int64(len(account)))).String()
	fmt.Println("sendRawTransaction son address ", value)

	//send main to son address
	for i := 0; i < count; i++ {
		//main unlock account
		if from == 0 {
			continue
		}
		if result, err := sendRawTransaction(client, account[from], account[i], value); err != nil {
			fmt.Println("sendRawTransaction son address error ", result)
			return
		}
	}

	//son address unlock account
	for i := 0; i < count; i++ {
		if from == 0 {
			continue
		}
		_, err = unlockAccount(client, account[i], "admin", 90000, "son address")
		if err != nil {
			fmt.Println("personal_unlockAccount Error:", err.Error())
			msg <- false
			return
		}
	}

	time.Sleep(time.Second * SLEEPTX)

	// send
	waitMain := &sync.WaitGroup{}
	for {
		waitMain.Add(1)
		go sendTransactions(client, account, count, waitMain)
		frequency--
		if frequency <= 0 {
			break
		}
		time.Sleep(interval)
		// get balance
		err = client.Call(&result, "etrue_getBalance", account[from], "latest")
		if err != nil {
			fmt.Println("etrue_getBalance Error:", err)
			msg <- false
			return
		}

		bl, _ := new(big.Int).SetString(result, 10)
		fmt.Println("etrue_getBalance Ok:", bl, result)

	}
	waitMain.Wait()
	msg <- true
}

//send count transaction
func sendTransactions(client *rpc.Client, account []string, count int, wait *sync.WaitGroup) {
	defer wait.Done()
	waitGroup := &sync.WaitGroup{}

	for i := 0; i < count; i++ {
		waitGroup.Add(1)
		go sendTransaction(client, account[i], waitGroup)
	}
	fmt.Println("Send in go Complete", count)
	waitGroup.Wait()
	fmt.Println("Complete", Count)
}

//send one transaction
func sendTransaction(client *rpc.Client, from string, wait *sync.WaitGroup) {
	defer wait.Done()

	address := genAddress()
	if to == 1 {
		if account[to] != "" {
			address = account[to]
		}
	}

	if result, _ := sendRawTransaction(client, from, address, "0x2100"); result != "" {
		Count++
	}
}

func sendRawTransaction(client *rpc.Client, from string, to string, value string) (string, error) {

	mapData := make(map[string]interface{})

	mapData["from"] = from
	mapData["to"] = to
	mapData["value"] = value

	var result string
	err := client.Call(&result, "etrue_sendTransaction", mapData)
	return result, err
}

func unlockAccount(client *rpc.Client, account string, password string, time int, name string) (bool, error) {
	var reBool bool
	err := client.Call(&reBool, "personal_unlockAccount", account, password, time)
	fmt.Println(name, " personal_unlockAccount Ok", reBool)
	return reBool, err
}

// Genesis address
func genAddress() string {
	priKey, _ := crypto.GenerateKey()
	address := crypto.PubkeyToAddress(priKey.PublicKey)
	return address.Hex()
}
