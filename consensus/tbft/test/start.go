package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	tbftPkg "github.com/truechain/truechain-engineering-code/consensus/tbft"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/config"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"io/ioutil"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

type PbftAgentProxyImp struct {
	Name string
}

func NewPbftAgent(name string) *PbftAgentProxyImp {
	pap := PbftAgentProxyImp{Name: name}
	return &pap
}

var ID = big.NewInt(0)

func getID() *big.Int {
	ID = new(big.Int).Add(ID, big.NewInt(1))
	return ID
}

func (pap *PbftAgentProxyImp) FetchFastBlock() (*types.Block, error) {
	header := new(types.Header)
	header.Number = getID()
	println("[AGENT]", "++++++++", "FetchFastBlock", "Number:", header.Number.Uint64())
	return types.NewBlock(header, nil, nil, nil), nil
}
func (pap *PbftAgentProxyImp) VerifyFastBlock(block *types.Block) error {
	//if rand.Intn(100) > 30 {
	//	return types.ErrHeightNotYet
	//}
	println("[AGENT]", pap.Name, "VerifyFastBlock", "Number:", block.Header().Number.Uint64())
	return nil
}

func (pap *PbftAgentProxyImp) BroadcastFastBlock(block *types.Block) error {
	println("[AGENT]", pap.Name, "BroadcastFastBlock", "Number:", block.Header().Number.Uint64())
	return nil
}

func (pap *PbftAgentProxyImp) BroadcastSign(sign *types.PbftSign, block *types.Block) error {
	println("[AGENT]", "--------", "BroadcastSign", "Number:", block.Header().Number.Uint64())
	return nil
}
func (pap *PbftAgentProxyImp) BroadcastConsensus(block *types.Block) error {
	println("[AGENT]", "--------", "BroadcastSign", "Number:", block.Header().Number.Uint64())
	return nil
}

func GetPub(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := ecdsa.PublicKey{
		Curve: priv.Curve,
		X:     new(big.Int).Set(priv.X),
		Y:     new(big.Int).Set(priv.Y),
	}
	return &pub
}

type savePri struct {
	D *big.Int
	X *big.Int
	Y *big.Int
}

type myMsp struct {
	Sps   []savePri
	Index int
}

func LoadConfig() *config.Config {

	var conf config.Config
	data, err := ioutil.ReadFile("E:/truechain/src/github.com/truechain/truechain-engineering-code/consensus/tbft/test/config.json")
	if err != nil {
		panic("config load fail: " + err.Error())
	}

	datajson := []byte(data)

	err = json.Unmarshal(datajson, &conf)
	if err != nil {
		panic("config load fail: " + err.Error())
	}

	return &conf
}

func LoadMspConfig() *myMsp {
	fmt.Println(os.Args)
	var conf myMsp
	data, err := ioutil.ReadFile("E:/truechain/src/github.com/truechain/truechain-engineering-code/consensus/tbft/test/msp.json")
	if err != nil {
		panic("config load fail: " + err.Error())
	}

	datajson := []byte(data)

	err = json.Unmarshal(datajson, &conf)
	if err != nil {
		panic("config load fail: " + err.Error())
	}

	return &conf
}

func keyChange(sps []savePri) []*ecdsa.PrivateKey {
	var pvs []*ecdsa.PrivateKey
	for i := 0; i < len(sps); i++ {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		key.Y = sps[i].Y
		key.X = sps[i].X
		key.D = sps[i].D
		pvs = append(pvs, key)
	}
	return pvs
}

var iplist = []string{"127.0.0.1", "127.0.0.1", "127.0.0.1", "127.0.0.1"}
var portlist = []uint{1111, 2222, 3333, 4444}

func main() {
	//与true log日志结合
	handler, _ := log.FileHandler("./test.log", log.LogfmtFormat())
	log.Root().SetHandler(handler)

	mymsp := LoadMspConfig()
	keySet := keyChange(mymsp.Sps)

	agent1 := NewPbftAgent("Agent1")
	myConf := LoadConfig()

	n, _ := tbftPkg.NewNode(myConf, "1", keySet[mymsp.Index], agent1)
	n.Start()

	c1 := new(types.CommitteeInfo)
	c1.Id = big.NewInt(1)
	c1.StartHeight = c1.Id
	for i := 0; i < len(keySet); i++ {
		m1 := new(types.CommitteeMember)
		m1.Publickey = GetPub(keySet[i])
		c1.Members = append(c1.Members, m1)
	}

	selfPort := strings.Split(myConf.P2P.ListenAddress, ":")[2]
	port, _ := strconv.ParseUint(selfPort, 10, 64)
	var nodes []*types.CommitteeNode
	for i := 0; i < 4; i++ {
		n1 := new(types.CommitteeNode)
		n1.IP = iplist[i]
		n1.Port = portlist[i]
		n1.Publickey = elliptic.Marshal(crypto.S256(), keySet[i].X, keySet[i].Y)
		//这里没有找到Coinbase 与公钥直接的转换函数
		//n1.Coinbase
		if uint(port) == n1.Port {
			continue
		}
		nodes = append(nodes, n1)
	}

	n.PutCommittee(c1)
	n.Notify(c1.Id, tbftPkg.Start)
	n.PutNodes(c1.Id, nodes)

	for {
		time.Sleep(time.Hour)
	}
}
