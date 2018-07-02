package truechain

import (
	"testing"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	//"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/core/types"
	"crypto/ecdsa"
	"encoding/hex"
)


var (
	th = New()
	privkeys = make([]*ecdsa.PrivateKey,0,0)
	keysCount = 6
	tx1 = types.NewTransaction(
		0,
		common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
		big.NewInt(0), 0, big.NewInt(0),
		nil,
	)
	tx2 = types.NewTransaction(
		0,
		common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
		big.NewInt(0), 0, big.NewInt(0),
		nil,
	)
	tx3, _ = types.NewTransaction(
		3,
		common.HexToAddress("b94f5374fce5edbc8e2a8697c15331677e6ebf0b"),
		big.NewInt(10),
		2000,
		big.NewInt(1),
		common.FromHex("5544"),
	).WithSignature(
		types.NewEIP155Signer(common.Big1),
		common.Hex2Bytes("98ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4a8887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a301"),
	)
)
func init(){
	for i:=0;i<keysCount;i++ {
		k,_ := crypto.GenerateKey()
		privkeys = append(privkeys,k)
	}
}

func MakePbftBlock(cmm *PbftCommittee) *TruePbftBlock {
	txs := make([]*types.Transaction,0,0)
	txs = append(txs,tx1,tx2)
	pbTxs := make([]*Transaction,0,0)
	for _,vv := range txs {
		to := make([]byte,0,0)
		if tt := vv.To(); tt != nil {
			to = tt.Bytes()
		}
		v,r,s := vv.RawSignatureValues()
		pbTxs = append(pbTxs,&Transaction{
			Data:       &TxData{
				AccountNonce:       vv.Nonce(),
				Price:              vv.GasPrice().Int64(),
				GasLimit:           new(big.Int).SetUint64(vv.Gas()).Int64(),
				Recipient:          to,
				Amount:             vv.Value().Int64(),
				Payload:            vv.Data(),
				V:                  v.Int64(),
				R:                  r.Int64(),
				S:                  s.Int64(),
			},
		})
	}
	// begin make pbft block
	now := time.Now().Unix()
	head := TruePbftBlockHeader{
		Number:				10,
		GasLimit:			100,
		GasUsed:			80,
		Time:				now,
	}
	block := TruePbftBlock{
		Header:			&head,
		Txs:			&Transactions{Txs:pbTxs},
	}
	msg := rlpHash(block.Txs)
	//cc := cmm.GetCmm()
	sigs := make([]string,0,0)
	// same priveatekey to sign the message
	for i:=0;i<keysCount/2;i++ {
		sig,err := crypto.Sign(msg,privkeys[i])
		if err == nil {
			sigs = append(sigs,common.ToHex(sig))
		}
	}
	return &block
}
func MakeFirstCommittee() *PbftCommittee{
	curCmm := make([]*CommitteeMember,0,0)
	curCmmCount := keysCount/2
	for i:=0;i<curCmmCount;i++ {
		cc := CommitteeMember{
			addr:			"127.0.0.1",
			port:			16745,
			Nodeid:			hex.EncodeToString(crypto.FromECDSAPub(
							&ecdsa.PublicKey{
								Curve: 	privkeys[i].Curve,
								X: 		big.NewInt(privkeys[i].X.Int64()),
								Y: 		big.NewInt(privkeys[i].Y.Int64())})),
		}
		curCmm = append(curCmm,&cc)
	}

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
	for i:=0;i<keysCount/2;i++ {
		k,_ := crypto.Sign(cmm.GetHash(),privkeys[i])
		sig = append(sig,common.ToHex(k))
	}
	cmm.SetSig(sig)
	return &cmm
}
func MakeCdCommittee() *PbftCdCommittee{
	cd := &PbftCdCommittee{
		Cm:					make([]*CdMember,0,0),
		VCdCrypMsg:			make([]*cdEncryptionMsg,0,0),
		NCdCrypMsg:			make([]*cdEncryptionMsg,0,0),
	}
	return cd
}
func TestCryptoMsg(t *testing.T) {
	priv := privkeys[0]
	pub := ecdsa.PublicKey{
		Curve: 	priv.Curve,
		X: 		big.NewInt(priv.X.Int64()),
		Y: 		big.NewInt(priv.Y.Int64()),
	}
	nodeid := hex.EncodeToString(crypto.FromECDSAPub(&pub))
	addr := crypto.PubkeyToAddress(pub)

	n := CdMember{
		Nodeid:				nodeid,
		coinbase:			addr.String(),
		addr:				"127.0.0.1",
		port:				16745,
		Height:				big.NewInt(100),
		comfire:			false,
	}
	cryMsg,err := MakeSignedStandbyNode(&n,priv)
	if err != nil {
		fmt.Println("makeCryptoMsg failed,err=",err)
	}
	res := verityMsg(cryMsg,nil)
	fmt.Println(res)
}
func TestNewCommittee(t *testing.T) {
	// new committee msg from py-PBFT for view-change
	// construct cdm
	th.Cdm = MakeCdCommittee()
	// make crypto msg
	cmsg := SignCommittee{

	}
	cmm,err := th.MakeNewCommittee(&cmsg)
	if err != nil {
		fmt.Println("makeNewCommittee,err=",err)
	}
	th.Cmm = cmm
}
func TestPbftBlock(t *testing.T) {
	th.Cmm = MakeFirstCommittee()
	// test make pbft block
	block := MakePbftBlock(th.Cmm)
	// verify the pbft block
	err := th.CheckBlock(block)
	if err != nil {
		fmt.Println("verify the block failed,err=",err)
		return
	}
}
func TestMainMembers(t *testing.T) {
	err := th.StartTrueChain(nil)
	if err != nil {
		fmt.Println(err)
	}
	// test make pbft block
	block := MakePbftBlock(th.Cmm)
	// verify the pbft block
	err = th.CheckBlock(block)
	if err != nil {
		fmt.Println("verify the block failed,err=",err)
		return
	}
	// test cryptomsg for candidate Member

	th.StopTrueChain()
}