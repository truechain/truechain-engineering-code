package truechain

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/crypto/sha3"
)

func prlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func (b *TruePbftBlock) Hash() common.Hash {
	//if hash := b.hash().Load(); hash != nil {
	//	return hash.(common.Hash)
	//}
	//v := b.Header.Hash()
	//b.hash().Store(v)
	//return v
	return prlpHash(b)
}
//func (b *TruePbftBlock) hash() atomic.Value {
//	return atomic.Value{}
//}
func (h TruePbftBlockHeader) Hash() common.Hash {
	return prlpHash(h)
}