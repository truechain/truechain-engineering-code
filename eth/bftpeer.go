package eth

import (
	"github.com/ethereum/go-ethereum/core/types"
	"gopkg.in/fatih/set.v0"
	"github.com/ethereum/go-ethereum/log"
	"sync"
	"github.com/ethereum/go-ethereum/common"
)

type bftpeer struct {
	peer
	bftqueuedProps chan *bftPropEvent
	knownbftBlocks *set.Set
}

type bftPropEvent struct {
	bftblock *types.TruePbftBlock
}

type bftpeerSet struct{
	bftpeers map[string]*bftpeer
	lock sync.RWMutex
	closed bool
}

func (p *bftpeer) BftAsyncSendNewBlock(block *types.TruePbftBlock)  {
	select {
	case p.bftqueuedProps <- &bftPropEvent{bftblock:block}:
		p.knownbftBlocks.Add(block.Hash())
	default:
		log.Debug("Dropping block propagation","hash",block.Hash())
	}
}

func (bps *bftpeerSet) BftPeersWithoutBlock(hash common.Hash) []*bftpeer {
	bps.lock.RLock()
	defer bps.lock.Unlock()
	list := make([]*bftpeer, 0, len(bps.bftpeers))
	for _, p := range bps.bftpeers {
		if !p.knownbftBlocks.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

