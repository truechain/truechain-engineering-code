package etrue

import (
	"math/big"
	"time"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/core/fastchain"
	"github.com/truechain/truechain-engineering-code/etrue/truechain"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/crypto/ecies"
	"crypto/ecdsa"
	"sync"
	"bytes"
	"fmt"
	"crypto/rand"
)

const (
	PbftActionStart = iota  // start pbft consensus
	PbftActionStop          // stop pbft consensus
	PbftActionSwitch       //switch pbft committee
)

type Pbftagent interface {
	FetchBlock() (*types.FastBlock,error)
	VerifyFastBlock() error
	ComplateSign (sign []*PbftSign) error
}

type PbftServer interface {
	MembersNodes(nodes []*PbftNode) error
	Actions(ac *PbftAction) error
}

type PbftSign struct {
	FastHeight *big.Int
	FastHash common.Hash	// fastblock hash
	ReceiptHash common.Hash	// fastblock receiptHash
	Sign []byte	// sign for fastblock hash
}

type PbftNode struct {
	NodeIP string
	NodePort uint
	CoinBase common.Address
	PublicKey *ecdsa.PublicKey

	eventMux      *event.TypeMux
	PbftNodeSub *event.TypeMuxSubscription
}

type  CryNodeInfo struct {
	InfoByte	[]byte	//签名前
	Sign 		[]byte	//签名后
}

type PbftAction struct {
	Id *big.Int		//committee times
	action int
}

type NewPbftNodeEvent struct{ cryNodeInfo *CryNodeInfo}

var privateKey *ecdsa.PrivateKey
var pks []*ecdsa.PublicKey	//接口得到的
var node PbftNode

func (self *PbftAgent) loop(){
	fmt.Println("loop...")
	for {
		select {
		// Handle ChainHeadEvent
		case committeeAction := <-self.committeeActionCh:
			if committeeAction.action ==PbftActionStart{
				//Actions(committeeAction)  //实现了该接口的对象
			}else if committeeAction.action ==PbftActionStop{

			}else if committeeAction.action ==PbftActionSwitch{
				self.SendPbftNode()//广播本节点信息
				self.Start()//接收其他本节点信息
			}
		}
	}
}

func (pbftAgent *PbftAgent) Start() {
	// broadcast mined blocks
	pbftAgent.PbftNodeSub = pbftAgent.eventMux.Subscribe(NewPbftNodeEvent{})
	go pbftAgent.handle()
}

func  (pbftAgent *PbftAgent) handle(){
	for obj := range pbftAgent.PbftNodeSub.Chan() {
		switch cryNodeInfo := obj.Data.(type) {
		case CryNodeInfo:
			pbftAgent.ReceivePbftNode(cryNodeInfo)
		}
	}
}

func (pbftAgent *PbftAgent)  SendPbftNode() []byte {
	var nodeInfos [][]byte
	nodeByte,_ :=truechain.ToByte(node)
	for _,pk := range pks{
		encryptMsg,err :=ecies.Encrypt(rand.Reader,ecies.ImportECDSAPublic(pk),
			nodeByte, nil, nil)
		if err != nil{
			return nil
		}
		nodeInfos =append(nodeInfos,encryptMsg)
	}
	infoByte,_ := truechain.ToByte(nodeInfos)

	sigInfo,err :=crypto.Sign(infoByte, privateKey)
	if err != nil{
		log.Info("sign error")
		return nil
	}
	cryNodeInfo :=&CryNodeInfo{infoByte,sigInfo}
	node.eventMux.Post(NewPbftNodeEvent{cryNodeInfo})
	return sigInfo
}
func (pbftAgent *PbftAgent)  ReceivePbftNode(cryNodeInfo CryNodeInfo) [][]byte {
	hash:= cryNodeInfo.InfoByte
	sig := cryNodeInfo.Sign

	pubKey,err :=crypto.SigToPub(hash,sig)
	if err != nil{
		log.Info("SigToPub error.")
		return nil
	}

	verifyFlag := false
	for _,pk := range pks{
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
	var nodeInfos [][]byte
	truechain.FromByte(hash,nodeInfos)
	for _,info := range nodeInfos{
		priKey :=ecies.ImportECDSA(privateKey)//ecdsa-->ecies
		encryptMsg,err :=priKey.Decrypt(info, nil, nil)
		if err != nil{
			return nil
		}
		nodeInfos =append(nodeInfos,encryptMsg)
	}
	return nodeInfos
}

type PbftVoteSign struct {
	Result uint	// 0--agree,1--against
	FastHeight *big.Int	// fastblock height
	Msg common.Hash		// hash(fasthash+ecdsa.PublicKey+Result)
	Sig []byte		// sign for SigHash
}



type PbftAgent struct {
	config *params.ChainConfig
	chain   *fastchain.FastBlockChain

	engine consensus.Engine
	eth     Backend
	signer types.Signer
	current *AgentWork
	currentMu sync.Mutex
	mux          *event.TypeMux

	snapshotMu    sync.RWMutex
	snapshotState *state.StateDB
	snapshotBlock *types.FastBlock

	committeeActionCh  chan PbftAction
	committeeSub event.Subscription

	eventMux      *event.TypeMux
	PbftNodeSub *event.TypeMuxSubscription
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

func NewPbftAgent(eth Backend, config *params.ChainConfig,mux *event.TypeMux, engine consensus.Engine) *PbftAgent {
	self := &PbftAgent{
		config:         	config,
		engine:         	engine,
		eth:            	eth,
		mux:            	mux,
		chain:          	eth.FastBlockChain(),
		committeeActionCh:	make(chan PbftAction, 3),
	}
	// Subscribe events from blockchain
	//self.committeeSub = self.chain.SubscribeChainHeadEvent(self.committeeActionCh)
	self.committeeSub = self.chain.SubscribeCommitteeActionEvent(self.committeeActionCh)
	go self.loop()
	return self
}

func  (self * PbftAgent)  FetchBlock() (*types.FastBlock,error){
	var fastBlock  *types.FastBlock

	//1 准备新区块的时间属性Header.Time
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

	//2 创建新区块的Header对象，
	num := parent.Number()
	header := &types.FastHeader{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   fastchain.FastCalcGasLimit(parent),
		Time:       big.NewInt(tstamp),
	}
	// 3 调用Engine.Prepare()函数，完成Header对象的准备。
	if err := self.engine.PrepareFast(self.chain, header); err != nil {
		log.Error("Failed to prepare header for generateFastBlock", "err", err)
		return	fastBlock,err
	}
	// 4 根据已有的Header对象，创建一个新的Work对象，并用其更新worker.current成员变量。
	// Create the current work task and check any fork transitions needed
	err := self.makeCurrent(parent, header)
	work := self.current

	//5 准备新区块的交易列表，来源是TxPool中那些最近加入的tx，并执行这些交易。
	pending, err := self.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return	fastBlock,err
	}
	txs := types.NewTransactionsByPriceAndNonce(self.current.signer, pending)
	work.commitTransactions(self.mux, txs, self.chain)

	// 6 对新区块“定型”，填充上Header.Root, TxHash, ReceiptHash等几个属性。
	// Create the new block to seal with the consensus engine
	if work.Block, err = self.engine.FinalizeFast(self.chain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return	fastBlock,err
	}
	//self.updateSnapshot()
	return	fastBlock,nil
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

func (self * PbftAgent) VerifyFastBlock(fb *types.FastBlock) error{
	bc := self.chain
	err :=bc.Engine().VerifyFastHeader(bc, fb.Header(),true)
	if err == nil{
		err = bc.Validator().ValidateBody(fb)
	}
	if err != nil{
		return err
	}
	var parent *types.FastBlock
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)

	//abort, results  :=bc.Engine().VerifyPbftFastHeader(bc, fb.Header(),parent.Header())

	state, err := bc.State()
	if err != nil{
		return err
	}
	receipts, _, usedGas, err := bc.Processor().Process(fb, state, vm.Config{})//update
	err = bc.Validator().ValidateState(fb, parent, state, receipts, usedGas)
	if err != nil{
		return err
	}
	/*// Write the block to the chain and get the status.
	status, err := bc.WriteBlockWithState(fb, receipts, state) //update
	if err != nil{
		return err
	}
	if status  == fastchain.CanonStatTy{
		log.Debug("Inserted new block", "number", fb.Number(), "hash", fb.Hash(), "uncles", 0,
			"txs", len(fb.Transactions()), "gas", fb.GasUsed(), "elapsed", "")
	}*/
	return nil

}


