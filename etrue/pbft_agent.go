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
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/utils"
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
	chainHeadSize       = 256
	electionChanSize    = 64
	nodeSize            = 50
	committeeIDChanSize = 3
	sendNodeTime        = 3 * time.Minute
	maxKnownNodes       = 512
	fetchBlockTime      = 2
)

var (
	oldReceivedMetrics       = metrics.NewRegisteredMeter("etrue/pbftAgent/old", nil)
	repeatReceivedMetrics    = metrics.NewRegisteredMeter("etrue/pbftAgent/repeat", nil)
	newReceivedMetrics       = metrics.NewRegisteredMeter("etrue/pbftAgent/new", nil)
	differentReceivedMetrics = metrics.NewRegisteredMeter("etrue/pbftAgent/different", nil)
	nodeHandleMetrics        = metrics.NewRegisteredMeter("etrue/pbftAgent/handle", nil)

	tpsMetrics           = metrics.NewRegisteredMeter("etrue/pbftAgent/tps", nil)
	pbftConsensusCounter = metrics.NewRegisteredCounter("etrue/pbftAgent/pbftConsensus", nil)
)

// Backend wraps all methods required for  pbft_agent
type Backend interface {
	BlockChain() *core.BlockChain
	SnailBlockChain() *snailchain.SnailBlockChain
	TxPool() *core.TxPool
	Config() *Config
	Etherbase() (etherbase common.Address, err error)
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

	currentCommitteeInfo     *types.CommitteeInfo
	nextCommitteeInfo        *types.CommitteeInfo
	isCurrentCommitteeMember bool
	committeeIds             []*big.Int
	endFastNumber            map[*big.Int]*big.Int

	server   types.PbftServerProxy
	election *elect.Election

	mu           *sync.Mutex //generateBlock mutex
	cacheBlockMu *sync.Mutex //PbftAgent.cacheBlock mutex
	mux          *event.TypeMux

	signFeed     event.Feed
	nodeInfoFeed event.Feed
	scope        event.SubscriptionScope //send scope

	electionCh    chan types.ElectionEvent
	cryNodeInfoCh chan *types.EncryptNodeMessage
	chainHeadCh   chan types.ChainFastHeadEvent

	electionSub       event.Subscription
	chainHeadAgentSub event.Subscription

	committeeNode *types.CommitteeNode
	privateKey    *ecdsa.PrivateKey
	vmConfig      vm.Config

	cacheBlock map[*big.Int]*types.Block //prevent receive same block
	singleNode bool

	nodeInfoWorks      []*nodeInfoWork
	knownRecievedNodes *utils.OrderedMap
	committeeNodeTag   *utils.OrderedMap

	gasFloor uint64
	gasCeil  uint64
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
func NewPbftAgent(etrue Backend, config *params.ChainConfig, engine consensus.Engine, election *elect.Election, gasFloor, gasCeil uint64) *PbftAgent {
	agent := &PbftAgent{
		config:               config,
		engine:               engine,
		eth:                  etrue,
		fastChain:            etrue.BlockChain(),
		snailChain:           etrue.SnailBlockChain(),
		currentCommitteeInfo: new(types.CommitteeInfo),
		nextCommitteeInfo:    new(types.CommitteeInfo),
		committeeIds:         make([]*big.Int, committeeIDChanSize),
		endFastNumber:        make(map[*big.Int]*big.Int),
		electionCh:           make(chan types.ElectionEvent, electionChanSize),
		chainHeadCh:          make(chan types.ChainFastHeadEvent, chainHeadSize),
		cryNodeInfoCh:        make(chan *types.EncryptNodeMessage, nodeSize),
		election:             election,
		mux:                  new(event.TypeMux),
		mu:                   new(sync.Mutex),
		cacheBlockMu:         new(sync.Mutex),
		cacheBlock:           make(map[*big.Int]*types.Block),
		vmConfig:             vm.Config{EnablePreimageRecording: etrue.Config().EnablePreimageRecording},
		gasFloor:             gasFloor,
		gasCeil:              gasCeil,
		knownRecievedNodes:   utils.NewOrderedMap(),
		committeeNodeTag:     utils.NewOrderedMap(),
	}
	agent.initNodeInfo(etrue)
	if !agent.singleNode {
		agent.subScribeEvent()
	}
	log.Info("new pbftAgent", "gasFloor", gasFloor, "gasCeil", gasCeil)
	return agent
}

//initialize node info
func (agent *PbftAgent) initNodeInfo(etrue Backend) {
	//config *Config, coinbase common.Address
	config := etrue.Config()
	coinbase, _ := etrue.Etherbase()
	agent.initNodeWork()
	agent.singleNode = config.NodeType
	agent.privateKey = config.PrivateKey
	agent.committeeNode = &types.CommitteeNode{
		IP:        config.Host,
		Port:      uint32(config.Port),
		Port2:     uint32(config.StandbyPort),
		Coinbase:  coinbase,
		Publickey: crypto.FromECDSAPub(&agent.privateKey.PublicKey),
	}
	//if singlenode start, self as committeeMember
	if agent.singleNode {
		committees := agent.election.GetGenesisCommittee()
		if len(committees) != 1 {
			log.Error("singlenode start,must init genesis_single.json")
		}
		agent.committeeNode.Coinbase = committees[0].Coinbase
		agent.committeeNode.Publickey = committees[0].Publickey
		agent.isCurrentCommitteeMember = true
	}
	log.Info("InitNodeInfo", "singleNode", agent.singleNode,
		", port", config.Port, ", standByPort", config.StandbyPort, ", Host", config.Host,
		", coinbase", agent.committeeNode.Coinbase.String(),
		",pubKey", hex.EncodeToString(agent.committeeNode.Publickey))
}

//initialize nodeInfoWorks
func (agent *PbftAgent) initNodeWork() {
	nodeWork1 := &nodeInfoWork{
		ticker:        time.NewTicker(sendNodeTime),
		committeeInfo: new(types.CommitteeInfo),
		tag:           1,
	}
	nodeWork2 := &nodeInfoWork{
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

// Unsubscribe all subscriptions registered from agent
func (agent *PbftAgent) stop() {
	agent.electionSub.Unsubscribe()
	agent.chainHeadAgentSub.Unsubscribe()
	agent.scope.Close()
}

func (agent *PbftAgent) subScribeEvent() {
	agent.electionSub = agent.election.SubscribeElectionEvent(agent.electionCh)
	agent.chainHeadAgentSub = agent.fastChain.SubscribeChainHeadEvent(agent.chainHeadCh)
}

type nodeInfoWork struct {
	tag               int
	committeeInfo     *types.CommitteeInfo
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

//IsCurrentCommitteeMember get whether self is committee member or not
func (agent *PbftAgent) IsCommitteeMember() bool {
	return agent.isCurrentCommitteeMember
}

//IsLeader get current committee leader
func (agent *PbftAgent) IsLeader() bool {
	if agent.currentCommitteeInfo == nil || agent.currentCommitteeInfo.Id == nil || !agent.currentCommitteeInfo.Id.IsUint64() {
		return false
	}
	return agent.server.IsLeader(agent.currentCommitteeInfo.Id)
}

func (agent *PbftAgent) getCurrentNodeWork() *nodeInfoWork {
	if !agent.nodeInfoWorks[0].isCurrent {
		return agent.nodeInfoWorks[0]
	}
	return agent.nodeInfoWorks[1]
}

func (nodeWork *nodeInfoWork) loadNodeWork(receivedCommitteeInfo *types.CommitteeInfo, isCommitteeMember bool) {
	nodeWork.committeeInfo = receivedCommitteeInfo
	nodeWork.isCommitteeMember = isCommitteeMember
}

//start send committeeNode
func (agent *PbftAgent) startSend(receivedCommitteeInfo *types.CommitteeInfo, isCommitteeMember bool) {
	nodeWork := agent.updateCurrentNodeWork()
	nodeWork.loadNodeWork(receivedCommitteeInfo, isCommitteeMember)
	if nodeWork.isCommitteeMember { //if node is current CommitteeMember
		log.Info("node is committee member", "committeeId", receivedCommitteeInfo.Id)
		agent.sendPbftNode(nodeWork)
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
		log.Info("node is not committee member", "committeeId", receivedCommitteeInfo.Id)
	}
}

//stop send committeeNode
func (agent *PbftAgent) stopSend() {
	nodeWork := agent.getCurrentNodeWork()
	if nodeWork.isCommitteeMember {
		log.Info("nodeWork ticker stop", "committeeId", nodeWork.committeeInfo.Id)
		nodeWork.ticker.Stop()
	}
	nodeWork.loadNodeWork(new(types.CommitteeInfo), false)
}

func (agent *PbftAgent) verifyCommitteeID(electionEventType uint, committeeID *big.Int) bool {
	switch electionEventType {
	case types.CommitteeStart:
		if agent.committeeIds[1] == committeeID {
			log.Warn("CommitteeStart two times", "committeeId", committeeID)
			return false
		}
		agent.committeeIds[1] = committeeID
	case types.CommitteeStop:
		if agent.committeeIds[2] == committeeID {
			log.Warn("CommitteeStop two times", "committeeId", committeeID)
			return false
		}
		agent.committeeIds[2] = committeeID
	case types.CommitteeSwitchover:
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
				if agent.isCommitteeMember(agent.currentCommitteeInfo) {
					agent.isCurrentCommitteeMember = true
					go help.CheckAndPrintError(agent.server.Notify(committeeID, int(ch.Option)))
				} else {
					agent.isCurrentCommitteeMember = false
				}
			case types.CommitteeStop:
				committeeID := copyCommitteeID(ch.CommitteeID)
				if !agent.verifyCommitteeID(ch.Option, committeeID) {
					continue
				}
				if agent.isCommitteeMember(agent.currentCommitteeInfo) {
					go help.CheckAndPrintError(agent.server.Notify(committeeID, int(ch.Option)))
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
					StartHeight: ch.BeginFastNumber,
					Members:     ch.CommitteeMembers,
					BackMembers: ch.BackupMembers,
				}
				agent.setCommitteeInfo(nextCommittee, receivedCommitteeInfo)

				if agent.IsUsedOrUnusedMember(receivedCommitteeInfo, agent.committeeNode.Publickey) {
					agent.startSend(receivedCommitteeInfo, true)
					help.CheckAndPrintError(agent.server.PutCommittee(receivedCommitteeInfo))
					help.CheckAndPrintError(agent.server.PutNodes(receivedCommitteeInfo.Id, []*types.CommitteeNode{agent.committeeNode}))
				} else {
					agent.startSend(receivedCommitteeInfo, false)
				}
			case types.CommitteeUpdate:
				committeeID := copyCommitteeID(ch.CommitteeID)
				receivedCommitteeInfo := &types.CommitteeInfo{
					Id:          ch.CommitteeID,
					StartHeight: ch.BeginFastNumber,
					EndHeight:   ch.EndFastNumber,
					Members:     ch.CommitteeMembers,
					BackMembers: ch.BackupMembers,
				}
				agent.updateCommittee(receivedCommitteeInfo)
				flag := agent.getMemberFlagFromCommittee(receivedCommitteeInfo)
				// flag : used  start  removed  stop
				if flag == types.StateRemovedFlag {
					agent.isCurrentCommitteeMember = false
					help.CheckAndPrintError(agent.server.Notify(committeeID, int(types.CommitteeStop)))
					agent.stopSend()
				} else if flag == types.StateUsedFlag {
					agent.isCurrentCommitteeMember = true
					help.CheckAndPrintError(agent.server.Notify(committeeID, int(types.CommitteeStart)))
					help.CheckAndPrintError(agent.server.UpdateCommittee(receivedCommitteeInfo))
				} else {
					agent.isCurrentCommitteeMember = false
				}
			case types.CommitteeOver:
				committeeID := copyCommitteeID(ch.CommitteeID)
				agent.endFastNumber[committeeID] = ch.EndFastNumber
				help.CheckAndPrintError(agent.server.SetCommitteeStop(committeeID, ch.EndFastNumber.Uint64()))
			default:
				log.Warn("unknown election option:", "option", ch.Option)
			}
			//receive nodeInfo
		case cryNodeInfo := <-agent.cryNodeInfoCh:
			if nodeTagHash, bool := agent.knownRecievedNodes.Get(cryNodeInfo.Hash()); bool {
				savedTime, bool := agent.committeeNodeTag.Get(nodeTagHash)
				if !bool {
					agent.MarkNodeTag(nodeTagHash.(common.Hash), cryNodeInfo.CreatedAt)
					continue
				}
				result := savedTime.(*big.Int).Cmp(cryNodeInfo.CreatedAt)
				switch result {
				case 1:
					oldReceivedMetrics.Mark(1)
				case 0:
					repeatReceivedMetrics.Mark(1)
					go agent.nodeInfoFeed.Send(types.NodeInfoEvent{cryNodeInfo})
				case -1:
					newReceivedMetrics.Mark(1)
					go agent.nodeInfoFeed.Send(types.NodeInfoEvent{cryNodeInfo})
					agent.MarkNodeTag(nodeTagHash.(common.Hash), cryNodeInfo.CreatedAt)
				}
				log.Debug("received repeat nodeInfo", "repeat", repeatReceivedMetrics.Count(), "old", oldReceivedMetrics.Count(), "new", newReceivedMetrics.Count())
			} else {
				if isCommittee, nodeWork, nodeTagHash, pubKey := agent.cryNodeInfoIsCommittee(cryNodeInfo); isCommittee {
					savedTime, bool := agent.committeeNodeTag.Get(nodeTagHash)
					if bool && savedTime.(*big.Int).Cmp(cryNodeInfo.CreatedAt) > 0 {
						continue
					}
					agent.MarkNodeInfo(cryNodeInfo, nodeTagHash)
					differentReceivedMetrics.Mark(1)

					agent.MarkNodeTag(nodeTagHash, cryNodeInfo.CreatedAt)
					newReceivedMetrics.Mark(1)

					go agent.nodeInfoFeed.Send(types.NodeInfoEvent{cryNodeInfo})
					if nodeWork.isCommitteeMember {
						nodeHandleMetrics.Mark(1)
						agent.handlePbftNode(cryNodeInfo, nodeWork, pubKey)
					}
					log.Debug("broadcast cryNodeInfo...", "committeeId", cryNodeInfo.CommitteeID, "nodeM", nodeHandleMetrics.Count(), "diff", differentReceivedMetrics.Count())
				}
			}

		case ch := <-agent.chainHeadCh:
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
		return nil
	}
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

//handleConsensusBlock committeeNode braodcat:if parentBlock is not in fastChain,put block  into cacheblock
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

		tpsMetrics.Mark(int64((len(receiveBlock.Transactions()))))
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

func (agent *PbftAgent) cryNodeInfoIsCommittee(encryptNode *types.EncryptNodeMessage) (bool, *nodeInfoWork, common.Hash, *ecdsa.PublicKey) {
	members1 := agent.nodeInfoWorks[0].committeeInfo.Members
	members2 := agent.nodeInfoWorks[1].committeeInfo.Members
	if len(members1) == 0 && len(members2) == 0 {
		log.Error("received cryNodeInfo members = 0")
		return false, nil, common.Hash{}, nil
	}
	committeeID1 := agent.nodeInfoWorks[0].committeeInfo.Id
	committeeID2 := agent.nodeInfoWorks[1].committeeInfo.Id
	if committeeID1 == nil && committeeID2 == nil {
		log.Error("received cryNodeInfo committeeId1 and committeeId2 is nil")
		return false, nil, common.Hash{}, nil
	}
	hashBytes := encryptNode.HashWithoutSign().Bytes()
	pubKey, err := crypto.SigToPub(hashBytes, encryptNode.Sign)
	if err != nil {
		log.Error("encryptoNode SigToPub error", "err", err)
		return false, nil, common.Hash{}, nil
	}
	pubKeyByte := crypto.FromECDSAPub(pubKey)
	nodeTag := &types.CommitteeNodeTag{encryptNode.CommitteeID, pubKeyByte}
	if committeeID1 != nil && committeeID1.Cmp(encryptNode.CommitteeID) == 0 &&
		agent.IsUsedOrUnusedMember(agent.nodeInfoWorks[0].committeeInfo, pubKeyByte) {
		return true, agent.nodeInfoWorks[0], nodeTag.Hash(), pubKey
	}
	if committeeID2 != nil && committeeID2.Cmp(encryptNode.CommitteeID) == 0 &&
		agent.IsUsedOrUnusedMember(agent.nodeInfoWorks[1].committeeInfo, pubKeyByte) {
		return true, agent.nodeInfoWorks[1], nodeTag.Hash(), pubKey
	}
	return false, nil, common.Hash{}, nil
}

//send committeeNode to p2p,make other committeeNode receive and decrypt
func (agent *PbftAgent) sendPbftNode(nodeWork *nodeInfoWork) {
	cryNodeInfo := encryptNodeInfo(nodeWork.committeeInfo, agent.committeeNode, agent.privateKey)
	agent.nodeInfoFeed.Send(types.NodeInfoEvent{cryNodeInfo})
}

func encryptNodeInfo(committeeInfo *types.CommitteeInfo, committeeNode *types.CommitteeNode, privateKey *ecdsa.PrivateKey) *types.EncryptNodeMessage {
	cryNodeInfo := &types.EncryptNodeMessage{
		CreatedAt:   big.NewInt(time.Now().Unix()),
		CommitteeID: committeeInfo.Id,
	}
	transportCommitteeNode := committeeNode.ConvertCommitteeNodeToTransport()
	nodeByte, err := rlp.EncodeToBytes(transportCommitteeNode)
	if err != nil {
		log.Error("EncodeToBytes error: ", "err", err)
	}
	var encryptNodes []types.EncryptCommitteeNode
	for _, member := range committeeInfo.GetAllMembers() {
		pubkey, _ := crypto.UnmarshalPubkey(member.Publickey)
		encryptNode, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pubkey), nodeByte, nil, nil)
		if err != nil {
			log.Error("publickey encrypt node error ", "member.Publickey", common.Bytes2Hex(member.Publickey), "err", err)
		}
		encryptNodes = append(encryptNodes, encryptNode)
	}
	cryNodeInfo.Nodes = encryptNodes
	hash := cryNodeInfo.HashWithoutSign().Bytes()
	cryNodeInfo.Sign, err = crypto.Sign(hash, privateKey)
	if err != nil {
		log.Error("sign node error", "err", err)
	}
	return cryNodeInfo
}

func (agent *PbftAgent) handlePbftNode(cryNodeInfo *types.EncryptNodeMessage, nodeWork *nodeInfoWork, pubKey *ecdsa.PublicKey) {
	committeeNode := decryptNodeInfo(cryNodeInfo, agent.privateKey, pubKey)
	if committeeNode != nil {
		help.CheckAndPrintError(agent.server.PutNodes(cryNodeInfo.CommitteeID, []*types.CommitteeNode{committeeNode}))
	}
}

//AddRemoteNodeInfo send cryNodeInfo of committeeNode to network,and recieved by other committeenode
func (agent *PbftAgent) AddRemoteNodeInfo(cryNodeInfo *types.EncryptNodeMessage) error {
	if cryNodeInfo == nil {
		log.Error("AddRemoteNodeInfo cryNodeInfo nil")
		return errors.New("AddRemoteNodeInfo cryNodeInfo nil")
	}
	agent.cryNodeInfoCh <- cryNodeInfo
	return nil
}

func decryptNodeInfo(cryNodeInfo *types.EncryptNodeMessage, privateKey *ecdsa.PrivateKey, pubKey *ecdsa.PublicKey) *types.CommitteeNode {
	//ecdsa.PrivateKey convert to ecies.PrivateKey
	priKey := ecies.ImportECDSA(privateKey)
	for _, encryptNode := range cryNodeInfo.Nodes {
		decryptNode, err := priKey.Decrypt(encryptNode, nil, nil)
		if err == nil { // can Decrypt by priKey
			transportCommitteeNode := new(types.TransportCommitteeNode) //receive nodeInfo
			rlp.DecodeBytes(decryptNode, transportCommitteeNode)
			committeeNode := transportCommitteeNode.ConvertTransportToCommitteeNode(pubKey)
			return committeeNode
		}
	}
	return nil
}

//GetFastLastProposer get last proposer
func (agent *PbftAgent) GetFastLastProposer() common.Address {
	return agent.fastChain.CurrentBlock().Proposer()
}

//FetchFastBlock  generate fastBlock as leader
func (agent *PbftAgent) FetchFastBlock(committeeID *big.Int, infos []*types.CommitteeMember) (*types.Block, error) {
	log.Info("into GenerateFastBlock...", "committeeId", committeeID)
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if agent.fastChain.IsFallback() {
		return nil, core.ErrIsFallback
	}
	var (
		parent       = agent.fastChain.CurrentBlock()
		parentNumber = parent.Number()
		fastBlock    *types.Block
		feeAmount    = big.NewInt(0)
		tstamp       = time.Now().Unix()
	)

	//validate newBlock number exceed endNumber
	if endNumber := agent.endFastNumber[committeeID]; endNumber != nil && endNumber.Cmp(parentNumber) != 1 {
		log.Error("FetchFastBlock error", "number:", endNumber, "err", core.ErrExceedNumber)
		return fastBlock, core.ErrExceedNumber
	}

	log.Info("FetchFastBlock ", "parent:", parent.Number(), "hash", parent.Hash())
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) > 0 {
		tstamp = parent.Time().Int64() + 1
	}
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     new(big.Int).Add(parentNumber, common.Big1),
		GasLimit:   core.FastCalcGasLimit(parent, agent.gasFloor, agent.gasCeil),
		Time:       big.NewInt(tstamp),
	}
	if err := agent.validateBlockSpace(header); err == types.ErrSnailBlockTooSlow {
		return nil, err
	}

	if infos != nil {
		header.CommitteeHash = types.RlpHash(infos)
	} else {
		header.CommitteeHash = params.EmptyHash
	}
	//assign Proposer
	pubKey, _ := crypto.UnmarshalPubkey(agent.committeeNode.Publickey)
	header.Proposer = crypto.PubkeyToAddress(*pubKey)

	//getParent by height and hash
	if err := agent.engine.Prepare(agent.fastChain, header); err != nil {
		log.Error("Failed to prepare header for generateFastBlock", "err", err)
		return fastBlock, err
	}

	//if infos not nil ,indicate committee members  has changed,
	// generate block without execute transaction
	if infos != nil {
		header.Root = parent.Root()
		fastBlock = types.NewBlock(header, nil, nil, nil, infos)
	} else {
		// Create the current work task and check any fork transitions needed
		err := agent.makeCurrent(parent, header)
		if err != nil {
			log.Error("makeCurrent error", "err", err)
			return fastBlock, err
		}
		work := agent.current
		pending, _ := agent.eth.TxPool().Pending()
		if len(pending) != 0 {
			log.Info("has transaction...")
		}
		txs := types.NewTransactionsByPriceAndNonce(work.signer, pending)
		work.commitTransactions(agent.mux, txs, agent.fastChain, feeAmount)
		//calculate snailBlock reward
		agent.rewardSnailBlock(header)
		//padding Header.Root, TxHash, ReceiptHash.  Create the new block to seal with the consensus engine
		if fastBlock, err = agent.engine.Finalize(agent.fastChain, header, work.state, work.txs, work.receipts, feeAmount); err != nil {
			log.Error("Failed to finalize block for sealing", "err", err)
			return fastBlock, err
		}
	}
	log.Info("generateFastBlock", "Height:", fastBlock.Number())

	voteSign, err := agent.GenerateSign(fastBlock)
	if err != nil {
		log.Error("generateBlock with sign error.", "err", err)
	}
	fastBlock.AppendSign(voteSign)
	log.Info("out GenerateFastBlock...")
	return fastBlock, err
}

//GetCurrentHeight return  current fastBlock number
func (agent *PbftAgent) GetCurrentHeight() *big.Int {
	num := new(big.Int).Set(agent.fastChain.CurrentBlock().Number())
	return num
}

//GetSeedMember get seed member
func (agent *PbftAgent) GetSeedMember() []*types.CommitteeMember {
	return nil
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
			log.Warn("validateBlockSpace snailBlockNumber=0", "currentFastNumber", header.Number, "space", space)
			return types.ErrSnailBlockTooSlow
		}
	}
	blockFruits := snailBlock.Body().Fruits
	if blockFruits != nil && len(blockFruits) > 0 {
		lastFruitNum := blockFruits[len(blockFruits)-1].FastNumber()
		space := new(big.Int).Sub(header.Number, lastFruitNum).Int64()
		if space >= params.FastToFruitSpace.Int64() {
			log.Warn("validateBlockSpace", "snailNumber", snailBlock.Number(), "lastFruitNum", lastFruitNum,
				"currentFastNumber", header.Number, "space", space)
			return types.ErrSnailBlockTooSlow
		}
	}
	return nil
}

//generate rewardSnailHegiht
func (agent *PbftAgent) rewardSnailBlock(header *types.Header) {
	rewardSnailHegiht := agent.fastChain.NextSnailNumberReward()
	space := new(big.Int).Sub(agent.snailChain.CurrentBlock().Number(), rewardSnailHegiht).Int64()
	if space >= params.SnailRewardInterval.Int64() {
		header.SnailNumber = rewardSnailHegiht
		sb := agent.snailChain.GetBlockByNumber(rewardSnailHegiht.Uint64())
		if sb != nil {
			header.SnailHash = sb.Hash()
		} else {
			log.Error("cannot find snailBlock by rewardSnailHegiht.", "snailHeight", rewardSnailHegiht.Uint64())
		}
	}
}

//GenerateSignWithVote  generate sign from committeeMember in fastBlock
func (agent *PbftAgent) GenerateSignWithVote(fb *types.Block, vote uint32, result bool) (*types.PbftSign, error) {
	if !result {
		vote = types.VoteAgreeAgainst
	}
	voteSign := &types.PbftSign{
		Result:     vote,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	if vote == types.VoteAgreeAgainst {
		log.Warn("vote AgreeAgainst", "number", fb.Number(), "hash", fb.Hash(), "vote", vote, "result", result)
	}
	var err error
	signHash := voteSign.HashWithNoSign().Bytes()
	voteSign.Sign, err = crypto.Sign(signHash, agent.privateKey)
	if err != nil {
		log.Error("fb GenerateSign error ", "err", err)
	}
	return voteSign, err
}

//GenerateSign  generate sign with default agree from committeeMember in fastBlock
func (agent *PbftAgent) GenerateSign(fb *types.Block) (*types.PbftSign, error) {
	return agent.GenerateSignWithVote(fb, types.VoteAgree, true)
}

//VerifyFastBlock  committee member  verify fastBlock  and vote agree or disagree sign
func (agent *PbftAgent) VerifyFastBlock(fb *types.Block, result bool) (*types.PbftSign, error) {
	if agent.fastChain.IsFallback() {
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, core.ErrIsFallback
	}
	var (
		bc     = agent.fastChain
		parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	)
	if parent == nil { //if cannot find parent return ErrUnSyncParentBlock
		log.Warn("VerifyFastBlock ErrHeightNotYet error", "header", fb.Number())
		return nil, types.ErrHeightNotYet
	}
	err := agent.engine.VerifyHeader(bc, fb.Header())
	if err != nil {
		log.Error("verifyFastBlock verifyHeader error", "header", fb.Number(), "err", err)
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, err
	}

	err = agent.verifyRewardInCommittee(fb)
	if err != nil {
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, err
	}

	err = bc.Validator().ValidateBody(fb, false)
	if err != nil {
		// if return blockAlready kown ,indicate block already insert chain by fetch
		if err == core.ErrKnownBlock && agent.fastChain.CurrentBlock().Number().Cmp(fb.Number()) >= 0 {
			log.Info("block already insert chain by fetch .", "number", fb.Number())
			voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgree, result)
			return voteSign, nil //if err equals ErrKnownBlock return nil
		}
		log.Error("verifyFastBlock validateBody error", "height:", fb.Number(), "err", err)
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, err
	}
	state, err := bc.State()
	if err != nil {
		log.Error("verifyFastBlock getCurrent state error", "height:", fb.Number(), "err", err)
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, err
	}

	err = validateTxInCommittee(fb)
	if err != nil {
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, err
	}

	receipts, _, usedGas, err := bc.Processor().Process(fb, state, agent.vmConfig) //update
	if err != nil {
		if err == types.ErrSnailHeightNotYet {
			log.Warn("verifyFastBlock :Snail height not yet", "currentFastNumber", fb.NumberU64(),
				"rewardSnailBlock", fb.SnailNumber().Uint64())
			return nil, err
		}
		log.Error("verifyFastBlock process error", "height:", fb.Number(), "err", err)
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, err
	}
	err = bc.Validator().ValidateState(fb, parent, state, receipts, usedGas)
	if err != nil {
		log.Error("verifyFastBlock validateState error", "Height:", fb.Number(), "err", err)
		voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgreeAgainst, result)
		return voteSign, err
	}
	voteSign, _ := agent.GenerateSignWithVote(fb, types.VoteAgree, result)
	return voteSign, nil
}

func validateTxInCommittee(fb *types.Block) error {
	var err error
	for _, tx := range fb.Transactions() {
		if tx.Size() > 32*1024 {
			err = core.ErrOversizedData
		} else if tx.Value().Sign() < 0 {
			err = core.ErrNegativeValue
		} else if tx.Fee() != nil && tx.Fee().Sign() < 0 {
			err = core.ErrNegativeFee
		} else if tx.GasPrice().Cmp(big.NewInt(core.MinimumGasPrice_local)) < 1 {
			err = core.ErrUnderpriced
		}
		if err != nil {
			log.Error("validateTxInCommittee", "hash", tx.Hash(), "err", err)
			return err
		}
	}
	return nil

}

func (agent *PbftAgent) verifyRewardInCommittee(fb *types.Block) error {
	supposedRewardedNumber := agent.fastChain.NextSnailNumberReward()
	currentSnailBlock := agent.snailChain.CurrentBlock().Number()
	space := new(big.Int).Sub(currentSnailBlock, supposedRewardedNumber).Int64()

	var err error
	if fb.SnailNumber() != nil && fb.SnailNumber().Uint64() > 0 {
		if space < params.SnailConfirmInterval.Int64() {
			err = core.ErrSnailNumberRewardTooFast
		}
	} else if space > params.SnailMaximumRewardInterval.Int64() {
		err = core.ErrSnailNumberRewardTooSlow
	}

	if err != nil {
		log.Error("verify reward in committee", "scn", agent.snailChain.CurrentBlock().Number(),
			"srn", supposedRewardedNumber, "space", space, "err", err)
		return err
	}
	return nil
}

//BroadcastConsensus  when More than 2/3 signs with agree,
//  committee Member Reach a consensus  and insert the fastBlock into fastBlockChain
func (agent *PbftAgent) BroadcastConsensus(fb *types.Block) error {
	//log.Info("into BroadcastSign.", "fastHeight", fb.Number())
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
	//log.Info("out BroadcastSign.", "fastHeight", fb.Number())
	return nil
}

func (agent *PbftAgent) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := agent.fastChain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &AgentWork{
		config:    agent.config,
		signer:    types.NewTIP1Signer(agent.config.ChainID),
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
			log.Warn("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			log.Warn("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Warn("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			env.tcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Warn("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
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

// SubscribeNewPbftSignEvent registers a subscription of PbftSignEvent and
// starts sending event to the given channel.
func (agent *PbftAgent) SubscribeNewPbftSignEvent(ch chan<- types.PbftSignEvent) event.Subscription {
	return agent.scope.Track(agent.signFeed.Subscribe(ch))
}

//SubscribeNodeInfoEvent  registers a subscription of NodeInfoEvent.
func (agent *PbftAgent) SubscribeNodeInfoEvent(ch chan<- types.NodeInfoEvent) event.Subscription {
	return agent.scope.Track(agent.nodeInfoFeed.Subscribe(ch))
}

//IsCommitteeMember  whether publickey in  committee member
func (agent *PbftAgent) updateCommittee(receivedCommitteeInfo *types.CommitteeInfo) {
	receivedID := receivedCommitteeInfo.Id
	if receivedID == nil {
		log.Error("updateCommittee receivedId is nil")
		return
	}
	if receivedID.Uint64() == agent.currentCommitteeInfo.Id.Uint64() {
		agent.currentCommitteeInfo = receivedCommitteeInfo
	} else {
		log.Error("updateCommittee error ", "cId", receivedID, "nId", agent.nextCommitteeInfo.Id,
			"receivedId", receivedCommitteeInfo.Id)
	}
	//update nodeInfoWorks
	if agent.nodeInfoWorks[0].committeeInfo.Id != nil && receivedID.Uint64() == agent.nodeInfoWorks[0].committeeInfo.Id.Uint64() {
		agent.nodeInfoWorks[0].committeeInfo = receivedCommitteeInfo
	} else if agent.nodeInfoWorks[1].committeeInfo.Id != nil && receivedID.Uint64() == agent.nodeInfoWorks[1].committeeInfo.Id.Uint64() {
		agent.nodeInfoWorks[1].committeeInfo = receivedCommitteeInfo
	} else {
		log.Error("update nodeInfoWorks committeeInfo error ", "cId", agent.currentCommitteeInfo.Id, "nId", agent.nextCommitteeInfo.Id,
			"receivedId", receivedID)
	}
}

//IsUsedOrUnusedMember  whether publickey in  committee member
func (agent *PbftAgent) IsUsedOrUnusedMember(committeeInfo *types.CommitteeInfo, publickey []byte) bool {
	flag := agent.election.GetMemberFlag(committeeInfo.GetAllMembers(), publickey)
	if flag == types.StateUsedFlag || flag == types.StateUnusedFlag {
		return true
	}
	return false
}

//IsCommitteeMember  whether agent in  committee member
func (agent *PbftAgent) getMemberFlagFromCommittee(committeeInfo *types.CommitteeInfo) uint32 {
	return agent.election.GetMemberFlag(committeeInfo.GetAllMembers(), agent.committeeNode.Publickey)
}

//IsCommitteeMember  whether agent in  committee member
func (agent *PbftAgent) isCommitteeMember(committeeInfo *types.CommitteeInfo) bool {
	flag := agent.getMemberFlagFromCommittee(committeeInfo)
	return flag == types.StateUsedFlag
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

//AcquireCommitteeAuth determine whether the node pubKey  is in the specified committee
func (agent *PbftAgent) AcquireCommitteeAuth(fastHeight *big.Int) bool {
	committeeMembers := agent.election.GetCommittee(fastHeight)
	return agent.election.IsCommitteeMember(committeeMembers, agent.committeeNode.Publickey)
}

//MarkNodeInfo Mark received NodeInfo
func (agent *PbftAgent) MarkNodeInfo(encryptNode *types.EncryptNodeMessage, nodeTagHash common.Hash) {
	// If we reached the memory allowance, drop a previously known nodeInfo hash
	for agent.knownRecievedNodes.Size() >= maxKnownNodes {
		agent.knownRecievedNodes.Pop()
	}
	agent.knownRecievedNodes.Set(encryptNode.Hash(), nodeTagHash)
}

//MarkNodeTag Mark received nodeTag,avoid old node information
func (agent *PbftAgent) MarkNodeTag(nodeTag common.Hash, timestamp *big.Int) {
	for agent.committeeNodeTag.Size() >= maxKnownNodes {
		agent.committeeNodeTag.Pop()
	}
	agent.committeeNodeTag.Set(nodeTag, timestamp)
}

//single node start
func (agent *PbftAgent) singleloop() {
	for {
		var (
			block *types.Block
			err   error
			cnt   = 0
		)
		for {
			block, err = agent.FetchFastBlock(nil, nil)
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
		_, err = agent.VerifyFastBlock(block, true)
		if err != nil {
			log.Error("VerifyFastBlock error", "err", err)
		}
		err = agent.BroadcastConsensus(block)
		if err != nil {
			log.Error("BroadcastConsensus error", "err", err)
		}
	}
}
