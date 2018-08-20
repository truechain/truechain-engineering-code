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
	"io"
	"fmt"
)

const (
	VoteAgree = iota		//vote agree
	VoteAgreeAgainst  		//vote against

	chainHeadChanSize = 10

	setCurrentCommittee = iota
	setNextCommittee

	BlockRewordSpace =12
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

	currentMu sync.Mutex
	mux          *event.TypeMux
	eventMux     *event.TypeMux

	signFeed       event.Feed
	nodeInfoFeed 	event.Feed
	NewFastBlockFeed	event.Feed
	scope        event.SubscriptionScope //send scope

	ElectionCh  chan core.ElectionEvent
	CommitteeCh  chan core.CommitteeEvent
	CryNodeInfoCh 	chan *CryNodeInfo
	//SnailBlockCh  chan snailchain.ChainHeadEvent

	electionSub event.Subscription
	committeeSub event.Subscription
	PbftNodeSub *event.TypeMuxSubscription
	//snailBlockSub event.Subscription

	CommitteeNode *types.CommitteeNode
	PrivateKey *ecdsa.PrivateKey
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
}

// NewPbftNodeEvent is posted when nodeInfo send
type NewPbftNodeEvent struct{ cryNodeInfo *CryNodeInfo}

type EncryptCommitteeNode []byte
type  CryNodeInfo struct {
	CommitteeId *big.Int
	Nodes       []EncryptCommitteeNode
	Sign 		[]byte	//sign msg
}

// DecodeRLP decodes the Ethereum
func (c *CryNodeInfo) DecodeRLP(s *rlp.Stream) error {
	err := s.Decode(c)
	return err
}
// EncodeRLP serializes b into the Ethereum RLP CryNodeInfo format.
func (c *CryNodeInfo) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, c)
}
func (c *CryNodeInfo) Hash() common.Hash {
	return RlpHash(c)
}

func NewPbftAgent(eth Backend, config *params.ChainConfig, engine consensus.Engine, election *Election) *PbftAgent {
	self := &PbftAgent{
		config:         	config,
		engine:         	engine,
		eth:            	eth,
		fastChain:          eth.BlockChain(),
		snailChain:			eth.SnailBlockChain(),
		CommitteeCh:	make(chan core.CommitteeEvent, 3),
		ElectionCh: 	make(chan core.ElectionEvent, 10),
		//SnailBlockCh:	make(chan snailchain.ChainHeadEvent, chainHeadChanSize),
		election: election,
	}

	prv_Hex := "c1581e25937d9ab91421a3e1a2667c85b0397c75a195e643109938e987acecfc"
	acc1Key, _ := crypto.HexToECDSA(prv_Hex)
	self.PrivateKey =acc1Key
	pubBytes :=crypto.FromECDSAPub(&acc1Key.PublicKey)
	self.CommitteeNode =&types.CommitteeNode{
		"127.0.0.1",
		80,
		crypto.PubkeyToAddress(acc1Key.PublicKey),
		pubBytes,
	}
	return self
}

func (self *PbftAgent) Start() {
	self.committeeSub = self.election.SubscribeCommitteeEvent(self.CommitteeCh)
	self.electionSub = self.election.SubscribeElectionEvent(self.ElectionCh)
	//self.snailBlockSub =self.snailChain.SubscribeChainHeadEvent(self.SnailBlockCh)
	// Subscribe
	go self.loop()
}


// Stop terminates the PbftAgent.
func (self *PbftAgent) Stop() {
	// Unsubscribe all subscriptions registered from agent
	self.committeeSub.Unsubscribe()
	self.electionSub.Unsubscribe()
	self.scope.Close()
}

func (self *PbftAgent) loop(){
	for {
		select {
		case ch := <-self.ElectionCh:
			if ch.Option ==types.CommitteeStart{
				fmt.Println("CommitteeStart:")
				self.SetCommitteeInfo(self.NextCommitteeInfo,setCurrentCommittee)
				self.SetCommitteeInfo(nil,setNextCommittee)
				//self.server.Notify(self.CommitteeInfo.Id,int(ch.Option))
			}else if ch.Option ==types.CommitteeStop{
				fmt.Println("CommitteeStop:")
				//self.server.Notify(self.CommitteeInfo.Id,int(ch.Option))
				self.SetCommitteeInfo(nil,setCurrentCommittee)
			}
		case ch := <-self.CommitteeCh:
			self.SetCommitteeInfo(self.NextCommitteeInfo,setNextCommittee)
			fmt.Println("CommitteeCh:",ch.CommitteeInfo)
			//if self.IsCommitteeMember(ch.CommitteeInfo){
			//	self.SendPbftNode(ch.CommitteeInfo)
			//	self.server.PutCommittee(ch.CommitteeInfo)
			//}

		//receive nodeInfo
		case cryNodeInfo := <-self.CryNodeInfoCh:
			//transpond  cryNodeInfo
			go self.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
			if  self.IsCommitteeMember(self.NextCommitteeInfo){
				self.ReceivePbftNode(cryNodeInfo)
			}
		}
	}
}

func (pbftAgent *PbftAgent) SendPbftNode(committeeInfo *types.CommitteeInfo) *CryNodeInfo{
	if committeeInfo == nil && len(committeeInfo.Members) <= 0{
		log.Info("committeeInfo is nil")
		return nil
	}
	cryNodeInfo := &CryNodeInfo{
		CommitteeId:committeeInfo.Id,
	}
	nodeByte,_ :=rlp.EncodeToBytes(pbftAgent.CommitteeNode) // TODO init committeeNode
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
	hash:=RlpHash(cryNodeInfo.Nodes)
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
	pbftAgent.CryNodeInfoCh <- cryNodeInfo
	return nil
}

func (pbftAgent *PbftAgent)  ReceivePbftNode(cryNodeInfo *CryNodeInfo) *types.CommitteeNode {
	fmt.Println("ReceivePbftNode ...")
	hash :=RlpHash(cryNodeInfo.Nodes)
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
func PrintNode(node *types.CommitteeNode){
	fmt.Println("*********************")
	fmt.Println("IP:",node.IP)
	fmt.Println("Port:",node.Port)
	fmt.Println("Coinbase:",node.Coinbase)
	fmt.Println("Publickey:",node.Publickey)
}

//generateBlock and broadcast
func (self * PbftAgent)  FetchFastBlock() (*types.Block,error){
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
	if fastBlock, err = self.engine.FinalizeFast(self.fastChain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return	fastBlock,err
	}

	currentFastBlock := self.fastChain.CurrentBlock()
	blockReward :=self.fastChain.GetSnailHeightByFastHeight(currentFastBlock.Hash(),currentFastBlock.Number().Uint64())
	var snailHegiht *big.Int = big.NewInt(0)
	if blockReward != nil{
		snailHegiht =blockReward.SnailNumber
	}
	snailHegiht = snailHegiht.Add(snailHegiht,big.NewInt(1))
	if temp :=snailHegiht; temp.Sub(self.snailChain.CurrentBlock().Number(),temp).Int64()>=BlockRewordSpace{
		fastBlock.Header().SnailNumber = snailHegiht
		sb :=self.snailChain.GetBlockByNumber(snailHegiht.Uint64())
		fastBlock.Header().SnailHash =sb.Hash()
	}

	return	fastBlock,nil
}

//broadcast blockAndSign
func (self * PbftAgent) BroadcastFastBlock(fb *types.Block) error{
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
	fb.SetLeaderSign(voteSign)
	self.NewFastBlockFeed.Send(core.NewBlockEvent{
		Block:	fb,
	})
	return err
}

func (self * PbftAgent) VerifyFastBlock(fb *types.Block) (bool,error){
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
		return false,err
	}

	//abort, results  :=bc.Engine().VerifyPbftFastHeader(bc, fb.Header(),parent.Header())
	state, err := bc.State()
	if err != nil{
		return false,err
	}
	receipts, _, usedGas, err := bc.Processor().Process(fb, state, vm.Config{})//update
	if err != nil{
		return false,err
	}
	err = bc.Validator().ValidateState(fb, parent, state, receipts, usedGas)
	if err != nil{
		return false,err
	}
	return true,nil
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

func (self *PbftAgent)  BroadcastSign(voteSign *types.PbftSign){
	/*fastBlocks	:= []*types.Block{fb}
	_,err :=self.fastChain.InsertChain(fastBlocks)
	if err != nil{
		panic(err)
	}*/
	self.signFeed.Send(core.PbftSignEvent{PbftSign:voteSign,})

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
	var feeAmount *big.Int;
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

type NodeInfoEvent struct{ nodeInfo *CryNodeInfo }
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
	return int32(len(self.CommitteeInfo.Members))
}

func (self *PbftAgent) SetCommitteeInfo(newCommitteeInfo *types.CommitteeInfo,CommitteeType int) error {
	if newCommitteeInfo == nil{
		newCommitteeInfo = &types.CommitteeInfo{}
	}
	if CommitteeType ==setCurrentCommittee{
		self.CommitteeInfo = newCommitteeInfo
	}else if CommitteeType ==setNextCommittee{
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