package truechain

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"
)

var z  = 10000
var k  = 50000
var P  = 100

type VoteuUse struct {
	wi 	int64  //本地算力值wi
	fe  int64  //总算力单元数w
	seed string
	b   bool
	j 	int


}
//本地计算出自己的算力单元w_i
func (v VoteuUse)LocalForce()int64{


	w := v.wi
	//w_i=(D_pf-〖[h]〗_(-k))/u
	return w

}


//抽签函数使用的幂函数，
func powerf(x float64, n int) float64 {
	ans := 1.0

	for n != 0 {
		if n%2 == 1 {
			ans *= x
		}
		x *= x
		n /= 2
	}
	return ans
}

//阶乘函数
func Factorial(){

}

//求和函数
func Sigma(j int,k int,wi int,P int64) {

}

//每个矿工本地进行计算抽签函数
//需要参数seed, w_i, W，P

//func (v VoteuUse)Sortition()int,bool{
//j := 0;
//
//for (seed / powerf(2,seedlen)) ^ [Sigma(j,0,wi,P) , Sigma(j+1,0,wi,P)]{
//
//j++;
//
//if  j > N {
//return j,true;
//	}
//}
//	return _,false;
//
//}

//签名验证
// Verify checks a raw ECDSA signature.
// Returns true if it's valid and false if not.
func Verify(data, signature []byte, pubkey *ecdsa.PublicKey) bool {
	// hash message
	digest := sha256.Sum256(data)

	curveOrderByteSize := pubkey.Curve.Params().P.BitLen() / 8

	r, s := new(big.Int), new(big.Int)
	r.SetBytes(signature[:curveOrderByteSize])
	s.SetBytes(signature[curveOrderByteSize:])

	return ecdsa.Verify(pubkey, digest[:], r, s)
}


