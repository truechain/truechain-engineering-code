package tbft

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/config"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"math/big"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type PbftAgentProxyImp struct {
	Name string
}

func NewPbftAgent(name string) *PbftAgentProxyImp {
	pap := PbftAgentProxyImp{Name: name}
	return &pap
}

var ID = big.NewInt(1)

func getID() *big.Int {
	ID = new(big.Int).Add(ID, big.NewInt(1))
	return ID
}

var IdCache = make(map[string]*big.Int)

func IdCacheInit() {
	lock = new(sync.Mutex)
	lock2 = new(sync.Mutex)

	IdCache["Agent1"] = big.NewInt(1)
	IdCache["Agent2"] = big.NewInt(1)
	IdCache["Agent3"] = big.NewInt(1)
	IdCache["Agent4"] = big.NewInt(1)
	IdCache["Agent5"] = big.NewInt(1)
}

var lock, lock2 *sync.Mutex

func getIDForCache(agent string) *big.Int {
	lock.Lock()
	defer lock.Unlock()
	return IdCache[agent]
}

func IdAdd(agent string) {
	lock2.Lock()
	defer lock2.Unlock()
	IdCache[agent] = new(big.Int).Add(IdCache[agent], big.NewInt(1))
}

func (pap *PbftAgentProxyImp) FetchFastBlock(committeeId *big.Int) (*types.Block, error) {
	header := new(types.Header)
	header.Number = getIDForCache(pap.Name) //getID()
	header.Time = big.NewInt(time.Now().Unix())
	println("[AGENT]", pap.Name, "++++++++", "FetchFastBlock", "Number:", header.Number.Uint64())
	//time.Sleep(time.Second * 5)
	return types.NewBlock(header, nil, nil, nil), nil
}

func (pap *PbftAgentProxyImp) GenerateSignWithVote(fb *types.Block, vote uint) (*types.PbftSign, error) {
	voteSign := &types.PbftSign{
		Result:     vote,
		FastHeight: fb.Header().Number,
		FastHash:   fb.Hash(),
	}
	if vote == types.VoteAgreeAgainst {
		log.Warn("vote AgreeAgainst", "number", fb.Number(), "hash", fb.Hash())
	}
	var err error
	signHash := voteSign.HashWithNoSign().Bytes()

	s := strings.Replace(pap.Name, "Agent", "", -1)
	num, e := strconv.Atoi(s)
	if e != nil || num == 1 {
		num = 0
	} else {
		num -= 1
	}
	pr1 := getPrivateKey(num)
	voteSign.Sign, err = crypto.Sign(signHash, pr1)
	if err != nil {
		log.Error("fb GenerateSign error ", "err", err)
	}
	return voteSign, err
}

func (pap *PbftAgentProxyImp) VerifyFastBlock(block *types.Block) (*types.PbftSign, error) {
	//if rand.Intn(100) > 30 {
	//	return types.ErrHeightNotYet
	//}
	println("[AGENT]", pap.Name, "VerifyFastBlock", "Number:", block.Header().Number.Uint64())

	return pap.GenerateSignWithVote(block, 1)
}

func (pap *PbftAgentProxyImp) BroadcastFastBlock(block *types.Block) {
	IdAdd(pap.Name)
	println("[AGENT]", pap.Name, "BroadcastFastBlock", "Number:", block.Header().Number.Uint64())
}

func (pap *PbftAgentProxyImp) BroadcastSign(sign *types.PbftSign, block *types.Block) error {
	println("[AGENT]", pap.Name, "--------", "BroadcastSign", "Number:", block.Header().Number.Uint64())
	return nil
}

var BcCount = 0

func (pap *PbftAgentProxyImp) BroadcastConsensus(block *types.Block) error {
	IdAdd(pap.Name)
	println("[AGENT]", pap.Name, "--------", "BroadcastConsensus", "Number:", block.Header().Number.Uint64())
	return nil
}

var comm = make(map[int][]byte)

func InitComm() {
	comm[0] = []byte("c1581e25937d9ab91421a3e1a2667c85b0397c75a195e643109938e987acecfc")
	comm[1] = []byte("42c4d734786eedaf5d0c51fd2bc9bbaa6c289ed23710d9381072932456aeca18")
	comm[2] = []byte("d878614f687c663baf7cdcbc32cc0fc88a036cdc6850023d880b03984426a629")
	comm[3] = []byte("26981a9479b7c4d98c546451c13a78b53c695df14c1968a086219edfe60bce2f")
	comm[4] = []byte("36981a9479b7c4d98c546451c13a78b53c695df14c1968a086219edfe60bce6f")
}

func getPrivateKey(id int) *ecdsa.PrivateKey {
	InitComm()
	key, err := hex.DecodeString(string(comm[id]))
	if err != nil {
		fmt.Println(err)
	}
	priv, err := crypto.ToECDSA(key)
	if err != nil {
		fmt.Println(err.Error())
	}
	return priv
}

func GetPub(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := ecdsa.PublicKey{
		Curve: priv.Curve,
		X:     new(big.Int).Set(priv.X),
		Y:     new(big.Int).Set(priv.Y),
	}
	return &pub
}

func TestPbftRunForOne(t *testing.T) {
	log.OpenLogDebug(4)
	IdCacheInit()
	start := make(chan int)
	pr := getPrivateKey(0)
	agent1 := NewPbftAgent("Agent1")
	n, _ := NewNode(config.DefaultConfig(), "1", pr, agent1)
	n.Start()
	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr)
	c1.Members = append(c1.Members, m1)
	c1.StartHeight = common.Big0
	n.PutCommittee(c1)
	n.Notify(c1.Id, Start)
	<-start
}

func TestPbftRunFor4(t *testing.T) {
	//log.OpenLogDebug(4)
	IdCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)

	agent1 := NewPbftAgent("Agent1")
	agent2 := NewPbftAgent("Agent2")
	agent3 := NewPbftAgent("Agent3")
	agent4 := NewPbftAgent("Agent4")

	config1 := new(config.Config)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress = "tcp://127.0.0.1:28890"
	p2p1.ExternalAddress = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

	config2 := new(config.Config)
	*config2 = *config.TestConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress = "tcp://127.0.0.1:28893"
	p2p2.ExternalAddress = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2

	con2 := new(config.ConsensusConfig)
	*con2 = *config2.Consensus
	con2.WalPath = filepath.Join("data", "cs.wal2", "wal")
	*config2.Consensus = *con2

	n2, _ := NewNode(config2, "1", pr2, agent2)
	n2.Start()

	config3 := new(config.Config)
	*config3 = *config.TestConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config3.P2P
	p2p3.ListenAddress = "tcp://127.0.0.1:28895"
	p2p3.ExternalAddress = "tcp://127.0.0.1:28896"
	*config3.P2P = *p2p3

	con3 := new(config.ConsensusConfig)
	*con3 = *config3.Consensus
	con3.WalPath = filepath.Join("data", "cs.wal3", "wal")
	*config3.Consensus = *con3

	n3, _ := NewNode(config3, "1", pr3, agent3)
	n3.Start()

	config4 := new(config.Config)
	*config4 = *config.TestConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress = "tcp://127.0.0.1:28897"
	p2p4.ExternalAddress = "tcp://127.0.0.1:28898"
	*config4.P2P = *p2p4

	con4 := new(config.ConsensusConfig)
	*con4 = *config4.Consensus
	con4.WalPath = filepath.Join("data", "cs.wal4", "wal")
	*config4.Consensus = *con4

	n4, _ := NewNode(config4, "1", pr4, agent4)
	n4.Start()

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

	n1.PutCommittee(c1)
	n1.PutNodes(common.Big1, cn)
	n1.Notify(c1.Id, Start)

	n2.PutCommittee(c1)
	n2.PutNodes(common.Big1, cn)
	n2.Notify(c1.Id, Start)

	n3.PutCommittee(c1)
	n3.PutNodes(common.Big1, cn)
	n3.Notify(c1.Id, Start)

	n4.PutCommittee(c1)
	n4.PutNodes(common.Big1, cn)
	n4.Notify(c1.Id, Start)

	<-start
}

func TestPbftRunFor5(t *testing.T) {
	log.OpenLogDebug(4)
	IdCacheInit()

	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)
	pr5 := getPrivateKey(4)

	agent1 := NewPbftAgent("Agent1")
	agent2 := NewPbftAgent("Agent2")
	agent3 := NewPbftAgent("Agent3")
	agent4 := NewPbftAgent("Agent4")
	agent5 := NewPbftAgent("Agent5")

	config1 := new(config.Config)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress = "tcp://127.0.0.1:28890"
	p2p1.ExternalAddress = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

	config2 := new(config.Config)
	*config2 = *config.TestConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress = "tcp://127.0.0.1:28893"
	p2p2.ExternalAddress = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2

	con2 := new(config.ConsensusConfig)
	*con2 = *config2.Consensus
	con2.WalPath = filepath.Join("data", "cs.wal2", "wal")
	*config2.Consensus = *con2

	n2, _ := NewNode(config2, "1", pr2, agent2)
	n2.Start()

	config3 := new(config.Config)
	*config3 = *config.TestConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config3.P2P
	p2p3.ListenAddress = "tcp://127.0.0.1:28895"
	p2p3.ExternalAddress = "tcp://127.0.0.1:28896"
	*config3.P2P = *p2p3

	con3 := new(config.ConsensusConfig)
	*con3 = *config3.Consensus
	con3.WalPath = filepath.Join("data", "cs.wal3", "wal")
	*config3.Consensus = *con3

	n3, _ := NewNode(config3, "1", pr3, agent3)
	n3.Start()

	config4 := new(config.Config)
	*config4 = *config.TestConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress = "tcp://127.0.0.1:28897"
	p2p4.ExternalAddress = "tcp://127.0.0.1:28898"
	*config4.P2P = *p2p4

	con4 := new(config.ConsensusConfig)
	*con4 = *config4.Consensus
	con4.WalPath = filepath.Join("data", "cs.wal4", "wal")
	*config4.Consensus = *con4

	n4, _ := NewNode(config4, "1", pr4, agent4)
	n4.Start()

	config5 := new(config.Config)
	*config5 = *config.TestConfig()
	p2p5 := new(config.P2PConfig)
	*p2p5 = *config5.P2P
	p2p5.ListenAddress = "tcp://127.0.0.1:28899"
	p2p5.ExternalAddress = "tcp://127.0.0.1:28900"
	*config5.P2P = *p2p5

	con5 := new(config.ConsensusConfig)
	*con5 = *config5.Consensus
	con5.WalPath = filepath.Join("data", "cs.wal5", "wal")
	*config5.Consensus = *con5

	n5, _ := NewNode(config5, "1", pr5, agent5)
	n5.Start()

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)

	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}

	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}

	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}

	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}

	m5 := new(types.CommitteeMember)
	m5.Publickey = GetPub(pr5)
	m5.Coinbase = common.Address{0}

	c1.Members = append(c1.Members, m1, m2, m3, m4, m5)
	c1.StartHeight = common.Big1

	n1.PutCommittee(c1)
	n1.Notify(c1.Id, Start)

	n2.PutCommittee(c1)
	n2.Notify(c1.Id, Start)

	n3.PutCommittee(c1)
	n3.Notify(c1.Id, Start)

	n4.PutCommittee(c1)
	n4.Notify(c1.Id, Start)

	n5.PutCommittee(c1)
	n5.Notify(c1.Id, Start)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28899, Coinbase: m5.Coinbase, Publickey: crypto.FromECDSAPub(m5.Publickey)})

	n5.PutNodes(common.Big1, cn)
	n4.PutNodes(common.Big1, cn)
	n1.PutNodes(common.Big1, cn)
	n2.PutNodes(common.Big1, cn)
	n3.PutNodes(common.Big1, cn)

	<-start
}

func TestRunPbft1(t *testing.T) {
	log.OpenLogDebug(4)
	IdCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)

	agent1 := NewPbftAgent("Agent1")

	config1 := new(config.Config)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress = "tcp://127.0.0.1:28890"
	p2p1.ExternalAddress = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1

	n1.Start()
	n1.PutCommittee(c1)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

	n1.PutNodes(common.Big1, cn)
	n1.Notify(c1.Id, Start)
	<-start
}

func TestRunPbft2(t *testing.T) {
	log.OpenLogDebug(4)
	IdCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)

	agent2 := NewPbftAgent("Agent2")

	config2 := new(config.Config)
	*config2 = *config.TestConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress = "tcp://127.0.0.1:28893"
	p2p2.ExternalAddress = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2

	con2 := new(config.ConsensusConfig)
	*con2 = *config2.Consensus
	con2.WalPath = filepath.Join("data", "cs.wal2", "wal")
	*config2.Consensus = *con2

	n2, _ := NewNode(config2, "1", pr2, agent2)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1

	n2.Start()
	n2.PutCommittee(c1)
	n2.Notify(c1.Id, Start)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

	n2.PutNodes(common.Big1, cn)

	<-start
}

func TestRunPbft3(t *testing.T) {
	log.OpenLogDebug(4)
	IdCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)

	agent3 := NewPbftAgent("Agent3")

	config3 := new(config.Config)
	*config3 = *config.TestConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config3.P2P
	p2p3.ListenAddress = "tcp://127.0.0.1:28895"
	p2p3.ExternalAddress = "tcp://127.0.0.1:28896"
	*config3.P2P = *p2p3

	con3 := new(config.ConsensusConfig)
	*con3 = *config3.Consensus
	con3.WalPath = filepath.Join("data", "cs.wal3", "wal")
	*config3.Consensus = *con3

	n3, _ := NewNode(config3, "1", pr3, agent3)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1

	n3.Start()
	n3.PutCommittee(c1)
	n3.Notify(c1.Id, Start)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

	n3.PutNodes(common.Big1, cn)

	<-start
}

func TestRunPbft4(t *testing.T) {
	log.OpenLogDebug(4)
	IdCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)

	agent4 := NewPbftAgent("Agent4")

	config4 := new(config.Config)
	*config4 = *config.TestConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress = "tcp://127.0.0.1:28897"
	p2p4.ExternalAddress = "tcp://127.0.0.1:28898"
	*config4.P2P = *p2p4

	con4 := new(config.ConsensusConfig)
	*con4 = *config4.Consensus
	con4.WalPath = filepath.Join("data", "cs.wal4", "wal")
	*config4.Consensus = *con4

	n4, _ := NewNode(config4, "1", pr4, agent4)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1

	n4.Start()
	n4.PutCommittee(c1)
	n4.Notify(c1.Id, Start)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

	n4.PutNodes(common.Big1, cn)

	<-start
}
