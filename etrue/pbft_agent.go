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
	VoteAgree        = iota //vote agree
	VoteAgreeAgainst        //vote against

	CurrentCommittee //current running committee
	NextCommittee    //next committee

)
const (
	BlockRewordSpace = 12
	sendNodeTime     = 30
	FetchBlockTime   = 5

	CryNodeInfoSize  = 20
	ChainHeadSize    = 3
	electionChanSize = 2

	singleNode = "single"
)

var testCommittee = []*types.CommitteeNode{
	{
		IP:        "192.168.46.88",
		Port:      10080,
		Coinbase:  common.HexToAddress("76ea2f3a002431fede1141b660dbb75c26ba6d97"),
		Publickey: common.Hex2Bytes("04044308742b61976de7344edb8662d6d10be1c477dd46e8e4c433c1288442a79183480894107299ff7b0706490f1fb9c9b7c9e62ae62d57bd84a1e469460d8ac1"),
	},
	{
		IP:        "192.168.46.19",
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

	CommitteeInfo     *types.CommitteeInfo
	NextCommitteeInfo *types.CommitteeInfo

	server   types.PbftServerProxy
	election *Election

	mu           *sync.Mutex //generateBlock mutex
	committeeMu  *sync.Mutex //committee mutex
	currentMu    *sync.Mutex //tx mutex
	cacheBlockMu *sync.Mutex

	mux *event.TypeMux

	signFeed         event.Feed
	nodeInfoFeed     event.Feed
	NewFastBlockFeed event.Feed
	scope            event.SubscriptionScope //send scope

	ElectionCh    chan core.ElectionEvent
	CommitteeCh   chan core.CommitteeEvent
	CryNodeInfoCh chan *CryNodeInfo
	ChainHeadCh   chan core.ChainHeadEvent

	electionSub       event.Subscription
	committeeSub      event.Subscription
	PbftNodeSub       *event.TypeMuxSubscription
	ChainHeadAgentSub event.Subscription

	CommitteeNode *types.CommitteeNode
	PrivateKey    *ecdsa.PrivateKey

	cacheSign  []Sign                    //prevent receive same sign
	cacheBlock map[*big.Int]*types.Block //prevent receive same block
}

type AgentWork struct {
	config *params.ChainConfig
	signer types.Signer

	state   *state.StateDB // apply state changes here
	tcount  int            // tx count in cycle
	gasPool *core.GasPool  // available gas used to pack transactions

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

type CryNodeInfo struct {

	CommitteeId *big.Int
	Nodes       []EncryptCommitteeNode
	Sign        //sign msg
}

func NewPbftAgent(eth Backend, config *params.ChainConfig, engine consensus.Engine, election *Election) *PbftAgent {
	log.Info("into NewPbftAgent...")
	self := &PbftAgent{
		config:        config,
		engine:        engine,
		eth:           eth,
		fastChain:     eth.BlockChain(),
		snailChain:    eth.SnailBlockChain(),
		CommitteeCh:   make(chan core.CommitteeEvent),
		ElectionCh:    make(chan core.ElectionEvent, electionChanSize),
		ChainHeadCh:   make(chan core.ChainHeadEvent, ChainHeadSize),
		CryNodeInfoCh: make(chan *CryNodeInfo, CryNodeInfoSize),
		election:      election,
		mu:            new(sync.Mutex),
		committeeMu:   new(sync.Mutex),
		currentMu:     new(sync.Mutex),
		cacheBlockMu:  new(sync.Mutex),
		cacheBlock:    make(map[*big.Int]*types.Block),
	}
	self.InitNodeInfo(eth.Config())
	self.committeeSub = self.election.SubscribeCommitteeEvent(self.CommitteeCh)
	self.electionSub = self.election.SubscribeElectionEvent(self.ElectionCh)
	self.ChainHeadAgentSub = self.fastChain.SubscribeChainHeadEvent(self.ChainHeadCh)
	return self
}

func (self *PbftAgent) InitNodeInfo(config *Config) {
	//self.NodeType = config.NodeType
	//TODO when IP or Port is nil
	if bytes.Equal(config.CommitteeKey, []byte{}) {
		if config.Host != "" || config.Port != 0 {
			self.CommitteeNode = &types.CommitteeNode{
				IP:   config.Host,
				Port: uint(config.Port),
			}
		}
		return
	}

	acc1Key, err := crypto.ToECDSA(config.CommitteeKey)
	if err != nil {
		log.Error("InitNodeInfo PrivateKey error ")
	}
	self.PrivateKey = acc1Key
	pubBytes := crypto.FromECDSAPub(&acc1Key.PublicKey)
	self.CommitteeNode = &types.CommitteeNode{
		IP:        config.Host,
		Port:      uint(config.Port),
		Coinbase:  crypto.PubkeyToAddress(acc1Key.PublicKey),
		Publickey: pubBytes,
	}
}

func (self *PbftAgent) nodeInfoIsExist() bool {
	if self.CommitteeNode == nil {
		log.Info("cannot load committeeNode config file.")
		return false
	}
	if self.CommitteeNode.IP == "" || self.CommitteeNode.Port == 0 ||
		bytes.Equal(self.CommitteeNode.Publickey, []byte{}) || self.CommitteeNode.Coinbase == [20]byte{} {
		log.Info("committeeNode config info is not complete ")
		return false
	}
	return true
}

func (self *PbftAgent) Start() {
	/*if self.NodeType == singleNode {
		self.StartSingleNode()
		return
	}*/
	go self.loop()
}

// Stop terminates the PbftAgent.
func (self *PbftAgent) Stop() {
	// Unsubscribe all subscriptions registered from agent
	self.committeeSub.Unsubscribe()
	self.electionSub.Unsubscribe()
	self.ChainHeadAgentSub.Unsubscribe()
	self.scope.Close()
}

func (self *PbftAgent) loop() {
	defer self.Stop()
	ticker := time.NewTicker(time.Second * sendNodeTime)
	for {
		select {
		case ch := <-self.ElectionCh:
			if ch.Option == types.CommitteeStart {
				log.Info("CommitteeStart...")
				self.committeeMu.Lock()
				self.SetCommitteeInfo(self.NextCommitteeInfo, CurrentCommittee)
				self.committeeMu.Unlock()
				if self.IsCommitteeMember(self.CommitteeInfo) {
					self.server.Notify(self.CommitteeInfo.Id, int(ch.Option))
				}
			} else if ch.Option == types.CommitteeStop {
				log.Info("CommitteeStop..")
				ticker.Stop()
				self.committeeMu.Lock()
				self.cacheSign = []Sign{}
				self.SetCommitteeInfo(nil, CurrentCommittee)
				self.committeeMu.Unlock()
				self.server.Notify(self.CommitteeInfo.Id, int(ch.Option))
			} else {
				log.Info("unknown Electionch:", ch.Option)
			}
		case ch := <-self.CommitteeCh:
			log.Info("CommitteeCh...")
			self.committeeMu.Lock()
			self.SetCommitteeInfo(ch.CommitteeInfo, NextCommittee)
			self.committeeMu.Unlock()
			if self.IsCommitteeMember(ch.CommitteeInfo) {
				self.server.PutCommittee(ch.CommitteeInfo)
				self.server.PutNodes(ch.CommitteeInfo.Id, testCommittee)
				go func() {
					for {
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
			if self.cryNodeInfoInCommittee(*cryNodeInfo) {
				go self.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
			} else {
				fmt.Println("cryNodeInfo of  node not in Committee.")
			}
			// if  node  is in committee  and the sign is not received
			if self.IsCommitteeMember(self.NextCommitteeInfo) && !self.cryNodeInfoInCachSign(cryNodeInfo.Sign) {
				log.Info("ReceivePbftNode method.")
				self.cacheSign = append(self.cacheSign, cryNodeInfo.Sign)
				self.ReceivePbftNode(cryNodeInfo)
			}
		case ch := <-self.ChainHeadCh:
			log.Info("ChainHeadCh update RewardNumber.")
			err := self.AddCacheIntoChain(ch.Block)
			if err != nil {
				log.Error("PutCacheIntoChain err")
				panic(err)
			}
		}
	}
}

// put cacheBlock into fastchain
func (self *PbftAgent) AddCacheIntoChain(receiveBlock *types.Block) error {
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
			panic(err)
			log.Info("sign error")
		}
		if voteSign == nil {
			fmt.Println("AddCacheIntoChain voteSign nil")
		}
		if fb == nil {
			fmt.Println("AddCacheIntoChain fb nil")
		}
		fmt.Println("AddCacheIntoChain")
		//go self.signFeed.Send(core.PbftSignEvent{PbftSign: voteSign})
		go self.signFeed.Send(core.PbftSignEvent{Block: fb, PbftSign: voteSign})

	}
	return nil
}

//committeeNode braodcat:if parentBlock is not in fastChain,put block  into cacheblock
func (self *PbftAgent) OperateCommitteeBlock(receiveBlock *types.Block) error {
	receiveBlockHeight := receiveBlock.Number()
	//receivedBlock has been into fashchain
	if self.fastChain.CurrentBlock().Number().Cmp(receiveBlockHeight) >= 0 {
		return nil
	}
	parent := self.fastChain.GetBlock(receiveBlock.ParentHash(), receiveBlock.NumberU64()-1)
	if parent != nil {
		//TODO isNeed find height+1
		var fastBlocks []*types.Block
		fastBlocks = append(fastBlocks, receiveBlock)
		fmt.Println("receiveBlockNumber:",receiveBlock.Number())
		fmt.Println("fastBlocks:",len(fastBlocks))
		//insertBlock
		_, err := self.fastChain.InsertChain(fastBlocks)
		if err != nil {
			return err
		}
		//generate sign
		voteSign, err := self.GenerateSign(receiveBlock)
		if err != nil {
			panic(err)
			log.Info("sign error")
		}

		if voteSign == nil {
			fmt.Println("OperateCommitteeBlock voteSign nil")
		}
		if receiveBlock == nil {
			fmt.Println("OperateCommitteeBlock receiveBlock nil")
		}
		fmt.Println("OperateCommitteeBlock")
		//braodcast sign and block
		self.signFeed.Send(core.PbftSignEvent{Block: receiveBlock, PbftSign: voteSign})
	} else {
		self.cacheBlockMu.Lock()
		self.cacheBlock[receiveBlockHeight] = receiveBlock
		self.cacheBlockMu.Unlock()
	}
	return nil
}

func (self *PbftAgent) cryNodeInfoInCommittee(cryNodeInfo CryNodeInfo) bool {
	if self.NextCommitteeInfo != nil && len(self.NextCommitteeInfo.Members) == 0 {
		log.Error("NextCommitteeInfo.Members is nil ...")
		return false
	}
	hash := RlpHash([]interface{}{cryNodeInfo.Nodes, cryNodeInfo.CommitteeId})
	pubKey, err := crypto.SigToPub(hash[:], cryNodeInfo.Sign)
	if err != nil {
		log.Error("SigToPub error.")
		panic(err)
		return false
	}
	for _, member := range self.NextCommitteeInfo.Members {
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

func (self *PbftAgent) cryNodeInfoInCachSign(sign Sign) bool {
	for _, cachesign := range self.cacheSign {
		if bytes.Equal(cachesign, sign) {
			return true
		}
	}
	return false
}

func (pbftAgent *PbftAgent) SendPbftNode(committeeInfo *types.CommitteeInfo) *CryNodeInfo {
	log.Info("into SendPbftNode.")
	if committeeInfo == nil && len(committeeInfo.Members) == 0 {
		log.Error("committeeInfo is nil,len(committeeInfo.Members):", len(committeeInfo.Members))
		return nil
	}
	cryNodeInfo := &CryNodeInfo{
		CommitteeId: committeeInfo.Id,
	}

	nodeByte, _ := rlp.EncodeToBytes(pbftAgent.CommitteeNode)
	var encryptNodes []EncryptCommitteeNode
	for _, member := range committeeInfo.Members {
		EncryptCommitteeNode, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(member.Publickey), nodeByte, nil, nil)
		if err != nil {
			panic(err)
			log.Error("encrypt error,pub:", ecies.ImportECDSAPublic(member.Publickey), ", coinbase:", member.Coinbase)
		}
		encryptNodes = append(encryptNodes, EncryptCommitteeNode)
	}
	cryNodeInfo.Nodes = encryptNodes

	hash := RlpHash([]interface{}{cryNodeInfo.Nodes, committeeInfo.Id})
	var err error
	cryNodeInfo.Sign, err = crypto.Sign(hash[:], pbftAgent.PrivateKey)
	if err != nil {
		panic(err)
		log.Error("sign error")
	}
	pbftAgent.nodeInfoFeed.Send(NodeInfoEvent{cryNodeInfo})
	return cryNodeInfo
}

func (pbftAgent *PbftAgent) AddRemoteNodeInfo(cryNodeInfo *CryNodeInfo) error {
	log.Info("into AddRemoteNodeInfo.")
	pbftAgent.CryNodeInfoCh <- cryNodeInfo
	return nil
}

func (pbftAgent *PbftAgent) ReceivePbftNode(cryNodeInfo *CryNodeInfo) *types.CommitteeNode {
	log.Info("ReceivePbftNode ...")
	hash := RlpHash([]interface{}{cryNodeInfo.Nodes, cryNodeInfo.CommitteeId})
	pubKey, err := crypto.SigToPub(hash[:], cryNodeInfo.Sign)
	if err != nil {
		panic(err)
		log.Error("SigToPub error.")
	}
	members := pbftAgent.election.GetComitteeById(cryNodeInfo.CommitteeId)
	verifyFlag := false
	for _, member := range members {
		if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
			verifyFlag = true
			break
		}
	}
	if !verifyFlag {
		log.Info("publicKey of send node is not in committee.")
		return nil
	}
	priKey := ecies.ImportECDSA(pbftAgent.PrivateKey) //ecdsa-->ecies
	for _, encryptNode := range cryNodeInfo.Nodes {
		decryptNode, err := priKey.Decrypt(encryptNode, nil, nil)
		if err == nil { // can Decrypt by priKey
			node := new(types.CommitteeNode) //receive nodeInfo
			rlp.DecodeBytes(decryptNode, node)
			pbftAgent.server.PutNodes(cryNodeInfo.CommitteeId, []*types.CommitteeNode{node})
			PrintNode(node)
			return node
		}
	}
	return nil
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
	fmt.Println("parentNum:",num)
	fmt.Println("currentNumber:",self.fastChain.CurrentHeader().Number)

	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.FastCalcGasLimit(parent),
		Time:       big.NewInt(tstamp),
	}
	fmt.Println("HeaderNum:",num)
	fmt.Println("common.Big1:",common.Big1)

	if err := self.engine.PrepareFast(self.fastChain, header); err != nil {
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

	//  padding Header.Root, TxHash, ReceiptHash.
	// Create the new block to seal with the consensus engine
	if fastBlock, err = self.engine.FinalizeFast(self.fastChain, header, work.state, work.txs, work.receipts); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return fastBlock, err
	}

	//generate rewardSnailHegiht  //TODO zanshi not used
	/*var rewardSnailHegiht *big.Int
	BlockReward :=self.fastChain.CurrentReward()
	if BlockReward == nil{
		rewardSnailHegiht = new(big.Int).Set(common.Big1)
	}else{
		rewardSnailHegiht = new(big.Int).Add(BlockReward.SnailNumber,common.Big1)
	}


	space := new(big.Int).Sub(self.snailChain.CurrentBlock().Number(),rewardSnailHegiht).Int64()
	if space >= BlockRewordSpace{
		fastBlock.Header().SnailNumber = rewardSnailHegiht
		sb :=self.snailChain.GetBlockByNumber(rewardSnailHegiht.Uint64())
		fastBlock.Header().SnailHash =sb.Hash()
	}*/

	fmt.Println("fastBlockHeight:", fastBlock.Header().Number)
	voteSign, err := self.GenerateSign(fastBlock)
	if err != nil {
		panic(err)
		log.Info("sign error")
	}
	if voteSign == nil {
		fmt.Println("leader sign nil ")
	}
	fmt.Println("leader sign:", voteSign)
	fastBlock.AppendSign(voteSign)

	return fastBlock, nil
}

func (self *PbftAgent) GenerateSign(fb *types.Block) (*types.PbftSign, error) {
	if !self.nodeInfoIsExist() {
		return nil, errors.New("nodeInfo is not exist ,cannot generateSign.")
	}
	voteSign := &types.PbftSign{
		Result:     VoteAgree,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	var err error
	signHash := GetSignHash(voteSign)
	voteSign.Sign, err = crypto.Sign(signHash, self.PrivateKey)
	return voteSign, err
}

//broadcast blockAndSign
func (self *PbftAgent) BroadcastFastBlock(fb *types.Block) error {
	log.Info("into BroadcastFastBlock.")
	if !self.nodeInfoIsExist() {
		return errors.New("nodeInfo is not exist ,cannot generateSign.")
	}
	voteSign, err := self.GenerateSign(fb)
	if err != nil {
		panic(err)
		log.Info("sign error")
	}
	if voteSign == nil {
		fmt.Println("leader sign nil ")
	}
	fmt.Println("leader sign:", voteSign)
	fb.AppendSign(voteSign)
	self.NewFastBlockFeed.Send(core.NewBlockEvent{Block: fb})
	return err
}

func (self *PbftAgent) VerifyFastBlock(fb *types.Block) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	log.Info("into VerifyFastBlock.")
	fmt.Println("hash:", fb.Hash(), "number:", fb.Header().Number)
	fmt.Println("parentHash:", fb.ParentHash())

	bc := self.fastChain
	// get current head
	var parent *types.Block
	parent = bc.GetBlock(fb.ParentHash(), fb.NumberU64()-1)
	if parent == nil { //if cannot find parent return ErrUnSyncParentBlock
		return types.ErrHeightNotYet
	}
	err := self.engine.VerifyFastHeader(bc, fb.Header(), true)
	if err != nil {
		panic(err)
		log.Error("VerifyFastHeader error")
	} else {
		err = bc.Validator().ValidateBody(fb)
	}
	if err != nil {
		return err
	}

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
	err := self.OperateCommitteeBlock(fb)
	if err != nil {
		return err
	}
	return nil
}

/*func (self *PbftAgent)  BroadcastSign(voteSign *types.PbftSign, fb *types.Block) error{
	//var fastBlocks	 []*types.Block
	log.Info("into BroadcastSign.")
	self.mu.Lock()
	defer self.mu.Unlock()
	err :=self.PutCacheIntoChain(fb)
	if err != nil{
		return err
	}

	if len(self.cacheBlock) !=0{
		var maxHeight =common.Big0
		for height,block :=range self.cacheBlock{
			if maxHeight.Cmp(height) ==-1{
				maxHeight =height
			}
			fastBlocks =append(fastBlocks,block)
		}

		currentchainBlockNumber :=self.current.Block.Number()
		distance :=fb.Number().Uint64() -currentchainBlockNumber.Uint64()-1
		if distance == uint64(len(self.cacheBlock)){
			fastBlocks	=append(fastBlocks,fb)
		}

	}else{
		fastBlocks	=append(fastBlocks,fb)
	}
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
}*/

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

func (self *PbftAgent) IsCommitteeMember(committeeInfo *types.CommitteeInfo) bool {
	if !self.nodeInfoIsExist() {
		return false
	}
	if committeeInfo == nil || len(committeeInfo.Members) == 0 {
		fmt.Println("IsCommitteeMember committeeInfo :", len(committeeInfo.Members))
		return false
	}
	for _, member := range committeeInfo.Members {
		if bytes.Equal(self.CommitteeNode.Publickey, crypto.FromECDSAPub(member.Publickey)) {
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

// verify sign of node is in committee
func (self *PbftAgent) VerifyCommitteeSign(signs []*types.PbftSign) (bool, string) {
	if self.CommitteeInfo == nil || len(self.CommitteeInfo.Members) == 0 {
		log.Error("CommitteeInfo.Members is nil ...")
	}

	for _, sign := range signs {
		if sign == nil {
			fmt.Println("cnm")
		}
		fmt.Println("sign:", sign)
		pubKey, err := crypto.SigToPub(GetSignHash(sign), sign.Sign)
		if err != nil {
			log.Error("SigToPub error.")
			return false, ""
		}
		for _, member := range self.CommitteeInfo.Members {
			if bytes.Equal(crypto.FromECDSAPub(pubKey), crypto.FromECDSAPub(member.Publickey)) {
				return true, hex.EncodeToString(crypto.FromECDSAPub(pubKey))
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
func (self *PbftAgent) ChangeCommitteeLeader(height *big.Int) bool {
	return false
}

// getCommitteeNumber return Committees number
func (self *PbftAgent) GetCommitteeNumber(height *big.Int) int32 {
	committees := self.election.GetCommittee(height)
	if committees == nil {
		return 0
	}
	return int32(len(committees))
}

func (self *PbftAgent) SetCommitteeInfo(newCommitteeInfo *types.CommitteeInfo, CommitteeType int) error {
	if newCommitteeInfo == nil {
		newCommitteeInfo = &types.CommitteeInfo{}
	}
	if CommitteeType == CurrentCommittee {
		self.CommitteeInfo = newCommitteeInfo
	} else if CommitteeType == NextCommittee {
		self.NextCommitteeInfo = newCommitteeInfo
	} else {
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

func PrintNode(node *types.CommitteeNode) {
	fmt.Println("*********************")
	fmt.Println("IP:", node.IP)
	fmt.Println("Port:", node.Port)
	fmt.Println("Coinbase:", node.Coinbase)
	fmt.Println("Publickey:", node.Publickey)
}

func (self *PbftAgent) AcquireCommitteeAuth(height *big.Int) bool {
	if !self.nodeInfoIsExist() {
		return false
	}
	committeeMembers := self.election.GetCommittee(height)
	for _, member := range committeeMembers {
		if bytes.Equal(self.CommitteeNode.Publickey, crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

func (agent *PbftAgent) SendBlock() {
	for {
		//获取区块
		block, err := agent.FetchFastBlock()
		if err != nil {
			panic(err)
		}
		time.Sleep(time.Second * 2)
		//发出区块
		err = agent.BroadcastFastBlock(block)
		if err != nil {
			panic(err)
		}
		time.Sleep(time.Second * 2)
		//验证区块
		err = agent.VerifyFastBlock(block)
		if err != nil {
			panic(err)
		}
		fmt.Println("validate true")
		time.Sleep(time.Second * 3)

		err = agent.BroadcastConsensus(block)
		if err != nil {
			panic(err)
		}
	}
}

func (agent *PbftAgent) StartSingleNode() {
	for {
		//获取区块
		t1 := time.Now()
		var block *types.Block
		var err error
		for {
			block, err = agent.FetchFastBlock()
			if err != nil {
				return
			}
			sub := time.Now().Sub(t1)
			if len(block.Transactions()) == 0 && sub < time.Second*FetchBlockTime*60 {
				time.Sleep(time.Second * FetchBlockTime)
			} else {
				break
			}
		}
		time.Sleep(time.Second * 1)
		//发出区块
		err = agent.BroadcastFastBlock(block)
		if err != nil {
			panic(err)
		}
		time.Sleep(time.Second * 1)
		//验证区块
		err = agent.VerifyFastBlock(block)
		if err != nil {
			panic(err)
		}
		fmt.Println("validate true")
		time.Sleep(time.Second * 2)

		err = agent.BroadcastConsensus(block)
		if err != nil {
			panic(err)
		}
	}
}
