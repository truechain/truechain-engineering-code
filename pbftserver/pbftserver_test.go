package pbftserver

import (
	"crypto/ecdsa"
	. "encoding/hex"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

/*
type PbftAgentProxy interface {
	FetchFastBlock() (*FastBlock, error)
	VerifyFastBlock(*FastBlock) error
	BroadcastFastBlock(*FastBlock)
	BroadcastSign(sign *PbftSign,block *FastBlock) error
}
*/

type PbftAgentProxyImp struct {
	Name string
}

func NewPbftAgent(name string) *PbftAgentProxyImp {
	pap := PbftAgentProxyImp{Name: name}
	return &pap
}

var ID = big.NewInt(0)

func getID() *big.Int {
	ID = new(big.Int).Add(ID, big.NewInt(1))
	return ID
}

func (pap *PbftAgentProxyImp) FetchFastBlock(committeeId *big.Int) (*types.Block, error) {
	header := new(types.Header)
	header.Number = getID()
	header.Time = big.NewInt(time.Now().Unix())
	println("[AGENT]", "++++++++", "FetchFastBlock", "Number:", header.Number.Uint64())
	return types.NewBlock(header, nil, nil, nil), nil
}
func (pap *PbftAgentProxyImp) VerifyFastBlock(block *types.Block) (*types.PbftSign, error) {
	//if rand.Intn(100) > 50 {
	//	println("[AGENT]", pap.Name, "VerifyFastBlock", "Number:", block.Header().Number.Uint64(), types.ErrHeightNotYet.Error())
	//	return types.ErrHeightNotYet
	//}
	println("[AGENT]", pap.Name, "VerifyFastBlock", "Number:", block.Header().Number.Uint64())
	return new(types.PbftSign), nil
}

func (pap *PbftAgentProxyImp) BroadcastFastBlock(block *types.Block) {
	println("[AGENT]", pap.Name, "BroadcastFastBlock", "Number:", block.Header().Number.Uint64())
}

func (pap *PbftAgentProxyImp) BroadcastSign(sign *types.PbftSign, block *types.Block) error {
	println("[AGENT]", "--------", "BroadcastSign", "Number:", block.Header().Number.Uint64())
	return nil
}
func (pap *PbftAgentProxyImp) BroadcastConsensus(block *types.Block) error {
	println("[AGENT]", "--------", "BroadcastSign", "Number:", block.Header().Number.Uint64())
	return nil
}

func GetPub(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := ecdsa.PublicKey{
		Curve: priv.Curve,
		X:     new(big.Int).Set(priv.X),
		Y:     new(big.Int).Set(priv.Y),
	}
	return &pub
}

var PbftServerMgrs map[int]*PbftServerMgr

var testCommittee = []*types.CommitteeNode{
	{
		IP:        "192.168.46.88",
		Port:      10080,
		Coinbase:  common.HexToAddress("76ea2f3a002431fede1141b660dbb75c26ba6d97"),
		Publickey: common.Hex2Bytes("04044308742b61976de7344edb8662d6d10be1c477dd46e8e4c433c1288442a79183480894107299ff7b0706490f1fb9c9b7c9e62ae62d57bd84a1e469460d8ac1"),
	},
	{
		IP:        "192.168.46.6",
		Port:      10080,
		Coinbase:  common.HexToAddress("831151b7eb8e650dc442cd623fbc6ae20279df85"),
		Publickey: common.Hex2Bytes("04ae5b1e301e167f9676937a2733242429ce7eb5dd2ad9f354669bc10eff23015d9810d17c0c680a1178b2f7d9abd925d5b62c7a463d157aa2e3e121d2e266bfc6"),
	},

	{
		IP:        "192.168.46.24",
		Port:      10080,
		Coinbase:  common.HexToAddress("1074f7deccf8c66efcd0106e034d3356b7db3f2c"),
		Publickey: common.Hex2Bytes("04013151837b19e4b0e7402ac576e4352091892d82504450864fc9fd156ddf15d22014a0f6bf3c8f9c12d03e75f628736f0c76b72322be28e7b6f0220cf7f4f5fb"),
	},

	{
		IP:        "192.168.46.9",
		Port:      10080,
		Coinbase:  common.HexToAddress("d985e9871d1be109af5a7f6407b1d6b686901fff"),
		Publickey: common.Hex2Bytes("04e3e59c07b320b5d35d65917d50806e1ee99e3d5ed062ed24d3435f61a47d29fb2f2ebb322011c1d2941b4853ce2dc71e8c4af57b59bbf40db66f76c3c740d41b"),
	},
}

func addServer(pap *PbftAgentProxyImp) *PbftServerMgr {
	pr, _ := crypto.GenerateKey()
	pk := GetPub(pr)
	return NewPbftServerMgr(pk, pr, pap)
}

var comm = make(map[int][]byte)

func InitComm() {
	comm[0] = []byte("c1581e25937d9ab91421a3e1a2667c85b0397c75a195e643109938e987acecfc")
	comm[1] = []byte("42c4d734786eedaf5d0c51fd2bc9bbaa6c289ed23710d9381072932456aeca18")
	comm[2] = []byte("d878614f687c663baf7cdcbc32cc0fc88a036cdc6850023d880b03984426a629")
	comm[3] = []byte("26981a9479b7c4d98c546451c13a78b53c695df14c1968a086219edfe60bce2f")
}

func addServer2(id int, agent *PbftAgentProxyImp) *PbftServerMgr {
	InitComm()
	fmt.Println(len(comm[id]), "ddd")
	key, err := DecodeString(string(comm[id]))
	if err != nil {
		fmt.Println(err)
	}
	priv, err := crypto.ToECDSA(key)
	if err != nil {
		fmt.Println(err.Error())
	}
	pk := &ecdsa.PublicKey{
		Curve: priv.Curve,
		X:     new(big.Int).Set(priv.X),
		Y:     new(big.Int).Set(priv.Y),
	}
	// var agent types.PbftAgentProxy
	return NewPbftServerMgr(pk, priv, agent)
}

func TestPbftServerTemp(t *testing.T) {
	b := types.NewBlock(&types.Header{GasLimit: 1}, nil, nil, nil)
	s := new(types.PbftSign)
	c := make([]*types.PbftSign, 0)
	c = append(c, s)
	b.SetSign(c)

	fmt.Println(b)
	return

	for {
		fmt.Println(rand.Intn(100) > 70)

	}

	a := 0
	for i := 49; i >= 0; i-- {
		a += i
	}
	fmt.Println(a)
	return

	//s := []int{1, 2, 3}
	//
	//fmt.Println(s[1:])
	//
	//for i := len(s) - 1; i >= 0; i-- {
	//	if i != 0 {
	//		z := append(s[:i], s[i+1:]...)
	//		fmt.Println(i, z)
	//	} else {
	//		z := s[1:]
	//		fmt.Println(i, z)
	//	}
	//
	//}
}

func TestPbftServerStart(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")
	pa4 := NewPbftAgent("Agent4")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)
	ser3 := addServer(pa3)
	ser4 := addServer(pa4)

	fmt.Println(ser1, ser2, ser3, ser4)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk
	m4 := new(types.CommitteeMember)
	m4.Publickey = ser4.pk

	c1.Members = append(c1.Members, m1, m2, m3, m4)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Port2 = 10021
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Port2 = 10022
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Port2 = 10023
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	node4 := new(types.CommitteeNode)
	node4.IP = "127.0.0.1"
	node4.Port = 10014
	node4.Port2 = 10024
	node4.Publickey = crypto.FromECDSAPub(m4.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3, node4)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)

	ser3.PutCommittee(c1)
	ser3.PutNodes(c1.Id, nodes)

	ser4.PutCommittee(c1)
	ser4.PutNodes(c1.Id, nodes)

	go ser2.Notify(c1.Id, 0)
	go ser3.Notify(c1.Id, 0)
	go ser4.Notify(c1.Id, 0)
	go ser1.Notify(c1.Id, 0)
	<-start
}

func TestPbftServerStart3(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)
	ser3 := addServer(pa3)

	fmt.Println(ser1, ser2, ser3)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	c2 := new(types.CommitteeInfo)
	c2.Id = big.NewInt(1)

	c3 := new(types.CommitteeInfo)
	c3.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk

	c1.Members = append(c1.Members, m1, m2, m3)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Port2 = 10012
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10021
	node2.Port2 = 10022
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10031
	node3.Port2 = 10032
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	go ser1.Notify(c1.Id, 0)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)
	go ser2.Notify(c1.Id, 0)

	ser3.PutCommittee(c1)
	ser3.PutNodes(c1.Id, nodes)
	go ser3.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStartChange3(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)
	ser3 := addServer(pa3)

	fmt.Println(ser1, ser2, ser3)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk

	c1.Members = append(c1.Members, m1, m2, m3)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	go ser1.Notify(c1.Id, 0)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)
	go ser2.Notify(c1.Id, 0)

	ser3.PutCommittee(c1)
	ser3.PutNodes(c1.Id, nodes)
	go ser3.Notify(c1.Id, 0)

	time.Sleep(time.Second * 32)
	ser1.Notify(c1.Id, 1)
	ser2.Notify(c1.Id, 1)
	ser3.Notify(c1.Id, 1)

	time.Sleep(time.Second * 30)

	c2 := new(types.CommitteeInfo)
	c2.Id = big.NewInt(2)
	c2.Members = append(c2.Members, m2, m1, m3)

	ser1.PutCommittee(c2)
	ser2.PutCommittee(c2)
	ser3.PutCommittee(c2)
	time.Sleep(time.Second * 5)
	ser1.PutNodes(c2.Id, nodes)
	ser2.PutNodes(c2.Id, nodes)
	ser3.PutNodes(c2.Id, nodes)

	ser1.Notify(c2.Id, 0)
	ser2.Notify(c2.Id, 0)
	ser3.Notify(c2.Id, 0)
	<-start
}

func TestPbftServerStart2(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)

	fmt.Println(ser1, ser2)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	c2 := new(types.CommitteeInfo)
	c2.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk

	c1.Members = append(c1.Members, m1, m2)
	c2.Members = append(c2.Members, m1, m2)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	go ser1.Notify(c1.Id, 0)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)
	go ser2.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStartOne(t *testing.T) {
	start := make(chan bool)
	pa1 := NewPbftAgent("Agent1")
	ser1 := addServer(pa1)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk

	c1.Members = append(c1.Members, m1)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10009
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	go ser1.Notify(c1.Id, 0)
	<-start
}

func TestPbftServerStartNode1(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")
	pa4 := NewPbftAgent("Agent4")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)
	ser3 := addServer(pa3)
	ser4 := addServer(pa4)

	fmt.Println(ser1, ser2, ser3, ser4)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk
	m4 := new(types.CommitteeMember)
	m4.Publickey = ser4.pk

	c1.Members = append(c1.Members, m1, m2, m3, m4)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	node4 := new(types.CommitteeNode)
	node4.IP = "127.0.0.1"
	node4.Port = 10014
	node4.Publickey = crypto.FromECDSAPub(m4.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3, node4)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	go ser1.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStartNode2(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")
	pa4 := NewPbftAgent("Agent4")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)
	ser3 := addServer(pa3)
	ser4 := addServer(pa4)

	fmt.Println(ser1, ser2, ser3, ser4)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk
	m4 := new(types.CommitteeMember)
	m4.Publickey = ser4.pk

	c1.Members = append(c1.Members, m1, m2, m3, m4)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	node4 := new(types.CommitteeNode)
	node4.IP = "127.0.0.1"
	node4.Port = 10014
	node4.Publickey = crypto.FromECDSAPub(m4.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3, node4)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)
	go ser2.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStartNode3(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")
	pa4 := NewPbftAgent("Agent4")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)
	ser3 := addServer(pa3)
	ser4 := addServer(pa4)

	fmt.Println(ser1, ser2, ser3, ser4)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk
	m4 := new(types.CommitteeMember)
	m4.Publickey = ser4.pk

	c1.Members = append(c1.Members, m1, m2, m3, m4)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	node4 := new(types.CommitteeNode)
	node4.IP = "127.0.0.1"
	node4.Port = 10014
	node4.Publickey = crypto.FromECDSAPub(m4.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3, node4)

	ser3.PutCommittee(c1)
	ser3.PutNodes(c1.Id, nodes)
	go ser3.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStartNode4(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")
	pa4 := NewPbftAgent("Agent4")

	ser1 := addServer(pa1)
	ser2 := addServer(pa2)
	ser3 := addServer(pa3)
	ser4 := addServer(pa4)

	fmt.Println(ser1, ser2, ser3, ser4)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk
	m4 := new(types.CommitteeMember)
	m4.Publickey = ser4.pk

	c1.Members = append(c1.Members, m1, m2, m3, m4)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	node4 := new(types.CommitteeNode)
	node4.IP = "127.0.0.1"
	node4.Port = 10014
	node4.Publickey = crypto.FromECDSAPub(m4.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3, node4)

	ser4.PutCommittee(c1)
	ser4.PutNodes(c1.Id, nodes)
	go ser4.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStart31(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")

	ser1 := addServer2(0, pa1)
	ser2 := addServer2(1, pa2)
	ser3 := addServer2(2, pa3)

	fmt.Println(ser1, ser2, ser3)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	c2 := new(types.CommitteeInfo)
	c2.Id = big.NewInt(1)

	c3 := new(types.CommitteeInfo)
	c3.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk

	c1.Members = append(c1.Members, m1, m2, m3)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Port2 = 10021
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Port2 = 10022
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Port2 = 10023
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	go ser1.Notify(c1.Id, 0)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)
	//go ser2.Notify(c1.Id, 0)

	ser3.PutCommittee(c1)
	ser3.PutNodes(c1.Id, nodes)
	//go ser3.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStart32(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")

	ser1 := addServer2(0, pa1)
	ser2 := addServer2(1, pa2)
	ser3 := addServer2(2, pa3)

	fmt.Println(ser1, ser2, ser3)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	c2 := new(types.CommitteeInfo)
	c2.Id = big.NewInt(1)

	c3 := new(types.CommitteeInfo)
	c3.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk

	c1.Members = append(c1.Members, m1, m2, m3)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Port2 = 10021
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Port2 = 10022
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Port2 = 10023
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	//go ser1.Notify(c1.Id, 0)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)
	go ser2.Notify(c1.Id, 0)

	ser3.PutCommittee(c1)
	ser3.PutNodes(c1.Id, nodes)
	//go ser3.Notify(c1.Id, 0)

	<-start
}

func TestPbftServerStart33(t *testing.T) {
	start := make(chan bool)

	pa1 := NewPbftAgent("Agent1")
	pa2 := NewPbftAgent("Agent2")
	pa3 := NewPbftAgent("Agent3")

	//ser1 := addServer(pa1)
	//ser2 := addServer(pa2)
	//ser3 := addServer(pa3)

	ser1 := addServer2(0, pa1)
	ser2 := addServer2(1, pa2)
	ser3 := addServer2(2, pa3)

	fmt.Println(ser1, ser2, ser3)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	c2 := new(types.CommitteeInfo)
	c2.Id = big.NewInt(1)

	c3 := new(types.CommitteeInfo)
	c3.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = ser1.pk
	m2 := new(types.CommitteeMember)
	m2.Publickey = ser2.pk
	m3 := new(types.CommitteeMember)
	m3.Publickey = ser3.pk

	c1.Members = append(c1.Members, m1, m2, m3)

	ser1.PutCommittee(c1)

	node1 := new(types.CommitteeNode)
	node1.IP = "127.0.0.1"
	node1.Port = 10011
	node1.Port2 = 10021
	node1.Publickey = crypto.FromECDSAPub(m1.Publickey)

	node2 := new(types.CommitteeNode)
	node2.IP = "127.0.0.1"
	node2.Port = 10012
	node2.Port2 = 10022
	node2.Publickey = crypto.FromECDSAPub(m2.Publickey)

	node3 := new(types.CommitteeNode)
	node3.IP = "127.0.0.1"
	node3.Port = 10013
	node3.Port2 = 10023
	node3.Publickey = crypto.FromECDSAPub(m3.Publickey)

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1, node2, node3)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	//go ser1.Notify(c1.Id, 0)

	ser2.PutCommittee(c1)
	ser2.PutNodes(c1.Id, nodes)
	//go ser2.Notify(c1.Id, 0)

	ser3.PutCommittee(c1)
	ser3.PutNodes(c1.Id, nodes)
	go ser3.Notify(c1.Id, 0)

	<-start
}
