// Copyright 2018 The TrueChain Authors
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
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"math/big"
	"sync"

	"github.com/hashicorp/golang-lru"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
)

const (
	fastChainHeadSize  = 4096
	snailchainHeadSize = 64

	committeeCacheLimit     = 256
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

type candidateMember struct {
	coinbase   common.Address
	address    common.Address
	publickey  *ecdsa.PublicKey
	difficulty *big.Int //
	upper      *big.Int //
	lower      *big.Int
}

type committee struct {
	id                  *big.Int
	beginFastNumber     *big.Int // the first fast block proposed by this committee
	endFastNumber       *big.Int // the last fast block proposed by this committee
	firstElectionNumber *big.Int // the begin snailblock to elect members
	lastElectionNumber  *big.Int // the end snailblock to elect members
	switchCheckNumber   *big.Int // the snailblock that start switch next committee
	members             types.CommitteeMembers
}

func (c *committee) Members() []*types.CommitteeMember {
	members := make([]*types.CommitteeMember, len(c.members))
	copy(members, c.members)
	return members
}

func electionBlocks(fastNumber, snailNumber *big.Int) (id, begin, end *big.Int) {
	id = new(big.Int).Div(snailNumber, params.ElectionPeriodNumber)
	end = new(big.Int).Mul(id, params.ElectionPeriodNumber)
	end.Sub(end, params.SnailConfirmInterval)
	begin = new(big.Int).Add(new(big.Int).Sub(end, params.ElectionPeriodNumber), common.Big1)
	if begin.Cmp(common.Big0) <= 0 {
		begin = new(big.Int).Set(common.Big0)
	}
	return
}

func committeeSection(CommitteeNumber *big.Int) (begin, end *big.Int) {
	end = new(big.Int).Mul(CommitteeNumber, params.ElectionPeriodNumber)
	begin = new(big.Int).Add(new(big.Int).Sub(end, params.ElectionPeriodNumber), common.Big1)
	if begin.Cmp(common.Big0) <= 0 {
		begin = new(big.Int).Set(common.Big0)
	}
	return
}

func electionSection(CommitteeNumber *big.Int) (begin, end *big.Int) {
	end = new(big.Int).Mul(CommitteeNumber, params.ElectionPeriodNumber)
	end.Sub(end, params.SnailConfirmInterval)
	begin = new(big.Int).Add(new(big.Int).Sub(end, params.ElectionPeriodNumber), common.Big1)
	if begin.Cmp(common.Big0) <= 0 {
		begin = new(big.Int).Set(common.Big1)
	}
	return
}

type Election struct {
	genesisCommittee []*types.CommitteeMember
	defaultMembers   []*types.CommitteeMember

	commiteeCache *lru.Cache
	committeeList map[uint64]*committee

	committee     	*committee
	nextCommittee 	*committee
	mu              sync.RWMutex

	startSwitchover bool //Flag bit for handling event switching
	singleNode      bool

	electionFeed event.Feed
	scope        event.SubscriptionScope

	fastChainEventCh  chan types.ChainFastEvent
	fastChainEventSub event.Subscription

	snailChainEventCh  chan types.ChainSnailEvent
	snailChainEventSub event.Subscription

	fastchain  *core.BlockChain
	snailchain *snailchain.SnailBlockChain

	engine consensus.Engine
}

func NewElction(fastBlockChain *core.BlockChain, snailBlockChain *snailchain.SnailBlockChain, config *Config) *Election {
	// init
	election := &Election{
		fastchain:        fastBlockChain,
		snailchain:       snailBlockChain,
		committeeList:    make(map[uint64]*committee),
		fastChainEventCh:  make(chan types.ChainFastEvent, fastChainHeadSize),
		snailChainEventCh: make(chan types.ChainSnailEvent, snailchainHeadSize),
		singleNode:       config.NodeType,
	}

	// get genesis committee
	election.genesisCommittee = election.snailchain.GetGenesisCommittee()

	election.fastChainEventSub = election.fastchain.SubscribeChainEvent(election.fastChainEventCh)
	election.snailChainEventSub = election.snailchain.SubscribeChainEvent(election.snailChainEventCh)
	election.commiteeCache, _ = lru.New(committeeCacheLimit)

	if election.singleNode {
		var members []*types.CommitteeMember
		election.genesisCommittee = election.snailchain.GetGenesisCommittee()[:1]
		election.defaultMembers = members
	} else {
		election.defaultMembers = election.genesisCommittee[:4]
	}

	return election
}

//whether assigned publickey  in  committeeMember pubKey
func (e *Election) GetMemberByPubkey(members []*types.CommitteeMember, publickey []byte) *types.CommitteeMember {
	if len(members) == 0 {
		log.Error("GetMemberByPubkey method len(members)= 0")
		return nil
	}
	for _, member := range members {
		if bytes.Equal(publickey, crypto.FromECDSAPub(member.Publickey)) {
			return member
		}
	}
	return nil
}

func (e *Election) IsCommitteeMember(members []*types.CommitteeMember, publickey []byte) bool {
	if len(members) == 0 {
		log.Error("IsCommitteeMember method len(members)= 0")
		return false
	}
	for _, member := range members {
		if bytes.Equal(publickey, crypto.FromECDSAPub(member.Publickey)) {
			return true
		}
	}
	return false
}

func (e *Election) VerifyPublicKey(fastHeight *big.Int, pubKeyByte []byte) (*types.CommitteeMember, error) {
	members := e.GetCommittee(fastHeight)
	if members == nil {
		log.Info("GetCommittee members is nil", "fastHeight", fastHeight)
		return nil, ErrCommittee
	}
	member := e.GetMemberByPubkey(members, pubKeyByte)
	/*if member == nil {
		return nil, ErrInvalidMember
	}*/
	return member, nil
}

func (e *Election) VerifySign(sign *types.PbftSign) (*types.CommitteeMember, error) {
	pubkey, err := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
	if err != nil {
		return nil, err
	}
	pubkeyByte := crypto.FromECDSAPub(pubkey)
	member, err := e.VerifyPublicKey(sign.FastHeight, pubkeyByte)
	return member, err
}

//VerifySigns verify signatures of bft committee in batches
func (e *Election) VerifySigns(signs []*types.PbftSign) ([]*types.CommitteeMember, []error) {
	members := make([]*types.CommitteeMember, len(signs))
	errs := make([]error, len(signs))

	for i, sign := range signs {
		member, err := e.VerifySign(sign)
		if err != nil {
			errs[i] = err
			continue
		}
		if member == nil {
			errs[i] = ErrInvalidMember
		} else {
			members[i] = member
		}
	}
	/*for i, sign := range signs {
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
	}*/

	return members, errs
}

// getCommittee returns the committee members who propose this fast block
func (e *Election) getCommittee(fastNumber *big.Int, snailNumber *big.Int) *committee {
	log.Debug("get committee ..", "fastnumber", fastNumber, "snailnumber", snailNumber)
	committeeNumber, beginElectionNumber, endElectionNumber := electionBlocks(fastNumber, snailNumber)

	if committeeNumber.Cmp(common.Big0) == 0 {
		// genesis committee
		log.Debug("get genesis committee")
		return &committee{
			id:                  big.NewInt(0),
			beginFastNumber:     big.NewInt(0),
			endFastNumber:       big.NewInt(0),
			firstElectionNumber: big.NewInt(0),
			lastElectionNumber:  big.NewInt(0),
			switchCheckNumber:   params.ElectionPeriodNumber,
			members:             e.genesisCommittee,
		}
	}

	lastFastNumber := e.getLastNumber(endElectionNumber)
	if lastFastNumber == nil {
		return nil
	}
	log.Debug("check last fast block", "committee", committeeNumber, "last fast", lastFastNumber, "current", fastNumber)

	if lastFastNumber.Cmp(fastNumber) < 0 {
		log.Info("get committee", "electFirst", beginElectionNumber, "electLast", endElectionNumber, "lastFast", lastFastNumber)
		return e.electCommittee(committeeNumber)
	} else {
		elected := e.electCommittee(new(big.Int).Sub(committeeNumber, common.Big1))
		elected.endFastNumber = lastFastNumber
		return elected
	}
}

// GetCommittee gets committee members propose this fast block
func (e *Election) GetCommittee(fastNumber *big.Int) []*types.CommitteeMember {
	fastHeadNumber := e.fastchain.CurrentHeader().Number
	snailHeadNumber := e.snailchain.CurrentHeader().Number
	/*
		newestFast := new(big.Int).Add(fastHeadNumber, params.ElectionSwitchoverNumber)
		if fastNumber.Cmp(newestFast) > 0 {
			log.Info("get committee failed", "fastnumber", fastNumber, "currentNumber", fastHeadNumber)
			return nil
		}*/
	e.mu.RLock()
	currentCommittee := e.committee
	nextCommittee := e.nextCommittee
	e.mu.RUnlock()

	if nextCommittee != nil {
		//log.Debug("next committee info..", "id", nextCommittee.id, "firstNumber", nextCommittee.beginFastNumber)
		/*
		if new(big.Int).Add(nextCommittee.beginFastNumber, params.ElectionSwitchoverNumber).Cmp(fastNumber) < 0 {
			log.Info("get committee failed", "fastnumber", fastNumber, "nextFirstNumber", nextCommittee.beginFastNumber)
			return nil
		}*/
		if fastNumber.Cmp(nextCommittee.beginFastNumber) >= 0 {
			log.Debug("get committee nextCommittee", "fastNumber", fastNumber, "nextfast", nextCommittee.beginFastNumber)
			return nextCommittee.Members()
		}
	}
	if currentCommittee != nil {
		//log.Debug("current committee info..", "id", currentCommittee.id, "firstNumber", currentCommittee.beginFastNumber)
		if fastNumber.Cmp(currentCommittee.beginFastNumber) >= 0 {
			return currentCommittee.Members()
		}
	}

	fastBlock := e.fastchain.GetBlockByNumber(fastNumber.Uint64())
	if fastBlock == nil {
		log.Info("get committee failed (no fast block)", "fastnumber", fastNumber, "currentNumber", fastHeadNumber)
		return nil
	}
	// get snail number
	var snailNumber *big.Int
	snailBlock, _ := e.snailchain.GetFruitByFastHash(fastBlock.Hash())
	if snailBlock == nil {
		// fast block has not stored in snail chain
		// TODO: when fast number is so far away from snail block
		snailNumber = snailHeadNumber
	} else {
		snailNumber = snailBlock.Number()
	}

	committee := e.getCommittee(fastNumber, snailNumber)
	if committee == nil {
		return nil
	}

	return committee.Members()
}

func (e *Election) GetComitteeById(id *big.Int) map[string]interface{} {
	var members 	[]*types.CommitteeMember

	e.mu.RLock()
	currentCommittee := e.committee
	nextCommittee := e.nextCommittee
	e.mu.RUnlock()

	info := make(map[string]interface{})

	if currentCommittee.id.Cmp(id) == 0 {
		members = currentCommittee.Members()
	}
	if nextCommittee != nil {
		if nextCommittee.id.Cmp(id) == 0 {
			members = nextCommittee.Members()
		}
		if nextCommittee.id.Cmp(id) < 0 {
			return nil
		}
	} else {
		if currentCommittee.id.Cmp(id) < 0 {
			return nil
		}
	}

	if id.Cmp(common.Big0) <= 0 {
		// Use genesis committee
		info["id"] = 0
		info["beginSnailNumber"] = 0
		info["endSnailNumber"] = 0
		info["memberCount"] = len(e.genesisCommittee)
		info["members"] = membersDisplay(e.genesisCommittee)
		return info
	}

	elected := e.electCommittee(id)

	if members != nil {
		info["id"] = id.Uint64()
		info["memberCount"] = len(elected.members)
		info["startSnailNumber"] = elected.firstElectionNumber.Uint64()
		info["endSnailNumber"] = elected.lastElectionNumber.Uint64()
		info["members"] = membersDisplay(members)
		return info
	}

	return nil
}

func membersDisplay(members []*types.CommitteeMember) []map[string]interface{} {
	var attrs []map[string]interface{}
	for _, member := range members {
		attrs = append(attrs, map[string]interface{}{
			"coinbase": member.Coinbase,
			"PKey": hex.EncodeToString(crypto.FromECDSAPub(member.Publickey)),
		})
	}
	return attrs
}

// getCandinates get candinate miners and seed from given snail blocks
func (e *Election) getCandinates(snailBeginNumber *big.Int, snailEndNumber *big.Int) (common.Hash, []*candidateMember) {
	var fruitsCount map[common.Address]uint64 = make(map[common.Address]uint64)
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
					coinbase:   f.Coinbase(),
					publickey:  pubkey,
					address:    addr,
					difficulty: new(big.Int).Sub(act, diff),
				}

				members = append(members, member)
				if _, ok := fruitsCount[addr]; ok {
					fruitsCount[addr] += 1
				} else {
					fruitsCount[addr] = 1
				}
			}
		}
		blockNumber = new(big.Int).Add(blockNumber, big.NewInt(1))
	}

	log.Debug("get committee candidate", "fruit", len(members), "members", len(fruitsCount))

	var candidates []*candidateMember
	td := big.NewInt(0)
	for _, member := range members {
		if cnt, ok := fruitsCount[member.address]; ok {
			log.Trace("get committee candidate", "keyAddr", member.address, "count", cnt, "diff", member.difficulty)
			if cnt >= params.ElectionFruitsThreshold {
				td.Add(td, member.difficulty)

				candidates = append(candidates, member)
			}
		}
	}
	log.Debug("get final candidate", "count", len(candidates), "td", td)
	if len(candidates) == 0 {
		log.Warn("getCandinates not get candidates")
		return common.Hash{}, nil
	}

	dd := big.NewInt(0)
	rate := new(big.Int).Div(maxUint256, td)
	for i, member := range candidates {
		member.lower = new(big.Int).Mul(rate, dd)

		dd = new(big.Int).Add(dd, member.difficulty)

		if i == len(candidates)-1 {
			member.upper = new(big.Int).Set(maxUint256)
		} else {
			member.upper = new(big.Int).Mul(rate, dd)
		}

		log.Trace("get power", "member", member.address, "lower", member.lower, "upper", member.upper)
	}

	return crypto.Keccak256Hash(seed), candidates
}

func (e *Election) getLastNumber(endSnail *big.Int) *big.Int {
	if endSnail.Cmp(common.Big0) <=0 {
		return big.NewInt(0)
	}

	endElectionBlock := e.snailchain.GetBlockByNumber(endSnail.Uint64())
	if endElectionBlock == nil {
		return nil
	}
	fruits := endElectionBlock.Fruits()
	lastFruitNumber := fruits[len(fruits)-1].FastNumber()
	lastFastNumber := new(big.Int).Add(lastFruitNumber, params.ElectionSwitchoverNumber)

	return lastFastNumber
}

func (e *Election) electCommittee(committeeNumber *big.Int) *committee {
	if cache, ok := e.commiteeCache.Get(committeeNumber.Uint64()); ok {
		cachedCommittee := cache.(*committee)
		return cachedCommittee
	}

	beginElectionNumber, endElectionNumber := electionSection(committeeNumber)
	_, check := committeeSection(committeeNumber)
	// Cache committee which is stored in database
	if members := rawdb.ReadCommittee(e.snailchain.GetDatabase(), committeeNumber.Uint64()); members != nil {
		saved := &committee{
			id:                  committeeNumber,
			beginFastNumber:     new(big.Int).Add(e.getLastNumber(check), common.Big1),
			endFastNumber:       big.NewInt(0),
			firstElectionNumber: beginElectionNumber,
			lastElectionNumber:  endElectionNumber,
			switchCheckNumber:   check,
			members:             members,
		}
		e.commiteeCache.Add(committeeNumber.Uint64(), saved)

		log.Info("Read committee from db", "committee", committeeNumber)
		return saved
	}

	// Elect new committee
	members := e.electMembers(beginElectionNumber, endElectionNumber)
	elected := &committee{
		id:                  committeeNumber,
		beginFastNumber:     new(big.Int).Add(e.getLastNumber(check), common.Big1),
		endFastNumber:       big.NewInt(0),
		firstElectionNumber: beginElectionNumber,
		lastElectionNumber:  endElectionNumber,
		switchCheckNumber:   check,
		members:             members,
	}
	rawdb.WriteCommittee(e.snailchain.GetDatabase(), committeeNumber.Uint64(), elected.members)
	e.commiteeCache.Add(committeeNumber.Uint64(), elected)
	return elected
}

// elect is a lottery function that select committee members from candidates miners
func (e *Election) elect(candidates []*candidateMember, seed common.Hash) []*types.CommitteeMember {
	var addrs map[common.Address]uint = make(map[common.Address]uint)
	var members []*types.CommitteeMember

	log.Debug("elect committee members ..", "count", len(candidates), "seed", seed)
	round := new(big.Int).Set(common.Big1)
	for {
		seedNumber := new(big.Int).Add(seed.Big(), round)
		hash := crypto.Keccak256Hash(seedNumber.Bytes())
		//prop := new(big.Int).Div(maxUint256, hash.Big())
		prop := hash.Big()

		for _, cm := range candidates {
			if prop.Cmp(cm.lower) < 0 {
				continue
			}
			if prop.Cmp(cm.upper) >= 0 {
				continue
			}

			log.Trace("get member", "seed", hash, "member", cm.address, "prop", prop)
			if _, ok := addrs[cm.address]; ok {
				break
			}
			addrs[cm.address] = 1
			member := &types.CommitteeMember{
				Coinbase:  cm.coinbase,
				Publickey: cm.publickey,
			}
			members = append(members, member)

			break
		}

		round = new(big.Int).Add(round, common.Big1)
		if round.Cmp(params.MaximumCommitteeNumber) > 0 {
			//if len(members) >= minCommitteeNumber {
			break
			//}
		}
	}

	log.Debug("get new committee members", "count", len(members))

	return members
}

// electMembers elect committee members from snail block.
func (e *Election) electMembers(snailBeginNumber *big.Int, snailEndNumber *big.Int) []*types.CommitteeMember {
	log.Info("elect new committee..", "begin", snailBeginNumber, "end", snailEndNumber, "threshold", params.ElectionFruitsThreshold, "max", params.MaximumCommitteeNumber)

	var committee []*types.CommitteeMember

	for _, member := range e.defaultMembers {
		committee = append(committee, member)
	}
	seed, candidates := e.getCandinates(snailBeginNumber, snailEndNumber)
	if candidates == nil {
		log.Info("can't get new committee, retain current committee")
	} else {
		members := e.elect(candidates, seed)

		for _, member := range members {
			committee = append(committee, member)
		}
	}

	return committee
}

func (e *Election) Start() error {
	// get current committee info
	fastHeadNumber := e.fastchain.CurrentBlock().Number()
	snailHeadNumber := e.snailchain.CurrentBlock().Number()

	currentCommittee := e.getCommittee(fastHeadNumber, snailHeadNumber)
	if currentCommittee == nil {
		return nil
	}

	e.committee = currentCommittee

	if currentCommittee.endFastNumber.Cmp(common.Big0) > 0 {
		// over the switch block, to elect next committee
		// get next committee
		e.nextCommittee = e.electCommittee(new(big.Int).Add(e.committee.id, common.Big1))
		// start switchover
		e.startSwitchover = true

		if e.committee.endFastNumber.Cmp(fastHeadNumber) == 0 {
			// committee has finish their work, start the new committee
			e.committee = e.nextCommittee
			e.nextCommittee = nil

			e.startSwitchover = false
		}
	}

	// send event to the subscripber
	go func(e *Election) {

		PrintCommittee(e.committee)
		e.electionFeed.Send(types.ElectionEvent{
			Option:           types.CommitteeSwitchover,
			CommitteeID:      e.committee.id,
			CommitteeMembers: e.committee.Members(),
			BeginFastNumber:  e.committee.beginFastNumber,
		})
		e.electionFeed.Send(types.ElectionEvent{
			Option:           types.CommitteeStart,
			CommitteeID:      e.committee.id,
			CommitteeMembers: e.committee.Members(),
			BeginFastNumber:  e.committee.beginFastNumber,
		})

		if e.startSwitchover {
			PrintCommittee(e.nextCommittee)
			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeOver,
				CommitteeID:      e.committee.id,
				CommitteeMembers: e.committee.Members(),
				BeginFastNumber:  e.committee.beginFastNumber,
				EndFastNumber:    e.committee.endFastNumber,
			})
			// send switch event to the subscripber
			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      e.nextCommittee.id,
				CommitteeMembers: e.nextCommittee.Members(),
				BeginFastNumber:  e.nextCommittee.beginFastNumber,
			})
		}
	}(e)

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
		case se := <-e.snailChainEventCh:
			if se.Block != nil {
				//Record Numbers to open elections
				if e.committee.switchCheckNumber.Cmp(se.Block.Number()) == 0 {
					e.committee.endFastNumber = e.getLastNumber(
						new(big.Int).Add(e.committee.lastElectionNumber, params.ElectionPeriodNumber))
					e.electionFeed.Send(types.ElectionEvent{
						Option:           types.CommitteeOver,
						CommitteeID:      e.committee.id,
						CommitteeMembers: e.committee.Members(),
						BeginFastNumber:  e.committee.beginFastNumber,
						EndFastNumber:    e.committee.endFastNumber,
					})
					log.Info("Election BFT committee elect start", "snail", se.Block.Number(), "endfast", e.committee.endFastNumber, "members", len(e.committee.members))
					nextCommittee := e.electCommittee(new(big.Int).Add(e.committee.id, common.Big1))

					if e.nextCommittee != nil {
						if e.nextCommittee.id.Cmp(nextCommittee.id) == 0 {
							// get next committee twice
							continue
						}
					}
					e.mu.Lock()
					e.nextCommittee = nextCommittee
					e.startSwitchover = true
					e.mu.Unlock()

					log.Info("Election switchover new committee", "id", e.nextCommittee.id, "startNumber", e.nextCommittee.beginFastNumber)
					PrintCommittee(e.nextCommittee)

					e.electionFeed.Send(types.ElectionEvent{
						Option:           types.CommitteeSwitchover,
						CommitteeID:      e.nextCommittee.id,
						CommitteeMembers: e.nextCommittee.Members(),
						BeginFastNumber:  e.nextCommittee.beginFastNumber,
					})
				}
			}
		// Switch to new committee based on the fastblock number
		case ev := <-e.fastChainEventCh:
			if ev.Block != nil {
				if e.startSwitchover {
					if e.committee.endFastNumber.Cmp(ev.Block.Number()) == 0 {
						log.Info("Election stop committee..", "id", e.committee.id)
						e.electionFeed.Send(types.ElectionEvent{
							Option:           types.CommitteeStop,
							CommitteeID:      e.committee.id,
							CommitteeMembers: e.committee.Members(),
							BeginFastNumber:  e.committee.beginFastNumber,
							EndFastNumber:    e.committee.endFastNumber,
						})

						e.mu.Lock()
						e.committee = e.nextCommittee
						e.nextCommittee = nil
						e.mu.Unlock()

						e.startSwitchover = false

						log.Info("Election start new BFT committee", "id", e.committee.id, "startNumber", e.committee.beginFastNumber)

						e.electionFeed.Send(types.ElectionEvent{
							Option:           types.CommitteeStart,
							CommitteeID:      e.committee.id,
							CommitteeMembers: e.committee.Members(),
							BeginFastNumber:  e.committee.beginFastNumber,
						})
					}
				}
			}
		}
	}
}

func (e *Election) SubscribeElectionEvent(ch chan<- types.ElectionEvent) event.Subscription {
	return e.scope.Track(e.electionFeed.Subscribe(ch))
}

func (e *Election) SetEngine(engine consensus.Engine) {
	e.engine = engine
}


func PrintCommittee(c *committee) {
	log.Info("Committee Info", "ID", c.id, "count", len(c.members), "start", c.beginFastNumber)
	for _, member := range c.members {
		key := crypto.FromECDSAPub(member.Publickey)
		log.Info("Committee member: ", "coinbase", member.Coinbase, "PKey", hex.EncodeToString(key))
	}
}
