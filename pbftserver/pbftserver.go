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


type addressInfo struct {
	IP 		string 
	Port 	uint
	Pk		*ecdsa.PublicKey
}
type serverInfo struct {
	leader 		*ecdsa.PublicKey
	nodeid 		string
	info 		[]*addressInfo
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
func (ss *serverInfo) insert(addr *addressInfo) {
	for _,v := range ss.info {
		if !bytes.Equal(crypto.FromECDSAPub(v.Pk), crypto.FromECDSAPub(addr.Pk)) {
			ss.info = append(ss.info,addr)
		}
	}
}
// initServer: 
func (ss *PbftServerMgr) initServer(id *big.Int, nodeid,leader *ecdsa.PublicKey, 
	infos []*addressInfo,start bool) {
	if id == nil {
		return 
	}
	if server,ok := ss.servers[id]; ok {
		server.leader = leader
		server.nodeid = common.ToHex(crypto.FromECDSAPub(nodeid))
		for _,v:=range infos {
			server.insert(v)
		}
	} else {
		ser := &serverInfo{
			leader:		leader,
			nodeid:		common.ToHex(crypto.FromECDSAPub(nodeid)),
			info:		infos,
			Height:		new(big.Int).Set(common.Big0),
			clear:		false,
		}
		ser.server = network.NewServer(ser.nodeid,id,ss)
		ss.servers[id] = ser
	}
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

func (ss *PbftServerMgr) actions(id *big.Int,ac int) error {
	switch ac {
	case Start:
		if server,ok := ss.servers[id]; ok {
			// set ip:port list and leader and viewID 
			server.server.Start()
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
		return nil
	}
	return errors.New("wrong action Num:"+strconv.Itoa(ac))
}

func (ss *PbftServerMgr) GetRequest(id *big.Int) (*consensus.RequestMsg,error) {
	// get new fastblock
	server,ok := ss.servers[id]
	if !ok {
		return nil,errors.New("wrong conmmitt ID:"+id.String())
	} 
	if !bytes.Equal(crypto.FromECDSAPub(ss.pk),crypto.FromECDSAPub(server.leader)) {
		return nil,errors.New("node was not leader")
	}
	fb:= ss.Agent.FetchFastBlock()  
	data,err := rlp.EncodeToBytes(fb)
	if err != nil {
		return nil,err
	}
	msg := hex.EncodeToString(data)
	val := &consensus.RequestMsg{
		ClientID:		server.nodeid,
		Timestamp:		time.Now().Unix(),
		Operation:		msg,
	}
	return val,nil
}
func (ss *PbftServerMgr) CheckMsg(msg *consensus.RequestMsg) (bool) {
	var fb types.FastBlock 
	err := rlp.DecodeBytes(common.FromHex(msg.Operation),&fb)
	if err != nil {
		return false
	}
	// will be get ReceiptHash after verify by agent
	// update fastblock
	err = ss.Agent.VerifyFastBlock(&fb)
	if err != nil {
		return false
	}
	ss.blocks[fb.Number()] = &fb
	return true
}

func (ss *PbftServerMgr) ReplyResult(msg *consensus.RequestMsg,res uint) {
	var fb types.FastBlock 
	err := rlp.DecodeBytes(common.FromHex(msg.Operation),&fb)
	if err != nil {
		return false
	}
	hash := rlpHash([]interfase {
		fb.Hash(),
		fb.Number(),
		res,
	})
	sig,err2 := crypto.Sign(hash,priv)
	if err2 != nil {
		return false
	}
	sign := types.PbftSign{
		FastHeight:		fb.Number(),
		FastHash:		fb.Hash(),
		Result:			res,
		Sign:			sig,
	}
	err = ss.Agent.BroadcastSign(&sign,&fb)
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

func (ss *PbftServerMgr) work() {
	for {
		select {
		case ac := <-consensus.ActionChan:
			if ac.AC == consensus.ActionFecth {
				req,err := GetRequest(ac.ID)
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
