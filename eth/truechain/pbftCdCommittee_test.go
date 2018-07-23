package truechain

import (
	"testing"
	"fmt"
	"encoding/hex"
	"math/big"
	"github.com/ethereum/go-ethereum/crypto"
	"log"
	"crypto/ecdsa"
	"math/rand"
	"time"
)



var (
	pcc *PbftCdCommittee
	height =0
)
const (
	count  = 5
)


func init(){
	fmt.Println("hah")
	pcc = MakeCdCommittee()
}
func MakeCdCommittee() *PbftCdCommittee{
	cd := &PbftCdCommittee{
		Cm:					make([]*CdMember,0,0),
		VCdCrypMsg:			make([]*CdEncryptionMsg,0,0),
		NCdCrypMsg:			make([]*CdEncryptionMsg,0,0),
	}
	for i:=0; i<count; i++ {
		height +=1
		privateKey,_ := crypto.GenerateKey()
		cdMemeber :=GenerateMember(privateKey,height)
		cd.Cm = append(cd.Cm,cdMemeber)
	}
	for i:=0;i<count;i++{
		height +=1
		privateKey,_ := crypto.GenerateKey()
		member :=GenerateMember(privateKey,height)
		msg,err :=ConvertCdMemberToMsg(member,privateKey)
		if err != nil{
			log.Panic(err)
		}
		cd.NCdCrypMsg =append(cd.NCdCrypMsg,msg)
	}
	for i:=0;i<count;i++{
		height +=1
		privateKey,_ := crypto.GenerateKey()
		member :=GenerateMember(privateKey,height)
		msg,err :=ConvertCdMemberToMsg(member,privateKey)
		if err != nil{
			log.Panic(err)
		}
		cd.VCdCrypMsg =append(cd.VCdCrypMsg,msg)
	}
	return cd
}
func (cm *CdMember) PrintCdMember(){
	fmt.Println("Nodeid:",cm.Nodeid)
	fmt.Println("Coinbase:",cm.Coinbase)
	fmt.Println("Addr:",cm.Addr)
	fmt.Println("Port:",cm.Port)
	fmt.Println("Height:",cm.Height)
	fmt.Println("Comfire:",cm.Comfire)
}
func (msg *CdEncryptionMsg) PrintMsg(){
	fmt.Println("Height:",msg.Height)
	fmt.Println("Msg:",msg.Msg)
	fmt.Println("Sig:",msg.Sig)
	fmt.Println("Use:",msg.Use)
}
func (pcc *PbftCdCommittee)	 PrintCdCommitteeDetail(){
	for _,cm := range  pcc.Cm{
		fmt.Println("***Cm***")
		cm.PrintCdMember()
		fmt.Printf("\n")
	}
	for _,vMsg := range  pcc.VCdCrypMsg{
		fmt.Println("***VCdCrypMsg***")
		vMsg.PrintMsg()
		fmt.Printf("\n")
	}
	for _,nMsg := range  pcc.NCdCrypMsg{
		fmt.Println("***NCdCrypMsg***")
		nMsg.PrintMsg()
		fmt.Printf("\n")
	}
}
func (pcc *PbftCdCommittee) PrintCdCommitteeInfo(){
	fmt.Println("pcc.NCdCrypMsg.size := ",len(pcc.NCdCrypMsg))
	fmt.Println("pcc.VCdCrypMsg.size := ",len(pcc.VCdCrypMsg))
	fmt.Println("pcc.Cm.size := ",len(pcc.Cm))
}


func Test_print1(t *testing.T) {
	fmt.Println("1111")
}

func Test_generateCdCommittee(t *testing.T) {
	pcc.PrintCdCommitteeDetail()
	pcc.PrintCdCommitteeInfo()
}

func Test_addMsgToNCdCrypMsg(t *testing.T) {
	height +=1
	privateKey,_ := crypto.GenerateKey()
	m :=GenerateMember(privateKey,height)
	msg,_ :=ConvertCdMemberToMsg(m,privateKey)

	/*//传入区块到链中
	blockchain =GenearetEthBlockchain()
	blocks :=GenerateEthBlocks(privateKey,int64(height))
	blockchain.InsertChain(blocks)
	block :=blockchain.CurrentBlock()
	fmt.Println(block)
	*/
	pcc.addMsgToNCdCrypMsg(msg)
	pcc.PrintCdCommitteeDetail()
}

func Test_checkTmpMsg(t *testing.T) {
	simulationCreateCommittee(th)
	/*trueBlock :=MakePbftBlock(th.Cmm)
	fmt.Println(trueBlock)*/

	/*Header
	Txs*/
	pcc.checkTmpMsg(blockchain)
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

//generate new pbftCdCommittee
/*func TestNewPbftCdCommittee(t *testing.T) {
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
}*/