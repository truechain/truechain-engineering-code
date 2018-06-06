package types

import "math/big"

type TruePbftNode struct {
	addr    string
	pubkey  string
	privkey string
}

type TruePbftBlockHeader struct {
	Number   *big.Int
	GasLimit *big.Int
	GasUsed  *big.Int
	Time     *big.Int
}

type TruePbftBlock struct {
	header       *TruePbftBlockHeader
	Transactions []*Transaction
	sig          []*string
}
