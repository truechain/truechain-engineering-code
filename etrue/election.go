package etrue

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"math/big"
	"github.com/ethereum/truechain-engineering-code/core/types"

)

var z  = 10000
var k  = 50000
var P  = 100

type VoteuUse struct {
	wi 		int64  //Local value
	seed 	string
	b   	bool
	j 		int

}

type committee struct{
	ip		[]string
	pk 		string
	data    []byte
	pubkey  *ecdsa.PublicKey

}

//Read creation block information and return public key for signature verification
//func  (v VoteuUse)ReadGenesis()string{
//
//	return v.pk
//}



//Calculate your own force unit locally
func (v VoteuUse)LocalForce()int64{


	w := v.wi
	//w_i=(D_pf-〖[h]〗_(-k))/u
	return w

}

//The power function used by the draw function
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

//Factorial function
func Factorial(){

}

//The sum function
func Sigma(j int,k int,wi int,P int64) {

}

// the draw function is calculated locally for each miner
// the parameters seed, w_i, W, P are required

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


// Verify checks a raw ECDSA signature.
// Returns true if it's valid and false if not.
//Original demand parameter(data,signature []byte,  pubkey *ecdsa.PublicKey)
func (c committee)Verify (fb types.FastBlock,)bool {

			data := c.data
			pubkey := c.pubkey
			//signature := fb.Signs()

	// hash message
	digest := sha256.Sum256(data)

	//curveOrderByteSize := pubkey.Curve.Params().P.BitLen() / 8

	r, s := new(big.Int), new(big.Int)
	//r.SetBytes(signature[:curveOrderByteSize])
	//s.SetBytes(signature[curveOrderByteSize:])

	return ecdsa.Verify(pubkey, digest[:], r, s)
}

//Gets the number of fast and slow chain transactions
//Demand parameters（k fastchain, z snialchain）

func counter()bool{
	//if z = 100 && k = 10000
	return true
}

//Provide committee address enquiries
func (c committee)AddrQuery(fb types.FastBlock)[]string{
	return c.ip
}
