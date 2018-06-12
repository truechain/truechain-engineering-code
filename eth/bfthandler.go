package eth

import (
	"github.com/ethereum/go-ethereum/core/types"
	"math"
	"github.com/ethereum/go-ethereum/log"
)

type BftProtocalManager struct{
	ProtocolManager
	bftpeers *bftpeerSet
}



//BftBroadcastBlock will either propagate TruePbftblock
func (bpm BftProtocalManager)BftBroadcastBlock(block *types.TruePbftBlock, propagate bool) {
	hash := block.Hash()
	peers := bpm.bftpeers.BftPeersWithoutBlock(hash)

	//if propagation is requested, send to a subset of the peer
	if propagate {
		transfer := peers[:int(math.Sqrt(float64(len(peers))))]
		for _, peer := range transfer {
			peer.BftAsyncSendNewBlock(block)
		}
		log.Trace("Propagated block", "hash", hash, "recipients", len(transfer))
		return
	}

}