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
	voteAgreeAgainst = iota //vote against
	voteAgree               //vote  agree

	preCommittee      //previous committee
	currentCommittee  //current running committee
	nextCommittee     //next committee
)
const (
	blockRewordSpace = 12
	fetchBlockTime   = 5
	chainHeadSize    = 256
	electionChanSize = 64
	sendNodeTime     = 40 * time.Second
)

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
	cacheBlockMu *sync.Mutex //PbftAgent.cacheBlock mutex

	mux *event.TypeMux

	signFeed         event.Feed
	nodeInfoFeed     event.Feed
	NewFastBlockFeed event.Feed
	scope            event.SubscriptionScope //send scope

	electionCh    chan core.ElectionEvent
	committeeCh   chan core.CommitteeEvent
	cryNodeInfoCh chan *types.EncrptoNodeMessage
	chainHeadCh   chan core.ChainHeadEvent

	electionSub       event.Subscription
	committeeSub      event.Subscription
	pbftNodeSub       *event.TypeMuxSubscription
	chainHeadAgentSub event.Subscription

	committeeNode *types.CommitteeNode
	privateKey    *ecdsa.PrivateKey

	cacheSign          map[string]types.Sign     //prevent receive same sign
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
		chainHeadCh:      make(chan core.ChainHeadEvent, chainHeadSize),
		cryNodeInfoCh:    make(chan *types.EncrptoNodeMessage),
		election:         election,
		mux:              new(event.TypeMux),
		mu:               new(sync.Mutex),
		cacheBlockMu:     new(sync.Mutex),
		cacheBlock:       make(map[*big.Int]*types.Block),
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
	log.Debug("InitNodeInfo", "singleNode:", self.singleNode, ", port:", config.Port,
		", Host:", config.Host, ", CommitteeKey:", config.CommitteeKey)
	if bytes.Equal(config.CommitteeKey, []byte{}) {
		log.Debug("config.CommitteeKey  is nil.")
		if config.Host != "" || config.Port != 0 {
			self.committeeNode = &types.CommitteeNode{
				IP:   config.Host,
				Port: uint(config.Port),
			}
		}
		return
	}
	if config.Host == "" || config.Port == 0 || bytes.Equal(config.CommitteeKey, []byte{}) {
		log.Debug("config is not complete .")
		return
	}

	//generate privateKey
	acc1Key, err := crypto.ToECDSA(config.CommitteeKey)
	if err != nil {
		log.Error("InitNodeInfo PrivateKey error,CommitteeKey is wrong ", "err", err)
		return
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

// Unsubscribe all subscriptions registered from agent
func (self *PbftAgent) stop() {
	self.committeeSub.Unsubscribe()
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
				log.Debug("CommitteeStart...")
				self.setCommitteeInfo(self.nextCommitteeInfo, currentCommittee)
				if self.isCommitteeMember(self.currentCommitteeInfo) {
					go self.server.Notify(self.currentCommitteeInfo.Id, int(ch.Option))
				}
			case types.CommitteeStop:
				log.Debug("CommitteeStop..")
				self.setCommitteeInfo(self.currentCommitteeInfo, preCommittee)
				self.setCommitteeInfo(nil, currentCommittee)
				go self.server.Notify(self.currentCommitteeInfo.Id, int(ch.Option))
			default:
				log.Debug("unknown electionch:", ch.Option)
			}
		case ch := <-self.committeeCh:
			log.Debug("CommitteeCh...")
			receivedCommitteeInfo := ch.CommitteeInfo //received committeeInfo
			self.setCommitteeInfo(receivedCommitteeInfo, nextCommittee)
			ticker.Stop()                                //stop ticker send nodeInfo
			self.cacheSign = make(map[string]types.Sign) //clear cacheSign map
			ticker = time.NewTicker(sendNodeTime)
			if self.isCommitteeMember(receivedCommitteeInfo) {
				self.server.PutCommittee(receivedCommitteeInfo)
				self.server.PutNodes(receivedCommitteeInfo.Id, []*types.CommitteeNode{self.committeeNode})
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
			//if cryNodeInfo of  node in Committee,
			if self.encryptoNodeInCommittee(cryNodeInfo) {
				go self.nodeInfoFeed.Send(core.NodeInfoEvent{cryNodeInfo})
				signStr := hex.EncodeToString(cryNodeInfo.Sign)
				// if  node  is in committee  and the sign is not received
				if self.isCommitteeMember(self.nextCommitteeInfo) && bytes.Equal(self.cacheSign[signStr], []byte{}) {
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
		log.Debug("delete from cacheBlock,number:", fb.Number())
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
	if self.fastChain.CurrentBlock().Number().Cmp(receiveBlockHeight) >= 0 { //TODO currentBlock mutex
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
			log.Error("self.fastChain.InsertChain error ", "err", err)
			return err
		}
		log.Debug("fastblock insert chain:", " num:", receiveBlock.Header().Number.Uint64())
		//generate sign
		voteSign, err := self.GenerateSign(receiveBlock)
		if err != nil {
			return err
		}
		//braodcast sign and block
		self.signFeed.Send(core.PbftSignEvent{Block: receiveBlock, PbftSign: voteSign})
	} else {
		log.Info("handleConsensusBlock parent is nil.")
		self.cacheBlockMu.Lock()
		self.cacheBlock[receiveBlockHeight] = receiveBlock
		self.cacheBlockMu.Unlock()
	}
	return nil
}

func (self *PbftAgent) encryptoNodeInCommittee(cryNodeInfo *types.EncrptoNodeMessage) bool {
	hash := RlpHash([]interface{}{cryNodeInfo.Nodes, cryNodeInfo.CommitteeId})
	pubKey, err := crypto.SigToPub(hash[:], cryNodeInfo.Sign)
	if err != nil {
		log.Error("encryptoNode SigToPub error", "err", err)
		return false
	}

	nextCommitteeInfo := self.nextCommitteeInfo
	if nextCommitteeInfo == nil || len(nextCommitteeInfo.Members) == 0 {
		log.Error("NextCommitteeInfo.Members is nil ...")
		return false
	}
	if nextCommitteeInfo.Id.Cmp(cryNodeInfo.CommitteeId) != 0 {
		log.Info("CommitteeId not consistence  ...")
		return false
	}

	pubKeyByte := crypto.FromECDSAPub(pubKey)
	for _, member := range nextCommitteeInfo.Members {
		if bytes.Equal(pubKeyByte, crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

//send committeeNode to p2p,make other committeeNode receive and decrypt
func (pbftAgent *PbftAgent) sendPbftNode(committeeInfo *types.CommitteeInfo) {
	log.Debug("into sendPbftNode.")
	var err error
	cryNodeInfo := &types.EncrptoNodeMessage{
		CommitteeId: committeeInfo.Id,
		CreatedAt:   time.Now(),
	}
	//PrintNode(pbftAgent.committeeNode)
	nodeByte, err := rlp.EncodeToBytes(pbftAgent.committeeNode)
	if err != nil {
		log.Error("EncodeToBytes error: ", "err", err)
	}
	var encryptNodes []types.EncryptCommitteeNode
	for _, member := range committeeInfo.Members {
		EncryptCommitteeNode, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(member.Publickey), nodeByte, nil, nil)
		if err != nil {
			log.Error("publickey encrypt error ", "member.Publickey:", member.Publickey, "err", err)
		}
		encryptNodes = append(encryptNodes, EncryptCommitteeNode)
	}
	cryNodeInfo.Nodes = encryptNodes

	hash := RlpHash([]interface{}{cryNodeInfo.Nodes, committeeInfo.Id})
	cryNodeInfo.Sign, err = crypto.Sign(hash[:], pbftAgent.privateKey)
	if err != nil {
		log.Error("sign error", "err", err)
	}
	pbftAgent.nodeInfoFeed.Send(core.NodeInfoEvent{cryNodeInfo})
}

func (pbftAgent *PbftAgent) AddRemoteNodeInfo(cryNodeInfo *types.EncrptoNodeMessage) error {
	log.Debug("into AddRemoteNodeInfo.")
	pbftAgent.cryNodeInfoCh <- cryNodeInfo
	return nil
}

func (self *PbftAgent) receivePbftNode(cryNodeInfo *types.EncrptoNodeMessage) {
	log.Debug("into ReceivePbftNode ...")
	/*hash := RlpHash([]interface{}{cryNodeInfo.Nodes, cryNodeInfo.CommitteeId})
	pubKey, err := crypto.SigToPub(hash[:], cryNodeInfo.Sign)
	if err != nil {
		log.Error("SigToPub error.", "err",err)
	}
	members := self.election.GetComitteeById(cryNodeInfo.CommitteeId)
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
	}*/
	priKey := ecies.ImportECDSA(self.privateKey) //ecdsa-->ecies
	for _, encryptNode := range cryNodeInfo.Nodes {
		decryptNode, err := priKey.Decrypt(encryptNode, nil, nil)
		if err == nil { // can Decrypt by priKey
			node := new(types.CommitteeNode) //receive nodeInfo
			rlp.DecodeBytes(decryptNode, node)
			self.server.PutNodes(cryNodeInfo.CommitteeId, []*types.CommitteeNode{node})
		}
	}
}

//generateBlock and broadcast
func (self *PbftAgent) FetchFastBlock() (*types.Block, error) {
	log.Debug("into GenerateFastBlock...")
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
	if space >= blockRewordSpace {
		header.SnailNumber = rewardSnailHegiht
		sb := self.snailChain.GetBlockByNumber(rewardSnailHegiht.Uint64())
		if sb != nil {
			header.SnailHash = sb.Hash()
		} else {
			log.Error("cannot find block.", "err", err)
		}
	}

	//  padding Header.Root, TxHash, ReceiptHash.
	// Create the new block to seal with the consensus engine
	if fastBlock, err = self.engine.Finalize(self.fastChain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return fastBlock, err
	}
	log.Debug("generateFastBlock", "Height:", fastBlock.Header().Number)
	voteSign, err := self.GenerateSign(fastBlock)
	fastBlock.AppendSign(voteSign)
	return fastBlock, err
}

func (self *PbftAgent) GenerateSign(fb *types.Block) (*types.PbftSign, error) {
	if !self.nodeInfoIsComplete {
		return nil, errors.New("nodeInfo is not exist ,cannot generateSign.")
	}
	voteSign := &types.PbftSign{
		Result:     voteAgree,
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
func (self *PbftAgent) BroadcastFastBlock(fb *types.Block){
	go self.NewFastBlockFeed.Send(core.NewBlockEvent{Block: fb})
}

func (self *PbftAgent) VerifyFastBlock(fb *types.Block) error {
	log.Debug("hash:", fb.Hash(), "number:", fb.Header().Number, "parentHash:", fb.ParentHash())
	bc := self.fastChain
	// get current head
	var parent *types.Block
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	if parent == nil { //if cannot find parent return ErrUnSyncParentBlock
		return types.ErrHeightNotYet
	}
	err := self.engine.VerifyHeader(bc, fb.Header(), true)
	if err != nil {
		log.Error("VerifyFastHeader error", "err", err)
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
	log.Debug("into BroadcastSign.")
	self.mu.Lock()
	defer self.mu.Unlock()
	//insert bockchain
	err := self.handleConsensusBlock(fb)
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

func (self *PbftAgent) SubscribeNodeInfoEvent(ch chan<- core.NodeInfoEvent) event.Subscription {
	return self.scope.Track(self.nodeInfoFeed.Subscribe(ch))
}

func (self *PbftAgent) isCommitteeMember(committeeInfo *types.CommitteeInfo) bool {
	if !self.nodeInfoIsComplete {
		return false
	}
	if committeeInfo == nil || len(committeeInfo.Members) == 0 {
		log.Error("received committeeInfo is nil or len(committeeInfo.Members) == 0 ")
		return false
	}
	for _, member := range committeeInfo.Members {
		if bytes.Equal(self.committeeNode.Publickey, crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}


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
}

// verify sign of node is in committee
func (self *PbftAgent) VerifyCommitteeSign(sign *types.PbftSign) (bool, string) {
	if sign == nil {
		return false, ""
	}
	pubKey, err := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
	if err != nil {
		log.Error("VerifyCommitteeSign SigToPub error.", "err", err)
		return false, ""
	}
	pubKeyBytes := crypto.FromECDSAPub(pubKey)
	if self.GetCommitteInfo(currentCommittee) == 0 {
		log.Error("CurrentCommittee is nil ...")
		return false, ""
	}
	for _, member := range self.currentCommitteeInfo.Members {
		if bytes.Equal(pubKeyBytes, crypto.FromECDSAPub(member.Publickey)) {
			return true, hex.EncodeToString(pubKeyBytes)
		}
	}

	if self.GetCommitteInfo(preCommittee) == 0 {
		log.Error("PreCommittee is nil ...")
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
		log.Error("GetCommitteeNumber method committees is nil")
		return 0
	}
	return int32(len(committees))
}

func (self *PbftAgent) setCommitteeInfo(newCommitteeInfo *types.CommitteeInfo, CommitteeType int) {
	if newCommitteeInfo == nil {
		log.Error("newCommitteeInfo is nil ")
		newCommitteeInfo = &types.CommitteeInfo{}
	}
	switch CommitteeType {
	case currentCommittee:
		self.currentCommitteeInfo = newCommitteeInfo
	case nextCommittee:
		self.nextCommitteeInfo = newCommitteeInfo
	case preCommittee:
		self.preCommitteeInfo = newCommitteeInfo
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

/*func PrintCryptNode(node *EncrptoNodeMessage) {
	fmt.Println("*********************")
	fmt.Println("createdAt:", node.createdAt)
	fmt.Println("Id:", node.committeeId)
	fmt.Println("Nodes.len:", len(node.nodes))
	fmt.Println("Sign:", node.sign)
}*/

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
	log.Debug("singleloop test.")
	for {
		// fetch block
		var block *types.Block
		var err error
		cnt := 0
		for {
			block, err = agent.FetchFastBlock()
			if err != nil {
				log.Error("singleloop FetchFastBlock error","err",err)
				time.Sleep(time.Second)
				continue
			}
			if len(block.Transactions()) == 0 && cnt < fetchBlockTime {
				cnt ++
				time.Sleep(time.Second)
				continue
			} else {
				break
			}
		}
		err = agent.BroadcastConsensus(block)
		if err != nil {
			log.Error("BroadcastConsensus error", "err", err)
		}
	}
}
