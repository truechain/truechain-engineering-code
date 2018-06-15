package truechain

import (
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
)




type BlockPool struct {

	blocks 			[]*TruePbftBlock		//每次接收到块的池
	txsCh        	chan core.NewTxsEvent	//发送交易
	blockCh         chan NewBlocksEvent	//接收pbft块
	th				*TrueHybrid
}

type NewBlocksEvent struct {block *TruePbftBlock}


//添加块
func (self *BlockPool) addBlock(block *TruePbftBlock) {


	//判断是否委员
	self.th.CheckBlock(block)

	//
	txs := make([]*types.Transaction,0,0)

	//txs := make([]*Transaction,0,0)

	for _,v := range block.Txs.Txs {

		to := make([]byte,0,0)
		txData := v.GetData()

		if t := txData.GetRecipient(); t != nil {
			//to = t.Bytes()
		}

		txs = append(txs,&types.Transaction{
			data:&types.txdata{
				AccountNonce:   	txData.GetAccountNonce(),
				price:              txData.GetPrice(),
				GasLimit:           txData.GetGasLimit(),
				Recipient:          to,
				Amount	:           txData.GetAmount(),
				Payload:            txData.GetPayload(),
				V:                  txData.GetV(),
				R:                  txData.GetR(),
				S:                  txData.GetS(),
			},
		})
	}



}




//func (self *BlockPool) wait()  {
//
//	for {
//		// A real event arrived, process interesting content
//		select {
//			// Handle ChainHeadEvent
//
//			case bc :=<- self.blockCh:
//				//验证自身是否委员
//
//				//验证是否委员会发出
//				self.th.CheckBlock(bc.block)
//
//				txs := [len(bc.block.Txs.Txs)] truechain.Transaction{}
//
//				for tx := range bc.block.Txs.Txs {
//
//
//
//					true_tx := truechain.Transaction
//
//					true_tx.
//
//
//				}
//
//
//
//
//
//
//
//		}
//	}
//
//
//
//}