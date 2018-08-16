package pbftserver

import (
	"strconv"
	"encoding/hex"
	"time"
	"bytes"
	"math/big"
	"errors"
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/pbftserver/network"
	"github.com/truechain/truechain-engineering-code/pbftserver/consensus"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
)

const (
	Start int = iota
	Stop
	Switch
)

type serverInfo struct {
	leader 		*ecdsa.PublicKey
	nodeid 		string
	info 		[]*types.CommitteeNode
	server		*network.Server
	Height		*big.Int
	clear		bool 
}

type PbftServerMgr struct {
	servers 		map[uint64]*serverInfo
	blocks			map[uint64]*types.Block
	pk 				*ecdsa.PublicKey
	priv 			*ecdsa.PrivateKey
	Agent			types.PbftAgentProxy
}

func NewPbftServerMgr(pk *ecdsa.PublicKey,priv *ecdsa.PrivateKey,agent types.PbftAgentProxy) *PbftServerMgr {
	ss := &PbftServerMgr {
		servers : 	make(map[uint64]*serverInfo),
		blocks:		make(map[uint64]*types.Block),
		pk:			pk,
		priv:		priv,
		Agent:		agent,
	}
	return ss
}
func (ss *PbftServerMgr) Finish() error{
	// sleep 1s
	for _,v := range ss.servers {
		v.server.Stop()
	}
	return nil
}
// note: all functions below this was not thread-safe

func (ss *serverInfo) insertMember(mm *types.CommitteeMember) {
	var update bool = false
	for _,v := range ss.info {
		if !bytes.Equal(v.Publickey, crypto.FromECDSAPub(mm.Publickey)) {
			ss.info = append(ss.info,&types.CommitteeNode {
				Coinbase:mm.Coinbase,
				Publickey:crypto.FromECDSAPub(mm.Publickey),
			})
			update = true
			break
		}
	}
	if !update {
		ss.info = append(ss.info,&types.CommitteeNode {
			Coinbase:mm.Coinbase,
			Publickey:crypto.FromECDSAPub(mm.Publickey),
		})
	}
}
func (ss *serverInfo) insertNode(n *types.CommitteeNode) {
	var update bool = false
	for i,v := range ss.info {
		if bytes.Equal(v.Publickey, n.Publickey) {
			ss.info[i] = n
			update = true
			break
		}
	}
	if !update {
		ss.info = append(ss.info,n)
	}
}
func (ss *PbftServerMgr) getLastBlock() *types.Block {
	cur := big.NewInt(0)
	var fb *types.Block = nil
	for _,v := range ss.blocks {
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
	delete(ss.blocks,height.Uint64())
}
func (ss *PbftServerMgr) clear(id *big.Int) {
	if id.Cmp(common.Big0) == 0{
		for k,v := range ss.servers  {
			if v.clear {
				delete(ss.servers,k)
				break
			}
		}
	} else {
		if _,ok := ss.servers[id.Uint64()]; ok {
			delete(ss.servers,id.Uint64())
		}
	}
}

func (ss *PbftServerMgr) GetRequest(id *big.Int) (*consensus.RequestMsg,error) {
	// get new fastblock
	server,ok := ss.servers[id.Uint64()]
	if !ok {
		return nil,errors.New("wrong conmmitt ID:"+id.String())
	} 
	// the node must be leader 
	if !bytes.Equal(crypto.FromECDSAPub(server.leader), crypto.FromECDSAPub(ss.pk)) {
		return nil,errors.New("local node must be leader...")	
	}
	fb,err := ss.Agent.FetchFastBlock()  
	if err != nil {
		return nil,err
	}
	if _,ok := ss.blocks[fb.NumberU64()]; ok {
		return nil,errors.New("same height:"+fb.Number().String())
	}
	sum := len(ss.blocks)
	if sum > 0 {
		last := ss.getLastBlock()
		if last != nil {
			cur := last.Number()
			cur.Add(cur,common.Big1)
			if cur.Cmp(fb.Number()) != 0 {
				return nil,errors.New("wrong fastblock,lastheight:"+cur.String()+" cur:"+fb.Number().String())
			}
		}
	}
	ss.blocks[fb.NumberU64()] = fb
	data,err := rlp.EncodeToBytes(fb)
	if err != nil {
		return nil,err
	}
	msg := hex.EncodeToString(data)
	val := &consensus.RequestMsg{
		ClientID:		server.nodeid,
		Timestamp:		time.Now().Unix(),
		Operation:		msg,
		Height:			fb.Number().Int64(),
	}
	return val,nil
}
func (ss *PbftServerMgr) CheckMsg(msg *consensus.RequestMsg) (bool) {
	height := big.NewInt(msg.Height)
	block,ok := ss.blocks[height.Uint64()]
	if !ok {
		return false
	}
	err := ss.Agent.VerifyFastBlock(block)
	if err != nil {
		return false
	}
	ss.blocks[height.Uint64()] = block
	return true
}
func (ss *PbftServerMgr) ReplyResult(msg *consensus.RequestMsg,res uint) bool {
	height := big.NewInt(msg.Height)
	block,ok := ss.blocks[height.Uint64()]
	if !ok {
		return false
	}
	hash := rlpHash([]interface{} {
		block.Hash(),
		block.Number(),
		res,
	})
	sig,err := crypto.Sign(hash[:],ss.priv)
	if err != nil {
		return false
	}
	sign := types.PbftSign{
		FastHeight:		block.Number(),
		FastHash:		block.Hash(),
		Result:			res,
		Sign:			sig,
	}
	err = ss.Agent.BroadcastSign(&sign,block)
	ss.removeBlock(height)
	if err != nil {
		return false
	}
	return true
}
func (ss *PbftServerMgr) Broadcast(height *big.Int) {
	if v,ok := ss.blocks[height.Uint64()]; ok {
		ss.Agent.BroadcastFastBlock(v)
	}
}
func (ss *PbftServerMgr) SignMsg(h int64,res uint) (*consensus.SignedVoteMsg) {
	height := big.NewInt(h)
	block,ok := ss.blocks[height.Uint64()]
	if !ok {
		return nil
	}
	hash := rlpHash([]interface{} {
		block.Hash(),
		height,
		res,
		ss.pk,
	})
	sig,err := crypto.Sign(hash[:],ss.priv)
	if err != nil {
		return nil
	}
	sign := &consensus.SignedVoteMsg{
		FastHeight:		height,
		Result:			res,
		Sign:			sig,
	}
	return sign
}

func (ss *PbftServerMgr) work(acChan <-chan *consensus.ActionIn) {
	for {
		select {
		case ac := <-acChan:
			if ac.AC == consensus.ActionFecth {
				req,err := ss.GetRequest(ac.ID)
				if err == nil  && req != nil {
					if server,ok := ss.servers[ac.ID.Uint64()];ok {
						server.Height = big.NewInt(req.Height)
						server.server.PutRequest(req)
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

func (ss *PbftServerMgr)PutCommittee(committeeInfo *types.CommitteeInfo) error {
	id := committeeInfo.Id
	members := committeeInfo.Members
	if id == nil || len(members) <= 0 {
		return errors.New("wrong params...")
	} 
	if _,ok := ss.servers[id.Uint64()];ok {
		return errors.New("repeat ID:"+id.String())
	}
	leader := members[0].Publickey
	infos := make([]*types.CommitteeNode,0)	
	server := serverInfo{
		leader:		leader,
		nodeid:		common.ToHex(crypto.FromECDSAPub(ss.pk)),
		info:		infos,
		Height:		new(big.Int).Set(common.Big0),
		clear:		false,
	}
	for _,v := range members {
		server.insertMember(v)
	}
	ss.servers[id.Uint64()] = &server
	return nil
}
func (ss *PbftServerMgr)PutNodes(id *big.Int, nodes []*types.CommitteeNode) error{
	if id == nil || len(nodes) <= 0 {
		return errors.New("wrong params...")
	} 
	server,ok := ss.servers[id.Uint64()]
	if !ok {
		return errors.New("wrong ID:"+id.String())
	}
	for _,v := range nodes {
		server.insertNode(v)
	}
	server.server = network.NewServer(server.nodeid,id,ss,ss,server.info)
	return nil
}
func (ss *PbftServerMgr)Notify(id *big.Int, action int) error {
	switch action {
	case Start:
		if server,ok := ss.servers[id.Uint64()]; ok {
			server.server.Start(ss.work)
			// start to fetch
			ac := &consensus.ActionIn{
				AC:		consensus.ActionFecth,
				ID:		id,
				Height:	common.Big0,
			}
			server.server.ActionChan <- ac
			return nil
		} 
		return errors.New("wrong conmmitt ID:"+id.String())
	case Stop:
		if server,ok := ss.servers[id.Uint64()]; ok {
			server.clear = true
		}
		ss.clear(id)
		return nil
	case Switch:
		// begin to make network..
		return nil
	}
	return errors.New("wrong action Num:"+strconv.Itoa(action))
}
func test() {
	// nodeID := os.Args[1]
	// server := network.NewServer(nodeID)
	// server.Start()
}
func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
