package consensus

import (
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/config"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
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
	n.PutCommittee(c1)
	n.Notify(c1.Id, Start)
	<-start
}
