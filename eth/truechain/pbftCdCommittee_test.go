package truechain

import (
	"testing"
	"fmt"
	"encoding/hex"
	"math/big"
	"github.com/ethereum/go-ethereum/crypto"
	"crypto/ecdsa"
	"math/rand"
	"time"
)

func GenerateMember(priv *ecdsa.PrivateKey,height int) *CdMember{
	if height == 0{
		//设置随机种子
		rand.Seed(time.Now().Unix())
		//通过随机值[0,3000)
		height= rand.Intn(100)
	}
	pub := GetPub(priv)
	nodeid := hex.EncodeToString(crypto.FromECDSAPub(pub))
	addr := crypto.PubkeyToAddress(*pub)
	fmt.Println("nodeid：",nodeid)
	fmt.Println("addr:",addr.String())
	n := &CdMember{
		Nodeid:				nodeid,
		Coinbase:			addr.String(),
		Addr:				"127.0.0.1",
		Port:				16745,
		Height:				big.NewInt(int64(height)),
		Comfire:			false,
	}
	return n
}

func MakeCdCommittee() *PbftCdCommittee{
	cd := &PbftCdCommittee{
		Cm:					make([]*CdMember,0,0),
		VCdCrypMsg:			make([]*CdEncryptionMsg,0,0),
		NCdCrypMsg:			make([]*CdEncryptionMsg,0,0),
	}
	for i:=0; i<keysCount/2; i++ {
		cdMemeber :=GenerateMember(privkeys[i],100+i)
		cd.Cm = append(cd.Cm,cdMemeber)
	}
	return cd
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

	member := &CdMember{
		Nodeid:				nodeid,
		Coinbase:			coinAddr.String(),
		Addr:				"127.0.0.1",
		Port:				16745,
		Height:				big.NewInt(123),
		Comfire:			false,
	}
	msg,err := ConvertCdMemberToMsg(member,coinPriv)
	if err != nil {
		fmt.Println("ConvertCdMemberToMsg failed...err=",err)
		return
	}
	// if not init the blockchain,it will be add the NCdCrypMsg queue.
	fmt.Println("current crypto msg count1=",len(th.Cdm.VCdCrypMsg)," count2=",len(th.Cdm.NCdCrypMsg))
	th.ReceiveSdmMsg(msg)
	fmt.Println("current crypto msg count1=",len(th.Cdm.VCdCrypMsg)," count2=",len(th.Cdm.NCdCrypMsg))
	th.StopTrueChain()
}

//generate new pbftCdCommittee
func TestNewPbftCdCommittee(t *testing.T) {
	var msgs []*CdEncryptionMsg
	for i:=0;i<10;i++{
		member :=GenerateMember(privkeys[0],i)
		msg,_ :=ConvertCdMemberToMsg(member,privkeys[0])
		msgs = append(msgs,msg)
	}
	pbftCommittee :=&PbftCdCommittee{
		NCdCrypMsg:msgs,
	}
	for msg := range pbftCommittee.NCdCrypMsg{
		fmt.Println(msg)
	}
}