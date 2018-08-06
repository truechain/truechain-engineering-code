package etrue

import (
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/common"
	"math/big"
	"log"
	"github.com/truechain/truechain-engineering-code/core/fastchain"
)
var ( 	z  = 100
 		k  = 10000
 		)

const (
	fastChainSize = 256

)
type VoteuUse struct {
	wi 		int64  //Local value
	seed 	string
	b   	bool
	j 		int

}


type Signature struct{
	blockHash common.Hash
	blockNumber *big.Int
	hash common.Hash
	sign []byte
}

type queue struct{
	queuesize int   //数组的大小
	head int  //队列的头下标
	tail int	//尾下标
	q []int    //数组
}

type CommitteeMember struct{
	ip		string
	port  uint
	coinbase common.Hash
	pubkey  *ecdsa.PublicKey
}

type Committee struct {
	Id 	*big.Int
	CommitteeList []*Committee
}



type Election struct {
	genesisCommittee []*CommitteeMember

	committee []*CommitteeMember

	fastHead *big.Int
	snailHead *big.Int

	switchFeed  event.Feed

	fastChainHeadCh  chan core.FastChainHeadEvent
	fastChainHeadSub event.Subscription

	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription

	fastchain *fastchain.FastBlockChain
	snailchain *core.BlockChain
}

//Read creation block information and return public key for signature verification
//func  (v VoteuUse)ReadGenesis()string{
//
//	return v.pk
//}


func (e *Election) Events() *event.Feed {
	return &e.switchFeed
}

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
func sortition()bool{
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
	return false;

}


func(qu queue) InitQueue(){

	qu.queuesize = z;

	//qu.q = make(int,qu.queuesize)

	qu.tail = 0;

	qu.head = 0;

}


func (qu queue) EnQueue(key int){

	tail := (qu.tail+1) % qu.queuesize //取余保证，当quil=queuesize-1时，再转回0

	if tail == qu.head{

		log.Print("the queue has been filled full!")

	} else {

	qu.q[qu.tail] = key

	qu.tail = tail

	}

}

func (qu queue) gettail() int {

	return qu.tail

}






// Verify checks a raw ECDSA signature.
// Returns true if it's valid and false if not.
//Original demand parameter(data,signature []byte,  pubkey *ecdsa.PublicKey)
func (e *Election)Verify(sign *Signature)bool {

	return true
}

//Provide committee address enquiries
func AddrQuery(fb types.FastBlock)[]string{
	//
	//	c := committee{}
	//
	//if fb.Signs() = c.pk{
	//		return c.ip
	//	}
	//	else {
	//			Sortition()
	//			if
	//}
	return nil
}

//Gets the number of fast and slow chain transactions
//Demand parameters（k fastchain, z snialchain）
func counter()bool{
	//if z = 100 && k = 10000
	return true
}


func (e *Election)elect(FastNumber *big.Int, FastHash common.Hash)[]*CommitteeMember {

	return nil
}

func (e *Election)GetCommittee(FastNumber *big.Int, FastHash common.Hash) (*big.Int, []*CommitteeMember){

	return nil, nil
}

//Monitor both chains and trigger elections at the same time
func (e *Election) loop() {

	// Keep waiting for and reacting to the various events
	Qu := queue{}
	Qu.InitQueue()
	for {
		select {
		// Handle ChainHeadEvent
		case fb := <-e.chainHeadCh:
			if fb.Block != nil {
				//Record Numbers to open elections
				Qu.EnQueue(0)
				if Qu.gettail() == z-1{

					sortition()
					//fb.GetBlock()
					//fb. GetBlockByNumber()



				}
			}

		case ev := <-e.fastChainHeadCh:
			if ev.Block != nil{

			}


		}
	}
}




func NewElction(fastHead *big.Int,snailHead *big.Int,fastchain *fastchain.FastBlockChain,snailchain *core.BlockChain)*Election {
	e := &Election{
		fastHead:		fastHead,
		snailHead: 		snailHead,
		fastchain:		fastchain,
		snailchain:		snailchain,

	}

	return e
}