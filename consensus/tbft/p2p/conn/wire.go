package conn

import (
	amino "github.com/truechain/truechain-engineering-code/consensus/tbft/go-amino"
)

var cdc *amino.Codec = amino.NewCodec()

func init() {
	// cryptoAmino.RegisterAmino(cdc)
	RegisterPacket(cdc)
}
