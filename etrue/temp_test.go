
package etrue

/*
func  SendBlock(agent *PbftAgent)  {
	for{
		//获取区块
		block,_ := agent.FetchFastBlock()
		//验证区块
		err :=agent.VerifyFastBlock(block)
		if err != nil{
			panic(err)
		}else{
			fmt.Println("validate true")
		}
		time.Sleep(time.Second * 5)

		//发出区块
		agent.BroadcastFastBlock(block)
		agent.BroadcastConsensus(block)
	}
}*/