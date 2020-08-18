package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/rpc"
	"math/big"
	"os"
	"strconv"
	"sync"
	"time"
)

//Count send complete
var Count int64

//Transaction from to account id
var from, to, frequency = 0, 1, 1

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
const SLEEPTX = 5

// get par
func main() {
	if len(os.Args) < 4 { // 1000   1000000  1000 1 0
		fmt.Printf("invalid args : %s [count] [frequency] [interval] [from] [to] [\"port\"]\n", os.Args[0])
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

	if len(os.Args) > 5 {
		to, err = strconv.Atoi(os.Args[5])
	} else {
		fmt.Println("to 0：Local address 1: Generate address")
	}

	ip := "127.0.0.1:"
	if len(os.Args) == 7 {
		ip = ip + os.Args[6]
	} else {
		ip = ip + "8888"
	}

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
		msg <- false
		return
	}

	// get balance
	result := getAccountBalance(client, account[from])
	if result == nil {
		return
	}

	if result.Uint64() < 21000 {
		fmt.Println("Lack of balance ", account[from], " from ", from)
		msg <- false
		return
	}

	fmt.Println("already have accounts is in local:", len(account))

	fmt.Println("personal_newAccount success ", len(account), " result ", createSonAccount(client, count), "main address ", account[from])

	//main unlock account
	_, err = unlockAccount(client, account[from], "admin", 9000000, "main")
	if err != nil {
		fmt.Println("personal_unlockAccount Error:", err.Error())
		msg <- false
		return
	}

	//send main to son address
	fmt.Println("send balance to ", count, "  new account ", sendBalanceNewAccount(client, count, result))

	//son address check account
	fmt.Println("check ", count, " son account ", checkSonAccountBalance(client, count, result))

	// send
	fmt.Println("start sendTransactions from ", count, " account to other new account")
	waitMain := &sync.WaitGroup{}
	for {
		waitMain.Add(1)
		go sendTransactions(client, account, count, waitMain)
		frequency--
		if frequency <= 0 {
			break
		}
		time.Sleep(interval)
	}
	waitMain.Wait()
	msg <- true
}

//send count transaction
func sendTransactions(client *rpc.Client, account []string, count int, wait *sync.WaitGroup) {
	defer wait.Done()
	waitGroup := &sync.WaitGroup{}
	Time := time.Now()

	accounts := make(map[string]int)
	for i := 0; i < count; i++ {
		balance := getAccountBalance(client, account[i])
		if balance == nil {
			return
		}
		if balance.Cmp(big.NewInt(int64(100000))) < 0 {
			fmt.Println(" Lack of balance  ", balance, " i ", i)
			// get balance
			balance := getAccountBalance(client, account[from])
			if balance == nil {
				return
			}
			average := new(big.Int).Div(balance, big.NewInt(int64(len(account)*1000)))
			value := "0x" + fmt.Sprintf("%x", average)
			waitGroup.Add(1)
			go sendTransaction(client, account[from], i, account[i], value, waitGroup)
			continue
		} else {
			accounts[account[i]] = i
		}
	}

	loop := count
	if to > 1 {
		loop = to
	}

	for i := 0; i < loop; i++ {
		for k, v := range accounts {
			waitGroup.Add(1)
			go sendTransaction(client, k, v, "", "0x2100", waitGroup)
		}
	}

	waitGroup.Wait()
	fmt.Println(" Complete ", Count, " time ", Time, " count ", len(accounts)*loop)
}

//send one transaction
func sendTransaction(client *rpc.Client, from string, index int, son string, value string, wait *sync.WaitGroup) {
	defer wait.Done()

	var address string

	if son == "" {
		address = genAddress()
	} else {
		address = son
	}

	result, err := sendRawTransaction(client, from, address, value)

	if err != nil {
		fmt.Println("sendRawTransaction", "result ", result, "index", index, " error", err, " address ", address)
	}

	if result != "" {
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

func getAccountBalance(client *rpc.Client, account string) *big.Int {
	var result hexutil.Big
	// get balance
	err := client.Call(&result, "etrue_getBalance", account, "latest")
	if err != nil {
		fmt.Println("etrue_getBalance Error:", err)
		msg <- false
		return nil
	}
	return (*big.Int)(&result)
}

func trueToWei(trueValue uint64) *big.Int {
	baseUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	value := new(big.Int).Mul(big.NewInt(int64(trueValue)), baseUnit)
	return value
}

func weiToTrue(value *big.Int) uint64 {
	baseUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	valueT := new(big.Int).Div(value, baseUnit).Uint64()
	return valueT
}

func createSonAccount(client *rpc.Client, count int) bool {
	for i := len(account); i < count; i++ {
		//new account
		var address string
		err := client.Call(&address, "personal_newAccount", "admin")
		if err != nil {
			fmt.Println("personal_newAccount Error:", err.Error())
			msg <- false
			return false
		}
		account = append(account, address)
		fmt.Println("personal_newAccount ", i, " accounts ", " Ok ", len(account), "address", address)
	}
	return true
}

func sendBalanceNewAccount(client *rpc.Client, count int, main *big.Int) bool {
	average := new(big.Int).Div(main, big.NewInt(int64(len(account)*1000)))
	value := "0x" + fmt.Sprintf("%x", average)
	fmt.Println("sendBalanceNewAccount ", " true ", weiToTrue(average), " average ", average)

	waitGroup := &sync.WaitGroup{}
	for i := 0; i < count; i++ {
		if from == i {
			continue
		}
		// get balance
		result := getAccountBalance(client, account[i])

		fmt.Println("unlock ", count, " son account index", i)
		waitGroup.Add(1)
		unlockSonAccount(client, account[i], i, waitGroup)

		if result == nil {
			return false
		}

		if result.Cmp(average) < 0 {
			waitGroup.Add(1)
			go sendTransaction(client, account[from], i, account[i], value, waitGroup)
		}
	}
	waitGroup.Wait()

	return true
}

func checkSonAccountBalance(client *rpc.Client, count int, main *big.Int) bool {
	average := new(big.Int).Div(main, big.NewInt(int64(len(account)*1000)))
	value := "0x" + fmt.Sprintf("%x", average)
	fmt.Println("sendBalanceNewAccount ", " true ", weiToTrue(average), " average ", average)

	for i, v := range account {
		// get balance
		balance := getAccountBalance(client, account[i])
		if balance == nil {
			return false
		}
		balanceTrue := new(big.Int).Set(balance)
		fmt.Println("etrue_getBalance son address ", account[i], " result ", balance, " i ", i, " true ", balanceTrue.Div(balanceTrue, big.NewInt(1000000000000000000)))
		if balance.Cmp(average) < 0 {
			fmt.Println(i, " Transaction main address ", account[from], " son address ", account[i], " value ", value)
			if result, err := sendRawTransaction(client, account[from], v, value); err != nil {
				fmt.Println("sendRawTransaction son address error ", result, " err ", err)
				return false
			}
		}
		if i > count {
			break
		}
	}

	return true
}

// unlockSonAccount
func unlockSonAccount(client *rpc.Client, account string, index int, wait *sync.WaitGroup) {
	defer wait.Done()
	fmt.Println("unlockAccount address index ", index, " son address ", account)
	_, err := unlockAccount(client, account, "admin", 9000000, "son address")
	if err != nil {
		fmt.Println("personal_unlockAccount Error:", err.Error(), " index ", index, "addr", account)
		msg <- false
	}
}
