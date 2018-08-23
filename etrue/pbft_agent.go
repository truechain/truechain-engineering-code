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

	//currentMu sync.Mutex //verifyBlock mutex
	mu *sync.Mutex //generateBlock mutex
	committeeMu  sync.Mutex //generateBlock mutex

	mux          *event.TypeMux
	eventMux     *event.TypeMux

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
	log.Trace("into NewPbftAgent...")
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
	}
	self.InitNodeInfo(eth.Config())
	self.SetCurrentRewardNumber()
	return self
}

func (self *PbftAgent)	SetCurrentRewardNumber() {
	/*currentFastBlock := self.fastChain.CurrentBlock()
	snailHegiht := common.Big0
	for i:=currentFastBlock.Number().Uint64(); i>0; i--{
		blockReward :=self.fastChain.GetSnailHeightByFastHeight(currentFastBlock.Hash(),currentFastBlock.Number().Uint64())
		if blockReward != nil{
			snailHegiht =blockReward.SnailNumber
		}
	}
	snailHegiht = snailHegiht.Add(snailHegiht,common.Big1)
	self.rewardNumber =snailHegiht*/

	currentSnailBlock := self.snailChain.CurrentBlock()
	snailHegiht := common.Big0
	for i:=currentSnailBlock.Number().Uint64(); i>0; i--{
		blockReward :=self.fastChain.GetFastHeightBySnailHeight(currentSnailBlock.Hash(),currentSnailBlock.Number().Uint64())
		if blockReward != nil{
			snailHegiht =blockReward.SnailNumber
			break
		}
	}
	self.rewardNumber =snailHegiht
	log.Info("rewardNumber:",self.rewardNumber.Int64())
}

func (self *PbftAgent) InitNodeInfo(config *Config) {
	acc1Key, _ := crypto.ToECDSA(config.CommitteeKey)
	self.PrivateKey =acc1Key
	pubBytes :=crypto.FromECDSAPub(&acc1Key.PublicKey)
	self.CommitteeNode =&types.CommitteeNode{
		config.Host,
		uint(config.Port),
		crypto.PubkeyToAddress(acc1Key.PublicKey),
		pubBytes,
	}
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
				log.Info("CommitteeStart:")
				self.committeeMu.Lock()
				self.SetCommitteeInfo(self.NextCommitteeInfo,CurrentCommittee)
				self.committeeMu.Unlock()
				self.server.Notify(self.CommitteeInfo.Id,int(ch.Option))
			}else if ch.Option ==types.CommitteeStop{
				log.Info("CommitteeStop:")
				ticker.Stop()
				self.committeeMu.Lock()
				self.cacheSign =[]Sign{}
				self.SetCommitteeInfo(nil,CurrentCommittee)
				self.committeeMu.Unlock()
				self.server.Notify(self.CommitteeInfo.Id,int(ch.Option))
			}
		case ch := <-self.CommitteeCh:
			log.Info("CommitteeCh:",ch.CommitteeInfo)
			self.committeeMu.Lock()
			self.SetCommitteeInfo(ch.CommitteeInfo,NextCommittee)
			self.committeeMu.Unlock()
			if self.IsCommitteeMember(ch.CommitteeInfo){
				self.server.PutCommittee(ch.CommitteeInfo)
				go func(){
					for{
						select {
						case <-ticker.C:
							log.Info("94")
							self.SendPbftNode(ch.CommitteeInfo)
						}
					}
				}()
			}
		//receive nodeInfo
		case cryNodeInfo := <-self.CryNodeInfoCh:
			//transpond  cryNodeInfo
			log.Info("receive nodeInfo.")
			go self.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo}) //TODO jduge the nodeInfo is in committee
			if  self.IsCommitteeMember(self.NextCommitteeInfo) {
				flag :=false
				for _,sign := range self.cacheSign{
					if bytes.Equal(sign,cryNodeInfo.Sign){
						flag =true
						break
					}
				}
				if !flag{
					log.Info("ReceivePbftNode method.")
					self.ReceivePbftNode(cryNodeInfo)
					self.cacheSign =append(self.cacheSign,cryNodeInfo.Sign)
				}
			}
		case ch := <- self.ChainHeadCh:
			log.Info("RewardNumberCh.")
			if ch.Block.Header().SnailNumber != nil &&
				self.rewardNumber.Cmp(ch.Block.Header().SnailNumber) == -1{
				self.rewardNumber =ch.Block.Header().SnailNumber
			}
		}
	}
}

func (pbftAgent *PbftAgent) SendPbftNode(committeeInfo *types.CommitteeInfo) *CryNodeInfo{
	log.Info("into SendPbftNode.")
	if committeeInfo == nil && len(committeeInfo.Members) <= 0{
		log.Info("committeeInfo is nil")
		return nil
	}
	cryNodeInfo := &CryNodeInfo{
		CommitteeId:committeeInfo.Id,
	}

	nodeByte,_ :=rlp.EncodeToBytes(pbftAgent.CommitteeNode)
	for _,member := range committeeInfo.Members{
		EncryptCommitteeNode,err :=ecies.Encrypt(rand.Reader,
			ecies.ImportECDSAPublic(member.Publickey),nodeByte, nil, nil)
		if err != nil{
			log.Info("encrypt error,pub:",ecies.ImportECDSAPublic(member.Publickey),
					", coinbase:",member.Coinbase)
			panic(err)
			return nil
		}
		cryNodeInfo.Nodes =append(cryNodeInfo.Nodes,EncryptCommitteeNode)
	}
	hash:=RlpHash([]interface{}{cryNodeInfo.Nodes,committeeInfo.Id,})
	sigInfo,err :=crypto.Sign(hash[:], pbftAgent.PrivateKey)
	if err != nil{
		log.Info("sign error")
		panic(err)
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
	fmt.Println("ReceivePbftNode ...")
	hash :=RlpHash([]interface{}{cryNodeInfo.Nodes,cryNodeInfo.CommitteeId,})
	pubKey,err :=crypto.SigToPub(hash[:],cryNodeInfo.Sign)
	if err != nil{
		log.Info("SigToPub error.")
		panic(err)
		return nil
	}
	pks :=pbftAgent.election.GetByCommitteeId(cryNodeInfo.CommitteeId)
	verifyFlag := false
	for _, pk:= range  pks{
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(pk)) {
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

	/*currentFastBlock := self.fastChain.CurrentBlock()
	snailHegiht := common.Big0
	for i:=currentFastBlock.Number().Uint64(); i>0; i--{
		blockReward :=self.fastChain.GetSnailHeightByFastHeight(currentFastBlock.Hash(),currentFastBlock.Number().Uint64())
		if blockReward != nil{
			snailHegiht =blockReward.SnailNumber
		}
	}
	snailHegiht = snailHegiht.Add(snailHegiht,common.Big1)
	*/

	snailHegiht := self.rewardNumber.Add(self.rewardNumber,common.Big1)
	if temp :=snailHegiht; temp.Sub(self.snailChain.CurrentBlock().Number(),temp).Int64()>=BlockRewordSpace{
		fastBlock.Header().SnailNumber = snailHegiht
		sb :=self.snailChain.GetBlockByNumber(snailHegiht.Uint64())
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
		log.Info("sign error")
		panic(err)
	}
	fb.AppendSign(voteSign)
	self.NewFastBlockFeed.Send(core.NewBlockEvent{Block:fb,})
	self.broadCastChainEvent(fb)
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
	self.mu.Lock()
	defer self.mu.Unlock()
	bc := self.fastChain
	// get current head
	var parent *types.Block
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	err :=self.engine.VerifyFastHeader(bc, fb.Header(),true)
	if err != nil{
		panic(err)
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
			panic(err)
		}
		self.signFeed.Send(core.PbftSignEvent{voteSigns})
	}
}*/

func (self *PbftAgent)  BroadcastSign(fb *types.Block, voteSign *types.PbftSign){
	log.Info("into BroadcastSign.")
	self.mu.Lock()
	defer self.mu.Unlock()
	fastBlocks	:= []*types.Block{fb}
	_,err :=self.fastChain.InsertChain(fastBlocks)
	if err != nil{
		panic(err)
	}
	if fb.SnailNumber() !=nil{
		self.rewardNumber = fb.SnailNumber()
	}
	self.signFeed.Send(core.PbftSignEvent{PbftSign:voteSign})
}

func (self *PbftAgent) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := self.fastChain.StateAt(parent.Root())
	if err != nil {
		panic(err)
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
		panic(err)
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
func (self * PbftAgent) VerifyCommitteeSign(signs []*types.PbftSign) bool {
	for _,sign := range signs{
		pubKey,err :=crypto.SigToPub(GetSignHash(sign),sign.Sign)
		if err != nil{
			log.Info("SigToPub error.")
			panic(err)
		}
		for _,member := range self.CommitteeInfo.Members { // TODO  self.CommitteeInfo is nil
			if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
				break;
				return true
			}
		}
	}
	return false
}

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