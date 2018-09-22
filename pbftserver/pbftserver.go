package pbftserver

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"github.com/truechain/truechain-engineering-code/pbftserver/lock"
	"github.com/truechain/truechain-engineering-code/pbftserver/network"
	"github.com/truechain/truechain-engineering-code/rlp"
	"math/big"
	"sync"
	"time"
)

const (
	Start int = iota
	Stop
	Switch

	ServerWait    = 5
	BlockSleepMax = 5
)

type serverInfo struct {
	leader *ecdsa.PublicKey
	nodeid string
	info   []*types.CommitteeNode
	server *network.Server
	Height *big.Int
	clear  bool
	Stop   uint64
}

type PbftServerMgr struct {
	servers    map[uint64]*serverInfo
	blocks     map[uint64]*types.Block
	pk         *ecdsa.PublicKey
	priv       *ecdsa.PrivateKey
	Agent      types.PbftAgentProxy
	blockLock  sync.Mutex
	blockMax   uint64
	blockSleep time.Duration
}

func NewPbftServerMgr(pk *ecdsa.PublicKey, priv *ecdsa.PrivateKey, agent types.PbftAgentProxy) *PbftServerMgr {
	ss := &PbftServerMgr{
		servers: make(map[uint64]*serverInfo),
		blocks:  make(map[uint64]*types.Block),
		pk:      pk,
		priv:    priv,
		Agent:   agent,
	}
	return ss
}

func (ss *PbftServerMgr) Finish() error {
	// sleep 1s
	for _, v := range ss.servers {
		v.server.Stop()
	}
	return nil
}

// note: all functions below this was not thread-safe

func (ss *serverInfo) insertMember(mm *types.CommitteeMember) {
	var update bool = false
	for _, v := range ss.info {
		if !bytes.Equal(v.Publickey, crypto.FromECDSAPub(mm.Publickey)) {
			ss.info = append(ss.info, &types.CommitteeNode{
				Coinbase:  mm.Coinbase,
				Publickey: crypto.FromECDSAPub(mm.Publickey),
			})
			update = true
			break
		}
	}
	if !update {
		ss.info = append(ss.info, &types.CommitteeNode{
			Coinbase:  mm.Coinbase,
			Publickey: crypto.FromECDSAPub(mm.Publickey),
		})
	}
}
func (ss *serverInfo) insertNode(n *types.CommitteeNode) {
	var update bool = false
	for i, v := range ss.info {
		if bytes.Equal(v.Publickey, n.Publickey) {
			ss.info[i] = n
			update = true
			break
		}
	}
	if !update {
		ss.info = append(ss.info, n)
	}
}

func (ss *PbftServerMgr) getBlock(h uint64) *types.Block {
	ss.blockLock.Lock()
	defer ss.blockLock.Unlock()

	if fb, ok := ss.blocks[h]; ok {
		return fb
	}

	if (h - 500) >= 0 {
		delete(ss.blocks, h-500)
	}
	return nil
}

func (ss *PbftServerMgr) getBlockLen() int {
	ss.blockLock.Lock()
	defer ss.blockLock.Unlock()
	return len(ss.blocks)
}

func (ss *PbftServerMgr) putBlock(h uint64, block *types.Block) {
	ss.blockLock.Lock()
	defer ss.blockLock.Unlock()
	//TODO make size 1000
	if ss.blockMax < h {
		ss.blockMax = h
	}
	ss.blocks[h] = block
}

func (ss *PbftServerMgr) getLastBlock() *types.Block {
	ss.blockLock.Lock()
	defer ss.blockLock.Unlock()
	cur := big.NewInt(0)
	var fb *types.Block = nil
	for _, v := range ss.blocks {
		if cur.Cmp(common.Big0) == 0 {
			fb = v
		}
		if cur.Cmp(v.Number()) == -1 {
			cur.Set(v.Number())
			fb = v
		}
	}
	return fb
}
func (ss *PbftServerMgr) removeBlock(height *big.Int) {
	ss.blockLock.Lock()
	defer ss.blockLock.Unlock()
	delete(ss.blocks, height.Uint64())
}
func (ss *PbftServerMgr) clear(id *big.Int) {
	if id.Cmp(common.Big0) == 0 {
		for k, v := range ss.servers {
			if v.clear {
				delete(ss.servers, k)
				break
			}
		}
	} else {
		if _, ok := ss.servers[id.Uint64()]; ok {
			delete(ss.servers, id.Uint64())
		}
	}
}

func (ss *PbftServerMgr) GetRequest(id *big.Int) (*consensus.RequestMsg, error) {
	// get new fastblock
	server, ok := ss.servers[id.Uint64()]
	if !ok {
		return nil, errors.New("wrong conmmitt ID:" + id.String())
	}
	// the node must be leader
	if !bytes.Equal(crypto.FromECDSAPub(server.leader), crypto.FromECDSAPub(ss.pk)) {
		return nil, errors.New("local node must be leader...")
	}

	lock.PSLog("AGENT", "FetchFastBlock", "start")

	if ss.blockSleep != 0 {
		time.Sleep(ss.blockSleep * time.Second)
		lock.PSLog("FetchFastBlock wait", ss.blockSleep, "second")
	}

	if server.clear {
		return nil, errors.New("server stop")
	}

	fb, err := ss.Agent.FetchFastBlock(id)

	if err != nil {
		lock.PSLog("[pbft server]", " FetchFastBlock Error", err.Error())
		return nil, err
	}

	if fb == nil {
		err = errors.New("FetchFastBlock is nil")
		lock.PSLog("[pbft server]", " FetchFastBlock Error", err.Error())
		return nil, err
	}

	lock.PSLog("[pbft server]", " FetchFastBlock header", fb.Header().Time)

	if len(fb.Body().Transactions) == 0 {
		if ss.blockSleep < BlockSleepMax {
			ss.blockSleep += 1
		}
	} else {
		ss.blockSleep = 0
	}

	lock.PSLog("AGENT", "FetchFastBlock", err == nil, "end")
	if err != nil {
		return nil, err
	}

	if fb := ss.getBlock(fb.NumberU64()); fb != nil {
		return nil, errors.New("same height:" + fb.Number().String())
	}

	sum := ss.getBlockLen()

	if sum > 0 {
		last := ss.getLastBlock()
		if last != nil {
			cur := last.Number()
			cur.Add(cur, common.Big1)
			if cur.Cmp(fb.Number()) != 0 {
				return nil, errors.New("wrong fastblock,lastheight:" + cur.String() + " cur:" + fb.Number().String())
			}
		}
	}

	ss.putBlock(fb.NumberU64(), fb)
	data, err := rlp.EncodeToBytes(fb)
	if err != nil {
		return nil, err
	}
	msg := hex.EncodeToString(data)
	val := &consensus.RequestMsg{
		ClientID:  server.nodeid,
		Timestamp: time.Now().Unix(),
		Operation: msg,
		Height:    fb.Number().Int64(),
	}
	return val, nil
}

func (ss *PbftServerMgr) InsertBlock(msg *consensus.PrePrepareMsg) bool {
	data := msg.RequestMsg.Operation
	fbByte, err := hex.DecodeString(data)
	if err != nil {
		return false
	}

	var fb *types.Block = new(types.Block)
	err = rlp.DecodeBytes(fbByte, fb)
	if err != nil {
		return false
	}
	ss.putBlock(fb.NumberU64(), fb)
	return true
}

func (ss *PbftServerMgr) CheckMsg(msg *consensus.RequestMsg) error {
	height := big.NewInt(msg.Height)

	block := ss.getBlock(height.Uint64())
	if block == nil {
		return errors.New("block not have")
	}
	lock.PSLog("AGENT", "VerifyFastBlock", "start")
	err := ss.Agent.VerifyFastBlock(block)
	lock.PSLog("AGENT", "VerifyFastBlock", err == nil, "end")
	if err != nil {
		lock.PSLog("AGENT", "VerifyFastBlock err", err.Error())
		return err
	}
	ss.putBlock(height.Uint64(), block)
	return nil
}

func (ss *PbftServerMgr) ReplyResult(msg *consensus.RequestMsg, res uint) bool {
	height := big.NewInt(msg.Height)

	block := ss.getBlock(height.Uint64())
	if block == nil {
		return false
	}
	lock.PSLog("[Agent]", "BroadcastConsensus", "start")
	err := ss.Agent.BroadcastConsensus(block)
	lock.PSLog("[Agent]", "BroadcastConsensus", err == nil, "end")
	//ss.removeBlock(height)
	if err != nil {
		return false
	}
	return true
}

func (ss *PbftServerMgr) Broadcast(height *big.Int) {
	if fb := ss.getBlock(height.Uint64()); fb != nil {
		lock.PSLog("[Agent]", "BroadcastFastBlock", "start")
		ss.Agent.BroadcastFastBlock(fb)
		lock.PSLog("[Agent]", "BroadcastFastBlock", "end")
	}
}
func (ss *PbftServerMgr) SignMsg(h int64, res uint) *consensus.SignedVoteMsg {
	height := big.NewInt(h)

	block := ss.getBlock(height.Uint64())
	if block == nil {
		return nil
	}

	hash := rlpHash([]interface{}{
		block.Hash(),
		height,
		res,
		ss.pk,
	})
	sig, err := crypto.Sign(hash[:], ss.priv)
	if err != nil {
		return nil
	}
	sign := &consensus.SignedVoteMsg{
		FastHeight: height,
		Result:     res,
		Sign:       sig,
	}
	return sign
}

func (ss *PbftServerMgr) work(cid *big.Int, acChan <-chan *consensus.ActionIn) {
	for {
		select {
		case ac := <-acChan:
			if ac.AC == consensus.ActionFecth {
				if server, ok := ss.servers[cid.Uint64()]; ok {
					if server.Height.Uint64() == server.Stop && server.Stop != 0 {
						lock.PSLog("ActionFecth", "Stop", "height stop")
						continue
					}
					if !server.clear {
						req, err := ss.GetRequest(cid)
						if err == nil && req != nil {
							if server, ok := ss.servers[cid.Uint64()]; ok {
								server.Height = big.NewInt(req.Height)
								server.server.PutRequest(req)
							} else {
								lock.PSLog(err.Error())
							}
						}
					}
				}
			} else if ac.AC == consensus.ActionBroadcast {
				ss.Broadcast(ac.Height)
			} else if ac.AC == consensus.ActionFinish {
				return
			}
		}
	}
}

func (ss *PbftServerMgr) SetCommitteeStop(committeeId *big.Int, stop uint64) error {
	if server, ok := ss.servers[committeeId.Uint64()]; ok {
		server.Stop = stop
		return nil
	}
	return errors.New("SetCommitteeStop Server is not have")
}

func (ss *PbftServerMgr) PutCommittee(committeeInfo *types.CommitteeInfo) error {
	lock.PSLog("PutCommittee", committeeInfo.Id, committeeInfo.Members)
	id := committeeInfo.Id
	members := committeeInfo.Members
	if id == nil || len(members) <= 0 {
		return errors.New("wrong params...")
	}
	if _, ok := ss.servers[id.Uint64()]; ok {
		return errors.New("repeat ID:" + id.String())
	}
	leader := members[0].Publickey
	infos := make([]*types.CommitteeNode, 0)
	server := serverInfo{
		leader: leader,
		nodeid: common.ToHex(crypto.FromECDSAPub(ss.pk)),
		info:   infos,
		Height: new(big.Int).Set(common.Big0),
		clear:  false,
		Stop:   0,
	}
	for _, v := range members {
		server.insertMember(v)
	}
	ss.servers[id.Uint64()] = &server
	return nil
}
func (ss *PbftServerMgr) PutNodes(id *big.Int, nodes []*types.CommitteeNode) error {
	if nodes[0] != nil {
		lock.PSLog("PutNodes", nodes[0].Port, nodes[0].IP, "committee id", id.Int64())
	} else {
		lock.PSLog("PutNodes nodes error")
	}
	if id == nil || len(nodes) <= 0 {
		return errors.New("wrong params...")
	}
	server, ok := ss.servers[id.Uint64()]
	if !ok {
		return errors.New("wrong ID:" + id.String())
	}
	for _, v := range nodes {
		server.insertNode(v)
	}

	if server.server == nil {
		server.server = network.NewServer(server.nodeid, id, ss, ss, server.info)
	} else {
		//update node table
		for _, v := range server.info {
			name := common.ToHex(v.Publickey)
			server.server.Node.NodeTable[name] = fmt.Sprintf("%s:%d", v.IP, v.Port)
		}
	}

	lock.PSLog("PutNodes update", fmt.Sprintf("%+v", server.server.Node.NodeTable))
	return nil
}

func serverCheck(server *serverInfo) (bool, int) {
	serverCompleteCnt := 0
	for _, v := range server.info {
		if v.IP != "" && v.Port != 0 {
			serverCompleteCnt += 1
		}
	}
	if serverCompleteCnt < 3 {
		return false, serverCompleteCnt
	}
	var successPre float64 = float64(serverCompleteCnt) / float64(len(server.info))
	return successPre > (float64(2) / float64(3)), serverCompleteCnt
}

func (ss *PbftServerMgr) runServer(server *serverInfo, id *big.Int) {
	if bytes.Equal(crypto.FromECDSAPub(server.leader), crypto.FromECDSAPub(ss.pk)) {
		for {
			b, c := serverCheck(server)
			lock.PSLog("[leader]", "server count", c)
			if b {
				time.Sleep(time.Second * ServerWait * 6)
				break
			}
			time.Sleep(time.Second)
		}
	}
	if id.Int64() > 0 {
		lock.PSLog("[switch]", "leader wait ", 5)
		time.Sleep(time.Second * ServerWait)
	}

	server.server.Start(ss.work)
	// start to fetch
	ac := &consensus.ActionIn{
		AC:     consensus.ActionFecth,
		ID:     id,
		Height: common.Big0,
	}
	server.server.ActionChan <- ac
}

func DelayStop(id uint64, ss *PbftServerMgr) {
	if server, ok := ss.servers[id]; ok {
		server.server.Node.Stop = true
	}
	lock.PSLog("[switch]", "stop wait ", 60)
	time.Sleep(time.Second * ServerWait)

	if server, ok := ss.servers[id]; ok {
		lock.PSLog("http server stop", "id", id)
		server.server.Stop()
		server.clear = true
	}
	ss.clear(big.NewInt(int64(id)))
}

func (ss *PbftServerMgr) Notify(id *big.Int, action int) error {
	lock.PSLog("Notify", id, action)
	switch action {
	case Start:
		if server, ok := ss.servers[id.Uint64()]; ok {
			go ss.runServer(server, id)
		} else {
			return errors.New("wrong conmmitt ID:" + id.String())
		}
	case Stop:
		DelayStop(id.Uint64(), ss)
		return nil
	case Switch:
		// begin to make network..
		return nil
	}
	return nil
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
