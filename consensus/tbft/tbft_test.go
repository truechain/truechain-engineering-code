package tbft

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	tcrypto "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	config "github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"math/rand"
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

var IDCache = make(map[string]*big.Int)

func IdCacheInit() {
	lock = new(sync.Mutex)

	IDCache["Agent1"] = big.NewInt(1)
	IDCache["Agent2"] = big.NewInt(1)
	IDCache["Agent3"] = big.NewInt(1)
	IDCache["Agent4"] = big.NewInt(1)
	IDCache["Agent5"] = big.NewInt(1)
}

var lock *sync.Mutex

func getIDForCache(agent string) *big.Int {
	lock.Lock()
	defer lock.Unlock()
	return IDCache[agent]
}

func IDAdd(agent string) {
	lock.Lock()
	defer lock.Unlock()
	tmp := new(big.Int).Set(IDCache[agent])
	IDCache[agent] = new(big.Int).Add(tmp, big.NewInt(1))
}

func (pap *PbftAgentProxyImp) FetchFastBlock(committeeID *big.Int, checkTX bool) (*types.Block, error) {
	header := new(types.Header)
	header.Number = getIDForCache(pap.Name) //getID()
	fmt.Println(pap.Name, header.Number)
	header.Time = big.NewInt(time.Now().Unix())
	println("[AGENT]", pap.Name, "++++++++", "FetchFastBlock", "Number:", header.Number.Uint64())
	//time.Sleep(time.Second * 5)
	return types.NewBlock(header, nil, nil, nil), nil
}

func (pap *PbftAgentProxyImp) GetCurrentHeight() *big.Int {
	return new(big.Int).Sub(getIDForCache(pap.Name), common.Big1)
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
		num--
	}
	pr1 := getPrivateKey(num)
	voteSign.Sign, err = crypto.Sign(signHash, pr1)
	if err != nil {
		log.Error("fb GenerateSign error ", "err", err)
	}
	return voteSign, err
}

func (pap *PbftAgentProxyImp) VerifyFastBlock(block *types.Block) (*types.PbftSign, error) {
	if rand.Intn(100) > 30 {
		return nil, types.ErrHeightNotYet
	}
	println("[AGENT]", pap.Name, "VerifyFastBlock", "Number:", block.Header().Number.Uint64())

	return pap.GenerateSignWithVote(block, 1)
}

func (pap *PbftAgentProxyImp) BroadcastFastBlock(block *types.Block) {
	IDAdd(pap.Name)
	println("[AGENT]", pap.Name, "BroadcastFastBlock", "Number:", block.Header().Number.Uint64())
}

func (pap *PbftAgentProxyImp) BroadcastSign(sign *types.PbftSign, block *types.Block) error {
	println("[AGENT]", pap.Name, "--------", "BroadcastSign", "Number:", block.Header().Number.Uint64())
	return nil
}

var BcCount = 0

func (pap *PbftAgentProxyImp) BroadcastConsensus(block *types.Block) error {
	IDAdd(pap.Name)
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
	if len(comm) == 0 {
		InitComm()
	}
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
	//log.OpenLogDebug(4)
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
func TestPbftRunFor2(t *testing.T) {
	//log.OpenLogDebug(3)
	IdCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)

	agent1 := NewPbftAgent("Agent1")
	agent2 := NewPbftAgent("Agent2")

	config1 := new(config.TbftConfig)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress1 = "tcp://127.0.0.1:28890"
	p2p1.ListenAddress2 = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

	config2 := new(config.TbftConfig)
	*config2 = *config.TestConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress1 = "tcp://127.0.0.1:28893"
	p2p2.ListenAddress2 = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2

	con2 := new(config.ConsensusConfig)
	*con2 = *config2.Consensus
	con2.WalPath = filepath.Join("data", "cs.wal2", "wal")
	*config2.Consensus = *con2

	n2, _ := NewNode(config2, "1", pr2, agent2)
	n2.Start()

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}

	c1.Members = append(c1.Members, m1, m2)
	c1.StartHeight = common.Big1

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})

	n1.PutCommittee(c1)
	n1.PutNodes(common.Big1, cn)
	n1.Notify(c1.Id, Start)

	n2.PutCommittee(c1)
	n2.PutNodes(common.Big1, cn)
	n2.Notify(c1.Id, Start)

	<-start
}
func TestPbftRunFor4(t *testing.T) {
	log.OpenLogDebug(3)
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

	config1 := new(config.TbftConfig)
	*config1 = *config.DefaultConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress1 = "tcp://127.0.0.1:28890"
	p2p1.ListenAddress2 = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

	config2 := new(config.TbftConfig)
	*config2 = *config.DefaultConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress1 = "tcp://127.0.0.1:28893"
	p2p2.ListenAddress2 = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2

	con2 := new(config.ConsensusConfig)
	*con2 = *config2.Consensus
	con2.WalPath = filepath.Join("data", "cs.wal2", "wal")
	*config2.Consensus = *con2

	n2, _ := NewNode(config2, "1", pr2, agent2)
	n2.Start()

	config3 := new(config.TbftConfig)
	*config3 = *config.DefaultConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config3.P2P
	p2p3.ListenAddress1 = "tcp://127.0.0.1:28895"
	p2p3.ListenAddress2 = "tcp://127.0.0.1:28896"
	*config3.P2P = *p2p3

	con3 := new(config.ConsensusConfig)
	*con3 = *config3.Consensus
	con3.WalPath = filepath.Join("data", "cs.wal3", "wal")
	*config3.Consensus = *con3

	n3, _ := NewNode(config3, "1", pr3, agent3)
	n3.Start()

	config4 := new(config.TbftConfig)
	*config4 = *config.DefaultConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress1 = "tcp://127.0.0.1:28897"
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
	c1.StartHeight = common.Big0

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Port2: 28894, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Port2: 28896, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Port2: 28898, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

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
func TestPbftRunFor4AndChange(t *testing.T) {
	//log.OpenLogDebug(3)
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

	config1 := new(config.TbftConfig)
	*config1 = *config.DefaultConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress1 = "tcp://127.0.0.1:28890"
	p2p1.ListenAddress2 = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

	config2 := new(config.TbftConfig)
	*config2 = *config.DefaultConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress1 = "tcp://127.0.0.1:28893"
	p2p2.ListenAddress2 = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2

	con2 := new(config.ConsensusConfig)
	*con2 = *config2.Consensus
	con2.WalPath = filepath.Join("data", "cs.wal2", "wal")
	*config2.Consensus = *con2

	n2, _ := NewNode(config2, "1", pr2, agent2)
	n2.Start()

	config3 := new(config.TbftConfig)
	*config3 = *config.DefaultConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config3.P2P
	p2p3.ListenAddress1 = "tcp://127.0.0.1:28895"
	p2p3.ListenAddress2 = "tcp://127.0.0.1:28896"
	*config3.P2P = *p2p3

	con3 := new(config.ConsensusConfig)
	*con3 = *config3.Consensus
	con3.WalPath = filepath.Join("data", "cs.wal3", "wal")
	*config3.Consensus = *con3

	n3, _ := NewNode(config3, "1", pr3, agent3)
	n3.Start()

	config4 := new(config.TbftConfig)
	*config4 = *config.DefaultConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress1 = "tcp://127.0.0.1:28897"
	p2p4.ListenAddress2 = "tcp://127.0.0.1:28898"
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
	c1.StartHeight = common.Big0

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28893, Port2: 28894, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28895, Port2: 28896, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28897, Port2: 28898, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

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

	n1.SetCommitteeStop(c1.Id, 16)
	n2.SetCommitteeStop(c1.Id, 16)
	n3.SetCommitteeStop(c1.Id, 16)
	n4.SetCommitteeStop(c1.Id, 16)

	time.Sleep(30 * time.Second)

	c2 := *c1
	c2.Id = common.Big2
	c2.StartHeight = big.NewInt(17)

	n1.PutCommittee(&c2)
	n1.PutNodes(common.Big2, cn)
	n1.Notify(c1.Id, Stop)
	n1.Notify(c2.Id, Start)

	n2.PutCommittee(&c2)
	n2.PutNodes(common.Big2, cn)
	n2.Notify(c1.Id, Stop)
	n2.Notify(c2.Id, Start)

	n3.PutCommittee(&c2)
	n3.PutNodes(common.Big2, cn)
	n3.Notify(c1.Id, Stop)
	n3.Notify(c2.Id, Start)

	n4.PutCommittee(&c2)
	n4.PutNodes(common.Big2, cn)
	n4.Notify(c1.Id, Stop)
	n4.Notify(c2.Id, Start)

	<-start
}

func TestPbftRunFor5(t *testing.T) {
	//log.OpenLogDebug(4)
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

	config1 := new(config.TbftConfig)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress1 = "tcp://127.0.0.1:28890"
	p2p1.ListenAddress2 = "tcp://127.0.0.1:28891"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

	config2 := new(config.TbftConfig)
	*config2 = *config.TestConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress1 = "tcp://127.0.0.1:28893"
	p2p2.ListenAddress2 = "tcp://127.0.0.1:28894"
	*config2.P2P = *p2p2

	con2 := new(config.ConsensusConfig)
	*con2 = *config2.Consensus
	con2.WalPath = filepath.Join("data", "cs.wal2", "wal")
	*config2.Consensus = *con2

	n2, _ := NewNode(config2, "1", pr2, agent2)
	n2.Start()

	config3 := new(config.TbftConfig)
	*config3 = *config.TestConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config3.P2P
	p2p3.ListenAddress1 = "tcp://127.0.0.1:28895"
	p2p3.ListenAddress2 = "tcp://127.0.0.1:28896"
	*config3.P2P = *p2p3

	con3 := new(config.ConsensusConfig)
	*con3 = *config3.Consensus
	con3.WalPath = filepath.Join("data", "cs.wal3", "wal")
	*config3.Consensus = *con3

	n3, _ := NewNode(config3, "1", pr3, agent3)
	n3.Start()

	config4 := new(config.TbftConfig)
	*config4 = *config.TestConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress1 = "tcp://127.0.0.1:28897"
	p2p4.ListenAddress2 = "tcp://127.0.0.1:28898"
	*config4.P2P = *p2p4

	con4 := new(config.ConsensusConfig)
	*con4 = *config4.Consensus
	con4.WalPath = filepath.Join("data", "cs.wal4", "wal")
	*config4.Consensus = *con4

	n4, _ := NewNode(config4, "1", pr4, agent4)
	n4.Start()

	config5 := new(config.TbftConfig)
	*config5 = *config.TestConfig()
	p2p5 := new(config.P2PConfig)
	*p2p5 = *config5.P2P
	p2p5.ListenAddress1 = "tcp://127.0.0.1:28899"
	p2p5.ListenAddress2 = "tcp://127.0.0.1:28900"
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
	//log.OpenLogDebug(4)
	IdCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)

	agent1 := NewPbftAgent("Agent1")

	config1 := new(config.TbftConfig)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress1 = "tcp://127.0.0.1:28890"
	p2p1.ListenAddress2 = "tcp://127.0.0.1:28891"
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

	config2 := new(config.TbftConfig)
	*config2 = *config.TestConfig()
	p2p2 := new(config.P2PConfig)
	*p2p2 = *config2.P2P
	p2p2.ListenAddress1 = "tcp://127.0.0.1:28893"
	p2p2.ListenAddress2 = "tcp://127.0.0.1:28894"
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

	config3 := new(config.TbftConfig)
	*config3 = *config.TestConfig()
	p2p3 := new(config.P2PConfig)
	*p2p3 = *config3.P2P
	p2p3.ListenAddress1 = "tcp://127.0.0.1:28895"
	p2p3.ListenAddress2 = "tcp://127.0.0.1:28896"
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

	config4 := new(config.TbftConfig)
	*config4 = *config.TestConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress1 = "tcp://127.0.0.1:28897"
	p2p4.ListenAddress2 = "tcp://127.0.0.1:28898"
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

func TestAddVote(t *testing.T) {
	IdCacheInit()
	const privCount int = 4
	var privs [privCount]*ecdsa.PrivateKey
	vals := make([]*ttypes.Validator, 0, 0)
	vPrivValidator := make([]ttypes.PrivValidator, 0, 0)

	var chainID_ string = "9999"
	var height_ uint64 = 1
	var round_ int = 0
	var type_ byte = ttypes.VoteTypePrevote

	for i := 0; i < privCount; i++ {
		privs[i] = getPrivateKey(i)
		pub := GetPub(privs[i])
		vp := ttypes.NewPrivValidator(*privs[i])
		vPrivValidator = append(vPrivValidator, vp)
		v := ttypes.NewValidator(tcrypto.PubKeyTrue(*pub), 1)
		vals = append(vals, v)
	}
	vset := ttypes.NewValidatorSet(vals)
	vVoteSet := ttypes.NewVoteSet(chainID_, height_, round_, type_, vset)
	// make block
	agent := NewPbftAgent("Agent1")
	cid := big.NewInt(1)
	block, _ := agent.FetchFastBlock(cid)
	hash := block.Hash()
	fmt.Println(common.ToHex(hash[:]))
	ps, _ := ttypes.MakePartSet(65535, block)
	// make vote
	for i, v := range vPrivValidator {
		var vote1 *ttypes.Vote
		if i == 3 {
			vote1 = signAddVote(v, vset, vVoteSet, height_, chainID_, uint(round_), type_, nil, ttypes.PartSetHeader{}, nil)
		} else {
			vote1 = signAddVote(v, vset, vVoteSet, height_, chainID_, uint(round_), type_, hash[:], ps.Header(), nil)
		}
		if vote1 != nil {
			vVoteSet.AddVote(vote1)
		}
	}
	bsuc := vVoteSet.HasTwoThirdsMajority()
	fmt.Println(bsuc)
	maj, _ := vVoteSet.TwoThirdsMajority()
	fmt.Println(maj.String())
	signs, _ := vVoteSet.MakePbftSigns(hash[:])
	fmt.Println(signs)

}

func signVote(privV ttypes.PrivValidator, vset *ttypes.ValidatorSet, height uint64, chainid_ string,
	round uint, type_ byte, hash []byte, header ttypes.PartSetHeader) (*ttypes.Vote, error) {
	addr := privV.GetAddress()
	valIndex, _ := vset.GetByAddress(addr)
	vote := &ttypes.Vote{
		ValidatorAddress: addr,
		ValidatorIndex:   uint(valIndex),
		Height:           height,
		Round:            round,
		Timestamp:        time.Now().UTC(),
		Type:             type_,
		BlockID:          ttypes.BlockID{hash, header},
	}

	err := privV.SignVote(chainid_, vote)
	return vote, err
}
func signAddVote(privV ttypes.PrivValidator, vset *ttypes.ValidatorSet, voteset *ttypes.VoteSet, height uint64, chainid_ string,
	round uint, type_ byte, hash []byte, header ttypes.PartSetHeader, keepsign *ttypes.KeepBlockSign) *ttypes.Vote {

	vote, err := signVote(privV, vset, height, chainid_, round, type_, hash, header)
	if err == nil {
		// if hash != nil && keepsign == nil {
		// 	if prevote := voteset.Prevotes(int(round)); prevote != nil {
		// 		keepsign = prevote.GetSignByAddress(privV.GetAddress())
		// 	}
		// }
		if hash != nil && keepsign != nil && bytes.Equal(hash, keepsign.Hash[:]) {
			vote.Result = keepsign.Result
			vote.ResultSign = make([]byte, len(keepsign.Sign))
			copy(vote.ResultSign, keepsign.Sign)
		}
		fmt.Println("Signed and pushed vote", "height", height, "round", round, "vote", vote, "err", err)
		return vote
	}
	fmt.Println("Error signing vote", "height", height, "round", round, "vote", vote, "err", err)
	return nil
}
func TestVote(t *testing.T) {
	bid := makeBlockID(nil, ttypes.PartSetHeader{})
	fmt.Println(bid.String())
	aa := len(bid.Hash)
	fmt.Println("aa:", aa)
}
func makeBlockID(hash []byte, header ttypes.PartSetHeader) ttypes.BlockID {
	blockid := ttypes.BlockID{hash, header}
	fmt.Println(blockid.String())
	return blockid
}

func TestTock(t *testing.T) {
	taskTimeOut := 3
	var d = time.Duration(taskTimeOut) * time.Second
	ttock := NewTimeoutTicker("ttock")
	ttock.Start()

	ttock.ScheduleTimeout(timeoutInfo{d, 1, uint(0), 0, 1})
	go TimeoutRoutine(&ttock)

	time.Sleep(30 * time.Second)
	ttock.Stop()
}

func TimeoutRoutine(tt *TimeoutTicker) {
	var pos uint = 0
	for {
		if pos >= 30 {
			return
		}
		select {
		case <-(*tt).Chan(): // tockChan:
			pos++
			fmt.Println(time.Now(), pos)
		}
	}

}
func TestPrivKey(t *testing.T) {
	priv1, _ := crypto.HexToECDSA("2ee9b9082e3eb19378d478f450e0e818e94cf7e3bf13ad5dd657ef2a35fbb0a8")
	tPriv1 := tcrypto.PrivKeyTrue(*priv1)
	addr1 := tPriv1.PubKey().Address()
	id1 := hex.EncodeToString(addr1[:])
	fmt.Println("id1", id1)

	priv2, _ := crypto.HexToECDSA("1bc73ab677ed9c3518417339bb5716e32fbc56e888c98d2e63e190dd51ca7eda")
	tPriv2 := tcrypto.PrivKeyTrue(*priv2)
	addr2 := tPriv2.PubKey().Address()
	id2 := hex.EncodeToString(addr2[:])
	fmt.Println("id2", id2)

	priv3, _ := crypto.HexToECDSA("d0c3b151031a8a90841dc18463d838cc8db29a10e7889b6991be0a3088702ca7")
	tPriv3 := tcrypto.PrivKeyTrue(*priv3)
	addr3 := tPriv3.PubKey().Address()
	id3 := hex.EncodeToString(addr3[:])
	fmt.Println("id3", id3)

	priv4, _ := crypto.HexToECDSA("c007a7302da54279edc472174a140b0093580d7d73cdbbb205654ea79f606c95")
	tPriv4 := tcrypto.PrivKeyTrue(*priv4)
	addr4 := tPriv4.PubKey().Address()
	id4 := hex.EncodeToString(addr4[:])
	fmt.Println("id4", id4)
}

//testting for putNodes
func TestPutNodes(t *testing.T) {
	//log.OpenLogDebug(3)
	IdCacheInit()
	start := make(chan int)
	pr1, _ := crypto.HexToECDSA("2ee9b9082e3eb19378d478f450e0e818e94cf7e3bf13ad5dd657ef2a35fbb0a8")
	pr2, _ := crypto.HexToECDSA("1bc73ab677ed9c3518417339bb5716e32fbc56e888c98d2e63e190dd51ca7eda")
	pr3, _ := crypto.HexToECDSA("d0c3b151031a8a90841dc18463d838cc8db29a10e7889b6991be0a3088702ca7")
	pr4, _ := crypto.HexToECDSA("c007a7302da54279edc472174a140b0093580d7d73cdbbb205654ea79f606c95")
	agent1 := NewPbftAgent("Agent1")

	config1 := new(config.TbftConfig)
	*config1 = *config.TestConfig()
	p2p1 := new(config.P2PConfig)
	*p2p1 = *config1.P2P
	p2p1.ListenAddress1 = "tcp://39.98.44.213:30310"
	p2p1.ListenAddress2 = "tcp://39.98.44.213:30311"
	*config1.P2P = *p2p1

	con1 := new(config.ConsensusConfig)
	*con1 = *config1.Consensus
	con1.WalPath = filepath.Join("data", "cs.wal1", "wal")
	*config1.Consensus = *con1

	n1, _ := NewNode(config1, "1", pr1, agent1)
	n1.Start()

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
	cn = append(cn, &types.CommitteeNode{IP: "39.98.44.213", Port: 30310, Port2: 30311, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "39.98.58.86", Port: 30310, Port2: 30311, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "39.98.56.108", Port: 30310, Port2: 30311, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "39.98.36.181", Port: 30310, Port2: 30311, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

	n1.PutCommittee(c1)
	n1.PutNodes(common.Big1, cn)
	n1.Notify(c1.Id, Start)

	<-start
}
