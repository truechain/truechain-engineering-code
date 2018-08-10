package etrue

import (
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/core"
	"math/big"
	"github.com/truechain/truechain-engineering-code/core/fastchain"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/common"
)

const (
	fastChainHeadSize = 256
	chainHeadSize  = 4096
	z = 99
	k  =  10000
	lamada = 12
)

type VoteuUse struct {
	wi 		int64  //Local value
	seed 	string
	b   	bool
	j 		int
}

type Committee struct {
	id *big.Int
	beginFastNumber *big.Int
	endFastNumber *big.Int
	beginSnailNumber *big.Int
	endSnailNumber *big.Int
	members []*types.CommitteeMember
}

type Election struct {
	genesisCommittee []*types.CommitteeMember

	fastNumber       *big.Int
	snailNumber      *big.Int
	committee        *Committee
	committeeList    map[*big.Int]*Committee

	committeeId		 *big.Int
	nextCommitteeId  *big.Int

	startSwitchover  bool //Flag bit for handling event switching
	number           *big.Int

	fastCount		uint
	snailCount		uint

	electionFeed	event.Feed
	committeeFeed	event.Feed
	scope         event.SubscriptionScope

	fastChainHeadCh  chan core.FastChainHeadEvent
	fastChainHeadSub event.Subscription

	snailChainHeadCh  chan core.SnailChainHeadEvent
	snailChainHeadSub event.Subscription

	fastchain *fastchain.FastBlockChain
	snailchain *snailchain.SnailBlockChain
}

//Read creation block information and return public key for signature verification
func  (v VoteuUse)ReadGenesis()[]string{
	return nil
}

//Calculate your own force unit locally
func (v VoteuUse)localForce()int64{
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
func factorial(){
}

//The sum function
func sigma(j int,k int,wi int,P int64) {

}

// the draw function is calculated locally for each miner
// the parameters seed, w_i, W, P are required
func sortition()bool{
	//j := 0;
	//for (seed / powerf(2,seedlen)) ^ [Sigma(j,0,wi,P) , Sigma(j+1,0,wi,P)]{
	//j++;
	//if  j > N {
	//return j,true;
	//	}
	//}
	return false;
}

// Verify checks a raw ECDSA signature.
// Returns true if it's valid and false if not.
/*
func (cm CommitteeMember)Verify(signature []byte)bool {
	// hash message
	digest := sha256.Sum256(signature)
	pubkey := cm.pubkey
	curveOrderByteSize := pubkey.Curve.Params().P.BitLen() / 8
	r, s := new(big.Int), new(big.Int)
	r.SetBytes(signature[:curveOrderByteSize])
	s.SetBytes(signature[curveOrderByteSize:])
	return ecdsa.Verify(pubkey,digest[:], r, s)

}
*/

//Another method for validation
func (e *Election)VerifySign(FastHeight *big.Int,FastHash common.Hash, msgHash common.Hash, Sign []byte)bool {
	return true
}

func (e *Election) VerifyLeaderBlock(height *big.Int, sign []byte) bool  {
	return true
}

//Verify the fast chain committee signatures in batches
func (e *Election) VerifySigns(pvs *[]types.PbftSign) (cfvf *[]types.CommitteeMember) {
	return cfvf
}


// GetCommittee returns the committee members who propose this fast block
func (e *Election)GetCommittee(FastNumber *big.Int, FastHash common.Hash) (*big.Int, []*types.CommitteeMember){

	// get fast block from fastchain
	fb := e.fastchain.GetBlock(FastHash, FastNumber.Uint64())
	if fb == nil {

	}
	// find fast block from committee map

	// find fruit/snail block pointer to this fast block from snail chain
	// find pre committee snail block, calculate committee begin and end number
	// sorition()

	return nil, nil
}

//elect
func (e *Election)elect(snailBeginNumber *big.Int, snailEndNumber *big.Int, committeeId *big.Int) {
	var members []*types.CommitteeMember
	committee := Committee {
		id : committeeId,
		members : members,
	}
	// get all fruits from all snail blocks
	sortition()
	e.committeeList[committeeId] = &committee
	//go e.electionFeed.Send(core.ElectionEvent{types.CommitteeSwitchover, committeeId})
	//go e.committeeFeed.Send(core.CommitteeEvent{committeeId, members})
}


//Monitor both chains and trigger elections at the same time
func (e *Election) loop() {
	// Keep waiting for and reacting to the various events
	for {
		select {
		// Handle ChainHeadEvent
		case se := <-e.snailChainHeadCh:
			if se.Block != nil {
				//Record Numbers to open elections
				e.snailCount++

				if e.snailCount == z {
					e.snailCount = 0
					// start switchover
					e.startSwitchover = true
					// get end fast block number
					snailEndNumber := new(big.Int).Sub(se.Block.Number(), big.NewInt(lamada))
					snailStartNumber := new(big.Int).Sub(snailEndNumber, big.NewInt(z))
					//sb := e.snailchain.GetBlockByNumber(snailEndNumber.Uint64())
					//fruits := sb.Fruits()
					//e.number = new(big.Int).Add(fruits[len(fruits) - 1].Number(), big.NewInt(k))

					e.nextCommitteeId = new(big.Int).Add(se.Block.Number(), common.Big1)

					go e.elect(snailStartNumber, snailEndNumber, e.nextCommitteeId)
				}

			}
			// Make logical decisions based on the Number provided by the ChainheadEvent
		case ev := <-e.fastChainHeadCh:
			if ev.Block != nil{
				if e.startSwitchover {
					if e.number.Cmp(ev.Block.Number()) == 0 {
						//go e.electionFeed.Send(core.ElectionEvent{types.CommitteeStop, e.committee.id})

						// find committee already exist in committee list
						e.committeeList[e.committee.id] = e.committee

						e.committee = e.committeeList[e.nextCommitteeId]
						e.committeeId = e.nextCommitteeId
						e.startSwitchover = false

						//go e.electionFeed.Send(core.ElectionEvent{types.CommitteeStart, e.committee.id})
					}
				}
			}
		}
	}
}

func (e *Election) SubscribeElectionEvent(ch chan<- core.ElectionEvent) event.Subscription {
	return e.scope.Track(e.electionFeed.Subscribe(ch))
}

func (e *Election) SubscribeCommitteeEvent(ch chan<- core.CommitteeEvent) event.Subscription {
	return e.scope.Track(e.committeeFeed.Subscribe(ch))
}


func NewElction()*Election {

	// init

	// get genesis committee

	// get current fast/snail

	// get current committee

	// get snail count

	// subscribe chainhead

	// start loop

	return nil
}

