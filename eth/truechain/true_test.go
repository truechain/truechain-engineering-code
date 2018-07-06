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
	th.setCommitteeCount(3)
}
func GetPub(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := ecdsa.PublicKey{
		Curve: 	priv.Curve,
		X: 		big.NewInt(priv.X.Int64()),
		Y: 		big.NewInt(priv.Y.Int64()),
	}
	return &pub
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
		sig,err := crypto.Sign(msg[:],privkeys[i])
		if err == nil {
			sigs = append(sigs,common.ToHex(sig))
		}
	}
	block.Sigs = sigs
	return &block
}
func MakeFirstCommittee(cnt int) *PbftCommittee{
	if cnt > keysCount {
		return nil
	}
	curCmm := make([]*CommitteeMember,0,0)
	curCmmCount := cnt
	for i:=0;i<curCmmCount;i++ {
		cc := CommitteeMember{
			Addr:			"127.0.0.1",
			Port:			16745,
			Nodeid:			hex.EncodeToString(crypto.FromECDSAPub(GetPub(privkeys[i]))),
		}
		curCmm = append(curCmm,&cc)
	}

	cmm := PbftCommittee{
		No:				1,
		Ct:				time.Now(),
		Lastt:			time.Now(),
		Count:			curCmmCount,
		Lcount:			0,
		Comm:			curCmm,
		Lcomm:			nil,
		Sig:			make([]string,0,0),
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
		VCdCrypMsg:			make([]*CdEncryptionMsg,0,0),
		NCdCrypMsg:			make([]*CdEncryptionMsg,0,0),
	}
	for i:=0;i<keysCount/2;i++ {
		priv := privkeys[i]
		pub := GetPub(priv)
		nodeid := hex.EncodeToString(crypto.FromECDSAPub(pub))
		addr := crypto.PubkeyToAddress(*pub)

		n := CdMember{
			Nodeid:				nodeid,
			Coinbase:			addr.String(),
			Addr:				"127.0.0.1",
			Port:				16745,
			Height:				big.NewInt(100+int64(i)),
			Comfire:			false,
		}
		cd.Cm = append(cd.Cm,&n)
	}
	return cd
}

func TestEncryptionMsgInTrueChain(t *testing.T) {
	tmp := struct {
		msg1	[]string
		msg2	[]byte
		msg3	int
		msg4 	float64
	}{
		msg3:	5,
		msg4:	4.5,
	}
	priv := privkeys[0]
	pub := hex.EncodeToString(crypto.FromECDSAPub(GetPub(priv)))
	hash := rlpHash(tmp)
	// sig message use priv
	sig,err := crypto.Sign(hash[:],priv)
	// verify message
	if err == nil {
		verifyPub, err := crypto.SigToPub(hash[:], sig)
		if err == nil {
			strverifyPub := hex.EncodeToString(crypto.FromECDSAPub(verifyPub))
			if pub == strverifyPub {
				fmt.Println("sign test seccess,pub = ", pub, " strverifyPub = ", strverifyPub)

			}
			//if !bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(signerKey)) {
			//	fmt.Println("sign test seccess,pub=",pub,"pub1=",strPub1)
			//}
		}
	}
}
func TestCryptoMsg(t *testing.T) {
	priv := privkeys[0]
	pub := GetPub(priv)
	nodeid := hex.EncodeToString(crypto.FromECDSAPub(pub))
	addr := crypto.PubkeyToAddress(*pub)

	n := CdMember{
		Nodeid:				nodeid,
		Coinbase:			addr.String(),
		Addr:				"127.0.0.1",
		Port:				16745,
		Height:				big.NewInt(100),
		Comfire:			false,
	}
	cmsg,err := MakeSignedStandbyNode(&n,priv)
	if err != nil {
		fmt.Println("makeCryptoMsg failed,err=",err)
	}
	res := verityMsg(cmsg,nil)
	fmt.Println(res)
	th.StopTrueChain()
}
func TestNewCommittee(t *testing.T) {
	// new committee msg from py-PBFT for view-change
	// construct cdm
	th.Cdm = MakeCdCommittee()
	th.Cmm = MakeFirstCommittee(3)
	curCnt := th.GetCommitteeCount()
	m,err := th.Vote(curCnt)
	if err != nil {
		fmt.Println("vote failed...err=",err)
		return
	}
	// make signCommittee from py-PBFT
	tmp := struct {
		msg1	[]*CommitteeMember
		msg2	[]*CommitteeMember
	}{
		msg1:	m,
		msg2:	th.Cmm.GetCmm(),
	}
	msg := rlpHash(tmp)
	cmsg := SignCommittee{
		Msg:		common.ToHex(msg[:]),
	}
	for i:=0;i<curCnt;i++ {
		k,_ := crypto.Sign(msg[:],privkeys[i])
		cmsg.Sigs = append(cmsg.Sigs,common.ToHex(k))
	}
	// verify the signCommittee from the py-PBFT
	cmm,err := th.MakeNewCommittee(&cmsg)
	if err != nil {
		fmt.Println("makeNewCommittee,err=",err)
	}
	th.UpdateLocalCommittee(cmm,true)
	th.StopTrueChain()
}
func TestPbftBlock(t *testing.T) {
	th.Cmm = MakeFirstCommittee(3)
	// test make pbft block
	block := MakePbftBlock(th.Cmm)
	// verify the pbft block
	err := th.CheckBlock(block)
	if err != nil {
		fmt.Println("verify the block failed,err=",err)
		return
	}
	th.StopTrueChain()
}
func TestCandidateMember(t *testing.T) {
	th.Cdm = MakeCdCommittee()
	// test cryptomsg for candidate Member
	// make cryptomsg for add candidate Member
	priv := privkeys[keysCount-1]
	pub := GetPub(priv)
	nodeid := hex.EncodeToString(crypto.FromECDSAPub(pub))
	coinPriv := privkeys[keysCount-2]
	coinAddr := crypto.PubkeyToAddress(*GetPub(coinPriv))

	n := CdMember{
		Nodeid:				nodeid,
		Coinbase:			coinAddr.String(),
		Addr:				"127.0.0.1",
		Port:				16745,
		Height:				big.NewInt(123),
		Comfire:			false,
	}
	cmsg,err := MakeSignedStandbyNode(&n,coinPriv)
	if err != nil {
		fmt.Println("MakeSignedStandbyNode failed...err=",err)
		return
	}
	// if not init the blockchain,it will be add the NCdCrypMsg queue.
	fmt.Println("current crypto msg count1=",len(th.Cdm.VCdCrypMsg)," count2=",len(th.Cdm.NCdCrypMsg))
	th.ReceiveSdmMsg(cmsg)
	fmt.Println("current crypto msg count1=",len(th.Cdm.VCdCrypMsg)," count2=",len(th.Cdm.NCdCrypMsg))
	th.StopTrueChain()
}