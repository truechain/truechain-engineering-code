// Copyright 2018 The Truechain Authors
// This file is part of the truechain-engineering-code library.
//
// The truechain-engineering-code library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The truechain-engineering-code library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the truechain-engineering-code library. If not, see <http://www.gnu.org/licenses/>.

package etrue

import (
	"crypto/ecdsa"
	"errors"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/event"
	"math/big"
)

const (
	fastChainHeadSize  = 256
	snailchainHeadSize = 64
	z                  = 40
	k                  = 10000
	lamada             = 12

	fruitThreshold		= 1		// fruit size threshold for committee election

	maxCommitteeNumber  = 40
	minCommitteeNumber  = 1

	powUnit				= 1
)

var (
	// maxUint256 is a big integer representing 2^256-1
	maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSign   = errors.New("invalid sign")
	ErrCommittee     = errors.New("get committee failed")
	ErrInvalidMember = errors.New("invalid committee member")
)


var testCommitteeNodes = []*types.CommitteeNode{
	{
		IP:        "192.168.46.8",
		Port:      10080,
		Coinbase:  common.HexToAddress("831151b7eb8e650dc442cd623fbc6ae20279df85"),
		Publickey: common.Hex2Bytes("04ae5b1e301e167f9676937a2733242429ce7eb5dd2ad9f354669bc10eff23015d9810d17c0c680a1178b2f7d9abd925d5b62c7a463d157aa2e3e121d2e266bfc6"),
	},
	{
		IP:        "192.168.46.33",
		Port:      10080,
		Coinbase:  common.HexToAddress("76ea2f3a002431fede1141b660dbb75c26ba6d97"),
		Publickey: common.Hex2Bytes("04044308742b61976de7344edb8662d6d10be1c477dd46e8e4c433c1288442a79183480894107299ff7b0706490f1fb9c9b7c9e62ae62d57bd84a1e469460d8ac1"),
	},
	{
		IP:        "192.168.46.24",
		Port:      10080,
		Coinbase:  common.HexToAddress("1074f7deccf8c66efcd0106e034d3356b7db3f2c"),
		Publickey: common.Hex2Bytes("04013151837b19e4b0e7402ac576e4352091892d82504450864fc9fd156ddf15d22014a0f6bf3c8f9c12d03e75f628736f0c76b72322be28e7b6f0220cf7f4f5fb"),
	},

	{
		IP:        "192.168.46.4",
		Port:      10080,
		Coinbase:  common.HexToAddress("d985e9871d1be109af5a7f6407b1d6b686901fff"),
		Publickey: common.Hex2Bytes("04e3e59c07b320b5d35d65917d50806e1ee99e3d5ed062ed24d3435f61a47d29fb2f2ebb322011c1d2941b4853ce2dc71e8c4af57b59bbf40db66f76c3c740d41b"),
	},
}

var testCommttee []*types.CommitteeMember


type candidateMember struct {
	coinbase   common.Address
	address    common.Address
	publickey  *ecdsa.PublicKey
	difficulty *big.Int //
	upper      *big.Int //
	lower      *big.Int
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

	engine   consensus.Engine
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

	//
	for _, node := range testCommitteeNodes {
		pubkey, _ := crypto.UnmarshalPubkey(node.Publickey)
		member := &types.CommitteeMember{
			Coinbase:node.Coinbase,
			Publickey: pubkey,
		}
		testCommttee = append(testCommttee, member)
	}

	return election
}


//VerifySigns verify signatures of bft committee in batches
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


// getCandinates get candinate miners and seed from given snail blocks
func (e *Election) getCandinates(snailBeginNumber *big.Int, snailEndNumber *big.Int) (common.Hash, []*candidateMember) {
	var fruitsCount map[common.Address]uint = make(map[common.Address]uint)
	var members []*candidateMember

	var seed []byte

	// get all fruits want to be elected and their pubic key is valid
	for blockNumber := snailBeginNumber; blockNumber.Cmp(snailEndNumber) <= 0; {
		block := e.snailchain.GetBlockByNumber(blockNumber.Uint64())
		if block == nil {
			return common.Hash{}, nil
		}

		seed = append(seed, block.Hash().Bytes()...)

		fruits := block.Fruits()
		for _, f := range fruits {
			if f.ToElect() {
				pubkey, err := f.GetPubKey()
				if err != nil {
					continue
				}
				addr := crypto.PubkeyToAddress(*pubkey)

				act, diff := e.engine.GetDifficulty(f.Header())

				member := &candidateMember{
					coinbase: f.Coinbase(),
					publickey: pubkey,
					address: addr,
					difficulty: new(big.Int).Sub(act, diff),
				}

				members = append(members, member)
				if _, ok :=fruitsCount[addr]; ok {
					fruitsCount[addr] += 1
				} else {
					fruitsCount[addr] = 1
				}
			}
		}
		blockNumber = new(big.Int).Add(blockNumber, big.NewInt(1))
	}

	log.Debug("get committee candidate", "fruit", len(members), "members", len(fruitsCount))

	// remove miner whose fruits count below threshold
	/*
	for addr, cnt := range fruitsCount {
		log.Trace("get committee candidate", "keyAddr", addr, "count", cnt)
		if cnt < fruitThreshold {
			delete(fruitsCount, addr)
		}
	}*/
	var candidates []*candidateMember
	td := big.NewInt(0)
	for _, member := range members {
		if cnt, ok := fruitsCount[member.address]; ok {
			log.Trace("get committee candidate", "keyAddr", member.address, "count", cnt, "diff", member.difficulty)
			if cnt >= fruitThreshold {
				td.Add(td, member.difficulty)

				candidates = append(candidates, member)
			}
		}
	}
	log.Debug("get final candidate", "count", len(candidates))

	dd := big.NewInt(0)
	rate := new(big.Int).Div(maxUint256, td)
	for i, member := range candidates {
		member.lower = new(big.Int).Mul(rate, dd)

		dd = new(big.Int).Add(dd, member.difficulty)

		if i == len(candidates) - 1 {
			member.upper = new(big.Int).Set(maxUint256)
		} else {
			member.upper = new(big.Int).Mul(rate, dd)
		}
	}

	return crypto.Keccak256Hash(seed), candidates
}


// elect is a lottery function that select committee members from candidates miners
func (e *Election) elect(candidates []*candidateMember, seed common.Hash) []*types.CommitteeMember {
	var addrs map[common.Address]uint = make(map[common.Address]uint)
	var members []*types.CommitteeMember

	log.Debug("elect committee members ..")
	round := new(big.Int).Set(common.Big0)
	for {
		seedNumber := new(big.Int).Add(seed.Big(), round)
		hash := crypto.Keccak256Hash(seedNumber.Bytes())
		prop := new(big.Int).Div(hash.Big(), maxUint256)

		for _, cm := range candidates {
			if prop.Cmp(cm.lower) < 0 {
				continue
			}
			if prop.Cmp(cm.upper) >= 0 {
				continue
			}
			if _, ok := addrs[cm.address]; ok {
				break
			}
			addrs[cm.address] = 1
			member := &types.CommitteeMember{
				Coinbase: cm.coinbase,
				Publickey: cm.publickey,
			}
			members = append(members, member)

			break
		}

		round = new(big.Int).Add(round, common.Big1)
		if round.Cmp(big.NewInt(maxCommitteeNumber)) >= 0 {
			if (len(members) >= minCommitteeNumber) {
				break
			}
		}
	}

	log.Debug("get new committee members", "count", len(members))


	return members
}


// electCommittee elect committee members from snail block.
func (e *Election) electCommittee(snailBeginNumber *big.Int, snailEndNumber *big.Int) []*types.CommitteeMember {
	log.Info("Election new BFT committee..", "begin", snailBeginNumber, "end", snailEndNumber, "threshold", fruitThreshold, "min", minCommitteeNumber, "max", maxCommitteeNumber )
	seed, candidates := e.getCandinates(snailBeginNumber, snailEndNumber)
	if candidates == nil {
		return nil
	}

	members:= e.elect(candidates, seed)

	// for test
	members = testCommttee
	return members
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
		e.committeeFeed.Send(core.ElectionEvent{types.CommitteeSwitchover, e.committee.id, e.committee.members})
		//time.Sleep(time.Millisecond*500)
		e.electionFeed.Send(core.ElectionEvent{types.CommitteeStart, e.committee.id, nil})

		if e.startSwitchover {
			// send switch event to the subscripber
			e.committeeFeed.Send(core.ElectionEvent{types.CommitteeSwitchover, e.nextCommittee.id, e.nextCommittee.members})
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
					var snailStartNumber *big.Int
					snailEndNumber := new(big.Int).Sub(se.Block.Number(), big.NewInt(lamada))
					if snailEndNumber.Cmp(big.NewInt(z)) < 0 {
						snailStartNumber = new(big.Int).Set(common.Big1)
					} else {
						snailStartNumber = new(big.Int).Sub(snailEndNumber, big.NewInt(z))
					}

					members := e.electCommittee(snailStartNumber, snailEndNumber)

					sb := e.snailchain.GetBlockByNumber(snailEndNumber.Uint64())
					fruits := sb.Fruits()
					e.committee.endFastNumber = new(big.Int).Add(fruits[len(fruits)-1].Number(), big.NewInt(k))

					log.Info("BFT committee election start..", "snail", se.Block.Number(), "end fast", e.committee.endFastNumber)

					e.nextCommittee = &committee{
						id:                snailStartNumber,
						beginSnailNumber:  snailStartNumber,
						endSnailNumber:    snailEndNumber,
						beginFastNumber:   new(big.Int).Add(e.committee.endFastNumber, common.Big1),
						switchSnailNumber: new(big.Int).Add(e.committee.switchSnailNumber, big.NewInt(z)),
						members:           members,
					}

					e.startSwitchover = true

					go e.committeeFeed.Send(core.ElectionEvent{types.CommitteeSwitchover, e.nextCommittee.id, e.nextCommittee.members})
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

						log.Info("Start new BFT committee..")

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


func (e *Election)SetEngine(engine consensus.Engine) {
	e.engine = engine
}
