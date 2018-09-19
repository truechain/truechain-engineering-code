package types

import (
	amino "github.com/truechain/truechain-engineering-code/consensus/tbft/go-amino"
	cryptoAmino "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto/cryptoamino"
)

var cdc = amino.NewCodec()

func init() {
	RegisterBlockAmino(cdc)
}

func RegisterBlockAmino(cdc *amino.Codec) {
	cryptoAmino.RegisterAmino(cdc)
}
