package truechain

import (
	"testing"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/core/vm"

	"crypto/ecdsa"
	"log"
	"encoding/hex"
)
const(
	keysCount = 5
)

var (
	th = New() //TrueHybrid Object
	privkeys = make([]*ecdsa.PrivateKey,0,0)
	blockchain  *core.BlockChain
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
		privateKey,_ := crypto.GenerateKey()
		privkeys = append(privkeys,privateKey)
	}
	th.Config.CmmCount = 3  //amount of Pbft Committee Members
	GenerateBlockchain()
}

func GetPub(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := ecdsa.PublicKey{
		Curve: 	priv.Curve,
		X: 		new(big.Int).Set(priv.X),
		Y: 		new(big.Int).Set(priv.Y),
	}
	return &pub
}

//convert types.Transaction(ethereum)  into Transaction
func ConvertTransaction(oldTxs []*types.Transaction) []*Transaction{
	pbTxs :=make([]*Transaction,0,0)
	for _,tx := range oldTxs {
		to := make([]byte,0,0)
		if tt := tx.To(); tt != nil {
			to = tt.Bytes()
		}
		v,r,s := tx.RawSignatureValues()
		newTx :=&Transaction{
			Data:       &TxData{
				AccountNonce:       tx.Nonce(),
				Price:              tx.GasPrice().Uint64(),
				GasLimit:           new(big.Int).SetUint64(tx.Gas()).Uint64(),
				Recipient:          to,
				Amount:             tx.Value().Uint64(),
				Payload:            tx.Data(),
				V:                  v.Uint64(),
				R:                  r.Uint64(),
				S:                  s.Uint64(),
			},
		}
		pbTxs = append(pbTxs,newTx)
	}
	return pbTxs
}

//generate sigs
func GenerateSigs(pbBlock TruePbftBlock) []string{
	sigs := make([]string,0,0)
	msg := rlpHash(pbBlock.Txs)
	for i:=0;i<keysCount;i++ {
		// same priveatekey to sign the message
		sig,err := crypto.Sign(msg[:],privkeys[i])
		if err != nil {
			log.Panic(err)
		}
		sigs = append(sigs,common.ToHex(sig))
	}
	return sigs
}

//generate pbftBlock
func MakePbftBlock(cmm *PbftCommittee) *TruePbftBlock {
	txs := make([]*types.Transaction,0,0)
	txs = append(txs,tx1,tx2)
	pbTxs :=ConvertTransaction(txs)
	// begin make pbft block
	head := TruePbftBlockHeader{
		Number:				10,
		GasLimit:			100,
		GasUsed:			80,
		Time:				uint64(time.Now().Unix()),
	}
	block := TruePbftBlock{
		Header:			&head,
		Txs:			&Transactions{Txs:pbTxs},
	}
	block.Sigs =GenerateSigs(block)
	return &block
}

func GenerateBlockchain() {
	blockchain, _ = core.NewBlockChain(ethdb.NewMemDatabase(), nil, params.AllEthashProtocolChanges, nil, vm.Config{})
}

func TestNewCommittee(t *testing.T) {
	// new committee msg from py-PBFT for view-change
	// construct cdm
	th.Cdm = MakeCdCommittee()
	th.Cmm = MakeFirstCommittee(3)
	curCnt := th.Config.CmmCount
	votedMemebers,err := th.Vote(curCnt)//[]*CommitteeMember
	if err != nil {
		fmt.Println("vote failed...err=",err)
		return
	}

	// make signCommittee from py-PBFT
	tmp := struct {
		msg1	[]*CommitteeMember
		msg2	[]*CommitteeMember
	}{
		msg1:	votedMemebers,//new voted pbftMemmbers
		msg2:	th.Cmm.Comm,//current pbftMemmbers
	}
	msg := rlpHash(tmp)
	signCommittee := SignCommittee{
		Msg:		common.ToHex(msg[:]),
	}
	for i:=0;i<curCnt;i++ {
		k,_ := crypto.Sign(msg[:],privkeys[i])
		signCommittee.Sigs = append(signCommittee.Sigs,common.ToHex(k))
	}
	// verify the signCommittee from the py-PBFT
	cmm,err := th.MakeNewCommittee(&signCommittee)
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

func TestDataStruct(t *testing.T) {
	cdMember :=GenerateMember(privkeys[0],1001)

	msg,err1 := toByte(cdMember)
	if err1 != nil {
		fmt.Println(err1)
	}
	sig,err2 := crypto.Sign(msg[:32],privkeys[0])
	if err2 != nil {
		log.Panic(err2)
	}
	fmt.Println(sig)

	n2 := CdMember{}
	err := fromByte(msg,&n2)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("height:",n2.Height)
	fmt.Println(n2)
}

func MakeFirstCommittee(curCmmCount int) *PbftCommittee{
	if curCmmCount > keysCount {
		return nil
	}
	curCmm := make([]*CommitteeMember,0,0)
	for i:=0;i<curCmmCount;i++ {
		nodeid :=hex.EncodeToString(crypto.FromECDSAPub(GetPub(privkeys[i])))
		cc := CommitteeMember{
			Addr:			"127.0.0.1",
			Port:			16745,
			Nodeid:			nodeid,
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
	sig := cmm.Sig
	for i:=0;i<curCmmCount ;i++ {
		k,_ := crypto.Sign(cmm.GetHash(),privkeys[i])
		sig = append(sig,common.ToHex(k))
	}
	cmm.Sig =sig
	return &cmm
}