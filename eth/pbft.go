package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/eth/truechain"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	maxKnownPbftBlocks = 10240
)

//type PbftPool interface {
//	AddRemotes(block *truechain.TruePbftBlock) []error
//	Pending()(map[common.Address]truechain.TruePbftBlock,error)
//	SubscribeNewPbftsEvent(chan<- []*truechain.TruePbftBlock) event.Subscription
//}

type NewPbftsEvent struct{ Pbfts []*truechain.TruePbftBlock }

type newBftBlockData struct {
	Block *truechain.TruePbftBlock
}

//func (b *truechain.TruePbftBlock) hash() atomic.Value {
//	return atomic.Value{}
//}

func (ps *peerSet) PeersWithoutPbftBlock(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownPbftBlocks.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}
func (ps *peerSet) PeersWithoutPbftCms(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownPbftCms.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}
func (ps *peerSet) PeersWithoutPbftCds(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownPbftCds.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

//
//func (h truechain.TruePbftBlockHeader) Hash() common.Hash {
//	return prlpHash(h)
//}

type CMS []*truechain.PbftCommittee
type CDS []*truechain.CdEncryptionMsg

func (h *CMS) Hash() common.Hash {
	return prlpHash(h)
}
func (h *CDS) Hash() common.Hash {
	return prlpHash(h)
}

//bpft
type propBftEvent struct{ block *truechain.TruePbftBlock }

func (p *peer) SendNewPbftBlock(b *truechain.TruePbftBlock) error {
	//p.knownBftBlocks.Add(b.Hash())
	return p2p.Send(p.rw, NewBftBlockMsg, []interface{}{b})
}

func (p *peer) AsyncSendNewBftBlocks(blocks []*truechain.TruePbftBlock) {
	select {
	case p.queuedPbftProps <- blocks:
		for _, b := range blocks {
			p.knownPbftBlocks.Add(b.Hash())
		}
	default:
		p.Log().Debug("Dropping block propagation", "block", b)
	}
}

//cms

func (p *peer) SendCMS(cms CMS) error {
	p.knownPbftCms.Add(cms.Hash())
	return p2p.Send(p.rw, CMSMsg, []interface{}{cms})
}

//oms
func (p *peer) SendCDS(cds CDS) error {
	p.knownPbftCds.Add(cds.Hash())
	return p2p.Send(p.rw, CDSMsg, []interface{}{cds})
}
func (pm *ProtocolManager) pbBroadcastloop() {
	for {
		select {
		case event := <-pm.pblocksCh:
			pm.BroadcastPbs(event)
			//case <-pm.pbsSub.Err():
			//	return
		}
	}
}

func (pm *ProtocolManager) BroadcastPbs(pbs []*truechain.TruePbftBlock) {
	var pbset = make(map[*peer][]*truechain.TruePbftBlock)
	for _, pb := range pbs {
		peers := pm.peers.PeersWithoutPbftBlock(pb.Hash())
		for _, peer := range peers {
			pbset[peer] = append(pbset[peer], pb)
		}
		log.Trace("Broadcast pbftblcok", "hash", pb.Hash())
	}
	for peer, pbs := range pbset {
		peer.AsyncSendNewBftBlocks(pbs)
	}
}

func prlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func (p *peer) MarkPbftBlock(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownBlocks.Size() >= maxKnownPbftBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(hash)
}
