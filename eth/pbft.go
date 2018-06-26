package eth

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth/truechain"
	"github.com/ethereum/go-ethereum/event"
)

type PbftPool interface {
	AddRemotes(block *truechain.TruePbftBlock) []error
	Pending()(map[common.Address]truechain.TruePbftBlock,error)
	SubscribeNewPbftsEvent(chan<- NewPbftsEvent) event.Subscription

}

type NewPbftsEvent struct {Pbfts []*truechain.TruePbftBlock }

type newBftBlockData struct {
	Block *truechain.TruePbftBlock
}
