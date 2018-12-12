package etrue

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/types"
	"time"

	"bytes"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"testing"
)

const (
	sendNodeTimeTest = 30 * time.Second
)

var agent *PbftAgent

func init() {
	agent = NewPbftAgetTest()
}

func NewPbftAgetTest() *PbftAgent {
	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey) //coinbase
	committeeNode := &types.CommitteeNode{
		IP:        "127.0.0.1",
		Port:      8080,
		Port2:     8090,
		Coinbase:  coinbase,
		Publickey: crypto.FromECDSAPub(&priKey.PublicKey),
	}
	PrintNode("send", committeeNode)
	pbftAgent := &PbftAgent{
		privateKey:    priKey,
		committeeNode: committeeNode,
	}
	return pbftAgent
}

func generateCommitteeMemberBySelfPriKey() *types.CommitteeMember {
	priKey := agent.privateKey
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey) //coinbase
	committeeMember := &types.CommitteeMember{coinbase, &priKey.PublicKey}
	return committeeMember
}

func InitCommitteeInfo() (*types.CommitteeInfo, []*ecdsa.PrivateKey) {
	var priKeys []*ecdsa.PrivateKey
	committeeInfo := &types.CommitteeInfo{
		Id:      common.Big1,
		Members: nil,
	}
	for i := 0; i < 4; i++ {
		priKey, err := crypto.GenerateKey()
		priKeys = append(priKeys, priKey)
		if err != nil {
			log.Error("initMembers", "error", err)
		}
		coinbase := crypto.PubkeyToAddress(priKey.PublicKey) //coinbase
		m := &types.CommitteeMember{coinbase, &priKey.PublicKey}
		committeeInfo.Members = append(committeeInfo.Members, m)
	}
	return committeeInfo, priKeys
}

func initCommitteeInfoIncludeSelf() *types.CommitteeInfo {
	committeeInfo, _ := InitCommitteeInfo()
	committeeMember := generateCommitteeMemberBySelfPriKey()
	committeeInfo.Members = append(committeeInfo.Members, committeeMember)
	return committeeInfo
}

func TestSendAndReceiveCommitteeNode(t *testing.T) {
	committeeInfo := initCommitteeInfoIncludeSelf()
	t.Log(agent.committeeNode)
	cryNodeInfo := encryptNodeInfo(committeeInfo, agent.committeeNode, agent.privateKey)
	receivedCommitteeNode := decryptNodeInfo(cryNodeInfo, agent.privateKey)
	t.Log(receivedCommitteeNode)
}

func TestSendAndReceiveCommitteeNode2(t *testing.T) {
	committeeInfo, _ := InitCommitteeInfo()
	t.Log(agent.committeeNode)
	cryNodeInfo := encryptNodeInfo(committeeInfo, agent.committeeNode, agent.privateKey)
	receivedCommitteeNode := decryptNodeInfo(cryNodeInfo, agent.privateKey)
	t.Log(receivedCommitteeNode)
}

func validateSign(fb *types.Block, prikey *ecdsa.PrivateKey) bool {
	sign, err := agent.GenerateSign(fb)
	if err != nil {
		log.Error("err", err)
		return false
	}
	signHash := sign.HashWithNoSign().Bytes()
	pubKey, err := crypto.SigToPub(signHash, sign.Sign)
	if err != nil {
		fmt.Println("get pubKey error", err)
	}
	pubBytes := crypto.FromECDSAPub(pubKey)
	pubBytes2 := crypto.FromECDSAPub(&prikey.PublicKey)
	if bytes.Equal(pubBytes, pubBytes2) {
		return true
	}
	return false
}

func generateFastBlock() *types.Block {
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

func TestGenerateSign(t *testing.T) {
	fb := generateFastBlock()
	t.Log(validateSign(fb, agent.privateKey))
}

func TestGenerateSign2(t *testing.T) {
	fb := generateFastBlock()
	priKey, _ := crypto.GenerateKey()
	t.Log(validateSign(fb, priKey))
}

func TestNodeWorkStartAndEnd(t *testing.T) {
	agent.initNodeWork()
	receivedCommitteeInfo := initCommitteeInfoIncludeSelf()
	for i := 0; i < 3; i++ {
		StartNodeWork(receivedCommitteeInfo, true, t)
		time.Sleep(time.Second * 4)
		StopNodeWork(t)
		time.Sleep(time.Second * 4)
	}
}

func StartNodeWork(receivedCommitteeInfo *types.CommitteeInfo, isCommitteeMember bool, t *testing.T) *types.EncryptNodeMessage {
	var cryNodeInfo *types.EncryptNodeMessage
	nodeWork := agent.updateCurrentNodeWork()
	//load nodeWork
	nodeWork.loadNodeWork(receivedCommitteeInfo, isCommitteeMember)
	if nodeWork.isCommitteeMember {
		t.Log("node in pbft committee", "committeeId=", receivedCommitteeInfo.Id)
		nodeWork.ticker = time.NewTicker(sendNodeTimeTest)
		go func() {
			for {
				select {
				case <-nodeWork.ticker.C:
					cryNodeInfo = encryptNodeInfo(nodeWork.committeeInfo, agent.committeeNode, agent.privateKey)
					t.Log("send", cryNodeInfo)
				}
			}
		}()
	} else {
		t.Log("node not in pbft committee", "committeeId", receivedCommitteeInfo.Id)
	}
	printNodeWork(t, nodeWork, "startSend...")
	return cryNodeInfo
}

func StopNodeWork(t *testing.T) {
	nodeWork := agent.getCurrentNodeWork()
	printNodeWork(t, nodeWork, "stopSend...")
	//clear nodeWork
	if nodeWork.isCommitteeMember {
		nodeWork.ticker.Stop() //stop ticker send nodeInfo
	}
	//clear nodeWork
	nodeWork.loadNodeWork(new(types.CommitteeInfo), false)
}

func printNodeWork(t *testing.T, nodeWork *nodeInfoWork, str string) {
	t.Log(str, " tag=", nodeWork.tag, ", isMember=", nodeWork.isCommitteeMember, ", isCurrent=", nodeWork.isCurrent,
		", nodeWork1=", agent.nodeInfoWorks[0].isCurrent, ", nodeWork2=", agent.nodeInfoWorks[1].isCurrent,
		", committeeId=", nodeWork.committeeInfo.Id, ", committeeInfoMembers=", len(nodeWork.committeeInfo.Members),
		", cacheSignLen=", len(nodeWork.cacheSign))
}

//////////////////////////////////////////////////////////////////////////////////
/*func (self *PbftAgent) sendSubScribedEvent() {
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
}*/
