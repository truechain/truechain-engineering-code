package etrue

import (
		"time"
	"fmt"
)

func sendBlock(agent *PbftAgent)  {
	time.Sleep(time.Minute*2)
	//for{
	//获取区块
	block,_ := agent.FetchFastBlock()
	//验证区块
	err :=agent.VerifyFastBlock(block)
	if err != nil{
		panic(err)
	}else{
		fmt.Println("validate true")
	}
	time.Sleep(time.Second * 20)

	//发出区块
	agent.BroadcastFastBlock(block)
	//agent.BroadcastSign(block.Body().Signs[0],block)
	time.Sleep(time.Second * 50)
	//}

}