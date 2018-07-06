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
	now := time.Now().Unix()
	head := TruePbftBlockHeader{
		Number:				10,
		GasLimit:			100,
		GasUsed:			80,
		Time:				now,
	}

	block.Header = &head

	msg := rlpHash(block.Txs)
	//cc := cmm.GetCmm()
	sigs := make([]string,0,0)
	//hex.DecodeString(crypto.toECDSA())
	// same priveatekey to sign the message
	_,_,priv := th.getNodeID()

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

func CreateCommittee(t *TrueHybrid) {
	curCmm := make([]*CommitteeMember,0,0)
	curCmmCount := 1
	_,pub,strpriv := t.getNodeID()

	cc := CommitteeMember{
		addr:			"127.0.0.1",
		port:			16745,
		Nodeid:			pub,
	}
	curCmm = append(curCmm,&cc)

	cmm := PbftCommittee{
		No:				1,
		ct:				time.Now(),
		lastt:			time.Now(),
		count:			curCmmCount,
		lcount:			0,
		cmm:			curCmm,
		lcmm:			nil,
		sig:			make([]string,0,0),
	}
	sig := cmm.GetSig()
	bp,_ := hex.DecodeString(strpriv)
	priv,_ := crypto.ToECDSA(bp)
	k,_ := crypto.Sign(cmm.GetHash(),priv)
	sig = append(sig,common.ToHex(k))
	cmm.SetSig(sig)
	t.Cmm = &cmm
}


