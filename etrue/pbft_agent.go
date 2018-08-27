package etrue

import (
	"math/big"
	"time"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/crypto/ecies"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/core/types"
	"crypto/ecdsa"
	"sync"
	"bytes"
	"crypto/rand"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"errors"
	"fmt"
	"encoding/hex"
)

const (
	VoteAgree = iota		//vote agree
	VoteAgreeAgainst  		//vote against

	electionChanSize = 10

	CurrentCommittee = iota
	NextCommittee

	BlockRewordSpace = 12
	sendNodeTime = 5
)

var testCommittee = []*types.CommitteeNode{
	{
		IP:        "192.168.46.33",
		Port:      10080,
		Coinbase:  common.HexToAddress("76ea2f3a002431fede1141b660dbb75c26ba6d97"),
		Publickey: common.Hex2Bytes("04044308742b61976de7344edb8662d6d10be1c477dd46e8e4c433c1288442a79183480894107299ff7b0706490f1fb9c9b7c9e62ae62d57bd84a1e469460d8ac1"),
	},
	{
		IP:        "192.168.46.8",
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
		IP:        "192.168.46.4",
		Port:      10080,
		Coinbase:  common.HexToAddress("d985e9871d1be109af5a7f6407b1d6b686901fff"),
		Publickey: common.Hex2Bytes("04e3e59c07b320b5d35d65917d50806e1ee99e3d5ed062ed24d3435f61a47d29fb2f2ebb322011c1d2941b4853ce2dc71e8c4af57b59bbf40db66f76c3c740d41b"),
	},
}

type PbftAgent struct {
	config *params.ChainConfig
	fastChain   *core.BlockChain
	snailChain *snailchain.SnailBlockChain
	engine consensus.Engine
	eth     Backend
	signer types.Signer
	current *AgentWork

	CommitteeInfo *types.CommitteeInfo
	NextCommitteeInfo *types.CommitteeInfo

	server types.PbftServerProxy
	election	*Election

	mu *sync.Mutex //generateBlock mutex
	committeeMu  *sync.Mutex //generateBlock mutex
	currentMu	*sync.Mutex	//tx mutex

	mux          *event.TypeMux
	//eventMux     *event.TypeMux

	signFeed       event.Feed
	nodeInfoFeed 	event.Feed
	NewFastBlockFeed	event.Feed
	scope        event.SubscriptionScope //send scope

	ElectionCh  chan core.ElectionEvent
	CommitteeCh  chan core.CommitteeEvent
	CryNodeInfoCh 	chan *CryNodeInfo
	//RewardNumberCh  chan core.RewardNumberEvent
	ChainHeadCh 	chan core.ChainHeadEvent

	electionSub event.Subscription
	committeeSub event.Subscription
	PbftNodeSub *event.TypeMuxSubscription
	RewardNumberSub  event.Subscription

	CommitteeNode *types.CommitteeNode
	PrivateKey *ecdsa.PrivateKey

	cacheSign    []Sign
	rewardNumber  *big.Int
}

type AgentWork struct {
	config *params.ChainConfig
	signer types.Signer

	state     *state.StateDB // apply state changes here
	tcount    int            // tx count in cycle
	gasPool   *core.GasPool  // available gas used to pack transactions

	Block *types.Block // the new block

	header   *types.Header
	txs      []*types.Transaction
	receipts []*types.Receipt

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
type Sign []byte
type  CryNodeInfo struct {
	CommitteeId *big.Int
	Nodes       []EncryptCommitteeNode
	Sign 		//sign msg
}

func NewPbftAgent(eth Backend, config *params.ChainConfig, engine consensus.Engine, election *Election) *PbftAgent {
	log.Info("into NewPbftAgent...")
	self := &PbftAgent{
		config:         	config,
		engine:         	engine,
		eth:            	eth,
		fastChain:          eth.BlockChain(),
		snailChain:			eth.SnailBlockChain(),
		CommitteeCh:	make(chan core.CommitteeEvent),
		ElectionCh: 	make(chan core.ElectionEvent, electionChanSize),
		ChainHeadCh:	make(chan core.ChainHeadEvent, 3),
		election: election,
		mu:	new(sync.Mutex),
		committeeMu:	new(sync.Mutex),
		currentMu:	new(sync.Mutex),
	}
	self.InitNodeInfo(eth.Config())
	self.SetCurrentRewardNumber()
	return self
}

func (self *PbftAgent)	SetCurrentRewardNumber() {
	currentSnailBlock := self.snailChain.CurrentBlock()
	snailHegiht := new(big.Int).Set(common.Big0)
	for i:=currentSnailBlock.Number().Uint64(); i>0; i--{
		blockReward :=self.fastChain.GetFastHeightBySnailHeight(currentSnailBlock.Hash(),currentSnailBlock.Number().Uint64())
		if blockReward != nil{
			snailHegiht =new(big.Int).Set(blockReward.SnailNumber)
			break
		}
	}
	self.rewardNumber =snailHegiht
	log.Info("new CurrentRewardNumber:",self.rewardNumber.Int64())
}

func (self *PbftAgent) InitNodeInfo(config *Config) {
	acc1Key, err := crypto.ToECDSA(config.CommitteeKey)
	if err != nil {
		log.Error("InitNodeInfo PrivateKey error ")
	}
	self.PrivateKey =acc1Key
	pubBytes :=crypto.FromECDSAPub(&acc1Key.PublicKey)
	self.CommitteeNode =&types.CommitteeNode{
		IP:config.Host,
		Port:uint(config.Port),
		Coinbase:crypto.PubkeyToAddress(acc1Key.PublicKey),
		Publickey:pubBytes,
	}
	//if self.CommitteeNode.IP == "" || self.CommitteeNode.Port == 0 ||
}

func (self *PbftAgent) Start() {
	self.committeeSub = self.election.SubscribeCommitteeEvent(self.CommitteeCh)
	self.electionSub = self.election.SubscribeElectionEvent(self.ElectionCh)
	self.RewardNumberSub = self.fastChain.SubscribeChainHeadEvent(self.ChainHeadCh)
	go self.loop()
}

// Stop terminates the PbftAgent.
func (self *PbftAgent) Stop() {
	// Unsubscribe all subscriptions registered from agent
	self.committeeSub.Unsubscribe()
	self.electionSub.Unsubscribe()
	self.RewardNumberSub.Unsubscribe()
	self.scope.Close()
}

func (self *PbftAgent) loop(){
	defer self.committeeSub.Unsubscribe()
	defer self.electionSub.Unsubscribe()
	defer self.RewardNumberSub.Unsubscribe()
	defer self.scope.Close()

	ticker := time.NewTicker(time.Minute * sendNodeTime)
	for {
		select {
		//TODO   committeeInfo set
		case ch := <-self.ElectionCh:
			if ch.Option ==types.CommitteeStart{
				log.Info("CommitteeStart...")
				self.committeeMu.Lock()
				self.SetCommitteeInfo(self.NextCommitteeInfo,CurrentCommittee)
				self.committeeMu.Unlock()
				self.server.Notify(self.CommitteeInfo.Id,int(ch.Option))
			}else if ch.Option ==types.CommitteeStop{
				log.Info("CommitteeStop..")
				ticker.Stop()
				self.committeeMu.Lock()
				self.cacheSign =[]Sign{}
				self.SetCommitteeInfo(nil,CurrentCommittee)
				self.committeeMu.Unlock()
				self.server.Notify(self.CommitteeInfo.Id,int(ch.Option))
			}
		case ch := <-self.CommitteeCh:
			log.Info("CommitteeCh...")
			self.committeeMu.Lock()
			self.SetCommitteeInfo(ch.CommitteeInfo,NextCommittee)
			self.committeeMu.Unlock()
			if self.IsCommitteeMember(ch.CommitteeInfo){
				self.server.PutCommittee(ch.CommitteeInfo)
				self.server.PutNodes(ch.CommitteeInfo.Id,  testCommittee)
				go func(){
					for{
						select {
						case <-ticker.C:
							log.Info("ticker send CommitteeInfo...")
							self.SendPbftNode(ch.CommitteeInfo)
						}
					}
				}()
			}
		//receive nodeInfo
		case cryNodeInfo := <-self.CryNodeInfoCh:
			//transpond  cryNodeInfo
			log.Info("receive nodeInfo.")
			//if cryNodeInfo of  node in Committee
			if self.cryNodeInfoInCommittee(*cryNodeInfo){
				go self.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
			}
			// if  node  is in committee  and the sign is not received
			if  self.IsCommitteeMember(self.NextCommitteeInfo) && !self.cryNodeInfoInCachSign(cryNodeInfo.Sign){
				log.Info("ReceivePbftNode method.")
				self.ReceivePbftNode(cryNodeInfo)
				self.cacheSign =append(self.cacheSign,cryNodeInfo.Sign)
			}
		case ch := <- self.ChainHeadCh:
			log.Info("ChainHeadCh update RewardNumber.")
			currentSnailNumber :=ch.Block.Header().SnailNumber
			if currentSnailNumber != nil && self.rewardNumber.Cmp(currentSnailNumber) == -1{
				self.rewardNumber =ch.Block.Header().SnailNumber
			}
		}
	}
}

func (self * PbftAgent) cryNodeInfoInCommittee(cryNodeInfo CryNodeInfo) bool{
	if len(self.NextCommitteeInfo.Members) == 0{
		log.Error("NextCommitteeInfo.Members is nil ...")

		return false
	}
	hash :=RlpHash([]interface{}{cryNodeInfo.Nodes,cryNodeInfo.CommitteeId,})
	pubKey,err :=crypto.SigToPub(hash[:],cryNodeInfo.Sign)
	if err != nil{
		log.Error("SigToPub error.")
		panic(err)
		return false
	}
	for _,member := range self.NextCommitteeInfo.Members {
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

func  (self *PbftAgent) cryNodeInfoInCachSign(sign Sign) bool{
	for _,cachesign := range self.cacheSign{
		if bytes.Equal(cachesign,sign){
			return 	true
		}
	}
	return false
}

func (pbftAgent *PbftAgent) SendPbftNode(committeeInfo *types.CommitteeInfo) *CryNodeInfo{
	log.Info("into SendPbftNode.")
	if committeeInfo == nil && len(committeeInfo.Members) <= 0{
		log.Error("committeeInfo is nil")
		return nil
	}
	cryNodeInfo := &CryNodeInfo{
		CommitteeId:committeeInfo.Id,
	}

	nodeByte,_ :=rlp.EncodeToBytes(pbftAgent.CommitteeNode)
	var encryptNodes []EncryptCommitteeNode
	for _,member := range committeeInfo.Members{
		EncryptCommitteeNode,err :=ecies.Encrypt(rand.Reader,ecies.ImportECDSAPublic(member.Publickey),nodeByte, nil, nil)
		if err != nil{
			panic(err)
			log.Error("encrypt error,pub:",ecies.ImportECDSAPublic(member.Publickey),", coinbase:",member.Coinbase)
		}
		encryptNodes=append(encryptNodes,EncryptCommitteeNode)
	}
	cryNodeInfo.Nodes =encryptNodes

	hash:=RlpHash([]interface{}{cryNodeInfo.Nodes,committeeInfo.Id,})
	sigInfo,err :=crypto.Sign(hash[:], pbftAgent.PrivateKey)
	if err != nil{
		panic(err)
		log.Error("sign error")
	}
	cryNodeInfo.Sign=sigInfo

	pbftAgent.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
	return cryNodeInfo
}

func (pbftAgent *PbftAgent)  AddRemoteNodeInfo(cryNodeInfo *CryNodeInfo) error{
	log.Info("into AddRemoteNodeInfo.")
	pbftAgent.CryNodeInfoCh <- cryNodeInfo
	return nil
}

func (pbftAgent *PbftAgent)  ReceivePbftNode(cryNodeInfo *CryNodeInfo) *types.CommitteeNode {
	log.Info("ReceivePbftNode ...")
	hash :=RlpHash([]interface{}{cryNodeInfo.Nodes,cryNodeInfo.CommitteeId,})
	pubKey,err :=crypto.SigToPub(hash[:],cryNodeInfo.Sign)
	if err != nil{
		panic(err)
		log.Error("SigToPub error.")
	}
	members :=pbftAgent.election.GetComitteeById(cryNodeInfo.CommitteeId)
	verifyFlag := false
	for _, member:= range  members{
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			verifyFlag = true
			break
		}
	}
	if !verifyFlag{
		log.Info("publicKey of send node is not in committee.")
		return nil
	}
	priKey :=ecies.ImportECDSA(pbftAgent.PrivateKey)//ecdsa-->ecies
	for _,encryptNode := range cryNodeInfo.Nodes{
		decryptNode,err :=priKey.Decrypt(encryptNode, nil, nil)
		if err == nil{// can Decrypt by priKey
			node := new(types.CommitteeNode) //receive nodeInfo
			rlp.DecodeBytes(decryptNode,node)
			pbftAgent.server.PutNodes(cryNodeInfo.CommitteeId,  []*types.CommitteeNode{node})
			PrintNode(node)
			return node
		}
	}
	return nil
}

//generateBlock and broadcast
func (self * PbftAgent)  FetchFastBlock() (*types.Block,error){
	log.Info("into GenerateFastBlock...")
	self.mu.Lock()
	defer self.mu.Unlock()
	var fastBlock  *types.Block

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

	if err := self.engine.PrepareFast(self.fastChain, header); err != nil {
		log.Error("Failed to prepare header for generateFastBlock", "err", err)
		return	fastBlock,err
	}
	// Create the current work task and check any fork transitions needed
	err := self.makeCurrent(parent, header)
	work := self.current

	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return	fastBlock,err
	}
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)
	work.commitTransactions(self.mux, txs, self.fastChain)

	//  padding Header.Root, TxHash, ReceiptHash.
	// Create the new block to seal with the consensus engine
	if work.Block, err = self.engine.FinalizeFast(self.fastChain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return	work.Block,err
	}
	fastBlock = work.Block

	rewardSnailHegiht := new(big.Int).Add(self.rewardNumber,common.Big1)
	space := new(big.Int).Sub(self.snailChain.CurrentBlock().Number(),rewardSnailHegiht).Int64()
	if space>=BlockRewordSpace{
		fastBlock.Header().SnailNumber = rewardSnailHegiht
		sb :=self.snailChain.GetBlockByNumber(rewardSnailHegiht.Uint64())
		fastBlock.Header().SnailHash =sb.Hash()
	}
	fmt.Println("fastBlockHeight:",fastBlock.Header().Number)
	return	fastBlock,nil
}

//broadcast blockAndSign
func (self * PbftAgent) BroadcastFastBlock(fb *types.Block) error{
	log.Info("into BroadcastFastBlock.")

	voteSign := &types.PbftSign{
		Result: VoteAgree,
		FastHeight:fb.Header().Number,
		FastHash:fb.Hash(),
	}
	var err error
	signHash :=GetSignHash(voteSign)
	voteSign.Sign,err =crypto.Sign(signHash, self.PrivateKey)
	if err != nil{
		panic(err)
		log.Info("sign error")
	}
	fb.AppendSign(voteSign)
	self.NewFastBlockFeed.Send(core.NewBlockEvent{Block:fb,})
	//self.broadCastChainEvent(fb)
	return err
}

func (self * PbftAgent) broadCastChainEvent(fb *types.Block){
	work := self.current

	// Update the block hash in all logs since it is now available and not when the
	// receipt/log of individual transactions were created.
	for _, r := range work.receipts {
		for _, l := range r.Logs {
			l.BlockHash = fb.Hash()
		}
	}
	for _, log := range work.state.Logs() {
		log.BlockHash = fb.Hash()
	}
	stat, err := self.fastChain.WriteBlockWithState(fb, work.receipts, work.state)
	if err != nil {
		log.Error("Failed writing block to chain", "err", err)
	}
	// Broadcast the block and announce chain insertion event

	var (
		events []interface{}
		logs   = work.state.Logs()
	)
	events = append(events, core.ChainEvent{Block: fb, Hash: fb.Hash(), Logs: logs})
	if stat == core.CanonStatTy {
		events = append(events, core.ChainHeadEvent{Block: fb})
	}
	self.fastChain.PostChainEvents(events, logs)
}

func (self * PbftAgent) VerifyFastBlock(fb *types.Block) error{
	log.Info("into VerifyFastBlock.")
	fmt.Println("hash:",fb.Hash(),"number:",fb.Header().Number)
	fmt.Println("parentHash:",fb.ParentHash())
	self.mu.Lock()
	defer self.mu.Unlock()
	bc := self.fastChain
	// get current head
	var parent *types.Block
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	err :=self.engine.VerifyFastHeader(bc, fb.Header(),true)
	if err != nil{
		panic(err)
		log.Error("VerifyFastHeader error")
	}else{
		err = bc.Validator().ValidateBody(fb)
	}
	if err != nil{
		return err
	}

	//abort, results  :=bc.Engine().VerifyPbftFastHeader(bc, fb.Header(),parent.Header())
	state, err := bc.State()
	if err != nil{
		return err
	}

	receipts, _, usedGas, err := bc.Processor().Process(fb, state, vm.Config{})//update
	if err != nil{
		return err
	}
	err = bc.Validator().ValidateState(fb, parent, state, receipts, usedGas)
	if err != nil{
		return err
	}
	return nil
}

//verify the sign , insert chain  and  broadcast the signs
/*func  (self *PbftAgent)  BroadcastSign(voteSigns []*types.PbftSign,fb *types.FastBlock){
	 voteNum := 0
	//get committee list  by height and hash
	_,members :=self.election.GetCommittee(fb.Header().Number,fb.Header().Hash())//dd
	for _,voteSign := range voteSigns{
		if voteSign.Result == VoteAgreeAgainst{
			continue
		}
		msg :=voteSign.PrepareData()
		pubKey,err :=crypto.SigToPub(msg,voteSign.Sign)
		if err != nil{
			log.Info("SigToPub error.")
			panic(err)
		}
		for _,member := range members {
			if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
				voteNum++
				break;
			}
		}
	}
	if 	voteNum > 2*len(members)/3 {
		fastBlocks	:= []*types.FastBlock{fb}
		_,err :=self.fastChain.InsertChain(fastBlocks)
		if err != nil{

		}
		self.signFeed.Send(core.PbftSignEvent{voteSigns})
	}
}*/

func (self *PbftAgent)  BroadcastSign(voteSign *types.PbftSign, fb *types.Block) error{
	log.Info("into BroadcastSign.")
	self.mu.Lock()
	defer self.mu.Unlock()
	fastBlocks	:= []*types.Block{fb}
	_,err :=self.fastChain.InsertChain(fastBlocks)
	if err != nil{
		panic(err)
		return err
	}
	if fb.SnailNumber() !=nil{
		self.rewardNumber = fb.SnailNumber()
	}
	self.signFeed.Send(core.PbftSignEvent{PbftSign:voteSign})
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

func (env *AgentWork) commitTransactions(mux *event.TypeMux, txs *types.TransactionsByPriceAndNonce,bc *core.BlockChain) {
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

		err, logs := env.commitTransaction(tx, bc,env.gasPool)
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

func (env *AgentWork) commitTransaction(tx *types.Transaction, bc *core.BlockChain,  gp *core.GasPool) (error, []*types.Log) {
	snap := env.state.Snapshot()
	feeAmount := big.NewInt(0);
	receipt, _, err := core.ApplyTransaction(env.config, bc, gp, env.state, env.header, tx, &env.header.GasUsed,feeAmount, vm.Config{})
	if err != nil {
		env.state.RevertToSnapshot(snap)
		return err, nil
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)

	return nil, receipt.Logs
}

func (self * PbftAgent) SubscribeNewFastBlockEvent(ch chan<- core.NewBlockEvent) event.Subscription {
	return self.scope.Track(self.NewFastBlockFeed.Subscribe(ch))
}

// SubscribeNewPbftSignEvent registers a subscription of PbftSignEvent and
// starts sending event to the given channel.
func (self * PbftAgent) SubscribeNewPbftSignEvent(ch chan<- core.PbftSignEvent) event.Subscription {
	return self.scope.Track(self.signFeed.Subscribe(ch))
}

func (self * PbftAgent)  SubscribeNodeInfoEvent(ch chan<- NodeInfoEvent) event.Subscription {
	return self.scope.Track(self.nodeInfoFeed.Subscribe(ch))
}

func (self *PbftAgent) IsCommitteeMember(committeeInfo *types.CommitteeInfo) bool{
	if committeeInfo==nil || len(committeeInfo.Members) <= 0 {
		return false
	}
	pubKey,err :=crypto.UnmarshalPubkey(self.CommitteeNode.Publickey)
	if err != nil{
		log.Error("UnmarshalPubkey error...")
	}
	for _,member := range committeeInfo.Members {
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

func  GetSignHash(sign *types.PbftSign) []byte{
	hash := RlpHash([]interface{} {
		sign.FastHash,
		sign.FastHeight,
		sign.Result,
	})
	return hash[:]
}

// VerifyCommitteeSign verify committee sign.
func (self * PbftAgent) VerifyCommitteeSign(signs []*types.PbftSign) (bool,string) {
	if len(self.CommitteeInfo.Members) == 0{
		log.Error("CommitteeInfo.Members is nil ...")
	}

	for _,sign := range signs{
		pubKey,err :=crypto.SigToPub(GetSignHash(sign),sign.Sign)
		if err != nil{
			log.Error("SigToPub error.")
			return false, ""
		}
		for _,member := range self.CommitteeInfo.Members {
			if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
				return true,hex.EncodeToString(crypto.FromECDSAPub(pubKey))
			}
		}
	}
	return false, ""
}

/*func (self * PbftAgent) VerifyCommitteeSign(signs []*types.PbftSign) bool{
	for _,sign := range signs{
		pubKey,err :=crypto.SigToPub(GetSignHash(sign),sign.Sign)
		if err != nil{
			log.Error("SigToPub error.")
			panic(err)
		}
		for _,member := range self.CommitteeInfo.Members {
			if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
				return true
			}
		}
	}
	return false
}*/

// ChangeCommitteeLeader trigger view change.
func (self * PbftAgent) ChangeCommitteeLeader(height *big.Int) bool {
	return false
}

// getCommitteeNumber return Committees number
func (self * PbftAgent) GetCommitteeNumber(height *big.Int) int32 {
	committees := self.election.GetCommittee(height)
	if committees == nil {
		return 0
	}
	return  int32(len(committees))
}

func (self *PbftAgent) SetCommitteeInfo(newCommitteeInfo *types.CommitteeInfo,CommitteeType int) error {
	if newCommitteeInfo == nil{
		newCommitteeInfo = &types.CommitteeInfo{}
	}
	if CommitteeType ==CurrentCommittee{
		self.CommitteeInfo = newCommitteeInfo
	}else if CommitteeType ==NextCommittee{
		self.NextCommitteeInfo = newCommitteeInfo
	}else{
		return errors.New("CommitteeType is nil")
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

func PrintNode(node *types.CommitteeNode){
	fmt.Println("*********************")
	fmt.Println("IP:",node.IP)
	fmt.Println("Port:",node.Port)
	fmt.Println("Coinbase:",node.Coinbase)
	fmt.Println("Publickey:",node.Publickey)
}