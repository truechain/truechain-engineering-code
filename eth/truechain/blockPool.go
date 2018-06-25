package truechain

import (
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"github.com/ethereum/go-ethereum/common"
)


type BlockPool struct {
	blocks  []*TruePbftBlock      //Every time a pool receives a block
	TrueTxsCh   chan core.NewTxsEvent //Send a deal
	th      *TrueHybrid

}


func (self *BlockPool) GetCcc() chan core.NewTxsEvent {
	return self.TrueTxsCh
}
//Add a piece of
func (self *BlockPool) AddBlock(block *TruePbftBlock) {

	//Judging committee members
	if(self.th.CheckBlock(block)==nil){
		self.blocks = append(self.blocks, block)
	}
}

//Access to the original aether pow mining process
func (self *BlockPool) JoinEth() {
	for{
		for _,block := range self.blocks {
			txs := make([]*types.Transaction,0,0)
			//Convert trading
			for _,v := range block.Txs.Txs {
				txData := v.GetData()
				//nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte
				var to common.Address
				to.SetBytes(txData.GetRecipient())
				//Create a new transaction
				transaction := types.NewTransaction(txData.GetAccountNonce(),to,big.NewInt(txData.GetAmount()),uint64(txData.GetGasLimit()),nil,txData.GetPayload())
				txs = append(txs,transaction)
			}
			//Send the transaction back
			self.TrueTxsCh <- core.NewTxsEvent{Txs:txs}
		}
	}
}

func (self *BlockPool) PushAEmptyBlock() {
	txs := make([]*types.Transaction,0,0)
	self.TrueTxsCh <- core.NewTxsEvent{Txs:txs}
}
