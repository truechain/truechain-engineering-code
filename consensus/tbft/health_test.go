package tbft

import (
	"fmt"
	config "github.com/truechain/truechain-engineering-code/params"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
	"path/filepath"
	"testing"
	"time"
)

func makeBlock() *types.Block {
	header := new(types.Header)
	header.Number = common.Big1
	header.Time = big.NewInt(time.Now().Unix())
	block := types.NewBlock(header, nil, nil, nil, nil)
	return block
}

func makePartSet(block *types.Block) (*ttypes.PartSet, error) {
	return ttypes.MakePartSet(ttypes.BlockPartSizeBytes, block)
}

func TestBlock(t *testing.T) {
	block := makeBlock()
	partset, _ := makePartSet(block)
	index := uint(0)
	part := partset.GetPart(index)
	msg := &BlockPartMessage{
		Height: 1,
		Round:  0,
		Part:   part,
	}
	data := cdc.MustMarshalBinaryBare(msg)
	msg2, err := decodeMsg(data)
	if err != nil {
		log.Error("Error decoding message", "bytes", data)
		return
	}
	log.Debug("Receive", "msg", msg2)
	msg3 := msg2.(*BlockPartMessage)
	fmt.Println(msg3)
}

func TestRlpBlock(t *testing.T) {
	header := new(types.Header)
	header.Number = common.Big1
	header.Time = big.NewInt(time.Now().Unix())
	block := types.NewBlock(header, nil, nil, nil, nil)
	bzs, err := rlp.EncodeToBytes(block)
	if err != nil {
		fmt.Println(err.Error())
	}

	var btmp types.Block

	err = rlp.DecodeBytes(bzs, &btmp)
	if err != nil {
		fmt.Println(err.Error())
	}
}


func TestPbftRunForHealth(t *testing.T) {
	log.OpenLogDebug(3)
	IDCacheInit()
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
	p2p2.ListenAddress1 = "tcp://127.0.0.1:28892"
	p2p2.ListenAddress2 = "tcp://127.0.0.1:28893"
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
	p2p3.ListenAddress1 = "tcp://127.0.0.1:28894"
	p2p3.ListenAddress2 = "tcp://127.0.0.1:28895"
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
	p2p4.ListenAddress1 = "tcp://127.0.0.1:28896"
	p2p4.ExternalAddress = "tcp://127.0.0.1:28897"
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
	m1.Flag = types.StateUsedFlag
	m1.Coinbase = common.Address{0}
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m2.Flag = types.StateUsedFlag
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m3.Flag = types.StateUsedFlag
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	m4.Flag = types.StateUsedFlag
	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big0

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28892, Port2: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28894, Port2: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28896, Port2: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})

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

	time.Sleep(time.Second * 20)

	n4.Notify(c1.Id, Stop)
	c1.Members[3].Flag = types.StateRemovedFlag

	time.Sleep(70 * time.Second)

	n4.PutCommittee(c1)
	n4.PutNodes(common.Big1, cn)
	n4.Notify(c1.Id, Start)

	n1.UpdateCommittee(c1)
	n2.UpdateCommittee(c1)
	n3.UpdateCommittee(c1)
	n4.UpdateCommittee(c1)

	<-start
}
