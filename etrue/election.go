package etrue

import (
	"crypto/ecdsa"
	"errors"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/event"
	"math/big"
)

const (
	fastChainHeadSize  = 256
	snailchainHeadSize = 4096
	z                  = 100
	k                  = 10000
	lamada             = 12
)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSign   = errors.New("invalid sign")
	ErrCommittee     = errors.New("get committee failed")
	ErrInvalidMember = errors.New("invalid committee member")
)


type VoteuUse struct {
	wi   int64 //Local value
	seed string
	b    bool
	j    int
}

type committee struct {
	id                *big.Int
	beginFastNumber   *big.Int //
	endFastNumber     *big.Int
	beginSnailNumber  *big.Int // the begin snailblock to elect members
	endSnailNumber    *big.Int // the end snailblock to elect members
	switchSnailNumber *big.Int // the start switch snailblock
	members           []*types.CommitteeMember
}

type Election struct {
	genesisCommittee []*types.CommitteeMember
	committeeList    map[*big.Int]*committee

	committee     *committee
	nextCommittee *committee

	fastHeadNumber  *big.Int
	snailHeadNumber *big.Int
	startSwitchover bool //Flag bit for handling event switching

	electionFeed  event.Feed
	committeeFeed event.Feed
	scope         event.SubscriptionScope

	fastChainHeadCh  chan core.ChainHeadEvent
	fastChainHeadSub event.Subscription

	snailChainHeadCh  chan snailchain.ChainHeadEvent
	snailChainHeadSub event.Subscription

	fastchain  *core.BlockChain
	snailchain *snailchain.SnailBlockChain
}

func NewElction(fastBlockChain *core.BlockChain, snailBlockChain *snailchain.SnailBlockChain) *Election {
	// init
	election := &Election{
		fastchain:        fastBlockChain,
		snailchain:       snailBlockChain,
		committeeList:    make(map[*big.Int]*committee),
		fastChainHeadCh:  make(chan core.ChainHeadEvent, fastChainHeadSize),
		snailChainHeadCh: make(chan snailchain.ChainHeadEvent, snailchainHeadSize),
	}

	// get genesis committee
	election.genesisCommittee = election.snailchain.GetGenesisCommittee()

	election.fastChainHeadSub = election.fastchain.SubscribeChainHeadEvent(election.fastChainHeadCh)
	election.snailChainHeadSub = election.snailchain.SubscribeChainHeadEvent(election.snailChainHeadCh)

	return election
}

//Calculate your own force unit locally
func (v VoteuUse) localForce() int64 {
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
func factorial() {
}

//The sum function
func sigma(j int, k int, wi int, P int64) {

}

// the draw function is calculated locally for each miner
// the parameters seed, w_i, W, P are required
func sortition() bool {
	//j := 0;
	//for (seed / powerf(2,seedlen)) ^ [Sigma(j,0,wi,P) , Sigma(j+1,0,wi,P)]{
	//j++;
	//if  j > N {
	//return j,true;
	//	}
	//}
	return false
}

//VerifySigns verify a batch of sings the fast chain committee signatures in batches
func (e *Election) VerifySigns(signs []*types.PbftSign) ([]*types.CommitteeMember, []error) {
	members := make([]*types.CommitteeMember, len(signs))
	errs := make([]error, len(signs))

	for i, sign := range signs {

		hash := sign.HashWithNoSign()
		pubkey, err := crypto.SigToPub(hash.Bytes(), sign.Sign)
		if err != nil {
			errs[i] = err
			continue
		}
		address := crypto.PubkeyToAddress(*pubkey)

		committee := e.GetCommittee(sign.FastHeight)
		if committee == nil {
			errs[i] = ErrCommittee
			continue
		}
		for _, member := range committee {
			committeeAddress := crypto.PubkeyToAddress(*member.Publickey)
			if address != committeeAddress {
				continue
			}
			members[i] = member
			break
		}
		if members[i] == nil {
			errs[i] = ErrInvalidMember
		}
	}

	return members, errs
}

func (e *Election) getCommitteeFromCache(fastNumber *big.Int, snailNumber *big.Int) *committee {
	var ids []*big.Int

	committeeNumber := new(big.Int).Div(snailNumber, big.NewInt(z))
	lastSnailNumber := new(big.Int).Mul(committeeNumber, big.NewInt(z))
	lastSwitchoverNumber := new(big.Int).Sub(lastSnailNumber, big.NewInt(lamada))

	if lastSwitchoverNumber.Cmp(common.Big0) <= 0 {
		ids = append(ids, common.Big0)
		ids = append(ids, common.Big1)
		lastSwitchoverNumber = new(big.Int).Set(common.Big0)
	} else {
		committeeId := new(big.Int).Add(lastSwitchoverNumber, common.Big1)
		ids = append(ids, committeeId)
		ids = append(ids, new(big.Int).Add(committeeId, big.NewInt(z)))
	}

	for _, id := range ids {
		if committee, ok := e.committeeList[id]; ok {
			if committee.beginFastNumber.Cmp(fastNumber) > 0 {
				return nil
			}
			if committee.endFastNumber.Cmp(common.Big0) > 0 && committee.endFastNumber.Cmp(fastNumber) < 0 {
				return nil
			}
			return committee
		}
	}

	return nil
}

// GetCommittee returns the committee members who propose this fast block
func (e *Election) getCommittee(fastNumber *big.Int, snailNumber *big.Int) *committee {
	committeeNumber := new(big.Int).Div(snailNumber, big.NewInt(z))
	lastSnailNumber := new(big.Int).Mul(committeeNumber, big.NewInt(z))
	lastSwitchoverNumber := new(big.Int).Sub(lastSnailNumber, big.NewInt(lamada))

	if committeeNumber.Cmp(common.Big0) == 0 {
		// genesis committee
		return &committee{
			id:                new(big.Int).Set(common.Big0),
			beginFastNumber:   new(big.Int).Set(common.Big1),
			endFastNumber:     new(big.Int).Set(common.Big0),
			beginSnailNumber:  new(big.Int).Set(common.Big0),
			endSnailNumber:    new(big.Int).Set(common.Big0),
			switchSnailNumber: big.NewInt(z),
			members:           e.genesisCommittee,
		}
	}

	// find the last committee end fastblock number
	lastSwitchoverBlock := e.snailchain.GetBlockByNumber(lastSwitchoverNumber.Uint64())
	if lastSwitchoverBlock == nil {
		return nil
	}
	fruits := lastSwitchoverBlock.Fruits()
	lastFastNumber := new(big.Int).Add(fruits[len(fruits)-1].Number(), big.NewInt(k))

	if lastFastNumber.Cmp(fastNumber) >= 0 {
		if committeeNumber.Cmp(common.Big1) == 0 {
			// snail number between 0 to 99
			// genesis committee
			return &committee{
				id:                new(big.Int).Set(common.Big0),
				beginFastNumber:   new(big.Int).Set(common.Big1),
				endFastNumber:     lastFastNumber,
				beginSnailNumber:  new(big.Int).Set(common.Big0),
				endSnailNumber:    new(big.Int).Set(common.Big0),
				switchSnailNumber: big.NewInt(z),
				members:           e.genesisCommittee,
			}
		}
		// pre committee
		endSnailNumber := new(big.Int).Sub(lastSwitchoverNumber, big.NewInt(z))
		beginSnailNumber := new(big.Int).Add(new(big.Int).Sub(endSnailNumber, big.NewInt(z)), common.Big1)
		if beginSnailNumber.Cmp(common.Big0) <= 0 {
			//
			beginSnailNumber = new(big.Int).Set(common.Big1)
		}
		endSnailBlock := e.snailchain.GetBlockByNumber(endSnailNumber.Uint64())
		fruits = endSnailBlock.Fruits()
		preEndFast := new(big.Int).Add(fruits[len(fruits)-1].FastNumber(), big.NewInt(k))
		// get begin number
		members := e.electCommittee(beginSnailNumber, endSnailNumber)
		return &committee{
			id:                beginSnailNumber,
			beginFastNumber:   new(big.Int).Add(preEndFast, common.Big1),
			endFastNumber:     lastFastNumber,
			beginSnailNumber:  beginSnailNumber,
			endSnailNumber:    endSnailNumber,
			switchSnailNumber: lastSnailNumber,
			members:           members,
		}
	}

	// current committee
	endSnailNumber := new(big.Int).Sub(lastSwitchoverNumber, common.Big1)
	beginSnailNumber := new(big.Int).Sub(endSnailNumber, big.NewInt(z))

	members := e.electCommittee(beginSnailNumber, endSnailNumber)

	return &committee{
		id:              beginSnailNumber,
		beginFastNumber: new(big.Int).Add(lastFastNumber, common.Big1),
		endFastNumber:   new(big.Int).Set(common.Big0),

		beginSnailNumber:  beginSnailNumber,
		endSnailNumber:    endSnailNumber,
		switchSnailNumber: new(big.Int).Add(lastSnailNumber, big.NewInt(z)),
		members:           members,
	}
}

func (e *Election) GetCommittee(fastNumber *big.Int) []*types.CommitteeMember {
	newestFast := new(big.Int).Add(e.fastHeadNumber, big.NewInt(k))
	if fastNumber.Cmp(newestFast) > 0 {
		return nil
	}
	if e.committee != nil {
		if fastNumber.Cmp(e.committee.beginFastNumber) >= 0 {
			return e.committee.members
		}
	}

	fastBlock := e.snailchain.GetBlockByNumber(fastNumber.Uint64())
	if fastBlock == nil {
		return nil
	}
	// get snail number
	var snailNumber *big.Int
	snailBlock, _ := e.snailchain.GetFruitByFastHash(fastBlock.Hash())
	if snailBlock == nil {
		// fast block has not stored in snail chain
		// TODO: when fast number is so far away from snail block
		snailNumber = e.snailHeadNumber
	} else {
		snailNumber = snailBlock.Number()
	}

	// find committee from map
	committee := e.getCommitteeFromCache(fastNumber, snailNumber)
	if committee != nil {
		return committee.members
	}

	return e.getCommittee(fastNumber, snailNumber).members
}

func (e *Election) GetComitteeById(id *big.Int) []*types.CommitteeMember {

	if e.committee.id.Cmp(id) == 0 {
		return e.committee.members
	}
	if e.nextCommittee != nil {
		if e.nextCommittee.id.Cmp(id) == 0 {
			return e.nextCommittee.members
		}
	}

	if committee, ok := e.committeeList[id]; ok {
		return committee.members
	}

	return nil
}

func (e *Election) GetByCommitteeId(id *big.Int) []*ecdsa.PublicKey {

	return nil
}

//elect
func (e *Election) elect(snailBeginNumber *big.Int, snailEndNumber *big.Int, committeeId *big.Int) {
	var members []*types.CommitteeMember
	committee := committee{
		id:      committeeId,
		members: members,
	}
	// get all fruits from all snail blocks
	sortition()
	e.committeeList[committeeId] = &committee
	go e.electionFeed.Send(core.ElectionEvent{types.CommitteeSwitchover, committeeId, nil})
	go e.committeeFeed.Send(core.CommitteeEvent{&types.CommitteeInfo{committeeId, members}})
}

func (e *Election) electCommittee(snailBeginNumber *big.Int, snailEndNumber *big.Int) []*types.CommitteeMember {

	return nil
}

func (e *Election) Start() error {
	// get current committee info
	e.fastHeadNumber = e.fastchain.CurrentHeader().Number
	e.snailHeadNumber = e.snailchain.CurrentHeader().Number

	comm := e.getCommittee(e.fastHeadNumber, e.snailHeadNumber)
	if comm == nil {
		return nil
	}

	e.committee = comm
	if comm.endFastNumber.Cmp(common.Big0) > 0 {
		electEndSnailNumber := new(big.Int).Add(comm.endSnailNumber, big.NewInt(z))
		electBeginSnailNumber := new(big.Int).Add(new(big.Int).Sub(electEndSnailNumber, big.NewInt(z)), common.Big1)

		members := e.electCommittee(electBeginSnailNumber, electEndSnailNumber)

		// get next committee
		e.nextCommittee = &committee{
			id:                electBeginSnailNumber,
			beginFastNumber:   new(big.Int).Add(comm.endFastNumber, common.Big1),
			endFastNumber:     new(big.Int).Set(common.Big0),
			beginSnailNumber:  electBeginSnailNumber,
			endSnailNumber:    electEndSnailNumber,
			switchSnailNumber: new(big.Int).Add(e.committee.switchSnailNumber, big.NewInt(z)),
			members:           members,
		}
		// start switchover
		e.startSwitchover = true

		if e.committee.endFastNumber.Cmp(e.fastHeadNumber) == 0 {
			// committee has finish their work, start the new committee

			if _, ok := e.committeeList[e.committee.id]; !ok {
				e.committeeList[e.committee.id] = e.committee
			}

			e.committee = e.nextCommittee
			e.nextCommittee = nil

			e.startSwitchover = false
		}
	}

	// send event to the subscripber
	go func(e *Election) {
		e.committeeFeed.Send(core.CommitteeEvent{&types.CommitteeInfo{e.committee.id, e.committee.members}})
		//time.Sleep(time.Millisecond*500)
		e.electionFeed.Send(core.ElectionEvent{types.CommitteeStart, e.committee.id, nil})

		if e.startSwitchover {
			// send switch event to the subscripber
			e.committeeFeed.Send(core.CommitteeEvent{&types.CommitteeInfo{e.nextCommittee.id, e.nextCommittee.members}})
		}
	} (e)

	// Start the event loop and return
	go e.loop()

	return nil
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
				if e.committee.switchSnailNumber.Cmp(se.Block.Number()) == 0 {
					// get end fast block number
					snailEndNumber := new(big.Int).Sub(se.Block.Number(), big.NewInt(lamada))
					snailStartNumber := new(big.Int).Sub(snailEndNumber, big.NewInt(z))
					members := e.electCommittee(snailStartNumber, snailEndNumber)

					sb := e.snailchain.GetBlockByNumber(snailEndNumber.Uint64())
					fruits := sb.Fruits()
					e.committee.endFastNumber = new(big.Int).Add(fruits[len(fruits)-1].Number(), big.NewInt(k))

					e.nextCommittee = &committee{
						id:                snailStartNumber,
						beginSnailNumber:  snailStartNumber,
						endSnailNumber:    snailEndNumber,
						beginFastNumber:   new(big.Int).Add(e.committee.endFastNumber, common.Big1),
						switchSnailNumber: new(big.Int).Add(e.committee.switchSnailNumber, big.NewInt(z)),
						members:           members,
					}

					e.startSwitchover = true

					go e.committeeFeed.Send(core.CommitteeEvent{&types.CommitteeInfo{e.nextCommittee.id, e.nextCommittee.members}})
				}

			}
			// Make logical decisions based on the Number provided by the ChainheadEvent
		case ev := <-e.fastChainHeadCh:
			if ev.Block != nil {
				if e.startSwitchover {
					if e.committee.endFastNumber.Cmp(ev.Block.Number()) == 0 {
						go e.electionFeed.Send(core.ElectionEvent{types.CommitteeStop, e.committee.id, nil})

						// find committee already exist in committee list
						e.committeeList[e.committee.id] = e.committee

						e.committee = e.nextCommittee
						e.nextCommittee = nil

						e.startSwitchover = false

						go e.electionFeed.Send(core.ElectionEvent{types.CommitteeStart, e.committee.id, nil})
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
