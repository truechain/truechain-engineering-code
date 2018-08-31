package pbftserver

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"math/big"
	"math/rand"
	"testing"
)

/*
type PbftAgentProxy interface {
	FetchFastBlock() (*FastBlock, error)
	VerifyFastBlock(*FastBlock) error
	BroadcastFastBlock(*FastBlock) error
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

var ID = big.NewInt(1)

func getID() *big.Int {
	ID = new(big.Int).Add(ID, big.NewInt(1))
	return ID
}

func (pap *PbftAgentProxyImp) FetchFastBlock() (*types.Block, error) {
	header := new(types.Header)
	header.Number = getID()
	println("[AGENT]", "++++++++", "FetchFastBlock", "Number:", header.Number.Uint64())
	return types.NewBlock(header, nil, nil, nil), nil
}
func (pap *PbftAgentProxyImp) VerifyFastBlock(block *types.Block) error {
	if rand.Intn(100) > 60 {
		return types.ErrHeightNotYet
	}
	println("[AGENT]", pap.Name, "VerifyFastBlock", "Number:", block.Header().Number.Uint64())
	return nil
}

func (pap *PbftAgentProxyImp) BroadcastFastBlock(block *types.Block) error {
	println("[AGENT]", pap.Name, "BroadcastFastBlock", "Number:", block.Header().Number.Uint64())
	return nil
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

func addServer(pap *PbftAgentProxyImp) *PbftServerMgr {
	pr, _ := crypto.GenerateKey()
	pk := GetPub(pr)
	return NewPbftServerMgr(pk, pr, pap)
}

func TestPbftServerTemp(t *testing.T) {
	for {
		fmt.Println(rand.Intn(100) > 70)

	}

	a := 0
	for i := 49; i >= 0; i-- {
		a += i
	}
	fmt.Println(a)
	return

	s := []int{1, 2, 3}

	fmt.Println(s[1:])

	for i := len(s) - 1; i >= 0; i-- {
		if i != 0 {
			z := append(s[:i], s[i+1:]...)
			fmt.Println(i, z)
		} else {
			z := s[1:]
			fmt.Println(i, z)
		}

	}
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
