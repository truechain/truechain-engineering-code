package truechain

import (
	"github.com/ethereum/go-ethereum/common"
)


func (b *TruePbftBlock) Hash() common.Hash {
	//if hash := b.hash().Load(); hash != nil {
	//	return hash.(common.Hash)
	//}
	//v := b.Header.Hash()
	//b.hash().Store(v)
	//return v
	return rlpHash(b)
}
//func (b *TruePbftBlock) hash() atomic.Value {
//	return atomic.Value{}
//}
func (h TruePbftBlockHeader) Hash() common.Hash {
	return rlpHash(h)
}