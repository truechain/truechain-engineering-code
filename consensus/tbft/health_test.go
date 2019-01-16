package tbft

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p"
	config "github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"encoding/hex"
	"crypto/ecdsa"
	"path/filepath"
	"testing"
	"time"
	//"github.com/golang/mock/gomock"
)
type hItem struct {
	mb 		*types.CommitteeMember
	addr	[]byte
	id 		tp2p.ID
}
type hUpdateItem struct {
	mgr 		*ttypes.HealthMgr
	hh     		[]*hItem
	repos		[]int
}
var (
	quit = false
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
func makeCommitteeInfo(cc,cid int) *types.CommitteeInfo {
	committeeCount := cc
	privs := make([]*ecdsa.PrivateKey,committeeCount)
	cinfo := new(types.CommitteeInfo)
	work := make([]*types.CommitteeMember,committeeCount)
	for i:=0;i<committeeCount;i++ {
		privs[i] = getPrivateKey(i)
		work[i] = &types.CommitteeMember{
			Publickey:		GetPub(privs[i]),
			Flag:			types.StateUsedFlag,
			Coinbase:		common.Address{0},
			MType:			types.TypeWorked,
		}
	}
	cinfo.Members = work
	cinfo.Id = big.NewInt(int64(cid))
	cinfo.StartHeight,cinfo.EndHeight = big.NewInt(1),big.NewInt(10000)
	return cinfo
}
func makeValidatorSet(info *types.CommitteeInfo) *ttypes.ValidatorSet {
	return MakeValidators(info)
}
func makeHealthMgr(cid,committeeCount int) (*ttypes.HealthMgr,[]*hItem) {
	h := make([]*hItem,committeeCount)
	mgr := ttypes.NewHealthMgr(uint64(cid))
	info := makeCommitteeInfo(committeeCount,cid)
	vset := makeValidatorSet(info)
	for i,v := range info.Members {
		id := pkToP2pID(v.Publickey)
		address,_ := hex.DecodeString(string(id))
		h[i] = &hItem{
			mb:			v,
			addr:		address,
			id:			id,
		}
		_,val := vset.GetByAddress(address)
		health := ttypes.NewHealth(id, v.MType, v.Flag, val, false)
		mgr.PutWorkHealth(health)
	}
	return mgr,h
}
func TestSwitchItemInCommittee(t *testing.T) {
	
	end := make(chan int)
	chanmgr := make(chan *hUpdateItem)
	go healthUpdate(chanmgr,end)

	cid,committeeCount := 1,4
	mgr,hh := makeHealthMgr(cid,committeeCount)
	mgr.Start()
	out := make(chan *ttypes.SwitchValidator)

	for i:=0;i<10;i++ {
		go func(){
			chanmgr <- &hUpdateItem{
				mgr:		mgr,
				hh:			hh,
				repos:		[]int{i+2},
			}
		}()
		go svHandle(mgr,1,out)
		go checkResult(end,out)
	
		<-end
	}

	mgr.Stop()
}
func healthUpdate(recv <-chan *hUpdateItem,end chan<-int) {
	// once update 
	for {
		if quit { return }
		var item *hUpdateItem
		func() {
			select{
			case item =<-recv:
			default:
			}
		}()
		if item != nil {
			sum := item.mgr.Sum()
			if sum > 0 {
				for _,pos := range item.repos {
					if pos > 0 && pos < sum {
						for i,v := range item.hh {
							if i != pos {
								item.mgr.Update(v.id)
							}
						}
					} else {
						go func() {	end <- 100 }()
					}
					time.Sleep(5*time.Millisecond)
				}	 
			} else {
				fmt.Println("healthUpdate func is quit,cause sum is 0")
				go func() {	end <- 100 }()
			}
		}
		time.Sleep(1*time.Second)
	}
}
func svHandle(mgr *ttypes.HealthMgr,times int,out chan<-*ttypes.SwitchValidator) {
	for {
		select {
		case sv := <- mgr.ChanTo():
			fmt.Println("get sv:",sv)
			if err := mgr.VerifySwitch(sv); err== nil {
				go func() {
					select {
					case mgr.ChanFrom() <- sv:
					default:
					}
					out<-sv
				}()
			} else {
				fmt.Println("verify sv failed,err:",err,"sv:",sv)
			}			
		}
		if times--; times <= 0 {
			return 
		}
	}
}
func checkResult(end chan<-int, out <-chan *ttypes.SwitchValidator) {
	rsv := <- out
	pos := 1
	for {
		fmt.Println("check the sv Result.....[",pos,"]")
		if rsv.Remove.State == types.StateRemovedFlag {
			fmt.Print("check Remove the SV success,sv:",rsv)
			go func() {	end <- 100 }()
			return
		}
		time.Sleep(1 * time.Second)	
		pos++
	}
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
	sv := block.SwitchInfos()
	if sv == nil {
		fmt.Println("sv is nil ")
	}
	signs := block.Signs()
	if signs == nil {
		fmt.Println("signs is nil ")
	}
	body := block.Body()
	if body == nil {
		fmt.Println("body is nil ")
	}
	fmt.Println("finish ")
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
	c1.EndHeight = common.Big0

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

func TestRunPbftChange1(t *testing.T) {
	log.OpenLogDebug(3)
	IDCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)
	pr5 := getPrivateKey(4)

	agent1 := NewPbftAgent("Agent1")

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

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m1.MType = types.TypeWorked
	m1.Flag = types.StateUsedFlag
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m2.MType = types.TypeWorked
	m2.Flag = types.StateUsedFlag
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m3.MType = types.TypeWorked
	m3.Flag = types.StateUsedFlag
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	m4.MType = types.TypeWorked
	m4.Flag = types.StateUsedFlag

	m5 := new(types.CommitteeMember)
	m5.Publickey = GetPub(pr5)
	m5.Coinbase = common.Address{0}
	m5.MType = types.TypeBack
	m5.Flag = types.StateUnusedFlag
	c1.BackMembers = append(c1.BackMembers, m5)

	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1
	c1.EndHeight = big.NewInt(11111)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28892, Port2: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28894, Port2: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28896, Port2: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28898, Port2: 28899, Coinbase: m5.Coinbase, Publickey: crypto.FromECDSAPub(m5.Publickey)})

	n1.Start()
	n1.PutCommittee(c1)
	n1.PutNodes(common.Big1, cn)
	n1.Notify(c1.Id, Start)

	//for {
	time.Sleep(time.Second * 220)
	c1.Members[3].Flag = types.StateRemovedFlag
	c1.Members[3].MType = types.TypeWorked
	c1.BackMembers[0].Flag = types.StateUsedFlag
	c1.StartHeight = getIDForCache("Agent1")
	c1.EndHeight = new(big.Int).Add(c1.StartHeight, big.NewInt(20))
	n1.UpdateCommittee(c1)
	//}
	<-start
}

func TestRunPbftChange2(t *testing.T) {
	log.OpenLogDebug(3)
	IDCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)
	pr5 := getPrivateKey(4)

	agent2 := NewPbftAgent("Agent2")

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

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m1.MType = types.TypeWorked
	m1.Flag = types.StateUsedFlag
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m2.MType = types.TypeWorked
	m2.Flag = types.StateUsedFlag
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m3.MType = types.TypeWorked
	m3.Flag = types.StateUsedFlag
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	m4.MType = types.TypeWorked
	m4.Flag = types.StateUsedFlag

	m5 := new(types.CommitteeMember)
	m5.Publickey = GetPub(pr5)
	m5.Coinbase = common.Address{0}
	m5.MType = types.TypeBack
	m5.Flag = types.StateUnusedFlag
	c1.BackMembers = append(c1.BackMembers, m5)

	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1
	c1.EndHeight = big.NewInt(11111)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28892, Port2: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28894, Port2: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28896, Port2: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28898, Port2: 28899, Coinbase: m5.Coinbase, Publickey: crypto.FromECDSAPub(m5.Publickey)})

	n2.Start()
	n2.PutCommittee(c1)
	n2.Notify(c1.Id, Start)
	n2.PutNodes(common.Big1, cn)

	//for {
	time.Sleep(time.Second * 220)
	c1.Members[3].Flag = types.StateRemovedFlag
	c1.Members[3].MType = types.TypeWorked
	c1.BackMembers[0].Flag = types.StateUsedFlag
	c1.StartHeight = getIDForCache("Agent2")
	c1.EndHeight = new(big.Int).Add(c1.StartHeight, big.NewInt(20))
	n2.UpdateCommittee(c1)
	//}
	<-start
}

func TestRunPbftChange3(t *testing.T) {
	log.OpenLogDebug(3)
	IDCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)
	pr5 := getPrivateKey(4)

	agent3 := NewPbftAgent("Agent3")

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

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m1.MType = types.TypeWorked
	m1.Flag = types.StateUsedFlag
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m2.MType = types.TypeWorked
	m2.Flag = types.StateUsedFlag
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m3.MType = types.TypeWorked
	m3.Flag = types.StateUsedFlag
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	m4.MType = types.TypeWorked
	m4.Flag = types.StateUsedFlag

	m5 := new(types.CommitteeMember)
	m5.Publickey = GetPub(pr5)
	m5.Coinbase = common.Address{0}
	m5.MType = types.TypeBack
	m5.Flag = types.StateUnusedFlag
	c1.BackMembers = append(c1.BackMembers, m5)

	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1
	c1.EndHeight = big.NewInt(11111)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28892, Port2: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28894, Port2: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28896, Port2: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28898, Port2: 28899, Coinbase: m5.Coinbase, Publickey: crypto.FromECDSAPub(m5.Publickey)})

	n3.Start()
	n3.PutCommittee(c1)
	n3.Notify(c1.Id, Start)
	n3.PutNodes(common.Big1, cn)

	//for {
	time.Sleep(time.Second * 220)
	c1.Members[3].Flag = types.StateRemovedFlag
	c1.Members[3].MType = types.TypeWorked
	c1.BackMembers[0].Flag = types.StateUsedFlag
	c1.StartHeight = getIDForCache("Agent3")
	c1.EndHeight = new(big.Int).Add(c1.StartHeight, big.NewInt(20))
	n3.UpdateCommittee(c1)
	//}
	<-start
}

func TestRunPbftChange4(t *testing.T) {
	log.OpenLogDebug(3)
	IDCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)
	pr5 := getPrivateKey(4)

	agent4 := NewPbftAgent("Agent4")

	config4 := new(config.TbftConfig)
	*config4 = *config.DefaultConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config4.P2P
	p2p4.ListenAddress1 = "tcp://127.0.0.1:28896"
	p2p4.ListenAddress2 = "tcp://127.0.0.1:28897"
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
	m1.MType = types.TypeWorked
	m1.Flag = types.StateUsedFlag
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m2.MType = types.TypeWorked
	m2.Flag = types.StateUsedFlag
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m3.MType = types.TypeWorked
	m3.Flag = types.StateUsedFlag
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	m4.MType = types.TypeWorked
	m4.Flag = types.StateUsedFlag

	m5 := new(types.CommitteeMember)
	m5.Publickey = GetPub(pr5)
	m5.Coinbase = common.Address{0}
	m5.MType = types.TypeBack
	m5.Flag = types.StateUnusedFlag
	c1.BackMembers = append(c1.BackMembers, m5)

	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1
	c1.EndHeight = big.NewInt(11111)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28892, Port2: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28894, Port2: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28896, Port2: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28898, Port2: 28899, Coinbase: m5.Coinbase, Publickey: crypto.FromECDSAPub(m5.Publickey)})

	n4.Start()
	n4.PutCommittee(c1)
	n4.Notify(c1.Id, Start)
	n4.PutNodes(common.Big1, cn)

	//for {
	time.Sleep(time.Second * 220)
	c1.Members[3].Flag = types.StateRemovedFlag
	c1.Members[3].MType = types.TypeWorked
	c1.BackMembers[0].Flag = types.StateUsedFlag
	c1.StartHeight = getIDForCache("Agent3")
	c1.EndHeight = new(big.Int).Add(c1.StartHeight, big.NewInt(20))
	n4.UpdateCommittee(c1)
	//}
	<-start
}

var Tbft5Start = big.NewInt(9)

func TestRunPbftChange5(t *testing.T) {
	log.OpenLogDebug(3)
	IDCacheInit()
	start := make(chan int)
	pr1 := getPrivateKey(0)
	pr2 := getPrivateKey(1)
	pr3 := getPrivateKey(2)
	pr4 := getPrivateKey(3)
	pr5 := getPrivateKey(4)

	agent5 := NewPbftAgent("Agent5")

	config5 := new(config.TbftConfig)
	*config5 = *config.DefaultConfig()
	p2p4 := new(config.P2PConfig)
	*p2p4 = *config5.P2P
	p2p4.ListenAddress1 = "tcp://127.0.0.1:28898"
	p2p4.ListenAddress2 = "tcp://127.0.0.1:28899"
	*config5.P2P = *p2p4

	con4 := new(config.ConsensusConfig)
	*con4 = *config5.Consensus
	con4.WalPath = filepath.Join("data", "cs.wal4", "wal")
	*config5.Consensus = *con4

	n4, _ := NewNode(config5, "1", pr5, agent5)

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	m1 := new(types.CommitteeMember)
	m1.Publickey = GetPub(pr1)
	m1.Coinbase = common.Address{0}
	m1.MType = types.TypeWorked
	m1.Flag = types.StateUsedFlag
	m2 := new(types.CommitteeMember)
	m2.Publickey = GetPub(pr2)
	m2.Coinbase = common.Address{0}
	m2.MType = types.TypeWorked
	m2.Flag = types.StateUsedFlag
	m3 := new(types.CommitteeMember)
	m3.Publickey = GetPub(pr3)
	m3.Coinbase = common.Address{0}
	m3.MType = types.TypeWorked
	m3.Flag = types.StateUsedFlag
	m4 := new(types.CommitteeMember)
	m4.Publickey = GetPub(pr4)
	m4.Coinbase = common.Address{0}
	m4.MType = types.TypeWorked
	m4.Flag = types.StateUsedFlag

	m5 := new(types.CommitteeMember)
	m5.Publickey = GetPub(pr5)
	m5.Coinbase = common.Address{0}
	m5.MType = types.TypeBack
	m5.Flag = types.StateUnusedFlag
	c1.BackMembers = append(c1.BackMembers, m5)

	c1.Members = append(c1.Members, m1, m2, m3, m4)
	c1.StartHeight = common.Big1
	c1.EndHeight = big.NewInt(11111)

	cn := make([]*types.CommitteeNode, 0)
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28890, Port2: 28891, Coinbase: m1.Coinbase, Publickey: crypto.FromECDSAPub(m1.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28892, Port2: 28893, Coinbase: m2.Coinbase, Publickey: crypto.FromECDSAPub(m2.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28894, Port2: 28895, Coinbase: m3.Coinbase, Publickey: crypto.FromECDSAPub(m3.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28896, Port2: 28897, Coinbase: m4.Coinbase, Publickey: crypto.FromECDSAPub(m4.Publickey)})
	cn = append(cn, &types.CommitteeNode{IP: "127.0.0.1", Port: 28898, Port2: 28899, Coinbase: m5.Coinbase, Publickey: crypto.FromECDSAPub(m5.Publickey)})

	n4.Start()
	n4.PutCommittee(c1)
	n4.Notify(c1.Id, Start)
	n4.PutNodes(common.Big1, cn)

	//for {
	c1.Members[3].Flag = types.StateRemovedFlag
	c1.Members[3].MType = types.TypeWorked
	c1.BackMembers[0].Flag = types.StateUsedFlag
	c1.StartHeight = Tbft5Start
	c1.EndHeight = new(big.Int).Add(c1.StartHeight, big.NewInt(20))
	n4.UpdateCommittee(c1)
	//}
	<-start
}
