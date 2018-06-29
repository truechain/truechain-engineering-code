package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/eth/truechain"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/log"
)

type PbftPool interface {
	AddRemotes(block *truechain.TruePbftBlock) []error
	Pending()(map[common.Address]truechain.TruePbftBlock,error)
	SubscribeNewPbftsEvent(chan<- []*truechain.TruePbftBlock) event.Subscription
}

type NewPbftsEvent struct {Pbfts []*truechain.TruePbftBlock }

type newBftBlockData struct {
	Block *truechain.TruePbftBlock
}

//func (b *truechain.TruePbftBlock) hash() atomic.Value {
//	return atomic.Value{}
//}
//
//func prlpHash(x interface{}) (h common.Hash) {
//	hw := sha3.NewKeccak256()
//	rlp.Encode(hw, x)
//	hw.Sum(h[:0])
//	return h
//}
//
func (ps *peerSet)PeersWithoutPbftBlock (hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	list := make([]*peer,0,len(ps.peers))
	for _,p := range ps.peers {
		if !p.knownBftBlocks.Has(hash) {
			list = append(list,p)
		}
	}
	return list
}
//
//func (h truechain.TruePbftBlockHeader) Hash() common.Hash {
//	return prlpHash(h)
//}
//func (h CMS) Hash() common.Hash {
//	return prlpHash(h)
//}
//func (h SBS) Hash() common.Hash {
//	return prlpHash(h)
//}
//
//type CMS []*truechain.CommitteeMember
//type SBS []*truechain.CommitteeMember

//bpft
type propBftEvent struct {
	block *truechain.TruePbftBlock
}
//type propCMSEvnet struct {
//	cms []*truechain.CommitteeMember
//}
//
//type propSBSEvent struct {
//	oms []*truechain.CommitteeMember
//}

func (p *peer) SendNewBftBlock(b *truechain.TruePbftBlock) error {
	//p.knownBftBlocks.Add(b.Hash())
	return p2p.Send(p.rw, NewBftBlockMsg, []interface{}{b})
}

func (p *peer) AsyncSendNewBftBlocks(blocks []*truechain.TruePbftBlock) {
	for _,b := range blocks {
		s := make([]*truechain.TruePbftBlock,1)
		s=append(s,b)
		select {
		case p.queuedBftProps <- s:
			p.knownBftBlocks.Add(b.Hash())
		default:
			p.Log().Debug("Dropping block propagation", "block", b)
		}
	}
}

////cms
//
//func (p *peer) SendCMS(cms CMS) error {
//	p.knownCurCmms.Add(cms.Hash())
//	return p2p.Send(p.rw, NewCMSMsg, []interface{}{cms})
//}
//
//func (p *peer) AsyncCMS(cms CMS) {
//	select {
//	case p.queuedCms <- &propCMSEvnet{cms: cms}:
//		p.knownCurCmms.Add(cms.Hash())
//	default:
//		p.Log().Debug("Dropping cms propagation", "hash", cms.Hash())
//	}
//}
//
////oms
//func (p *peer) SendSBS(sbs SBS) error {
//	p.knownCurCmms.Add(sbs.Hash())
//	return p2p.Send(p.rw, NewCMSMsg, []interface{}{sbs})
//}
//
//func (p *peer) AsyncSBS(oms SBS) {
//	select {
//	case p.queuedSbs <- &propSBSEvent{oms: oms}:
//		p.knownBftBlocks.Add(oms.Hash())
//	default:
//		p.Log().Debug("Dropping oms propagation", "hash", oms.Hash())
//	}
//}

//func (b *truechain.TruePbftBlock) Hash() common.Hash {
//	if hash := b.hash().Load(); hash != nil {
//		return hash.(common.Hash)
//	}
//	v := b.Header.Hash()
//	b.hash().Store(v)
//	return v
//}


func (pm *ProtocolManager) pbBroadcastloop()  {
	for {
		select {
		case event := <-pm.pblocksCh:
			pm.BroadcastPbs(event)
		case <- pm.pbsSub.Err():
			return
		}
	}
}

func(pm *ProtocolManager) BroadcastPbs(pbs []*truechain.TruePbftBlock) {
	var pbset = make(map[*peer][]*truechain.TruePbftBlock)
	for _,pb := range pbs {
		peers := pm.peers.PeersWithoutPbftBlock(pb.Hash())
		for _,peer := range peers {
			pbset[peer] = append(pbset[peer],pb)
		}
		log.Trace("Broadcast pbftblcok","hash",pb.Hash())
	}
	for peer,pbs := range pbset {
		peer.AsyncSendNewBftBlocks(pbs)
	}
}

func (p *peer)SendCms(cms []*truechain.CommitteeMember) error {
	return p2p.Send(p.rw,MainMumbersMsg,cms)
}
func (p *peer)SendCds(cms []*truechain.CommitteeMember) error {
	return p2p.Send(p.rw,CDSMsg,cms)
}

