package truechain

import (
	"testing"
	"time"
	"encoding/hex"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/common"
	"log"
	"fmt"
	"crypto/ecdsa"
)

func (pcm *PbftCommittee)	printPbftCommittee(){
	fmt.Println("No:",pcm.No)
	fmt.Printf("Ct: %s \n",pcm.Ct.Format("2006-01-02 03:04:05 PM"))
	fmt.Printf("Lastt: %s \n",pcm.Lastt.Format("2006-01-02 03:04:05 PM"))
	fmt.Println("Count:",pcm.Count)
	fmt.Println("Lcount:",pcm.Lcount)

	fmt.Println("***currentCommitteeMember***")
	if pcm.Comm != nil{
		for _,committeeMember :=range pcm.Comm{
			fmt.Println("Nodeid:",committeeMember.Nodeid)
			fmt.Println("Addr:",committeeMember.Addr)
			fmt.Println("Port:",committeeMember.Port)
		}
	}
	fmt.Println("***oldCommitteeMember***")
	if pcm.Lcomm != nil{
		for _,committeeMember :=range pcm.Lcomm{
			fmt.Println("Nodeid:",committeeMember.Nodeid)
			fmt.Println("Addr:",committeeMember.Addr)
			fmt.Println("Port:",committeeMember.Port)
		}
	}
	if pcm.Sig != nil{
		for _,s :=range pcm.Sig{
			fmt.Println("sig:",s)
		}
	}
}

func Test_simulationCreateCommittee(t *testing.T) {
	simulationCreateCommittee(th)
	th.Cmm.printPbftCommittee()
}

//参照CreateCommittee方法，去掉了P2PServer,联调时再引入
func simulationCreateCommittee(t *TrueHybrid) {
	curCmmCount := keysCount
	cmm := PbftCommittee{
		No:				1,
		Ct:				time.Now(),
		Lastt:			time.Now(),
		Count:			curCmmCount,
		Lcount:			0,
		Comm:			nil,
		Lcomm:			nil,
		Sig:			nil,
	}
	curCmm := make([]*CommitteeMember,0,0)
	//obtain Sig
	sig := make([]string,0,0)
	for i:=0;i<curCmmCount;i++{
		cm := GenerateCommitteeMember(privkeys[i])
		curCmm = append(curCmm,cm)

		k,err:= crypto.Sign(cmm.GetHash(),privkeys[i])
		if err != nil{
			log.Panic(err)
		}
		sig = append(sig,common.ToHex(k))

	}
	cmm.Comm =curCmm
	cmm.Sig = sig
	t.Cmm = &cmm
}
func GenerateCommitteeMember(privateKey *ecdsa.PrivateKey)	*CommitteeMember{
	publikKey :=GetPub(privateKey)
	nodeid := hex.EncodeToString(crypto.FromECDSAPub(publikKey))
	return  &CommitteeMember{
		Addr:			"127.0.0.1",
		Port:			16745,
		Nodeid:			nodeid,
	}
}

func Test_CreateCommittee(t *testing.T) {
	simulationCreateCommittee(th)
	fmt.Println(th)
	//th.Cdm =pcc
}