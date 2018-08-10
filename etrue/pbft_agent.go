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
	"github.com/truechain/truechain-engineering-code/core/fastchain"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/crypto/ecies"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/rlp"
	"crypto/ecdsa"
	"sync"
	"bytes"
	"crypto/rand"
	"encoding/gob"
)

const (
	VoteAgree = iota		//vote agree
	VoteAgreeAgainst  		//vote against
)

type PbftAgent struct {
	config *params.ChainConfig
	chain   *fastchain.FastBlockChain

	id *big.Int
	members []*types.CommitteeMember

	nextId  *big.Int
	nextMembers []*types.CommitteeMember

	engine consensus.Engine
	eth     Backend
	signer types.Signer
	current *AgentWork
	
	server types.PbftServerProxy

	currentMu sync.Mutex
	mux          *event.TypeMux

	agentFeed       event.Feed
	nodeInfoFeed 	event.Feed
	scope        event.SubscriptionScope

	CommitteeCh  chan core.CommitteeEvent
	committeeSub event.Subscription
	ElectionCh  chan core.ElectionEvent
	electionSub event.Subscription

	eventMux      *event.TypeMux
	PbftNodeSub *event.TypeMuxSubscription
	election	*Election

	cryNodeInfo   *CryNodeInfo
	committeeNode *types.CommitteeNode	//node info
	privateKey *ecdsa.PrivateKey

	snapshotMu    sync.RWMutex
	snapshotState *state.StateDB
	snapshotBlock *types.FastBlock

}

type PbftAction struct {
	Id *big.Int		//committee times
	action int
}

type AgentWork struct {
	config *params.ChainConfig
	signer types.Signer

	state     *state.StateDB // apply state changes here
	tcount    int            // tx count in cycle
	gasPool   *fastchain.GasPool  // available gas used to pack transactions

	Block *types.FastBlock // the new block

	header   *types.FastHeader
	txs      []*types.Transaction
	receipts []*types.Receipt

	createdAt time.Time
}

type Backend interface {
	AccountManager() *accounts.Manager
	FastBlockChain() *fastchain.FastBlockChain
	TxPool() *core.TxPool
	ChainDb() ethdb.Database
}

// NewPbftNodeEvent is posted when nodeInfo send
type NewPbftNodeEvent struct{ cryNodeInfo *CryNodeInfo}

// NewMinedFastBlockEvent is posted when a block has been imported.
type NewMinedFastBlockEvent struct{ blockAndSign *BlockAndSign}

type EncryptCommitteeNode []byte
type  CryNodeInfo struct {//dd update
	Nodes       []EncryptCommitteeNode
	//InfoByte	[]byte	//before sign msg hash
	Sign 		[]byte	//sign msg
	CommitteeId *big.Int
}

type  BlockAndSign struct{//dd sign put into block
	Block *types.FastBlock
	Sign  *types.PbftSign
}

func NewPbftAgent(eth Backend, config *params.ChainConfig,mux *event.TypeMux, engine consensus.Engine, election *Election) *PbftAgent {
	self := &PbftAgent{
		config:         	config,
		engine:         	engine,
		eth:            	eth,
		mux:            	mux,
		chain:          	eth.FastBlockChain(),
		CommitteeCh:	make(chan core.CommitteeEvent, 3),
		election: election,
	}
	//self.committeeSub = self.chain.SubscribeChainHeadEvent(self.committeeActionCh)
	self.committeeSub = self.election.SubscribeCommitteeEvent(self.CommitteeCh)
	self.electionSub = self.election.SubscribeElectionEvent(self.ElectionCh)

	go self.loop()
	return self
}

func (self *PbftAgent) loop(){
	for {
		select {
		case ch := <-self.ElectionCh:
			if ch.Option ==types.CommitteeStart{
				self.server.Notify(self.id,int(ch.Option))
			}else if ch.Option ==types.CommitteeStop{
				self.server.Notify(self.id,int(ch.Option))
			}
		case ch := <-self.CommitteeCh:
			if self.CommitteeIncludeNode(ch.Members){
				self.nextId=ch.CommitteeId
				self.nextMembers=ch.Members

				self.SendPbftNode()
			}
			self.server.PutCommittee(ch.CommitteeId,ch.Members)

			/*self.server.Notify(ch.pbftAction.Id,ch.pbftAction.action)
			self.Start()//receive nodeInfo from other member
			self.SendPbftNode()//bro*/

			/*type CommitteeEvent struct {
				CommitteeId	*big.Int
				Members []*types.CommitteeMember
			}*/
		}
	}
}

func (self *PbftAgent) CommitteeIncludeNode(members []*types.CommitteeMember) bool{
	pubKey := self.committeeNode.CM.Publickey
	for _,member := range members {
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			return true
			break;
		}
	}
	return false
}

func (pbftAgent *PbftAgent) SendPbftNode()	*CryNodeInfo{
	var cryNodeInfo *CryNodeInfo
	nodeByte,_ :=ToByte(pbftAgent.committeeNode)
	pks :=pbftAgent.election.GetByCommitteeId(pbftAgent.id)
	for _,pk := range pks{
		EncryptCommitteeNode,err :=ecies.Encrypt(rand.Reader,
			ecies.ImportECDSAPublic(pk),nodeByte, nil, nil)
		if err != nil{
			return nil
		}
		cryNodeInfo.Nodes =append(cryNodeInfo.Nodes,EncryptCommitteeNode)
	}
	hash:=RlpHash(cryNodeInfo.Nodes)
	sigInfo,err :=crypto.Sign(hash[:], pbftAgent.privateKey)
	if err != nil{
		log.Info("sign error")
	}
	cryNodeInfo.Sign=sigInfo
	cryNodeInfo.CommitteeId =pbftAgent.id
	//pbftAgent.eventMux.Post(NewPbftNodeEvent{cryNodeInfo})
	pbftAgent.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
	pbftAgent.cryNodeInfo =cryNodeInfo
	return cryNodeInfo
}

func (pbftAgent *PbftAgent) Start() {
	// broadcast mined blocks
	pbftAgent.PbftNodeSub = pbftAgent.eventMux.Subscribe(NewPbftNodeEvent{})
	go pbftAgent.handle()
}

func  (pbftAgent *PbftAgent) handle(){
	for obj := range pbftAgent.PbftNodeSub.Chan() {
		switch ev := obj.Data.(type) {
		case NewPbftNodeEvent:
			pbftAgent.ReceivePbftNode(ev.cryNodeInfo)
		}
	}
}

func (pbftAgent *PbftAgent)  ReceivePbftNode(cryNodeInfo *CryNodeInfo) *types.CommitteeNode {
	var node *types.CommitteeNode
	hash :=RlpHash(cryNodeInfo.Nodes)
	pubKey,err :=crypto.SigToPub(hash[:],cryNodeInfo.Sign)
	if err != nil{
		log.Info("SigToPub error.")
		return nil
	}
	if pbftAgent.id !=cryNodeInfo.CommitteeId{
		log.Info("commiteeId  is not consistency .")
		return nil
	}
	verifyFlag := false
	pks :=pbftAgent.election.GetByCommitteeId( pbftAgent.id)
	for _, pk:= range pks{
		if !bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(pk)) {
			continue
		}else{
			verifyFlag = true
		}
	}
	if !verifyFlag{
		log.Info("publicKey is not exist.")
		return nil
	}
	priKey :=ecies.ImportECDSA(pbftAgent.privateKey)//ecdsa-->ecies
	for _,info := range cryNodeInfo.Nodes{
		encryptMsg,err :=priKey.Decrypt(info, nil, nil)
		if err != nil{
			FromByte(encryptMsg,node)
			return node
		}
	}
	pbftAgent.server.PutNodes(cryNodeInfo.CommitteeId,  []*types.CommitteeNode{node})
	return nil
}

//generateBlock and broadcast
func  (self * PbftAgent)  FetchFastBlock() (*types.FastBlock,error){
	var fastBlock  *types.FastBlock

	tstart := time.Now()
	parent := self.chain.CurrentBlock()

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
	header := &types.FastHeader{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   fastchain.FastCalcGasLimit(parent),
		Time:       big.NewInt(tstamp),
	}

	if err := self.engine.PrepareFast(self.chain, header); err != nil {
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
	work.commitTransactions(self.mux, txs, self.chain)

	//  padding Header.Root, TxHash, ReceiptHash.
	// Create the new block to seal with the consensus engine
	if fastBlock, err = self.engine.FinalizeFast(self.chain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return	fastBlock,err
	}
	voteSign := &types.PbftSign{
		Result: VoteAgree,
		FastHeight:fastBlock.Header().Number,
		FastHash:fastBlock.Hash(),
	}
	msgByte :=voteSign.PrepareData()
	hash :=RlpHash(msgByte)//dd
	voteSign.Sign,err =crypto.Sign(hash[:], self.privateKey)
	if err != nil{
		log.Info("sign error")
	}
	//broadcast blockAndSign
	//self.BroadcastFastBlock(fastBlock,voteSign)
	return	fastBlock,nil
}

func (self * PbftAgent) BroadcastFastBlock(fb *types.FastBlock,	sign *types.PbftSign) error{
	// sign
	blockAndSign := &BlockAndSign{
		fb,
		sign,
	}
	err :=self.mux.Post(NewMinedFastBlockEvent{blockAndSign})

	return err
}

func (self * PbftAgent) VerifyFastBlock(fb *types.FastBlock) error{
	bc := self.chain

	// get current head

	var parent *types.FastBlock
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)

	err :=bc.Engine().VerifyFastHeader(bc, fb.Header(),true)
	if err == nil{
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
func  (self *PbftAgent)  BroadcastSign(voteSigns []*types.PbftSign,fb *types.FastBlock){
	var voteNum int =0
	//get committee list  by height and hash
	_,members :=self.election.GetCommittee(fb.Header().Number,fb.Header().Hash())
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
		_,err :=self.chain.InsertChain(fastBlocks,voteSigns)
		if err != nil{
			panic(err)
		}
		self.agentFeed.Send(core.PbftSignEvent{voteSigns})
	}
}

func (self *PbftAgent) makeCurrent(parent *types.FastBlock, header *types.FastHeader) error {
	state, err := self.chain.StateAt(parent.Root())
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

func (env *AgentWork) commitTransactions(mux *event.TypeMux, txs *types.TransactionsByPriceAndNonce,
	bc *fastchain.FastBlockChain) {
	if env.gasPool == nil {
		env.gasPool = new(fastchain.GasPool).AddGas(env.header.GasLimit)
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

func (env *AgentWork) commitTransaction(tx *types.Transaction, bc *fastchain.FastBlockChain,  gp *fastchain.GasPool) (error, []*types.Log) {
	snap := env.state.Snapshot()

	receipt, _, err := fastchain.FastApplyTransaction(env.config, bc, gp, env.state, env.header, tx, &env.header.GasUsed, vm.Config{})
	if err != nil {
		env.state.RevertToSnapshot(snap)
		return err, nil
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)

	return nil, receipt.Logs
}


// SubscribeNewPbftSignEvent registers a subscription of PbftSignEvent and
// starts sending event to the given channel.
func (self * PbftAgent) SubscribeNewPbftSignEvent(ch chan<- core.PbftSignEvent) event.Subscription {
	return self.scope.Track(self.agentFeed.Subscribe(ch))
}

type NodeInfoEvent struct{ nodeInfo *CryNodeInfo }
func (self * PbftAgent)  SubscribeNodeInfoEvent(ch chan<- NodeInfoEvent) event.Subscription {
	return self.scope.Track(self.nodeInfoFeed.Subscribe(ch))
}

// Stop terminates the PbftAgent.
func (self * PbftAgent) Stop() {
	// Unsubscribe all subscriptions registered from agent
	self.scope.Close()
}

// VerifyCommitteeSign verify committee sign.
func (self * PbftAgent) VerifyCommitteeSign(signs []*types.PbftSign) bool {
	return false
}

// ChangeCommitteeLeader trigger view change.
func (self * PbftAgent) ChangeCommitteeLeader(height *big.Int) bool {
	return false
}

// getCommitteeNumber return Committees number
func (self * PbftAgent) GetCommitteeNumber(height *big.Int) int32 {
	return 0
}

func FromByte(data []byte,to interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	dec.Decode(to)
	return nil
}

func ToByte(e interface{}) ([]byte,error) {
	buf := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(e)
	if err != nil {
		return nil,err
	}
	return buf.Bytes(),nil
}

func RlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}