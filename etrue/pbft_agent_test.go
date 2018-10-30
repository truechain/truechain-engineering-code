package etrue

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
	"time"

	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"testing"
	"github.com/truechain/truechain-engineering-code/common"
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"bytes"
)

var agent *pbftAgentTest

type pbftAgentTest struct {
	priKey          *ecdsa.PrivateKey
	committeeMember *types.CommitteeMember
	committeeNode   *types.CommitteeNode
}

func init() {
	agent = NewPbftAgetTest()
}

func NewPbftAgetTest() *pbftAgentTest {
	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey) //coinbase
	member := &types.CommitteeMember{coinbase, &priKey.PublicKey}

	committeeNode := &types.CommitteeNode{
		IP:        "127.0.0.1",
		Port:      8080,
		Port2:     8090,
		Coinbase:  coinbase,
		Publickey: crypto.FromECDSAPub(&priKey.PublicKey),
	}
	PrintNode("send", committeeNode)
	agentTest := &pbftAgentTest{
		priKey,
		member,
		committeeNode,
	}
	return agentTest
}

func initCommitteeInfo() *types.CommitteeInfo {
	committeeInfo := &types.CommitteeInfo{
		Id:      common.Big1,
		Members: nil,
	}
	for i := 0; i < 4; i++ {
		priKey, err := crypto.GenerateKey()
		if err != nil {
			log.Error("initMembers", "error", err)
		}
		coinbase := crypto.PubkeyToAddress(priKey.PublicKey) //coinbase
		m := &types.CommitteeMember{coinbase, &priKey.PublicKey}
		committeeInfo.Members = append(committeeInfo.Members, m)
	}
	return committeeInfo
}

func TestSendAndReceiveCommitteeNode(t *testing.T) {
	committeeInfo := initCommitteeInfo()
	committeeInfo.Members = append(committeeInfo.Members, agent.committeeMember)
	t.Log(agent.committeeNode)
	cryNodeInfo := encryptNodeInfo(committeeInfo, agent.committeeNode, agent.priKey)
	receivedCommitteeNode := decryptNodeInfo(cryNodeInfo, agent.priKey)
	t.Log(receivedCommitteeNode)
}

func TestSendAndReceiveCommitteeNode2(t *testing.T) {
	committeeInfo := initCommitteeInfo()
	t.Log(agent.committeeNode)
	cryNodeInfo := encryptNodeInfo(committeeInfo, agent.committeeNode, agent.priKey)
	receivedCommitteeNode := decryptNodeInfo(cryNodeInfo, agent.priKey)
	t.Log(receivedCommitteeNode)
}


func validateSign(fb *types.Block,prikey *ecdsa.PrivateKey) bool{
	sign := GenerateSign(fb, prikey)
	voteSign := &types.PbftSign{
		Result:     types.VoteAgree,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	signHash := voteSign.HashWithNoSign().Bytes()
	pubKey, err := crypto.SigToPub(signHash, sign.Sign)
	if err != nil {
		fmt.Println("get pubKey error", err)
	}
	selfPubkey := crypto.FromECDSAPub(agent.committeeMember.Publickey)
	pubBytes := crypto.FromECDSAPub(pubKey)
	if bytes.Equal(pubBytes, selfPubkey) {
		return true
	}else{
		return false
	}
}
func generateFastBlock()	*types.Block{
	db := ethdb.NewMemDatabase()
	BaseGenesis := new(core.Genesis)
	genesis := BaseGenesis.MustFastCommit(db)
	header := &types.Header{
		ParentHash: genesis.Hash(),
		Number:     common.Big1,
		GasLimit:   core.FastCalcGasLimit(genesis),
	}
	fb := types.NewBlock(header, nil, nil, nil)
	return fb
}

func GenerateSign(fb *types.Block, priKey *ecdsa.PrivateKey) *types.PbftSign {
	voteSign := &types.PbftSign{
		Result:     types.VoteAgree,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	var err error
	signHash := voteSign.HashWithNoSign().Bytes()
	voteSign.Sign, err = crypto.Sign(signHash, priKey)
	if err != nil {
		log.Error("fb GenerateSign error ", "err", err)
	}
	return voteSign
}

func TestGenerateSign(t *testing.T) {
	fb :=generateFastBlock()
	t.Log(validateSign(fb,agent.priKey))
}

func TestGenerateSign2(t *testing.T) {
	fb :=generateFastBlock()
	priKey, _ := crypto.GenerateKey()
	t.Log(validateSign(fb,priKey))
}


func TestElectionEvent(t *testing.T) {

}

//////////////////////////////////////////////////////////////////////////////////
func (self *PbftAgent) sendSubScribedEvent() {
	self.electionSub = self.election.SubscribeElectionEvent(self.electionCh)
}

func (self *PbftAgent) sendElectionEvent() {
	e := self.election
	go func() {
		members := e.snailchain.GetGenesisCommittee()[:3]
		fmt.Println("loop")
		if self.singleNode {
			time.Sleep(time.Second * 10)
			fmt.Println("len(members)", len(members))
			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(0),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(0),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(1),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStop,
				CommitteeID:      big.NewInt(0),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(1),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(2),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStop,
				CommitteeID:      big.NewInt(1),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(2),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(3),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStop,
				CommitteeID:      big.NewInt(2),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(3),
				CommitteeMembers: members,
			})
		}
	}()
}
