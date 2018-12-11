package p2p

import (
	cryptoAmino "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto/cryptoamino"
	amino "go-amino"
)

var cdc = amino.NewCodec()

func init() {
	cryptoAmino.RegisterAmino(cdc)
}
