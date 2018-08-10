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
	servers 		map[*big.Int]*serverInfo
	blocks			map[*big.Int]*types.FastBlock
	pk 				*ecdsa.PublicKey
	priv 			*ecdsa.PrivateKey
	Agent			types.PbftAgentProxy
}

func NewPbftServerMgr(pk *ecdsa.PublicKey,priv *ecdsa.PrivateKey) *PbftServerMgr {
	ss := &PbftServerMgr {
		servers : 	make(map[*big.Int]*serverInfo),
		blocks:		make(map[*big.Int]*types.FastBlock),
		pk:			pk,
		priv:		priv,
	}
	return ss
}
// note: all functions below this was not thread-safe

func (ss *serverInfo) insertMember(mm *types.CommitteeMember) {
	for _,v := range ss.info {
		if !bytes.Equal(crypto.FromECDSAPub(v.CM.Publickey), crypto.FromECDSAPub(mm.Publickey)) {
			ss.info = append(ss.info,&types.CommitteeNode {
				CM:			mm,
			})
		}
	}
}
func (ss *serverInfo) insertNode(n *types.CommitteeNode) {
	for i,v := range ss.info {
		if bytes.Equal(crypto.FromECDSAPub(v.CM.Publickey), crypto.FromECDSAPub(n.CM.Publickey)) {
			ss.info[i] = n
		}
	}
}
func (ss *PbftServerMgr) getLastBlock() *types.FastBlock {
	cur := big.NewInt(0)
	var fb *types.FastBlock = nil
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
	delete(ss.blocks,height)
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
		if _,ok := ss.servers[id]; ok {
			delete(ss.servers,id)
		}
	}
}

func (ss *PbftServerMgr) GetRequest(id *big.Int) (*consensus.RequestMsg,error) {
	// get new fastblock
	server,ok := ss.servers[id]
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
	if _,ok := ss.blocks[fb.Number()]; ok {
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
	ss.blocks[fb.Number()] = fb
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
	block,ok := ss.blocks[height]
	if !ok {
		return false
	}
	err := ss.Agent.VerifyFastBlock(block)
	if err != nil {
		return false
	}
	ss.blocks[height] = block
	return true
}
func (ss *PbftServerMgr) ReplyResult(msg *consensus.RequestMsg,res uint) bool {
	height := big.NewInt(msg.Height)
	block,ok := ss.blocks[height]
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
	if v,ok := ss.blocks[height]; ok {
		ss.Agent.BroadcastFastBlock(v)
	}
}
func (ss *PbftServerMgr) SignMsg(h int64,res uint) (*consensus.SignedVoteMsg) {
	height := big.NewInt(h)
	block,ok := ss.blocks[height]
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

func (ss *PbftServerMgr) work() {
	for {
		select {
		case ac := <-consensus.ActionChan:
			if ac.AC == consensus.ActionFecth {
				req,err := ss.GetRequest(ac.ID)
				if err != nil  && req != nil {
					if server,ok := ss.servers[ac.ID];ok {
						server.server.PutRequest(req)
					} 
				}
			} else if ac.AC == consensus.ActionBroadcast {
				ss.Broadcast(ac.Height)
			} 
		}
	}
}

func (ss *PbftServerMgr)PutCommittee(id *big.Int, members []*types.CommitteeMember) error {
	if id == nil || len(members) <= 0 {
		return errors.New("wrong params...")
	} 
	if _,ok := ss.servers[id];ok {
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
	ss.servers[id] = &server
	return nil
}
func (ss *PbftServerMgr)PutNodes(id *big.Int, nodes []*types.CommitteeNode) error{
	if id == nil || len(nodes) <= 0 {
		return errors.New("wrong params...")
	} 
	server,ok := ss.servers[id]
	if !ok {
		return errors.New("wrong ID:"+id.String())
	}
	for _,v := range nodes {
		server.insertNode(v)
	}
	server.server = network.NewServer(server.nodeid,id,ss,ss)
	return nil
}
func (ss *PbftServerMgr)Notify(id *big.Int, action int) error {
	switch action {
	case Start:
		if server,ok := ss.servers[id]; ok {
			server.server.Start()
			// start to fetch
			ac := &consensus.ActionIn{
				AC:		consensus.ActionFecth,
				ID:		id,
				Height:	common.Big0,
			}
			consensus.ActionChan <- ac
			return nil
		} 
		return errors.New("wrong conmmitt ID:"+id.String())
	case Stop:
		if server,ok := ss.servers[id]; ok {
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
