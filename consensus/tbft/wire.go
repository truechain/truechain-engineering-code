package consensus

import (
	amino "github.com/truechain/truechain-engineering-code/consensus/tbft/go-amino"
)

var cdc = amino.NewCodec()

func init() {
	RegisterConsensusMessages(cdc)
	RegisterWALMessages(cdc)
	// types.RegisterBlockAmino(cdc)
}
