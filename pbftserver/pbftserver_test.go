package pbftserver

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"math/big"
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

func (pap *PbftAgentProxyImp) FetchFastBlock() (*types.Block, error) {
	println(pap.Name, "FetchFastBlock")
	header := new(types.Header)
	return types.NewBlock(header, nil, nil, nil), nil
}
func (pap *PbftAgentProxyImp) VerifyFastBlock(*types.Block) error {
	println(pap.Name, "VerifyFastBlock")
	return nil
}

func (pap *PbftAgentProxyImp) BroadcastFastBlock(*types.Block) error {
	println(pap.Name, "BroadcastFastBlock")
	return nil
}

func (pap *PbftAgentProxyImp) BroadcastSign(sign *types.PbftSign, block *types.FastBlock) error {
	println(pap.Name, "BroadcastSign")
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

	c2 := new(types.CommitteeInfo)
	c2.Id = big.NewInt(2)

	c3 := new(types.CommitteeInfo)
	c3.Id = big.NewInt(3)

	c4 := new(types.CommitteeInfo)
	c4.Id = big.NewInt(4)

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
	node1.IP = "http://127.0.0.1"
	node1.Port = 10001
	node1.CM = m1

	node2 := new(types.CommitteeNode)
	node2.IP = "http://127.0.0.1"
	node2.Port = 10002
	node2.CM = m2

	node3 := new(types.CommitteeNode)
	node3.IP = "http://127.0.0.1"
	node3.Port = 10003
	node3.CM = m3

	node4 := new(types.CommitteeNode)
	node4.IP = "http://127.0.0.1"
	node4.Port = 10004
	node4.CM = m4

	var nodes []*types.CommitteeNode
	nodes = append(nodes, node1)

	ser1.PutCommittee(c1)
	ser1.PutNodes(c1.Id, nodes)
	ser1.Notify(c1.Id, 0)

	ser2.PutCommittee(c2)
	ser2.PutNodes(c2.Id, nodes)
	ser2.Notify(c2.Id, 0)

	ser2.PutCommittee(c3)
	ser2.PutNodes(c3.Id, nodes)
	ser2.Notify(c3.Id, 0)

	ser2.PutCommittee(c4)
	ser2.PutNodes(c4.Id, nodes)
	ser2.Notify(c4.Id, 0)

	<-start
}
