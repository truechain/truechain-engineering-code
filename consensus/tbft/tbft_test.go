package consensus

import (
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/config"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"math/big"
	"testing"
)

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

func (pap *PbftAgentProxyImp) FetchFastBlock() (*types.Block, error) {
	header := new(types.Header)
	header.Number = getID()
	println("[AGENT]", "++++++++", "FetchFastBlock", "Number:", header.Number.Uint64())
	return types.NewBlock(header, nil, nil, nil), nil
}
func (pap *PbftAgentProxyImp) VerifyFastBlock(block *types.Block) error {
	//if rand.Intn(100) > 30 {
	//	return types.ErrHeightNotYet
	//}
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

func TestPbftRunForOne(t *testing.T) {
	start := make(chan int)
	pr, _ := crypto.GenerateKey()
	agent1 := NewPbftAgent("Agent1")
	n, _ := NewNode(config.DefaultConfig(), "1", pr, agent1)
	n.Start()
	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr)
	c1.Members = append(c1.Members, m1)
	c1.StartHeight = common.Big1
	n.PutCommittee(c1)
	n.Notify(c1.Id, Start)
	<-start
}

func TestPbftRunFor4(t *testing.T) {
	log.OpenLogDebug(4)
	start := make(chan int)
	pr1, _ := crypto.GenerateKey()
	pr2, _ := crypto.GenerateKey()
	pr3, _ := crypto.GenerateKey()
	pr4, _ := crypto.GenerateKey()
	agent1 := NewPbftAgent("Agent1")
	agent2 := NewPbftAgent("Agent2")
	agent3 := NewPbftAgent("Agent3")
	agent4 := NewPbftAgent("Agent4")

	config1 := new(config.Config)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress = "tcp://127.0.0.1:28890"
	p2p1.ExternalAddress = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1
	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

	config2 := new(config.Config)
	*config2 = *config.TestConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config1.P2P
	p2p2.ListenAddress = "tcp://127.0.0.1:28893"
	p2p2.ExternalAddress = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2
	n2, _ := NewNode(config2, "1", pr2, agent2)
	n2.Start()

	config3 := new(config.Config)
	*config3 = *config.TestConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config1.P2P
	p2p3.ListenAddress = "tcp://127.0.0.1:28895"
	p2p3.ExternalAddress = "tcp://127.0.0.1:28896"
	*config3.P2P = *p2p3
	n3, _ := NewNode(config3, "1", pr3, agent3)
	n3.Start()

	config4 := new(config.Config)
	*config4 = *config.TestConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config1.P2P
	p2p4.ListenAddress = "tcp://127.0.0.1:28897"
	p2p4.ExternalAddress = "tcp://127.0.0.1:28898"
	*config4.P2P = *p2p4
	n4, _ := NewNode(config4, "1", pr4, agent4)
	n4.Start()

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1

	n1.PutCommittee(c1)
	n1.Notify(c1.Id, Start)

	n2.PutCommittee(c1)
	n2.Notify(c1.Id, Start)

	n3.PutCommittee(c1)
	n3.Notify(c1.Id, Start)

	n4.PutCommittee(c1)
	n4.Notify(c1.Id, Start)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

	n1.PutNodes(common.Big1, cn)
	n2.PutNodes(common.Big1, cn)
	n3.PutNodes(common.Big1, cn)
	n4.PutNodes(common.Big1, cn)

	<-start
}
