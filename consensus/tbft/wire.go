package tbft

import (
	"github.com/tendermint/go-amino"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/types"
)

var cdc = amino.NewCodec()

func init() {
	RegisterConsensusMessages(cdc)
	// RegisterWALMessages(cdc)
	types.RegisterBlockAmino(cdc)
}
