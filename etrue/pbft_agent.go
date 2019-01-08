// Copyright 2018 The Truechain Authors
// This file is part of the truechain-engineering-code library.
//
// The truechain-engineering-code library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The truechain-engineering-code library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the truechain-engineering-code library. If not, see <http://www.gnu.org/licenses/>.

package etrue

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/consensus"
	elect "github.com/truechain/truechain-engineering-code/consensus/election"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/metrics"
	"github.com/truechain/truechain-engineering-code/params"
)

const (
	currentCommittee = iota //current running committee
	nextCommittee           //next committee
)
const (
	chainHeadSize    = 256
	electionChanSize = 64
	sendNodeTime     = 30 * time.Second
	//subSignStr       = 24

	fetchBlockTime = 1
	blockInterval  = 20
)

var (
	tpsMetrics           = metrics.NewRegisteredMeter("etrue/pbftAgent/tps", nil)
	pbftConsensusCounter = metrics.NewRegisteredCounter("etrue/pbftAgent/pbftConsensus", nil)
)

var (
	txSum           uint64
	timeSlice       []uint64
	txSlice         []uint64
	instantTpsSlice []float32
	averageTpsSlice []float32
)

// Backend wraps all methods required for  pbft_agent
type Backend interface {
	BlockChain() *core.BlockChain
	SnailBlockChain() *snailchain.SnailBlockChain
	TxPool() *core.TxPool
	Config() *Config
}

// PbftAgent receive events from election and communicate with pbftServer
type PbftAgent struct {
	config     *params.ChainConfig
	fastChain  *core.BlockChain
	snailChain *snailchain.SnailBlockChain
	engine     consensus.Engine
	eth        Backend
	signer     types.Signer
	current    *AgentWork

	currentCommitteeInfo *types.CommitteeInfo
	nextCommitteeInfo    *types.CommitteeInfo
	committeeIds         []*big.Int
	endFastNumber        map[*big.Int]*big.Int

	server   types.PbftServerProxy
	election *elect.Election

	mu           *sync.Mutex //generateBlock mutex
	cacheBlockMu *sync.Mutex //PbftAgent.cacheBlock mutex
	mux          *event.TypeMux

	signFeed         event.Feed
	nodeInfoFeed     event.Feed
	NewFastBlockFeed event.Feed
	scope            event.SubscriptionScope //send scope

	electionCh    chan types.ElectionEvent
	cryNodeInfoCh chan *types.EncryptNodeMessage
	chainHeadCh   chan types.ChainFastHeadEvent

	electionSub       event.Subscription
	chainHeadAgentSub event.Subscription

	committeeNode *types.CommitteeNode
	privateKey    *ecdsa.PrivateKey
	vmConfig      vm.Config

	//cacheSign  map[string]types.Sign     //prevent receive same sign
	cacheBlock map[*big.Int]*types.Block //prevent receive same block
	singleNode bool

	nodeInfoWorks []*nodeInfoWork
}

// AgentWork is the leader current environment and holds
// all of the current state information
type AgentWork struct {
	config *params.ChainConfig
	signer types.Signer

	state   *state.StateDB // apply state changes here
	tcount  int            // tx count in cycle
	gasPool *core.GasPool  // available gas used to pack transactions

	Block     *types.Block // the new block
	header    *types.Header
	txs       []*types.Transaction
	receipts  []*types.Receipt
	createdAt time.Time
}

// NewPbftAgent creates a new pbftAgent ,receive events from election and communicate with pbftServer
func NewPbftAgent(eth Backend, config *params.ChainConfig, engine consensus.Engine, election *elect.Election, coinbase common.Address) *PbftAgent {
	agent := &PbftAgent{
		config:               config,
		engine:               engine,
		eth:                  eth,
		fastChain:            eth.BlockChain(),
		snailChain:           eth.SnailBlockChain(),
		currentCommitteeInfo: new(types.CommitteeInfo),
		nextCommitteeInfo:    new(types.CommitteeInfo),
		committeeIds:         make([]*big.Int, 3),
		endFastNumber:        make(map[*big.Int]*big.Int),
		electionCh:           make(chan types.ElectionEvent, electionChanSize),
		chainHeadCh:          make(chan types.ChainFastHeadEvent, chainHeadSize),
		cryNodeInfoCh:        make(chan *types.EncryptNodeMessage),
		election:             election,
		mux:                  new(event.TypeMux),
		mu:                   new(sync.Mutex),
		cacheBlockMu:         new(sync.Mutex),
		cacheBlock:           make(map[*big.Int]*types.Block),
		vmConfig:             vm.Config{EnablePreimageRecording: eth.Config().EnablePreimageRecording},
	}
	agent.initNodeInfo(eth.Config(), coinbase)
	if !agent.singleNode {
		agent.subScribeEvent()
	}
	return agent
}

//initialize node info
func (agent *PbftAgent) initNodeInfo(config *Config, coinbase common.Address) {
	agent.initNodeWork()
	agent.singleNode = config.NodeType
	agent.privateKey = config.PrivateKey
	agent.committeeNode = &types.CommitteeNode{
		IP:        config.Host,
		Port:      uint(config.Port),
		Port2:     uint(config.StandbyPort),
		Coinbase:  coinbase,
		Publickey: crypto.FromECDSAPub(&agent.privateKey.PublicKey),
	}
	//if singlenode start, node as committeeMember
	if agent.singleNode {
		committees := agent.election.GetGenesisCommittee()
		if len(committees) != 1 {
			log.Error("singlenode start,must assign genesis_single.json")
		}
		agent.committeeNode.Coinbase = committees[0].Coinbase
		agent.committeeNode.Publickey = crypto.FromECDSAPub(committees[0].Publickey)
	}
	log.Info("InitNodeInfo", "singleNode", agent.singleNode,
		", port", config.Port, ", standByPort", config.StandbyPort, ", Host", config.Host,
		", coinbase", agent.committeeNode.Coinbase,
		",pubKey", hex.EncodeToString(agent.committeeNode.Publickey))
}

//initialize nodeInfoWorks
func (agent *PbftAgent) initNodeWork() {
	nodeWork1 := &nodeInfoWork{
		cacheSign:     make(map[string]types.Sign),
		ticker:        time.NewTicker(sendNodeTime),
		committeeInfo: new(types.CommitteeInfo),
		tag:           1,
	}
	nodeWork2 := &nodeInfoWork{
		cacheSign:     make(map[string]types.Sign),
		ticker:        time.NewTicker(sendNodeTime),
		committeeInfo: new(types.CommitteeInfo),
		tag:           2,
	}
	agent.nodeInfoWorks = append(agent.nodeInfoWorks, nodeWork1, nodeWork2)
}

//Start means receive events from election and send pbftNode infomation
func (agent *PbftAgent) Start() {
	if agent.singleNode { //single node model start
		go agent.singleloop()
	} else {
		go agent.loop()
	}
}

func (agent *PbftAgent) subScribeEvent() {
	agent.electionSub = agent.election.SubscribeElectionEvent(agent.electionCh)
	agent.chainHeadAgentSub = agent.fastChain.SubscribeChainHeadEvent(agent.chainHeadCh)
}

// Unsubscribe all subscriptions registered from agent
func (agent *PbftAgent) stop() {
	agent.electionSub.Unsubscribe()
	agent.chainHeadAgentSub.Unsubscribe()
	agent.scope.Close()
}

type nodeInfoWork struct {
	tag               int
	committeeInfo     *types.CommitteeInfo
	cryptoNode        *types.EncryptNodeMessage
	cacheSign         map[string]types.Sign //prevent receive same sign
	ticker            *time.Ticker
	isCommitteeMember bool
	isCurrent         bool
}

//update the nodeInfoWorks element isCurrent,and return nodeInfoWork that the isCurrent equals true
func (agent *PbftAgent) updateCurrentNodeWork() *nodeInfoWork {
	if agent.nodeInfoWorks[0].isCurrent {
		agent.nodeInfoWorks[0].isCurrent = false
		agent.nodeInfoWorks[1].isCurrent = true
		return agent.nodeInfoWorks[1]
	}
	agent.nodeInfoWorks[0].isCurrent = true
	agent.nodeInfoWorks[1].isCurrent = false
	return agent.nodeInfoWorks[0]

}

func (agent *PbftAgent) getCurrentNodeWork() *nodeInfoWork {
	if !agent.nodeInfoWorks[0].isCurrent {
		return agent.nodeInfoWorks[0]
	}
	return agent.nodeInfoWorks[1]
}

//stop send committeeNode
func (agent *PbftAgent) stopSend() {
	nodeWork := agent.getCurrentNodeWork()
	agent.debugNodeInfoWork(nodeWork, "stopSend...")
	//clear nodeWork
	if nodeWork.isCommitteeMember {
		log.Info("nodeWork ticker stop", "committeeId", nodeWork.committeeInfo.Id)
		nodeWork.ticker.Stop() //stop ticker send nodeInfo
	}
	//clear nodeWork
	nodeWork.loadNodeWork(new(types.CommitteeInfo), false)
}

func (nodeWork *nodeInfoWork) loadNodeWork(receivedCommitteeInfo *types.CommitteeInfo, isCommitteeMember bool) {
	nodeWork.committeeInfo = receivedCommitteeInfo
	nodeWork.isCommitteeMember = isCommitteeMember
	nodeWork.cacheSign = make(map[string]types.Sign)
	nodeWork.cryptoNode = nil
}

func (agent *PbftAgent) debugNodeInfoWork(node *nodeInfoWork, str string) {
	log.Debug(str, "tag", node.tag, "isMember", node.isCommitteeMember, "isCurrent", node.isCurrent,
		"nodeWork1", agent.nodeInfoWorks[0].isCurrent, "nodeWork2", agent.nodeInfoWorks[1].isCurrent,
		"committeeId", node.committeeInfo.Id, "committeeInfoMembers", len(node.committeeInfo.Members),
		"cacheSignLen", len(node.cacheSign))
	if node.cryptoNode != nil {
		log.Info(str, "len(cryptoNode.Nodes)", len(node.cryptoNode.Nodes))
	}
}

//start send committeeNode
func (agent *PbftAgent) startSend(receivedCommitteeInfo *types.CommitteeInfo, isCommitteeMember bool) {
	nodeWork := agent.updateCurrentNodeWork()
	//load nodeWork
	nodeWork.loadNodeWork(receivedCommitteeInfo, isCommitteeMember)
	if nodeWork.isCommitteeMember { //if node is current CommitteeMember
		log.Info("node in pbft committee", "committeeId", receivedCommitteeInfo.Id)
		nodeWork.ticker = time.NewTicker(sendNodeTime)
		go func() {
			for {
				select {
				case <-nodeWork.ticker.C:
					agent.sendPbftNode(nodeWork)
				}
			}
		}()
	} else {
		log.Info("node not in pbft committee", "committeeId", receivedCommitteeInfo.Id)
	}
	agent.debugNodeInfoWork(nodeWork, "into startSend...After...")
}

func (agent *PbftAgent) handlePbftNode(cryNodeInfo *types.EncryptNodeMessage, nodeWork *nodeInfoWork) {
	log.Debug("into handlePbftNode...", "commiteeId", cryNodeInfo.CommitteeID)
	/*signStr := hex.EncodeToString(cryNodeInfo.Sign)
	if len(signStr) > subSignStr {
		signStr = signStr[:subSignStr]
	}
	if bytes.Equal(nodeWork.cacheSign[signStr], []byte{}) {
		agent.debugNodeInfoWork(nodeWork, "into handlePbftNode2...")
		nodeWork.cacheSign[signStr] = cryNodeInfo.Sign
		agent.receivePbftNode(cryNodeInfo)
	} else {
		log.Debug("not received pbftnode.")
	}*/

	agent.debugNodeInfoWork(nodeWork, "into handlePbftNode2...")
	agent.receivePbftNode(cryNodeInfo)
}

func (agent *PbftAgent) verifyCommitteeID(electionType uint, committeeID *big.Int) bool {
	switch electionType {
	case types.CommitteeStart:
		log.Debug("CommitteeStart...", "Id", committeeID)
		if agent.committeeIds[1] == committeeID {
			log.Warn("CommitteeStart two times", "committeeId", committeeID)
			return false
		}
		agent.committeeIds[1] = committeeID
	case types.CommitteeStop:
		log.Debug("CommitteeStop..", "Id", committeeID)
		if agent.committeeIds[2] == committeeID {
			log.Warn("CommitteeStop two times", "committeeId", committeeID)
			return false
		}
		agent.committeeIds[2] = committeeID
	case types.CommitteeSwitchover:
		log.Debug("CommitteeSwitchover...", "Id", committeeID)
		if agent.committeeIds[0] == committeeID {
			log.Warn("CommitteeSwitchover two times", "committeeId", committeeID)
			return false
		}
		agent.committeeIds[0] = committeeID
	}
	return true
}

func (agent *PbftAgent) loop() {
	defer agent.stop()
	for {
		select {
		case ch := <-agent.electionCh:
			switch ch.Option {
			case types.CommitteeStart:
				committeeID := copyCommitteeID(ch.CommitteeID)
				if !agent.verifyCommitteeID(ch.Option, committeeID) {
					continue
				}
				agent.setCommitteeInfo(currentCommittee, agent.nextCommitteeInfo)
				if agent.IsCommitteeMember(agent.currentCommitteeInfo) {
					go agent.server.Notify(committeeID, int(ch.Option))
				}
			case types.CommitteeStop:
				committeeID := copyCommitteeID(ch.CommitteeID)
				if !agent.verifyCommitteeID(ch.Option, committeeID) {
					continue
				}
				if agent.IsCommitteeMember(agent.currentCommitteeInfo) {
					go agent.server.Notify(committeeID, int(ch.Option))
				}
				agent.stopSend()
			case types.CommitteeSwitchover:
				committeeID := copyCommitteeID(ch.CommitteeID)
				if !agent.verifyCommitteeID(ch.Option, committeeID) {
					continue
				}
				if len(ch.CommitteeMembers) == 0 {
					log.Error("CommitteeSwitchover receivedMembers is nil ", "committeeId", committeeID)
				}
				receivedCommitteeInfo := &types.CommitteeInfo{
					Id:          committeeID,
					Members:     ch.CommitteeMembers,
					StartHeight: ch.BeginFastNumber, // todo @shuxun fix this default value
				}
				agent.setCommitteeInfo(nextCommittee, receivedCommitteeInfo)

				if agent.IsCommitteeMember(receivedCommitteeInfo) {
					agent.startSend(receivedCommitteeInfo, true)
					agent.server.PutCommittee(receivedCommitteeInfo)
					agent.server.PutNodes(receivedCommitteeInfo.Id, []*types.CommitteeNode{agent.committeeNode})
				} else {
					agent.startSend(receivedCommitteeInfo, false)
				}
			case types.CommitteeOver:
				log.Debug("CommitteeOver...", "CommitteeID", ch.CommitteeID, "EndFastNumber", ch.EndFastNumber)
				committeeID := copyCommitteeID(ch.CommitteeID)
				agent.endFastNumber[committeeID] = ch.EndFastNumber
				agent.server.SetCommitteeStop(committeeID, ch.EndFastNumber.Uint64())
			default:
				log.Warn("unknown election option:", "option", ch.Option)
			}
			//receive nodeInfo
		case cryNodeInfo := <-agent.cryNodeInfoCh:
			if isCommittee, nodeWork := agent.encryptoNodeInCommittee(cryNodeInfo); isCommittee {
				log.Debug("broadcast cryNodeInfo...", "committeeId", cryNodeInfo.CommitteeID)
				go agent.nodeInfoFeed.Send(types.NodeInfoEvent{cryNodeInfo})
				if nodeWork.isCommitteeMember {
					agent.handlePbftNode(cryNodeInfo, nodeWork)
				}
			}
		case ch := <-agent.chainHeadCh:
			//log.Debug("ChainHeadCh putCacheIntoChain.", "Block", ch.Block.Number())
			go agent.putCacheInsertChain(ch.Block)
		}
	}
}

func copyCommitteeID(CommitteeID *big.Int) *big.Int {
	copyID := *CommitteeID
	return &copyID
}

//  when receive block insert chain event ,put cacheBlock into fastchain
func (agent *PbftAgent) putCacheInsertChain(receiveBlock *types.Block) error {
	agent.cacheBlockMu.Lock()
	defer agent.cacheBlockMu.Unlock()
	if len(agent.cacheBlock) == 0 {
		log.Debug("len(agent.cacheBlock) ==0")
		return nil
	}
	log.Debug("len(agent.cacheBlock) !=0")
	var (
		fastBlocks         []*types.Block
		receiveBlockHeight = receiveBlock.Number()
	)
	//delete from cacheBlock map where receiveBlockHeight >= heightNumber
	for number := range agent.cacheBlock {
		if receiveBlockHeight.Cmp(number) >= 0 {
			delete(agent.cacheBlock, number)
		}
	}
	//insert block into Blockchain from cacheBlock map where heightNumber > receiveBlockHeight
	for i := receiveBlockHeight.Uint64() + 1; ; i++ {
		if block, ok := agent.cacheBlock[big.NewInt(int64(i))]; ok {
			fastBlocks = append(fastBlocks, block)
		} else {
			break
		}
	}
	log.Info("putCacheIntoChain", "fastBlocks", len(fastBlocks))
	//insertBlock
	for _, fb := range fastBlocks {
		_, err := agent.fastChain.InsertChain([]*types.Block{fb})
		if err != nil {
			log.Error("putCacheIntoChain Insertchain error", "number", fb.Number())
			return err
		}
		delete(agent.cacheBlock, fb.Number())
		log.Info("delete from cacheBlock", "number", fb.Number())
		//braodcast sign
		voteSign, err := agent.GenerateSign(fb)
		if err != nil {
			continue
		}
		go agent.signFeed.Send(types.PbftSignEvent{Block: fb, PbftSign: voteSign})
	}
	return nil
}

//committeeNode braodcat:if parentBlock is not in fastChain,put block  into cacheblock
func (agent *PbftAgent) handleConsensusBlock(receiveBlock *types.Block) error {
	receiveBlockHeight := receiveBlock.Number()
	if agent.fastChain.CurrentBlock().Number().Cmp(receiveBlockHeight) >= 0 {
		if err := agent.sendSign(receiveBlock); err != nil {
			return err
		}
		log.Info("handleConsensusBlock: blok already insert blockchain",
			"CurrentBlockNumber", agent.fastChain.CurrentBlock().Number(), "receiveBlockNumber", receiveBlockHeight)
		return nil
	}
	//agent.fastChain.CurrentBlock()
	parent := agent.fastChain.GetBlock(receiveBlock.ParentHash(), receiveBlock.NumberU64()-1)
	if parent != nil {
		var fastBlocks []*types.Block
		fastBlocks = append(fastBlocks, receiveBlock)

		//insertBlock
		_, err := agent.fastChain.InsertChain(fastBlocks)
		for _, fb := range fastBlocks {
			log.Info("Finalize: BroadcastConsensus", "Height:", fb.Number(), "len:", len(fastBlocks))
		}
		if err != nil {
			log.Error("agent.fastChain.InsertChain error ", "err", err)
			return err
		}
		//test tps
		GetTps(receiveBlock)
		if err := agent.sendSign(receiveBlock); err != nil {
			return err
		}
	} else {
		log.Warn("handleConsensusBlock parent not in fastchain.")
		agent.cacheBlockMu.Lock()
		agent.cacheBlock[receiveBlockHeight] = receiveBlock
		agent.cacheBlockMu.Unlock()
	}
	return nil
}

func (agent *PbftAgent) sendSign(receiveBlock *types.Block) error {
	//generate sign
	voteSign, err := agent.GenerateSign(receiveBlock)
	if err != nil {
		return err
	}
	log.Info("handleConsensusBlock generate sign ", "FastHeight", voteSign.FastHeight,
		"FastHash", voteSign.FastHash, "Result", voteSign.Result)
	//braodcast sign and block
	agent.signFeed.Send(types.PbftSignEvent{Block: receiveBlock, PbftSign: voteSign})
	return nil
}

func (agent *PbftAgent) encryptoNodeInCommittee(encryptNode *types.EncryptNodeMessage) (bool, *nodeInfoWork) {
	members1 := agent.nodeInfoWorks[0].committeeInfo.Members
	members2 := agent.nodeInfoWorks[1].committeeInfo.Members
	if len(members1) == 0 && len(members2) == 0 {
		log.Error("received cryNodeInfo members = 0")
		return false, nil
	}
	committeeID1 := agent.nodeInfoWorks[0].committeeInfo.Id
	committeeID2 := agent.nodeInfoWorks[1].committeeInfo.Id
	if committeeID1 == nil && committeeID2 == nil {
		log.Error("received cryNodeInfo committeeId1 and committeeId2 is nil")
		return false, nil
	}

	hash := encryptNode.HashWithoutSign().Bytes()
	pubKey, err := crypto.SigToPub(hash, encryptNode.Sign)
	if err != nil {
		log.Error("encryptoNode SigToPub error", "err", err)
		return false, nil
	}
	pubKeyByte := crypto.FromECDSAPub(pubKey)

	if committeeID1 != nil && committeeID1.Cmp(encryptNode.CommitteeID) == 0 &&
		agent.election.IsCommitteeMember(members1, pubKeyByte) {
		return true, agent.nodeInfoWorks[0]
	}
	if committeeID2 != nil && committeeID2.Cmp(encryptNode.CommitteeID) == 0 &&
		agent.election.IsCommitteeMember(members2, pubKeyByte) {
		return true, agent.nodeInfoWorks[1]
	}
	return false, nil
}

//send committeeNode to p2p,make other committeeNode receive and decrypt
func (agent *PbftAgent) sendPbftNode(nodeWork *nodeInfoWork) {
	/*if nodeWork.cryptoNode != nil {
		log.Debug("into sendPbftNode already")
		nodeWork.cryptoNode.CreatedAt = time.Now()
		agent.nodeInfoFeed.Send(types.NodeInfoEvent{nodeWork.cryptoNode})
		return
	}*/
	log.Debug("into sendPbftNode", "committeeId", nodeWork.committeeInfo.Id)
	cryNodeInfo := encryptNodeInfo(nodeWork.committeeInfo, agent.committeeNode, agent.privateKey)
	//nodeWork.cryptoNode = cryNodeInfo
	agent.nodeInfoFeed.Send(types.NodeInfoEvent{cryNodeInfo})
}

func encryptNodeInfo(committeeInfo *types.CommitteeInfo, committeeNode *types.CommitteeNode,
	privateKey *ecdsa.PrivateKey) *types.EncryptNodeMessage {
	var (
		err         error
		cryNodeInfo = &types.EncryptNodeMessage{
			CreatedAt:   time.Now(),
			CommitteeID: committeeInfo.Id,
		}
	)
	PrintNode("send", committeeNode)
	nodeByte, err := rlp.EncodeToBytes(committeeNode)
	if err != nil {
		log.Error("EncodeToBytes error: ", "err", err)
	}
	var encryptNodes []types.EncryptCommitteeNode
	for _, member := range committeeInfo.Members {
		EncryptCommitteeNode, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(member.Publickey), nodeByte, nil, nil)
		if err != nil {
			log.Error("publickey encrypt node error ", "member.Publickey:", member.Publickey, "err", err)
		}
		encryptNodes = append(encryptNodes, EncryptCommitteeNode)
	}
	cryNodeInfo.Nodes = encryptNodes

	hash := cryNodeInfo.HashWithoutSign().Bytes()
	cryNodeInfo.Sign, err = crypto.Sign(hash, privateKey)
	if err != nil {
		log.Error("sign node error", "err", err)
	}
	return cryNodeInfo
}

//AddRemoteNodeInfo send cryNodeInfo of committeeNode to network
// and recieved by other committeenode
func (agent *PbftAgent) AddRemoteNodeInfo(cryNodeInfo *types.EncryptNodeMessage) error {
	if cryNodeInfo == nil {
		log.Error("AddRemoteNodeInfo cryNodeInfo nil")
		return errors.New("AddRemoteNodeInfo cryNodeInfo nil")
	}
	agent.cryNodeInfoCh <- cryNodeInfo
	return nil
}

func (agent *PbftAgent) receivePbftNode(cryNodeInfo *types.EncryptNodeMessage) {
	log.Debug("into ReceivePbftNode ...")
	committeeNode := decryptNodeInfo(cryNodeInfo, agent.privateKey)
	if committeeNode != nil {
		agent.server.PutNodes(cryNodeInfo.CommitteeID, []*types.CommitteeNode{committeeNode})
	}
}

func decryptNodeInfo(cryNodeInfo *types.EncryptNodeMessage, privateKey *ecdsa.PrivateKey) *types.CommitteeNode {
	//ecdsa.PrivateKey convert to ecies.PrivateKey
	priKey := ecies.ImportECDSA(privateKey)
	for _, encryptNode := range cryNodeInfo.Nodes {
		decryptNode, err := priKey.Decrypt(encryptNode, nil, nil)
		if err == nil { // can Decrypt by priKey
			committeeNode := new(types.CommitteeNode) //receive nodeInfo
			rlp.DecodeBytes(decryptNode, committeeNode)
			PrintNode("receive", committeeNode)
			return committeeNode
		}
	}
	return nil
}

//FetchFastBlock  generate fastBlock as leader
func (agent *PbftAgent) FetchFastBlock(committeeID *big.Int, checkTX bool) (*types.Block, error) {
	log.Debug("into GenerateFastBlock...", "committeeId", committeeID)
	agent.mu.Lock()
	defer agent.mu.Unlock()
	var (
		parent       = agent.fastChain.CurrentBlock()
		parentNumber = parent.Number()
		fastBlock    *types.Block
		feeAmount    = big.NewInt(0)
		tstamp       = time.Now().Unix()
	)

	//validate newBlock number exceed endNumber
	if endNumber := agent.endFastNumber[committeeID]; endNumber != nil && endNumber.Cmp(parent.Number()) != 1 {
		log.Error("FetchFastBlock error", "number:", endNumber, "err", core.ErrExceedNumber)
		return fastBlock, core.ErrExceedNumber
	}
	log.Info("FetchFastBlock ", "parent:", parent.Number(), "hash", parent.Hash())

	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) > 0 {
		tstamp = parent.Time().Int64() + 1
	}
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     parentNumber.Add(parentNumber, common.Big1),
		GasLimit:   core.FastCalcGasLimit(parent),
		Time:       big.NewInt(tstamp),
	}
	//checkTX equals true and pending in txPool has no transaction
	if checkTX {
		if !agent.eth.TxPool().HasTxInPending() {
			return types.NewBlock(header, nil, nil, nil), nil
		}
	}
	//assign Proposer
	pubKey, _ := crypto.UnmarshalPubkey(agent.committeeNode.Publickey)
	header.Proposer = crypto.PubkeyToAddress(*pubKey)

	if err := agent.validateBlockSpace(header); err == types.ErrSnailBlockTooSlow {
		return nil, err
	}
	//getParent by height and hash
	if err := agent.engine.Prepare(agent.fastChain, header); err != nil {
		log.Error("Failed to prepare header for generateFastBlock", "err", err)
		return fastBlock, err
	}
	// Create the current work task and check any fork transitions needed
	err := agent.makeCurrent(parent, header)
	if err != nil {
		log.Error("makeCurrent error", "err", err)
		return fastBlock, err
	}
	work := agent.current

	pending, _ := agent.eth.TxPool().Pending()
	txs := types.NewTransactionsByPriceAndNonce(agent.current.signer, pending)
	work.commitTransactions(agent.mux, txs, agent.fastChain, feeAmount)
	//calculate snailBlock reward
	agent.rewardSnailBlock(header)
	// padding Header.Root, TxHash, ReceiptHash.
	// Create the new block to seal with the consensus engine
	if fastBlock, err = agent.engine.Finalize(agent.fastChain, header, work.state, work.txs, work.receipts, feeAmount); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return fastBlock, err
	}
	log.Debug("generateFastBlock", "Height:", fastBlock.Number())

	voteSign, err := agent.GenerateSign(fastBlock)
	if err != nil {
		log.Error("generateBlock with sign error.", "err", err)
	}
	log.Debug("FetchFastBlock generate sign ", "FastHeight", voteSign.FastHeight,
		"FastHash", voteSign.FastHash, "Result", voteSign.Result)
	if voteSign != nil {
		fastBlock.AppendSign(voteSign)
	}
	log.Debug("out GenerateFastBlock...")
	return fastBlock, err
}

//GetCurrentHeight return  current fastBlock number
func (agent *PbftAgent) GetCurrentHeight() *big.Int {
	num := new(big.Int).Set(agent.fastChain.CurrentBlock().Number())
	log.Info("Server GetCurrentHeight", "height", num.Uint64())
	return num
}

//validate space between latest fruit number of snailchain  and  lastest fastBlock number
func (agent *PbftAgent) validateBlockSpace(header *types.Header) error {
	if agent.singleNode {
		return nil
	}
	snailBlock := agent.snailChain.CurrentBlock()
	if snailBlock.NumberU64() == 0 {
		space := new(big.Int).Sub(header.Number, common.Big0).Int64()
		if space >= params.FastToFruitSpace.Int64() {
			log.Warn("snailBlockNumber equals zero", "currentFastNumber", header.Number)
			return types.ErrSnailBlockTooSlow
		}
	}
	blockFruits := snailBlock.Body().Fruits
	if blockFruits != nil && len(blockFruits) > 0 {
		lastFruitNum := blockFruits[len(blockFruits)-1].FastNumber()
		space := new(big.Int).Sub(header.Number, lastFruitNum).Int64()
		if space >= params.FastToFruitSpace.Int64() {
			log.Info("validateBlockSpace method ", "snailNumber", snailBlock.Number(), "lastFruitNum", lastFruitNum,
				"currentFastNumber", header.Number)
			log.Warn("fetchFastBlock validateBlockSpace error", "space", space)
			return types.ErrSnailBlockTooSlow
		}
	}
	return nil
}

//generate rewardSnailHegiht
func (agent *PbftAgent) rewardSnailBlock(header *types.Header) {
	rewardSnailHegiht := agent.fastChain.NextSnailNumberReward()
	space := new(big.Int).Sub(agent.snailChain.CurrentBlock().Number(), rewardSnailHegiht).Int64()
	if space >= params.SnailConfirmInterval.Int64() {
		header.SnailNumber = rewardSnailHegiht
		sb := agent.snailChain.GetBlockByNumber(rewardSnailHegiht.Uint64())
		if sb != nil {
			header.SnailHash = sb.Hash()
		} else {
			log.Error("cannot find snailBlock by rewardSnailHegiht.")
		}
		log.Debug("reward", "rewardSnailHegiht:", rewardSnailHegiht, "currentSnailBlock:",
			agent.snailChain.CurrentBlock().Number(), "space:", space)
	}
}

// GetTps  test Tps
func GetTps(currentBlock *types.Block) float32 {
	/*r.Seed(time.Now().Unix())
	txNum := uint64(r.Intn(1000))*/
	var (
		instantTps float32
		nowTime    = uint64(time.Now().UnixNano() / 1000000)
		txNum      = uint64(len(currentBlock.Transactions()))
	)
	timeSlice = append(timeSlice, nowTime)
	txSum += txNum
	txSlice = append(txSlice, txSum)
	if len(txSlice) > 1 && len(timeSlice) > 1 {
		eachTimeInterval := nowTime - timeSlice[len(timeSlice)-1-1]
		instantTps = 1000 * float32(txNum) / float32(eachTimeInterval)
		log.Debug("tps:", "block", currentBlock.NumberU64(), "instantTps", instantTps, "tx", txNum, "time", eachTimeInterval)
		instantTpsSlice = append(instantTpsSlice, instantTps)

		var timeInterval, txInterval uint64
		if len(timeSlice)-blockInterval > 0 && len(txSlice)-blockInterval > 0 {
			timeInterval = nowTime - timeSlice[len(timeSlice)-1-blockInterval]
			txInterval = txSum - txSlice[len(txSlice)-1-blockInterval]
		} else {
			timeInterval = nowTime - timeSlice[0]
			txInterval = txSum - txSlice[0]
		}
		averageTps := 1000 * float32(txInterval) / float32(timeInterval)
		log.Debug("tps average", "tps", averageTps, "tx", txInterval, "time", timeInterval)
		averageTpsSlice = append(averageTpsSlice, averageTps)
	}
	/*r.Seed(time.Now().Unix())
		instantTps := uint64(r.Intn(100))
	 if len(txSlice)== 1 && len(timeSlice) == 1 {
		 tpsMetrics.Mark(int64(500))
	 }*/
	tpsMetrics.Mark(int64(txNum))
	return instantTps
}

//GenerateSignWithVote  generate sign from committeeMember in fastBlock
func (agent *PbftAgent) GenerateSignWithVote(fb *types.Block, vote uint) (*types.PbftSign, error) {
	voteSign := &types.PbftSign{
		Result:     vote,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	if vote == types.VoteAgreeAgainst {
		log.Warn("vote AgreeAgainst", "number", fb.Number(), "hash", fb.Hash())
	}
	var err error
	signHash := voteSign.HashWithNoSign().Bytes()
	voteSign.Sign, err = crypto.Sign(signHash, agent.privateKey)
	if err != nil {
		log.Error("fb GenerateSign error ", "err", err)
	}
	if voteSign == nil {
		log.Warn("voteSign is nil ", "voteSign", voteSign)
	}
	return voteSign, err
}

//GenerateSign  generate sign with default agree from committeeMember in fastBlock
func (agent *PbftAgent) GenerateSign(fb *types.Block) (*types.PbftSign, error) {
	return agent.GenerateSignWithVote(fb, types.VoteAgree)
}

//BroadcastFastBlock broadcast blockAndSign
func (agent *PbftAgent) BroadcastFastBlock(fb *types.Block) {
	//go agent.NewFastBlockFeed.Send(types.NewBlockEvent{Block: fb})
}

//VerifyFastBlock  committee member  verify fastBlock  and vote agree or disagree sign
func (agent *PbftAgent) VerifyFastBlock(fb *types.Block) (*types.PbftSign, error) {
	log.Debug("into VerifyFastBlock:", "hash:", fb.Hash(), "number:", fb.Number(), "parentHash:", fb.ParentHash())
	// get current head
	var (
		bc     = agent.fastChain
		parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	)
	if parent == nil { //if cannot find parent return ErrUnSyncParentBlock
		log.Warn("VerifyFastBlock ErrHeightNotYet error", "header", fb.Number())
		return nil, types.ErrHeightNotYet
	}
	err := agent.engine.VerifyHeader(bc, fb.Header(), true)
	if err != nil {
		log.Error("verifyFastBlock verifyHeader error", "header", fb.Number(), "err", err)
		voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst)
		if signError != nil {
			return nil, signError
		}
		return voteSign, err
	}
	err = bc.Validator().ValidateBody(fb, false)
	if err != nil {
		// if return blockAlready kown ,indicate block already insert chain by fetch
		if err == core.ErrKnownBlock && agent.fastChain.CurrentBlock().Number().Cmp(fb.Number()) >= 0 {
			log.Info("block already insert chain by fetch .", "number", fb.Number())
			voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgree)
			if signError != nil {
				return nil, signError
			}
			return voteSign, nil //if err equals ErrKnownBlock return nil
		}
		log.Error("verifyFastBlock validateBody error", "height:", fb.Number(), "err", err)
		voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst)
		if signError != nil {
			return nil, signError
		}
		return voteSign, err
	}
	if err != nil {
		log.Error("verifyFastBlock verifyRewardBlock error", "header", fb.Number(), "err", err)
		voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst)
		if signError != nil {
			return nil, signError
		}
		return voteSign, err
	}

	//abort, results  :=bc.Engine().VerifyPbftFastHeader(bc, fb.Header(),parent.Header())
	state, err := bc.State()
	if err != nil {
		log.Error("verifyFastBlock getCurrent state error", "height:", fb.Number(), "err", err)
		voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst)
		if signError != nil {
			return nil, signError
		}
		return voteSign, err
	}
	receipts, _, usedGas, err := bc.Processor().Process(fb, state, agent.vmConfig) //update
	log.Info("Finalize: verifyFastBlock", "Height:", fb.Number())
	if err != nil {
		if err == types.ErrSnailHeightNotYet {
			log.Warn("verifyFastBlock :Snail height not yet", "currentFastNumber", fb.NumberU64(),
				"rewardSnailBlock", fb.SnailNumber().Uint64())
			return nil, err
		}
		log.Error("verifyFastBlock process error", "height:", fb.Number(), "err", err)
		voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst)
		if signError != nil {
			return nil, signError
		}
		return voteSign, err
	}
	err = bc.Validator().ValidateState(fb, parent, state, receipts, usedGas)
	if err != nil {
		log.Error("verifyFastBlock validateState error", "Height:", fb.Number(), "err", err)
		voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst)
		if signError != nil {
			return nil, signError
		}
		return voteSign, err
	}
	voteSign, signError := agent.GenerateSignWithVote(fb, types.VoteAgree)
	if signError != nil {
		return nil, signError
	}
	log.Debug("out VerifyFastBlock:", "hash:", fb.Hash(), "number:", fb.Number(), "parentHash:", fb.ParentHash())
	return voteSign, nil
}

//BroadcastConsensus  when More than 2/3 signs with agree,
//  committee Member Reach a consensus  and insert the fastBlock into fastBlockChain
func (agent *PbftAgent) BroadcastConsensus(fb *types.Block) error {
	log.Debug("into BroadcastSign.", "fastHeight", fb.Number())
	agent.mu.Lock()
	defer agent.mu.Unlock()
	//insert bockchain
	err := agent.handleConsensusBlock(fb)
	if err != nil {
		return err
	}
	//record consensus time  of committee
	consensusTime := time.Now().Unix() - fb.Header().Time.Int64()
	pbftConsensusCounter.Clear()
	pbftConsensusCounter.Inc(consensusTime)
	log.Debug("out BroadcastSign.", "fastHeight", fb.Number())
	return nil
}

func (agent *PbftAgent) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := agent.fastChain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &AgentWork{
		config:    agent.config,
		signer:    types.NewEIP155Signer(agent.config.ChainID),
		state:     state,
		header:    header,
		createdAt: time.Now(),
	}
	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	agent.current = work
	return nil
}

func (env *AgentWork) commitTransactions(mux *event.TypeMux, txs *types.TransactionsByPriceAndNonce, bc *core.BlockChain, feeAmount *big.Int) {
	if env.gasPool == nil {
		env.gasPool = new(core.GasPool).AddGas(env.header.GasLimit)
	}
	var coalescedLogs []*types.Log
	for {
		// If we don't have enough gas for any further transactions then we're done
		if env.gasPool.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "have", env.gasPool, "want", params.TxGas)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		from, _ := types.Sender(env.signer, tx)

		// Start executing the transaction
		env.state.Prepare(tx.Hash(), common.Hash{}, env.tcount)

		logs, err := env.commitTransaction(tx, bc, env.gasPool, feeAmount)
		switch err {
		case core.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			env.tcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if len(coalescedLogs) > 0 || env.tcount > 0 {
		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		go func(logs []*types.Log, tcount int) {
			if len(logs) > 0 {
				mux.Post(types.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(types.PendingStateEvent{})
			}
		}(cpy, env.tcount)
	}
}

func (env *AgentWork) commitTransaction(tx *types.Transaction, bc *core.BlockChain, gp *core.GasPool, feeAmount *big.Int) ([]*types.Log, error) {
	snap := env.state.Snapshot()
	receipt, _, err := core.ApplyTransaction(env.config, bc, gp, env.state, env.header, tx, &env.header.GasUsed, feeAmount, vm.Config{})
	if err != nil {
		env.state.RevertToSnapshot(snap)
		return nil, err
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)
	return receipt.Logs, nil
}

//SubscribeNewFastBlockEvent  registers a subscription of NewBlockEvent.
func (agent *PbftAgent) SubscribeNewFastBlockEvent(ch chan<- types.NewBlockEvent) event.Subscription {
	return agent.scope.Track(agent.NewFastBlockFeed.Subscribe(ch))
}

// SubscribeNewPbftSignEvent registers a subscription of PbftSignEvent and
// starts sending event to the given channel.
func (agent *PbftAgent) SubscribeNewPbftSignEvent(ch chan<- types.PbftSignEvent) event.Subscription {
	return agent.scope.Track(agent.signFeed.Subscribe(ch))
}

//SubscribeNodeInfoEvent  registers a subscription of NodeInfoEvent.
func (agent *PbftAgent) SubscribeNodeInfoEvent(ch chan<- types.NodeInfoEvent) event.Subscription {
	return agent.scope.Track(agent.nodeInfoFeed.Subscribe(ch))
}

//IsCommitteeMember  whether agent in  committee member
func (agent *PbftAgent) IsCommitteeMember(committeeInfo *types.CommitteeInfo) bool {
	return agent.election.IsCommitteeMember(committeeInfo.Members, agent.committeeNode.Publickey)
}

// VerifyCommitteeSign verify sign of node is in committee
func (agent *PbftAgent) VerifyCommitteeSign(sign *types.PbftSign) bool {
	if sign == nil {
		log.Error("VerifyCommitteeSign sign is nil")
		return false
	}
	member, err := agent.election.VerifySign(sign)
	if err != nil {
		log.Warn("VerifyCommitteeSign  error", "err", err)
		return false
	}
	return member != nil
}

// ChangeCommitteeLeader trigger view change.
func (agent *PbftAgent) ChangeCommitteeLeader(height *big.Int) bool {
	return false
}

// GetCommitteeNumber get  Committees number from blockHeight
func (agent *PbftAgent) GetCommitteeNumber(blockHeight *big.Int) int32 {
	committees := agent.election.GetCommittee(blockHeight)
	if committees == nil {
		log.Error("GetCommitteeNumber method committees is nil")
		return 0
	}
	if agent.singleNode {
		return 1
	}
	return int32(len(committees))
}

//CommitteeNumber return current committee number
func (agent *PbftAgent) CommitteeNumber() uint64 {
	if nil == agent.currentCommitteeInfo.Id {
		return 0
	}
	return agent.currentCommitteeInfo.Id.Uint64()
}

//GetCommitteeStatus return current committee infomation
func (agent *PbftAgent) GetCommitteeStatus() map[string]interface{} {
	committeeID := agent.CommitteeNumber()
	return agent.server.GetCommitteeStatus(big.NewInt((int64)(committeeID)))
}

func (agent *PbftAgent) setCommitteeInfo(CommitteeType int, newCommitteeInfo *types.CommitteeInfo) {
	if newCommitteeInfo == nil {
		log.Error("setCommitteeInfo is nil ", "CommitteeType", CommitteeType)
		newCommitteeInfo = &types.CommitteeInfo{}
	}
	switch CommitteeType {
	case currentCommittee:
		agent.currentCommitteeInfo = newCommitteeInfo
	case nextCommittee:
		agent.nextCommitteeInfo = newCommitteeInfo
	default:
		log.Warn("CommitteeType is error ")
	}
}

//PrintNode  print CommitteeNode
func PrintNode(str string, node *types.CommitteeNode) {
	log.Debug(str+" CommitteeNode", "IP:", node.IP, "Port:", node.Port,
		"Coinbase:", node.Coinbase, "Publickey:", hex.EncodeToString(node.Publickey)[:6]+"***")
}

//AcquireCommitteeAuth determine whether the node pubKey  is in the specified committee
func (agent *PbftAgent) AcquireCommitteeAuth(fastHeight *big.Int) bool {
	committeeMembers := agent.election.GetCommittee(fastHeight)
	return agent.election.IsCommitteeMember(committeeMembers, agent.committeeNode.Publickey)
}

//func GetSigns(committees []*types.CommitteeMember,fb *types.Block) []

func (agent *PbftAgent) singleloop() {
	log.Info("singleloop start.")
	// sleep a minute to wait election module start and other nodes' connection
	//time.Sleep(time.Minute)
	for {
		// fetch block
		var (
			block *types.Block
			err   error
			cnt   = 0
		)
		for {
			block, err = agent.FetchFastBlock(nil, false)
			if err != nil {
				log.Error("singleloop FetchFastBlock error", "err", err)
				time.Sleep(time.Second)
				continue
			}
			if len(block.Transactions()) == 0 && cnt < fetchBlockTime {
				cnt++
				time.Sleep(time.Second)
				continue
			} else {
				break
			}
		}
		_, err = agent.VerifyFastBlock(block)
		if err != nil {
			log.Error("VerifyFastBlock error", "err", err)
		}
		err = agent.BroadcastConsensus(block)
		if err != nil {
			log.Error("BroadcastConsensus error", "err", err)
		}
	}
}
