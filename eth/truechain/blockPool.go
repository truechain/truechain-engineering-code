package truechain

import (
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"github.com/ethereum/go-ethereum/common"
)


type BlockPool struct {
	blocks  []*TruePbftBlock      //每次接收到块的池
	TrueTxsCh   chan core.NewTxsEvent //发送交易
	th      *TrueHybrid

}


func (self *BlockPool) GetCcc() chan core.NewTxsEvent {
	return self.TrueTxsCh
}
//添加块
func (self *BlockPool) AddBlock(block *TruePbftBlock) {

	//判断是否委员
	if(self.th.CheckBlock(block)==nil){
		self.blocks = append(self.blocks, block)
	}



}


//接入到原来以太的pow挖矿流程
func (self *BlockPool) JoinEth() {

	for{

		for _,block := range self.blocks {

			txs := make([]*types.Transaction,0,0)


			//转换交易
			for _,v := range block.Txs.Txs {

				txData := v.GetData()
				//nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte
				var to common.Address
				to.SetBytes(txData.GetRecipient())
				//创建新的交易
				transaction := types.NewTransaction(txData.GetAccountNonce(),to,big.NewInt(txData.GetAmount()),uint64(txData.GetGasLimit()),nil,txData.GetPayload())
				txs = append(txs,transaction)
			}

			//将交易传回
			self.TrueTxsCh <- core.NewTxsEvent{Txs:txs}

		}




	}



}

