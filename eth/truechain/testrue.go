package truechain

import (
	"math/big"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
	"time"
	"github.com/ethereum/go-ethereum/crypto"
	"encoding/hex"
)



func ConvTransaction(th *TrueHybrid,txs []*types.Transaction)  {
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
				Price:                vv.GasPrice().Uint64(),
				GasLimit:             new(big.Int).SetUint64(vv.Gas()).Uint64(),
				Recipient:            to,
				Amount:               vv.Value().Uint64(),
				Payload:              vv.Data(),
				V:                    v.Uint64(),
				R:                    r.Uint64(),
				S:                    s.Uint64(),
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
	now := time.Now().Unix()
	head := TruePbftBlockHeader{
		Number:				10,
		GasLimit:			100,
		GasUsed:			80,
		Time:				uint64(now),
	}

	block.Header = &head

	msg := rlpHash(block.Txs)
	//cc := cmm.GetCmm()
	sigs := make([]string,0,0)
	//hex.DecodeString(crypto.toECDSA())
	// same priveatekey to sign the message
	_,_,priv := th.GetNodeID()

	priv_d,_ := hex.DecodeString(priv)
	privKey,_ :=crypto.ToECDSA(priv_d)
	sig,err := crypto.Sign(msg[:],privKey)
	if err == nil {
		sigs = append(sigs,common.ToHex(sig))
	}

	block.Sigs=sigs

	th.AddBlock(block)
	//rw := & p2p.MsgReadWriter{}
	//p2p.Send(rw, NewBftBlockMsg, []interface{}{block})

}






