package etrue

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/truechain/truechain-engineering-code/accounts"
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
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rlp"
)

const (
	VoteAgreeAgainst = iota //vote agree
	VoteAgree               //vote against

	PreCommittee
	CurrentCommittee //current running committee
	NextCommittee    //next committee
)
const (
	BlockRewordSpace = 12
	sendNodeTime     = 40
	FetchBlockTime   = 5

	ChainHeadSize    = 300
	electionChanSize = 100
)

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

type PbftAgent struct {
	config     *params.ChainConfig
	fastChain  *core.BlockChain
	snailChain *snailchain.SnailBlockChain
	engine     consensus.Engine
	eth        Backend
	signer     types.Signer
	current    *AgentWork

	preCommitteeInfo     *types.CommitteeInfo
	currentCommitteeInfo *types.CommitteeInfo
	nextCommitteeInfo    *types.CommitteeInfo

	server   types.PbftServerProxy
	election *Election

	mu           *sync.Mutex //generateBlock mutex
	committeeMu  *sync.Mutex //committee mutex
	cacheBlockMu *sync.Mutex //PbftAgent.cacheBlock mutex

	mux *event.TypeMux

	signFeed         event.Feed
	nodeInfoFeed     event.Feed
	NewFastBlockFeed event.Feed
	scope            event.SubscriptionScope //send scope

	electionCh    chan core.ElectionEvent
	committeeCh   chan core.CommitteeEvent
	cryNodeInfoCh chan *CryNodeInfo
	chainHeadCh   chan core.ChainHeadEvent

	electionSub       event.Subscription
	committeeSub      event.Subscription
	pbftNodeSub       *event.TypeMuxSubscription
	chainHeadAgentSub event.Subscription

	committeeNode *types.CommitteeNode
	privateKey    *ecdsa.PrivateKey

	cacheSign          map[string]sign           //prevent receive same sign
	cacheBlock         map[*big.Int]*types.Block //prevent receive same block
	singleNode         bool
	nodeInfoIsComplete bool
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

type Backend interface {
	AccountManager() *accounts.Manager
	BlockChain() *core.BlockChain
	SnailBlockChain() *snailchain.SnailBlockChain
	TxPool() *core.TxPool
	ChainDb() ethdb.Database
	Config() *Config
}

// NodeInfoEvent is posted when nodeInfo send
type NodeInfoEvent struct{ nodeInfo *CryNodeInfo }

type EncryptCommitteeNode []byte

type sign []byte

type CryNodeInfo struct {
	createdAt   time.Time
	committeeId *big.Int
	nodes       []EncryptCommitteeNode
	sign        //sign msg
}

func NewPbftAgent(eth Backend, config *params.ChainConfig, engine consensus.Engine, election *Election) *PbftAgent {
	self := &PbftAgent{
		config:           config,
		engine:           engine,
		eth:              eth,
		fastChain:        eth.BlockChain(),
		snailChain:       eth.SnailBlockChain(),
		preCommitteeInfo: new(types.CommitteeInfo),
		committeeCh:      make(chan core.CommitteeEvent),
		electionCh:       make(chan core.ElectionEvent, electionChanSize),
		chainHeadCh:      make(chan core.ChainHeadEvent, ChainHeadSize),
		cryNodeInfoCh:    make(chan *CryNodeInfo),
		election:         election,
		mux:              new(event.TypeMux),
		mu:               new(sync.Mutex),
		committeeMu:      new(sync.Mutex),
		cacheBlockMu:     new(sync.Mutex),
		cacheBlock:       make(map[*big.Int]*types.Block),
		/*singleNode:			false,
		nodeInfoIsComplete:	false,*/
	}
	self.InitNodeInfo(eth.Config())
	if !self.singleNode {
		self.committeeSub = self.election.SubscribeCommitteeEvent(self.committeeCh)
		self.electionSub = self.election.SubscribeElectionEvent(self.electionCh)
		self.chainHeadAgentSub = self.fastChain.SubscribeChainHeadEvent(self.chainHeadCh)
	}
	return self
}

func (self *PbftAgent) InitNodeInfo(config *Config) {
	self.singleNode = config.NodeType
	log.Info("singleNode:", self.singleNode, ", port:", config.Port, ", Host:", config.Host, ", CommitteeKey:", config.CommitteeKey)
	//TODO when IP or Port is nil
	if bytes.Equal(config.CommitteeKey, []byte{}) {
		log.Info("config.CommitteeKey  is nil.")
		if config.Host != "" || config.Port != 0 {
			self.committeeNode = &types.CommitteeNode{
				IP:   config.Host,
				Port: uint(config.Port),
			}
		}
		return
	}
	if config.Host == "" || config.Port == 0 || bytes.Equal(config.CommitteeKey, []byte{}) {
		log.Info("config is not complete .")
		return
	}
	//generate privateKey
	acc1Key, err := crypto.ToECDSA(config.CommitteeKey)
	if err != nil {
		printStr("InitNodeInfo PrivateKey error ")
	}
	self.privateKey = acc1Key
	pubBytes := crypto.FromECDSAPub(&acc1Key.PublicKey)
	self.committeeNode = &types.CommitteeNode{
		IP:        config.Host,
		Port:      uint(config.Port),
		Coinbase:  crypto.PubkeyToAddress(acc1Key.PublicKey),
		Publickey: pubBytes,
	}
	self.nodeInfoIsComplete = true
}

func (self *PbftAgent) Start() {
	if self.singleNode {
		go self.singleloop()
	} else {
		go self.loop()
	}
}

// Stop terminates the PbftAgent.
func (self *PbftAgent) stop() {
	// Unsubscribe all subscriptions registered from agent
	self.committeeSub.Unsubscribe()
	self.electionSub.Unsubscribe()
	self.chainHeadAgentSub.Unsubscribe()
	self.scope.Close()
}

func (self *PbftAgent) loop() {
	defer self.stop()
	ticker := time.NewTicker(time.Second * sendNodeTime)
	for {
		select {
		case ch := <-self.electionCh:
			switch ch.Option {
			case types.CommitteeStart:
				log.Info("CommitteeStart...")
				self.committeeMu.Lock()
				self.setCommitteeInfo(self.nextCommitteeInfo, CurrentCommittee)
				self.committeeMu.Unlock()
				if self.isCommitteeMember(self.currentCommitteeInfo) {
					go self.server.Notify(self.currentCommitteeInfo.Id, int(ch.Option))
				}
			case types.CommitteeStop:
				log.Info("CommitteeStop..")
				self.committeeMu.Lock()
				self.setCommitteeInfo(self.currentCommitteeInfo, PreCommittee)
				self.setCommitteeInfo(nil, CurrentCommittee)
				self.committeeMu.Unlock()
				go self.server.Notify(self.currentCommitteeInfo.Id, int(ch.Option))
			default:
				log.Info("unknown electionch:", ch.Option)
			}
		case ch := <-self.committeeCh:
			log.Info("CommitteeCh...")
			self.committeeMu.Lock()
			self.setCommitteeInfo(ch.CommitteeInfo, NextCommittee)
			self.committeeMu.Unlock()
			ticker.Stop() //stop ticker send nodeInfo
			ticker = time.NewTicker(time.Second * sendNodeTime)
			self.cacheSign = make(map[string]sign)    //clear cacheSign map
			receivedCommitteeInfo := ch.CommitteeInfo //received committeeInfo
			if self.isCommitteeMember(receivedCommitteeInfo) {
				self.server.PutCommittee(receivedCommitteeInfo)
				//self.server.PutNodes(ch.CommitteeInfo.Id, testCommittee) //TODO delete
				self.server.PutNodes(ch.CommitteeInfo.Id, []*types.CommitteeNode{self.committeeNode})
				go func() {
					for {
						select {
						case <-ticker.C:
							self.sendPbftNode(receivedCommitteeInfo)
						}
					}
				}()
			}
		//receive nodeInfo
		case cryNodeInfo := <-self.cryNodeInfoCh:
			//if cryNodeInfo of  node in Committee
			if self.cryNodeInfoInCommittee(*cryNodeInfo) {
				go self.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
			} else {
				printWarn("cryNodeInfo of  node not in Committee.")
			}
			// if  node  is in committee  and the sign is not received
			signStr := hex.EncodeToString(cryNodeInfo.sign)
			if self.isCommitteeMember(self.nextCommitteeInfo) && bytes.Equal(self.cacheSign[signStr], []byte{}) {
				self.cacheSign[signStr] = cryNodeInfo.sign
				self.receivePbftNode(cryNodeInfo)
			}
		case ch := <-self.chainHeadCh:
			log.Info("ChainHeadCh putCacheIntoChain.")
			self.putCacheIntoChain(ch.Block)
		}
	}
}

//  when receive block insert chain event ,put cacheBlock into fastchain
func (self *PbftAgent) putCacheIntoChain(receiveBlock *types.Block) error {
	var fastBlocks []*types.Block
	receiveBlockHeight := receiveBlock.Number()
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
		//braodcast sign
		voteSign, err := self.GenerateSign(fb)
		if err != nil {
			continue
		}
		//go self.signFeed.Send(core.PbftSignEvent{PbftSign: voteSign})
		go self.signFeed.Send(core.PbftSignEvent{Block: fb, PbftSign: voteSign})
	}
	return nil
}

//committeeNode braodcat:if parentBlock is not in fastChain,put block  into cacheblock
func (self *PbftAgent) operateCommitteeBlock(receiveBlock *types.Block) error {
	receiveBlockHeight := receiveBlock.Number()
	//receivedBlock has been into fashchain
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
		if err != nil {
			printStrAndError("self.fastChain.InsertChain error ", err)
			return err
		}
		log.Info("fastblock insert chain:", " num:", receiveBlock.Header().Number.Uint64())
		//generate sign
		voteSign, err := self.GenerateSign(receiveBlock)
		if err != nil {
			return err
		}
		//braodcast sign and block
		self.signFeed.Send(core.PbftSignEvent{Block: receiveBlock, PbftSign: voteSign})
	} else {
		printWarn("OperateCommitteeBlock parent is nil.")
		self.cacheBlockMu.Lock()
		self.cacheBlock[receiveBlockHeight] = receiveBlock
		self.cacheBlockMu.Unlock()
	}
	return nil
}

func (self *PbftAgent) cryNodeInfoInCommittee(cryNodeInfo CryNodeInfo) bool {
	nextCommitteeInfo := self.nextCommitteeInfo
	if nextCommitteeInfo != nil && len(nextCommitteeInfo.Members) == 0 {
		printStr("NextCommitteeInfo.Members is nil ...")
		return false
	}
	if nextCommitteeInfo.Id.Cmp(cryNodeInfo.committeeId) != 0 {
		printStr("CommitteeId not consistence  ...")
		return false
	}

	hash := RlpHash([]interface{}{cryNodeInfo.nodes, cryNodeInfo.committeeId})
	pubKey, err := crypto.SigToPub(hash[:], cryNodeInfo.sign)
	if err != nil {
		printStrAndError("SigToPub error.", err)
		return false
	}
	for _, member := range nextCommitteeInfo.Members {
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

//send committeeNode to p2p,make other committeeNode receive and decrypt
func (pbftAgent *PbftAgent) sendPbftNode(committeeInfo *types.CommitteeInfo) *CryNodeInfo {
	log.Info("into sendPbftNode.")

	if committeeInfo == nil || len(committeeInfo.Members) == 0 {
		printStr("committeeInfo is nil")
		return nil
	}
	cryNodeInfo := &CryNodeInfo{
		committeeId: committeeInfo.Id,
		createdAt:   time.Now(),
	}
	PrintNode(pbftAgent.committeeNode)
	nodeByte, _ := rlp.EncodeToBytes(pbftAgent.committeeNode)
	var encryptNodes []EncryptCommitteeNode
	for _, member := range committeeInfo.Members {
		EncryptCommitteeNode, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(member.Publickey), nodeByte, nil, nil)
		if err != nil {
			printError(err)
			log.Error("encrypt error,pub:", ecies.ImportECDSAPublic(member.Publickey), ", coinbase:", member.Coinbase)
		}
		encryptNodes = append(encryptNodes, EncryptCommitteeNode)
	}
	cryNodeInfo.nodes = encryptNodes

	hash := RlpHash([]interface{}{cryNodeInfo.nodes, committeeInfo.Id})
	var err error
	cryNodeInfo.sign, err = crypto.Sign(hash[:], pbftAgent.privateKey)
	if err != nil {
		printStrAndError("sign error", err)
	}
	pbftAgent.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
	return cryNodeInfo
}

func (pbftAgent *PbftAgent) AddRemoteNodeInfo(cryNodeInfo *CryNodeInfo) error {
	log.Info("into AddRemoteNodeInfo.")
	pbftAgent.cryNodeInfoCh <- cryNodeInfo
	return nil
}

func (self *PbftAgent) receivePbftNode(cryNodeInfo *CryNodeInfo) {
	log.Info("into ReceivePbftNode ...")
	hash := RlpHash([]interface{}{cryNodeInfo.nodes, cryNodeInfo.committeeId})
	pubKey, err := crypto.SigToPub(hash[:], cryNodeInfo.sign)
	if err != nil {
		printStrAndError("SigToPub error.", err)
	}
	members := self.election.GetComitteeById(cryNodeInfo.committeeId)
	verifyFlag := false
	for _, member := range members {
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			verifyFlag = true
			break
		}
	}
	if !verifyFlag {
		printWarn("publicKey of send node is not in committee.")
		return
	}
	priKey := ecies.ImportECDSA(self.privateKey) //ecdsa-->ecies
	for _, encryptNode := range cryNodeInfo.nodes {
		decryptNode, err := priKey.Decrypt(encryptNode, nil, nil)
		if err == nil { // can Decrypt by priKey
			node := new(types.CommitteeNode) //receive nodeInfo
			rlp.DecodeBytes(decryptNode, node)
			self.server.PutNodes(cryNodeInfo.committeeId, []*types.CommitteeNode{node})
		}
	}
}

//generateBlock and broadcast
func (self *PbftAgent) FetchFastBlock() (*types.Block, error) {
	log.Info("into GenerateFastBlock...")
	self.mu.Lock()
	defer self.mu.Unlock()
	var fastBlock *types.Block

	tstart := time.Now()
	parent := self.fastChain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("generateFastBlock too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.FastCalcGasLimit(parent),
		Time:       big.NewInt(tstamp),
	}

	if err := self.engine.Prepare(self.fastChain, header); err != nil {
		printStrAndError("Failed to prepare header for generateFastBlock", err)
		return fastBlock, err
	}
	// Create the current work task and check any fork transitions needed
	err := self.makeCurrent(parent, header)
	work := self.current

	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		printStrAndError("Failed to fetch pending transactions", err)
		return fastBlock, err
	}
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)
	work.commitTransactions(self.mux, txs, self.fastChain)

	//generate rewardSnailHegiht
	var rewardSnailHegiht *big.Int
	BlockReward := self.fastChain.CurrentReward()
	if BlockReward == nil {
		rewardSnailHegiht = new(big.Int).Set(common.Big1)
	} else {
		rewardSnailHegiht = new(big.Int).Add(BlockReward.SnailNumber, common.Big1)
	}
	space := new(big.Int).Sub(self.snailChain.CurrentBlock().Number(), rewardSnailHegiht).Int64()
	if space >= BlockRewordSpace {
		fastBlock.Header().SnailNumber = rewardSnailHegiht
		sb := self.snailChain.GetBlockByNumber(rewardSnailHegiht.Uint64())
		fastBlock.Header().SnailHash = sb.Hash()
	}

	//  padding Header.Root, TxHash, ReceiptHash.
	// Create the new block to seal with the consensus engine
	if fastBlock, err = self.engine.FinalizeFast(self.fastChain, header, work.state, work.txs, work.receipts); err != nil {
		printStrAndError("Failed to finalize block for sealing", err)
		return fastBlock, err
	}

	fmt.Println("fastBlockHeight:", fastBlock.Header().Number)
	voteSign, err := self.GenerateSign(fastBlock)
	fastBlock.AppendSign(voteSign)
	return fastBlock, err
}

func (self *PbftAgent) GenerateSign(fb *types.Block) (*types.PbftSign, error) {
	if !self.nodeInfoIsComplete {
		return nil, errors.New("nodeInfo is not exist ,cannot generateSign.")
	}
	voteSign := &types.PbftSign{
		Result:     VoteAgree,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	var err error
	signHash := GetSignHash(voteSign)
	voteSign.Sign, err = crypto.Sign(signHash, self.privateKey)
	if err != nil {
		printError(err)
	}
	return voteSign, err
}

//broadcast blockAndSign
func (self *PbftAgent) BroadcastFastBlock(fb *types.Block) error {

	go self.NewFastBlockFeed.Send(core.NewBlockEvent{Block: fb})

	return nil
}

func (self *PbftAgent) VerifyFastBlock(fb *types.Block) error {
	fmt.Println("hash:", fb.Hash(), "number:", fb.Header().Number, "parentHash:", fb.ParentHash())
	bc := self.fastChain
	// get current head
	var parent *types.Block
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	if parent == nil { //if cannot find parent return ErrUnSyncParentBlock
		return types.ErrHeightNotYet
	}
	err := self.engine.VerifyHeader(bc, fb.Header(), true)
	if err != nil {
		printStrAndError("VerifyFastHeader error", err)
		return err
	}
	err = bc.Validator().ValidateBody(fb)
	//abort, results  :=bc.Engine().VerifyPbftFastHeader(bc, fb.Header(),parent.Header())
	state, err := bc.State()
	if err != nil {
		return err
	}

	receipts, _, usedGas, err := bc.Processor().Process(fb, state, vm.Config{}) //update
	if err != nil {
		return err
	}
	err = bc.Validator().ValidateState(fb, parent, state, receipts, usedGas)
	if err != nil {
		return err
	}
	return nil
}

func (self *PbftAgent) BroadcastConsensus(fb *types.Block) error {
	log.Info("into BroadcastSign.")
	self.mu.Lock()
	defer self.mu.Unlock()
	//insert bockchain
	err := self.operateCommitteeBlock(fb)
	if err != nil {
		return err
	}
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

func (env *AgentWork) commitTransactions(mux *event.TypeMux, txs *types.TransactionsByPriceAndNonce, bc *core.BlockChain) {
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

		err, logs := env.commitTransaction(tx, bc, env.gasPool)
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

func (env *AgentWork) commitTransaction(tx *types.Transaction, bc *core.BlockChain, gp *core.GasPool) (error, []*types.Log) {
	snap := env.state.Snapshot()
	feeAmount := big.NewInt(0)
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

func (self *PbftAgent) SubscribeNodeInfoEvent(ch chan<- NodeInfoEvent) event.Subscription {
	return self.scope.Track(self.nodeInfoFeed.Subscribe(ch))
}

func (self *PbftAgent) isCommitteeMember(committeeInfo *types.CommitteeInfo) bool {
	if !self.nodeInfoIsComplete {
		return false
	}
	if committeeInfo == nil || len(committeeInfo.Members) == 0 {
		return false
	}
	for _, member := range committeeInfo.Members {
		if bytes.Equal(self.committeeNode.Publickey, crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

func GetSignHash(sign *types.PbftSign) []byte {
	hash := RlpHash([]interface{}{
		sign.FastHash,
		sign.FastHeight,
		sign.Result,
	})
	return hash[:]
}

func (self *PbftAgent) GetCommitteInfo(committeeType int64) int {
	switch committeeType {
	case CurrentCommittee:
		if self.currentCommitteeInfo == nil {
			return 0
		}
		return len(self.currentCommitteeInfo.Members)
	case NextCommittee:
		if self.nextCommitteeInfo == nil {
			return 0
		}
		return len(self.nextCommitteeInfo.Members)
	case PreCommittee:
		if self.preCommitteeInfo == nil {
			return 0
		}
		return len(self.preCommitteeInfo.Members)
	default:
		return 0
	}
	return 0
}

// verify sign of node is in committee
func (self *PbftAgent) VerifyCommitteeSign(sign *types.PbftSign) (bool, string) {
	if sign == nil {
		return false, ""
	}
	pubKey, err := crypto.SigToPub(GetSignHash(sign), sign.Sign)
	if err != nil {
		printStrAndError("SigToPub error.", err)
		return false, ""
	}
	pubKeyBytes := crypto.FromECDSAPub(pubKey)
	if self.GetCommitteInfo(CurrentCommittee) == 0 {
		printStr("CurrentCommittee is nil ...")
		return false, ""
	}
	for _, member := range self.currentCommitteeInfo.Members {
		if bytes.Equal(pubKeyBytes, crypto.FromECDSAPub(member.Publickey)) {
			return true, hex.EncodeToString(pubKeyBytes)
		}
	}

	if self.GetCommitteInfo(PreCommittee) == 0 {
		printStr("PreCommittee is nil ...")
		return false, ""
	}
	for _, member := range self.preCommitteeInfo.Members {
		if bytes.Equal(pubKeyBytes, crypto.FromECDSAPub(member.Publickey)) {
			return true, hex.EncodeToString(pubKeyBytes)
		}
	}
	return false, hex.EncodeToString(pubKeyBytes)
}

// ChangeCommitteeLeader trigger view change.
func (self *PbftAgent) ChangeCommitteeLeader(height *big.Int) bool {
	return false
}

// getCommitteeNumber return Committees number
func (self *PbftAgent) GetCommitteeNumber(blockHeight *big.Int) int32 {
	committees := self.election.GetCommittee(blockHeight)
	if committees == nil {
		printStr("GetCommitteeNumber method committees is nil")
		return 0
	}
	return int32(len(committees))
}

func (self *PbftAgent) setCommitteeInfo(newCommitteeInfo *types.CommitteeInfo, CommitteeType int) error {
	if newCommitteeInfo == nil {
		newCommitteeInfo = &types.CommitteeInfo{}
	}
	switch CommitteeType {
	case CurrentCommittee:
		self.currentCommitteeInfo = newCommitteeInfo
	case NextCommittee:
		self.nextCommitteeInfo = newCommitteeInfo
	case PreCommittee:
		self.preCommitteeInfo = newCommitteeInfo
	default:
		return errors.New("CommitteeType is error ")
	}
	return nil
}

func RlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func (c *CryNodeInfo) Hash() common.Hash {
	return RlpHash(c)
}

func PrintCryptNode(node *CryNodeInfo) {
	fmt.Println("*********************")
	fmt.Println("createdAt:", node.createdAt)
	fmt.Println("Id:", node.committeeId)
	fmt.Println("Nodes.len:", len(node.nodes))
	fmt.Println("Sign:", node.sign)
}

func PrintNode(node *types.CommitteeNode) {
	fmt.Println("*********************")
	fmt.Println("IP:", node.IP)
	fmt.Println("Port:", node.Port)
	fmt.Println("Coinbase:", node.Coinbase)
	fmt.Println("Publickey:", node.Publickey)
}

//Determine whether the node pubKey  is in the specified committee
func (self *PbftAgent) AcquireCommitteeAuth(blockHeight *big.Int) bool {
	if !self.nodeInfoIsComplete {
		return false
	}
	committeeMembers := self.election.GetCommittee(blockHeight)
	for _, member := range committeeMembers {
		if bytes.Equal(self.committeeNode.Publickey, crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

func (agent *PbftAgent) singleloop() {
	for {
		// fetch block
		var block *types.Block
		var err error
		cnt := 0
		for {
			block, err = agent.FetchFastBlock()
			if err != nil {
				time.Sleep(time.Second)
				continue
			}
			if len(block.Transactions()) == 0 && cnt < FetchBlockTime {
				cnt++
				time.Sleep(time.Second)
				continue
			} else {
				break
			}
		}
		// broadcast fast block
		err = agent.BroadcastFastBlock(block)
		if err != nil {
			continue
		}
		err = agent.BroadcastConsensus(block)
		if err != nil {
			printError(err)
		}
	}
}

func printWarn(str string) {
	log.Warn("warn infomation:", str)
}

func printError(err error) {
	log.Error("error infomation:", err)
}
func printStrAndError(str string, err error) {
	log.Error("error infomation:", str, err)
}

func printStr(str string) {
	log.Error("error infomation:", str)
}
