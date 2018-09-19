package p2p

import (
	amino "github.com/truechain/truechain-engineering-code/consensus/tbft/go-amino"
	cryptoAmino "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto/cryptoamino"
)

var cdc = amino.NewCodec()

func init() {
	cryptoAmino.RegisterAmino(cdc)
}
