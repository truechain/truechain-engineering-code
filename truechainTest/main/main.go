package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/rpc"
	"github.com/truechain/truechain-engineering-code/truechainTest/test"
	"sync"
	"time"
)

func main() {
	b := 20
	wait := sync.WaitGroup{}
	timeNow := time.Now().Format("2006-01-02 15:04:05")
	client, _ := rpc.Dial("http://47.92.209.38:8889")
	var account []string
	err := client.Call(&account, "etrue_accounts")
	fmt.Println(account)
	//解锁账户
	//var result1 string
	//err = client.Call(&result1, "personal_unlockAccount", account[0],"admin",0)
	//fmt.Println(err)
	fmt.Println(err)
	for a := 0; a < b; a++ {
		wait.Add(1)
		go test.Test(&wait, client, b)
	}

	wait.Wait()
	timeNow1 := time.Now().Format("2006-01-02 15:04:05")
	fmt.Println("开始时间：" + timeNow + ",现在时间：" + timeNow1)

}
