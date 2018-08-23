package etrue

import (
	"fmt"
	"time"
)

func sendBlock(agent *PbftAgent)  {

	for{
		//获取区块
		block,_ := agent.FetchFastBlock()
		//验证区块
		bool,err :=agent.VerifyFastBlock(block)
		if err != nil{
			panic(err)
		}
		fmt.Println("validate",bool)
		//发出区块
		agent.BroadcastFastBlock(block)
		agent.BroadcastSign(block,block.Body().Signs[0])
		time.Sleep(time.Second*5)
	}

}