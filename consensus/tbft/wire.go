package tbft

import (
	"github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	amino "go-amino"
)

var cdc = amino.NewCodec()

func init() {
	RegisterConsensusMessages(cdc)
	// RegisterWALMessages(cdc)
	types.RegisterBlockAmino(cdc)
}
