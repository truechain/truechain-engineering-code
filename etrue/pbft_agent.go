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
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/crypto/ecies"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rlp"
)

const (
	//preCommittee     = iota //previous committee
	currentCommittee = iota //current running committee
	nextCommittee           //next committee
)
const (
	blockRewordSpace = 12
	chainHeadSize    = 256
	electionChanSize = 64
	sendNodeTime     = 30 * time.Second
	subSignStr       = 8

	fetchBlockTime = 5
	blockInterval  = 20
)

var (
	txSum           uint64 = 0
	timeSlice       []uint64
	txSlice         []uint64
	tpsSlice        []float32
	averageTpsSlice []float32
)

type Backend interface {
	//AccountManager() *accounts.Manager
	BlockChain() *core.BlockChain
	SnailBlockChain() *snailchain.SnailBlockChain
	TxPool() *core.TxPool
	//ChainDb() ethdb.Database
	Config() *Config
}

type PbftAgent struct {
	config     *params.ChainConfig
	fastChain  *core.BlockChain
	snailChain *snailchain.SnailBlockChain
	engine     consensus.Engine
	eth        Backend
	signer     types.Signer
	current    *AgentWork

	//preCommitteeInfo     *types.CommitteeInfo
	currentCommitteeInfo *types.CommitteeInfo
	nextCommitteeInfo    *types.CommitteeInfo
	committeeId          *big.Int
	isCommitteeMember    bool

	server   types.PbftServerProxy
	election *Election

	mu           *sync.Mutex //generateBlock mutex
	cacheBlockMu *sync.Mutex //PbftAgent.cacheBlock mutex
	mux          *event.TypeMux

	signFeed         event.Feed
	nodeInfoFeed     event.Feed
	NewFastBlockFeed event.Feed
	scope            event.SubscriptionScope //send scope

	electionCh    chan core.ElectionEvent
	cryNodeInfoCh chan *types.EncryptNodeMessage
	chainHeadCh   chan core.ChainHeadEvent

	electionSub       event.Subscription
	chainHeadAgentSub event.Subscription

	committeeNode *types.CommitteeNode
	commiteePorts []int
	privateKey    *ecdsa.PrivateKey
	vmConfig      vm.Config

	cacheSign  map[string]types.Sign     //prevent receive same sign
	cacheBlock map[*big.Int]*types.Block //prevent receive same block
	singleNode bool
	//nodeInfoIsComplete bool
}

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

// NodeInfoEvent is posted when nodeInfo send
func NewPbftAgent(eth Backend, config *params.ChainConfig, engine consensus.Engine, election *Election) *PbftAgent {
	self := &PbftAgent{
		config:     config,
		engine:     engine,
		eth:        eth,
		fastChain:  eth.BlockChain(),
		snailChain: eth.SnailBlockChain(),
		//preCommitteeInfo: new(types.CommitteeInfo),
		currentCommitteeInfo: new(types.CommitteeInfo),
		nextCommitteeInfo:    new(types.CommitteeInfo),
		committeeId:          new(big.Int).SetInt64(-1),
		electionCh:           make(chan core.ElectionEvent, electionChanSize),
		chainHeadCh:          make(chan core.ChainHeadEvent, chainHeadSize),
		cryNodeInfoCh:        make(chan *types.EncryptNodeMessage),
		election:             election,
		mux:                  new(event.TypeMux),
		mu:                   new(sync.Mutex),
		cacheBlockMu:         new(sync.Mutex),
		cacheBlock:           make(map[*big.Int]*types.Block),
	}
	self.InitNodeInfo(eth.Config())
	if !self.singleNode {
		self.electionSub = self.election.SubscribeElectionEvent(self.electionCh)
		self.chainHeadAgentSub = self.fastChain.SubscribeChainHeadEvent(self.chainHeadCh)
	}
	return self
}

func (self *PbftAgent) InitNodeInfo(config *Config) {
	self.singleNode = config.NodeType
	/*if !config.NodeType && (config.Host == "" || config.Port == 0) {
		log.Debug("host or IP is not complete .")
		return
	}*/
	self.privateKey = config.PrivateKey
	pubKey := self.privateKey.PublicKey
	pubBytes := crypto.FromECDSAPub(&pubKey)
	self.committeeNode = &types.CommitteeNode{
		IP:        config.Host,
		Port:      uint(config.Port),
		Coinbase:  crypto.PubkeyToAddress(pubKey),
		Publickey: pubBytes,
	}
	self.commiteePorts = append(self.commiteePorts, config.Port, config.StandByPort)
	//self.nodeInfoIsComplete = true
	self.vmConfig = vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
	log.Info("InitNodeInfo", "singleNode", self.singleNode, ", port",
		config.Port, ", standByPort", config.StandByPort, ", Host", config.Host,
		", coinbase", self.committeeNode.Coinbase,", self.vmConfig",self.vmConfig.EnablePreimageRecording)
}

func (self *PbftAgent) Start() {
	if self.singleNode {
		go self.singleloop()
	} else {
		go self.loop()
	}
}

// Unsubscribe all subscriptions registered from agent
func (self *PbftAgent) stop() {
	self.electionSub.Unsubscribe()
	self.chainHeadAgentSub.Unsubscribe()
	self.scope.Close()
}

func (self *PbftAgent) loop() {
	defer self.stop()
	ticker := time.NewTicker(sendNodeTime)
	for {
		select {
		case ch := <-self.electionCh:
			switch ch.Option {
			case types.CommitteeStart:
				log.Debug("CommitteeStart...", "Id", ch.CommitteeID)
				/*if !self.verifyCommitteeId(types.CommitteeStart, ch.CommitteeID) {
					continue
				}*/
				self.setCommitteeInfo(currentCommittee, self.nextCommitteeInfo)
				if self.IsCommitteeMember(self.currentCommitteeInfo) {
					go self.server.Notify(ch.CommitteeID, int(ch.Option))
				}
			case types.CommitteeStop:
				log.Debug("CommitteeStop..", "Id", ch.CommitteeID)
				/*if !self.verifyCommitteeId(types.CommitteeStop, ch.CommitteeID) {
					continue
				}*/
				if self.IsCommitteeMember(self.currentCommitteeInfo) {
					go self.server.Notify(ch.CommitteeID, int(ch.Option))
				}
				//self.setCommitteeInfo(preCommittee, self.currentCommitteeInfo)
				//self.setCommitteeInfo(nil, currentCommittee)

			case types.CommitteeSwitchover:
				log.Debug("CommitteeCh...", "Id", ch.CommitteeID)
				/*if !self.verifyCommitteeId(types.CommitteeSwitchover, ch.CommitteeID) {
					continue
				}*/
				receivedCommitteeInfo := &types.CommitteeInfo{
					Id:      ch.CommitteeID,
					Members: ch.CommitteeMembers,
				}
				self.setCommitteeInfo(nextCommittee, receivedCommitteeInfo)
				//self.committeeId = ch.CommitteeID

				ticker.Stop()                                //stop ticker send nodeInfo
				self.cacheSign = make(map[string]types.Sign) //clear cacheSign map
				ticker = time.NewTicker(sendNodeTime)
				if self.IsCommitteeMember(receivedCommitteeInfo) {
					self.isCommitteeMember = true
					self.server.PutCommittee(receivedCommitteeInfo)
					self.updateCommitteeNode()
					self.server.PutNodes(receivedCommitteeInfo.Id, []*types.CommitteeNode{self.committeeNode})
					go func() {
						for {
							select {
							case <-ticker.C:
								self.sendPbftNode(receivedCommitteeInfo)
							}
						}
					}()
				} else {
					log.Info("node not in pbft member")
					self.isCommitteeMember = false
				}
			default:
				log.Warn("unknown election option:", "option", ch.Option)
			}
			//receive nodeInfo
		case cryNodeInfo := <-self.cryNodeInfoCh:
			//if cryNodeInfo of  node in Committee,
			if self.encryptoNodeInCommittee(cryNodeInfo) {
				go self.nodeInfoFeed.Send(core.NodeInfoEvent{cryNodeInfo})
				signStr := hex.EncodeToString(cryNodeInfo.Sign)
				if len(signStr) > subSignStr {
					signStr = signStr[:subSignStr]
				}
				// if  node  is in committee  and the sign is not received
				//TODO every time getcommittee
				//if bytes.Equal(self.cacheSign[signStr], []byte{}) && self.IsCommitteeMember(self.nextCommitteeInfo) {
				if self.isCommitteeMember && bytes.Equal(self.cacheSign[signStr], []byte{}) {
					self.cacheSign[signStr] = cryNodeInfo.Sign
					self.receivePbftNode(cryNodeInfo)
				} else {
					log.Debug("not received pbftnode.")
				}
			} else {
				log.Warn("receive cryNodeInfo of node not in Committee.")
			}
		case ch := <-self.chainHeadCh:
			log.Debug("ChainHeadCh putCacheIntoChain.")
			go self.putCacheIntoChain(ch.Block)
		}
	}
}

func (self *PbftAgent) updateCommitteeNode() {
	members := self.currentCommitteeInfo.Members
	isCommitteeeMember := self.election.IsCommitteeMember(members, self.committeeNode.Publickey)
	if isCommitteeeMember {
		for _, port := range self.commiteePorts {
			if self.committeeNode.Port != uint(port) {
				self.committeeNode.Port = uint(port)
			}
		}
	}
}

func (self *PbftAgent) verifyCommitteeId(committeeEventType int64, committeeId *big.Int) bool {
	if committeeId == nil {
		log.Error("verifyCommitteeId committeeId is nil", "committeeEventType", committeeEventType)
		return false
	}
	switch committeeEventType {
	case types.CommitteeStart:
		if self.committeeId.Cmp(committeeId) != 0 {
			log.Error("CommitteeStart CommitteeId err ",
				"currentCommitteeId", self.committeeId, "receivedCommitteeId", committeeId)
			//return false
		}
	case types.CommitteeStop:
		if self.committeeId.Cmp(committeeId) != 0 {
			log.Error("CommitteeStop CommitteeId err ",
				"currentCommitteeId", self.committeeId, "receivedCommitteeId", committeeId)
			//return false
		}
	case types.CommitteeSwitchover:
		if new(big.Int).Add(self.committeeId, common.Big1).Cmp(committeeId) != 0 {
			log.Error("CommitteeSwitchover CommitteeId err ",
				"currentCommitteeId", self.committeeId, "receivedCommitteeId", committeeId)
			//return false
		}
	default:
		log.Warn("unknown election option:")
	}
	return true
}

//  when receive block insert chain event ,put cacheBlock into fastchain
func (self *PbftAgent) putCacheIntoChain(receiveBlock *types.Block) error {
	var (
		fastBlocks         []*types.Block
		receiveBlockHeight = receiveBlock.Number()
	)
	self.cacheBlockMu.Lock()
	defer self.cacheBlockMu.Unlock()
	for i := receiveBlockHeight.Uint64() + 1; ; i++ {
		if block, ok := self.cacheBlock[big.NewInt(int64(i))]; ok {
			fastBlocks = append(fastBlocks, block)
		} else {
			break
		}
	}
	//insertBlock
	for _, fb := range fastBlocks {
		_, err := self.fastChain.InsertChain([]*types.Block{fb})
		if err != nil {
			return err
		}
		delete(self.cacheBlock, fb.Number())
		log.Info("delete from cacheBlock,number:", fb.Number())
		//braodcast sign
		voteSign, err := self.GenerateSign(fb)
		if err != nil {
			continue
		}
		go self.signFeed.Send(core.PbftSignEvent{Block: fb, PbftSign: voteSign})
	}
	return nil
}

//committeeNode braodcat:if parentBlock is not in fastChain,put block  into cacheblock
func (self *PbftAgent) handleConsensusBlock(receiveBlock *types.Block) error {
	receiveBlockHeight := receiveBlock.Number()
	if self.fastChain.CurrentBlock().Number().Cmp(receiveBlockHeight) >= 0 {
		return nil
	}
	//self.fastChain.CurrentBlock()
	parent := self.fastChain.GetBlock(receiveBlock.ParentHash(), receiveBlock.NumberU64()-1)
	if parent != nil {
		var fastBlocks []*types.Block
		fastBlocks = append(fastBlocks, receiveBlock)

		//insertBlock
		_, err := self.fastChain.InsertChain(fastBlocks)
		for _,fb := range fastBlocks{
			log.Info("Finalize: BroadcastConsensus", "Height:", fb.Header().Number,"len:",len(fastBlocks))
		}
		if err != nil {
			log.Error("self.fastChain.InsertChain error ", "err", err)
			return err
		}
		//test tps
		GetTps(receiveBlock)

		//generate sign
		voteSign, err := self.GenerateSign(receiveBlock)
		if err != nil {
			return err
		}
		log.Info("handleConsensusBlock generate sign ", "FastHeight", voteSign.FastHeight,
			"FastHash", voteSign.FastHash, "Result", voteSign.Result)
		//braodcast sign and block
		self.signFeed.Send(core.PbftSignEvent{Block: receiveBlock, PbftSign: voteSign})
	} else {
		log.Info("handleConsensusBlock parent not in fastchain.")
		self.cacheBlockMu.Lock()
		self.cacheBlock[receiveBlockHeight] = receiveBlock
		self.cacheBlockMu.Unlock()
	}
	return nil
}

func (self *PbftAgent) encryptoNodeInCommittee(cryNodeInfo *types.EncryptNodeMessage) bool {
	hash := cryNodeInfo.HashWithoutSign().Bytes()
	pubKey, err := crypto.SigToPub(hash, cryNodeInfo.Sign)
	if err != nil {
		log.Error("encryptoNode SigToPub error", "err", err)
		return false
	}

	nextCommitteeInfo := self.nextCommitteeInfo
	if len(nextCommitteeInfo.Members) == 0 {
		log.Error("encryptoNodeInCommittee method NextCommitteeInfo.Members = 0")
		return false
	}
	if nextCommitteeInfo.Id.Cmp(cryNodeInfo.CommitteeId) != 0 {
		log.Warn("received nodeInfo CommitteeId not consistence  ...")
		return false
	}

	pubKeyByte := crypto.FromECDSAPub(pubKey)
	return self.election.IsCommitteeMember(nextCommitteeInfo.Members, pubKeyByte)
}

//send committeeNode to p2p,make other committeeNode receive and decrypt
func (self *PbftAgent) sendPbftNode(committeeInfo *types.CommitteeInfo) {
	log.Debug("into sendPbftNode.")
	var (
		err         error
		cryNodeInfo = &types.EncryptNodeMessage{
			CreatedAt:   time.Now(),
			CommitteeId: committeeInfo.Id,
		}
	)
	PrintNode("send", self.committeeNode)
	nodeByte, err := rlp.EncodeToBytes(self.committeeNode)
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
	cryNodeInfo.Sign, err = crypto.Sign(hash, self.privateKey)
	if err != nil {
		log.Error("sign node error", "err", err)
	}
	self.nodeInfoFeed.Send(core.NodeInfoEvent{cryNodeInfo})
}

func (pbftAgent *PbftAgent) AddRemoteNodeInfo(cryNodeInfo *types.EncryptNodeMessage) error {
	log.Debug("into AddRemoteNodeInfo.")
	if cryNodeInfo == nil {
		log.Error("AddRemoteNodeInfo cryNodeInfo nil")
		return errors.New("AddRemoteNodeInfo cryNodeInfo nil")
	}
	pbftAgent.cryNodeInfoCh <- cryNodeInfo
	return nil
}

func (self *PbftAgent) receivePbftNode(cryNodeInfo *types.EncryptNodeMessage) {
	log.Debug("into ReceivePbftNode ...")
	//ecdsa.PrivateKey convert to ecies.PrivateKey
	priKey := ecies.ImportECDSA(self.privateKey)
	for _, encryptNode := range cryNodeInfo.Nodes {
		decryptNode, err := priKey.Decrypt(encryptNode, nil, nil)
		if err == nil { // can Decrypt by priKey
			node := new(types.CommitteeNode) //receive nodeInfo
			rlp.DecodeBytes(decryptNode, node)
			PrintNode("receive", node)
			self.server.PutNodes(cryNodeInfo.CommitteeId, []*types.CommitteeNode{node})
		}
	}
}

//generateBlock and broadcast
func (self *PbftAgent) FetchFastBlock(committeeId *big.Int) (*types.Block, error) {
	log.Debug("into GenerateFastBlock...","committeeId",committeeId)
	self.mu.Lock()
	defer self.mu.Unlock()
	var (
		fastBlock *types.Block
		feeAmount = big.NewInt(0)
	)

	tstart := time.Now()
	parent := self.fastChain.CurrentBlock()
	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) > 0 {
		tstamp = parent.Time().Int64() + 1
	}

	// this will ensure we're not going off too far in the future
	//if now := time.Now().Unix(); tstamp > now+1 {
	//	wait := time.Duration(tstamp-now) * time.Second
	//	log.Info("generateFastBlock too far in the future", "wait", common.PrettyDuration(wait))
	//	time.Sleep(wait)
	//}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.FastCalcGasLimit(parent),
		Time:       big.NewInt(tstamp),
	}
	//validate height and hash
	if err := self.engine.Prepare(self.fastChain, header); err != nil {
		log.Error("Failed to prepare header for generateFastBlock", "err", err)
		return fastBlock, err
	}
	// Create the current work task and check any fork transitions needed
	err := self.makeCurrent(parent, header)
	work := self.current

	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return fastBlock, err
	}
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)
	work.commitTransactions(self.mux, txs, self.fastChain, feeAmount)
	self.rewardSnailBlock(header)
	// padding Header.Root, TxHash, ReceiptHash.
	// Create the new block to seal with the consensus engine
	if fastBlock, err = self.engine.Finalize(self.fastChain, header, work.state, work.txs, work.receipts, feeAmount); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return fastBlock, err
	}
	log.Info("Finalize: leader generateBlock", "Height:", fastBlock.Header().Number)
	log.Debug("generateFastBlock", "Height:", fastBlock.Header().Number)

	voteSign, err := self.GenerateSign(fastBlock)
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

//generate rewardSnailHegiht
func (self *PbftAgent) rewardSnailBlock(header *types.Header) {
	var rewardSnailHegiht *big.Int
	blockReward := self.fastChain.CurrentReward()
	if blockReward == nil {
		rewardSnailHegiht = new(big.Int).Set(common.Big1)
	} else {
		rewardSnailHegiht = new(big.Int).Add(blockReward.SnailNumber, common.Big1)
	}
	space := new(big.Int).Sub(self.snailChain.CurrentBlock().Number(), rewardSnailHegiht).Int64()

	if space >= blockRewordSpace {
		header.SnailNumber = rewardSnailHegiht
		sb := self.snailChain.GetBlockByNumber(rewardSnailHegiht.Uint64())
		if sb != nil {
			header.SnailHash = sb.Hash()
		} else {
			log.Error("cannot find snailBlock by rewardSnailHegiht.")
		}
		log.Debug("reward", "rewardSnailHegiht:", rewardSnailHegiht, "currentSnailBlock:",
			self.snailChain.CurrentBlock().Number(), "space:", space)
	}
}

func GetTps(currentBlock *types.Block) {
	/*r.Seed(time.Now().Unix())
	txNum := uint64(r.Intn(1000))*/

	nowTime := uint64(time.Now().UnixNano() / 1000000)
	timeSlice = append(timeSlice, nowTime)

	txNum := uint64(len(currentBlock.Transactions()))
	txSum += txNum
	txSlice = append(txSlice, txSum)
	if len(txSlice) > 1 && len(timeSlice) > 1 {
		eachTimeInterval := nowTime - timeSlice[len(timeSlice)-1-1]
		tps := 1000 * float32(txNum) / float32(eachTimeInterval)
		log.Info("tps:", "block", currentBlock.NumberU64(), "tps", tps, "tx", txNum, "time", eachTimeInterval)
		tpsSlice = append(tpsSlice, tps)

		var timeInterval, txInterval uint64
		if len(timeSlice)-blockInterval > 0 && len(txSlice)-blockInterval > 0 {
			timeInterval = nowTime - timeSlice[len(timeSlice)-1-blockInterval]
			txInterval = txSum - txSlice[len(txSlice)-1-blockInterval]
		} else {
			timeInterval = nowTime - timeSlice[0]
			txInterval = txSum - txSlice[0]
		}
		averageTps := 1000 * float32(txInterval) / float32(timeInterval)
		log.Info("tps average", "tps", averageTps, "tx", txInterval, "time", timeInterval)
		averageTpsSlice = append(averageTpsSlice, averageTps)
	}
}

func (self *PbftAgent) GenerateSign(fb *types.Block) (*types.PbftSign, error) {
	/*if !self.nodeInfoIsComplete {
		return nil, errors.New("nodeInfo is not exist ,cannot generateSign.")
	}*/
	voteSign := &types.PbftSign{
		Result:     types.VoteAgree,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	var err error
	signHash := voteSign.HashWithNoSign().Bytes()
	voteSign.Sign, err = crypto.Sign(signHash, self.privateKey)
	if err != nil {
		log.Error("fb GenerateSign error ", "err", err)
	}
	return voteSign, err
}

//broadcast blockAndSign
func (self *PbftAgent) BroadcastFastBlock(fb *types.Block) {
	go self.NewFastBlockFeed.Send(core.NewBlockEvent{Block: fb})
}

func (self *PbftAgent) VerifyFastBlock(fb *types.Block) error {
	log.Debug("into VerifyFastBlock:", "hash:", fb.Hash(), "number:", fb.Header().Number, "parentHash:", fb.ParentHash())
	bc := self.fastChain
	// get current head
	var parent *types.Block
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	if parent == nil { //if cannot find parent return ErrUnSyncParentBlock
		return types.ErrHeightNotYet
	}
	err := self.engine.VerifyHeader(bc, fb.Header(), true)
	if err != nil {
		log.Error("VerifyFastHeader error","header",fb.Header(), "err", err)
		return err
	}
	err = bc.Validator().ValidateBody(fb)
	if err != nil {
		log.Error("VerifyFastBlock: validate body error", "err", err)
		return err
	}
	//abort, results  :=bc.Engine().VerifyPbftFastHeader(bc, fb.Header(),parent.Header())
	state, err := bc.State()
	if err != nil {
		return err
	}
	receipts, _, usedGas, err := bc.Processor().Process(fb, state, self.vmConfig) //update
	log.Info("Finalize: verifyFastBlock", "Height:", fb.Header().Number)
	if err != nil {
		return err
	}
	err = bc.Validator().ValidateState(fb, parent, state, receipts, usedGas)
	if err != nil {
		return err
	}
	log.Debug("out VerifyFastBlock:", "hash:", fb.Hash(), "number:", fb.Header().Number, "parentHash:", fb.ParentHash())
	return nil
}

func (self *PbftAgent) BroadcastConsensus(fb *types.Block) error {
	log.Debug("into BroadcastSign.","fastHeight",fb.Header().Number)
	self.mu.Lock()
	defer self.mu.Unlock()
	//insert bockchain
	err := self.handleConsensusBlock(fb)
	if err != nil {
		return err
	}
	log.Debug("out BroadcastSign.","fastHeight",fb.Header().Number)
	return nil
}

func (self *PbftAgent) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := self.fastChain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &AgentWork{
		config:    self.config,
		signer:    types.NewEIP155Signer(self.config.ChainID),
		state:     state,
		header:    header,
		createdAt: time.Now(),
	}
	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	self.current = work
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
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.Protected() && !env.config.IsEIP155(env.header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", env.config.EIP155Block)

			txs.Pop()
			continue
		}
		// Start executing the transaction
		env.state.Prepare(tx.Hash(), common.Hash{}, env.tcount)

		err, logs := env.commitTransaction(tx, bc, env.gasPool, feeAmount)
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
				mux.Post(core.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(core.PendingStateEvent{})
			}
		}(cpy, env.tcount)
	}
}

func (env *AgentWork) commitTransaction(tx *types.Transaction, bc *core.BlockChain, gp *core.GasPool, feeAmount *big.Int) (error, []*types.Log) {
	snap := env.state.Snapshot()
	receipt, _, err := core.ApplyTransaction(env.config, bc, gp, env.state, env.header, tx, &env.header.GasUsed, feeAmount, vm.Config{})
	if err != nil {
		env.state.RevertToSnapshot(snap)
		return err, nil
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)

	return nil, receipt.Logs
}

func (self *PbftAgent) SubscribeNewFastBlockEvent(ch chan<- core.NewBlockEvent) event.Subscription {
	return self.scope.Track(self.NewFastBlockFeed.Subscribe(ch))
}

// SubscribeNewPbftSignEvent registers a subscription of PbftSignEvent and
// starts sending event to the given channel.
func (self *PbftAgent) SubscribeNewPbftSignEvent(ch chan<- core.PbftSignEvent) event.Subscription {
	return self.scope.Track(self.signFeed.Subscribe(ch))
}

func (self *PbftAgent) SubscribeNodeInfoEvent(ch chan<- core.NodeInfoEvent) event.Subscription {
	return self.scope.Track(self.nodeInfoFeed.Subscribe(ch))
}

func (self *PbftAgent) IsCommitteeMember(committeeInfo *types.CommitteeInfo) bool {
	/*if !self.nodeInfoIsComplete {
		return false
	}*/
	return self.election.IsCommitteeMember(committeeInfo.Members, self.committeeNode.Publickey)
}

/*
func (self *PbftAgent) GetCommitteInfo(committeeType int64) int {
	switch committeeType {
	case currentCommittee:
		if self.currentCommitteeInfo == nil {
			return 0
		}
		return len(self.currentCommitteeInfo.Members)
	case nextCommittee:
		if self.nextCommitteeInfo == nil {
			return 0
		}
		return len(self.nextCommitteeInfo.Members)
	case preCommittee:
		if self.preCommitteeInfo == nil {
			return 0
		}
		return len(self.preCommitteeInfo.Members)
	default:
		return 0
	}
	return 0
}*/

// verify sign of node is in committee
func (self *PbftAgent) VerifyCommitteeSign(sign *types.PbftSign) bool {
	/*if sign == nil {
		return false, ""
	}
	pubKey, err := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
	if err != nil {
		log.Error("VerifyCommitteeSign SigToPub error.", "err", err)
		return false, ""
	}
	pubKeyBytes := crypto.FromECDSAPub(pubKey)
	members := self.election.GetCommittee(sign.FastHeight)
	for _, member := range members {
		if bytes.Equal(pubKeyBytes, crypto.FromECDSAPub(member.Publickey)) {
			return true, hex.EncodeToString(pubKeyBytes)
		}
	}
	return false, hex.EncodeToString(pubKeyBytes)*/
	if sign == nil {
		log.Error("VerifyCommitteeSign sign is nil")
		return false
	}
	member, err := self.election.VerifySign(sign)
	if err != nil {
		log.Error("VerifyCommitteeSign  error", "err", err)
		return false
	}
	return member!= nil
}

// ChangeCommitteeLeader trigger view change.
func (self *PbftAgent) ChangeCommitteeLeader(height *big.Int) bool {
	return false
}

// getCommitteeNumber return Committees number
func (self *PbftAgent) GetCommitteeNumber(blockHeight *big.Int) int32 {
	committees := self.election.GetCommittee(blockHeight)
	if committees == nil {
		log.Error("GetCommitteeNumber method committees is nil")
		return 0
	}
	if self.singleNode {
		return 1
	}
	return int32(len(committees))
}

func (self *PbftAgent) setCommitteeInfo(CommitteeType int, newCommitteeInfo *types.CommitteeInfo) {
	if newCommitteeInfo == nil {
		log.Error("setCommitteeInfo is nil ", "CommitteeType", CommitteeType)
		newCommitteeInfo = &types.CommitteeInfo{}
	}
	switch CommitteeType {
	case currentCommittee:
		self.currentCommitteeInfo = newCommitteeInfo
	case nextCommittee:
		self.nextCommitteeInfo = newCommitteeInfo
		// case preCommittee:
		// 	self.preCommitteeInfo = newCommitteeInfo
	default:
		log.Warn("CommitteeType is error ")
	}
}

func RlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func DebugCryptNode(node *types.EncryptNodeMessage) {
	log.Debug("DebugCryptNode ", "createdAt:", node.CreatedAt, "Id:", node.CommitteeId, "Nodes.len:",
		len(node.Nodes))
}

func PrintNode(str string, node *types.CommitteeNode) {
	log.Info(str+" CommitteeNode", "IP:", node.IP, "Port:", node.Port,
		"Coinbase:", node.Coinbase, "Publickey:", hex.EncodeToString(node.Publickey)[:6]+"***")
}

//AcquireCommitteeAuth determine whether the node pubKey  is in the specified committee
func (self *PbftAgent) AcquireCommitteeAuth(fastHeight *big.Int) bool {
	/*if !self.nodeInfoIsComplete {
		return false
	}*/

	committeeMembers := self.election.GetCommittee(fastHeight)
	return self.election.IsCommitteeMember(committeeMembers,self.committeeNode.Publickey)
}

func (agent *PbftAgent) singleloop() {
	log.Info("singleloop start.")
	// sleep a minute to wait election module start and other nodes' connection
	time.Sleep(time.Minute)
	for {
		// fetch block
		var (
			block *types.Block
			err   error
			cnt   = 0
		)
		for {
			block, err = agent.FetchFastBlock()
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
		err = agent.VerifyFastBlock(block)
		if err != nil {
			log.Error("VerifyFastBlock error", "err", err)
		}
		err = agent.BroadcastConsensus(block)
		if err != nil {
			log.Error("BroadcastConsensus error", "err", err)
		}
	}
}
