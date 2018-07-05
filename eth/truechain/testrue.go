package truechain

import (
	"math/big"
	"github.com/ethereum/go-ethereum/core/types"
)



func ConvTransaction(bp *BlockPool,txs []*types.Transaction)  {


	pbTxs := make([]*Transaction,0,0)
	for _,vv := range txs {
		to := make([]byte,0,0)
		if tt := vv.To(); tt != nil {
			to = tt.Bytes()
		}
		v,r,s := vv.RawSignatureValues()
		pbTxs = append(pbTxs,&Transaction{
			Data:       &TxData{
				AccountNonce:         vv.Nonce(),
				Price:                vv.GasPrice().Int64(),
				GasLimit:             new(big.Int).SetUint64(vv.Gas()).Int64(),
				Recipient:            to,
				Amount:               vv.Value().Int64(),
				Payload:              vv.Data(),
				V:                    v.Int64(),
				R:                    r.Int64(),
				S:                    s.Int64(),
				Hash:                 nil,
				XXX_NoUnkeyedLiteral: struct{}{},
				XXX_unrecognized:     nil,
				XXX_sizecache:        0,
			},
		})
	}

	block  := &TruePbftBlock{ }
	Txs := &Transactions{Txs:pbTxs}
	block.Txs = Txs
	//bp.AddBlock(block)
	bp.AddBlock(block)
	//rw := & p2p.MsgReadWriter{}

	//p2p.Send(rw, NewBftBlockMsg, []interface{}{block})


}
