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

package les

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/hashicorp/golang-lru"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/light"
	"github.com/truechain/truechain-engineering-code/light/fast"
	"github.com/truechain/truechain-engineering-code/consensus/election"
)

const (
	snailchainHeadSize  = 64
	committeeCacheLimit = 256
)

var (
	ErrCommittee     = errors.New("get committee failed")
	ErrInvalidMember = errors.New("invalid committee member")
	ErrInvalidSwitch = errors.New("invalid switch block info")
)

type Election struct {
	genesisCommittee []*types.CommitteeMember
	defaultMembers   []*types.CommitteeMember

	fastchain  *fast.LightChain
	snailchain *light.LightChain

	commiteeCache *lru.Cache
}

func ElectionEpoch(id *big.Int) (begin *big.Int, end *big.Int) {
	end = new(big.Int).Mul(id, params.ElectionPeriodNumber)
	end = end.Sub(end, params.SnailConfirmInterval)
	if id.Cmp(common.Big1) <= 0 {
		begin = big.NewInt(1)
	} else {
		begin = new(big.Int).Add(new(big.Int).Sub(end, params.ElectionPeriodNumber), common.Big1)
	}
	return
}

// NewLightElection create the instance of committee electioin
func NewLightElection(fastBlockChain *fast.LightChain, snailBlockChain *light.LightChain) *Election {
	// init
	election := &Election{
		fastchain:         fastBlockChain,
		snailchain:        snailBlockChain,
	}
	election.commiteeCache, _ = lru.New(committeeCacheLimit)
	return election
}

func (e *Election) Start() {
	num := e.fastchain.CurrentHeader().Number
	snail := e.snailchain.CurrentHeader().Number

	log.Info("Latest block", "number", num)
	log.Info("Latest snail", "number", snail)
	e.GetCommittee(num)
}

func (e *Election) GenerateFakeSigns(fb *types.Block) ([]*types.PbftSign, error) {
	return nil, nil
}

// GetMemberByPubkey returns committeeMember specified by public key bytes
func (e *Election) GetMemberByPubkey(members []*types.CommitteeMember, publickey []byte) *types.CommitteeMember {
	if len(members) == 0 {
		log.Error("GetMemberByPubkey method len(members)= 0")
		return nil
	}
	for _, member := range members {
		if bytes.Equal(publickey, member.Publickey) {
			return member
		}
	}
	return nil
}

func (e *Election) GetMemberFlag(members []*types.CommitteeMember, publickey []byte) uint32 {
	if len(members) == 0 {
		log.Error("IsCommitteeMember method len(members)= 0")
		return 0
	}
	for _, member := range members {
		if bytes.Equal(publickey, member.Publickey) {
			return member.Flag
		}
	}
	return 0
}

// IsCommitteeMember reports whether the provided public key is in committee
func (e *Election) IsCommitteeMember(members []*types.CommitteeMember, publickey []byte) bool {
	flag := e.GetMemberFlag(members, publickey)
	return flag == types.StateUsedFlag
}

// VerifyPublicKey get the committee member by public key
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

// VerifySign lookup the pbft sign and return the committee member who signs it
func (e *Election) VerifySign(sign *types.PbftSign) (*types.CommitteeMember, error) {
	pubkey, err := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
	if err != nil {
		return nil, err
	}
	pubkeyByte := crypto.FromECDSAPub(pubkey)
	member, err := e.VerifyPublicKey(sign.FastHeight, pubkeyByte)
	return member, err
}

// VerifySigns verify signatures of bft committee in batches
func (e *Election) VerifySigns(signs []*types.PbftSign) ([]*types.CommitteeMember, []error) {
	members := make([]*types.CommitteeMember, len(signs))
	errs := make([]error, len(signs))

	if len(signs) == 0 {
		log.Warn("Veriry signs get nil pbftsigns")
		return nil, nil
	}
	// All signs should have the same fastblock height
	committeeMembers := e.GetCommittee(signs[0].FastHeight)
	if len(committeeMembers) == 0 {
		log.Error("Election get none committee for verify pbft signs")
		for i := range errs {
			errs[i] = ErrCommittee
		}
		return members, errs
	}

	for i, sign := range signs {
		// member, err := e.VerifySign(sign)
		pubkey, _ := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
		member := e.GetMemberByPubkey(committeeMembers, crypto.FromECDSAPub(pubkey))
		if member == nil {
			errs[i] = ErrInvalidMember
		} else {
			members[i] = member
		}
	}
	return members, errs
}

// VerifySwitchInfo verify committee members and it's state
func (e *Election) VerifySwitchInfo(fastNumber *big.Int, info []*types.CommitteeMember) error {
	return nil
}

// GetCommittee gets committee members which propose the fast block
func (e *Election) GetCommittee(fastNumber *big.Int) []*types.CommitteeMember {
	var (
		id    *big.Int
		snail *big.Int
		c     *types.ElectionCommittee
	)

	blockHead := e.fastchain.GetHeaderByNumber(fastNumber.Uint64())
	if fruitHead := e.snailchain.GetFruitHeaderByHash(blockHead.Hash()); fruitHead != nil {
		snail = fruitHead.Number
	} else {
		snail = e.snailchain.CurrentHeader().Number
	}

	id = new(big.Int).Div(snail, params.ElectionPeriodNumber)
	if id.Cmp(common.Big0) == 0 {
		c = e.getCommittee(common.Big0)
		return c.Members
	}
	_, end := ElectionEpoch(id)
	fruitNum := e.GetEndFruitNumber(end)

	if fastNumber.Cmp(new(big.Int).Add(fruitNum, params.ElectionSwitchoverNumber)) > 0 {
		c = e.getCommittee(id)
	} else {
		c = e.getCommittee(new(big.Int).Sub(id, common.Big1))
	}
	return c.Members
}

func (e *Election) getCommittee(id *big.Int) *types.ElectionCommittee {
	if cache, ok := e.commiteeCache.Get(id.Uint64()); ok {
		committee := cache.(*types.ElectionCommittee)
		return committee
	}

	var c *types.ElectionCommittee
	if id.Cmp(common.Big0) == 0 {
		// genesis committee for committee 0
		c = &types.ElectionCommittee{Members: e.genesisCommittee}
	} else {
		// elect committee based on snail fruits
		begin, end := ElectionEpoch(id)
		c = election.ElectCommittee(e.snailchain, nil, begin, end)
	}
	e.commiteeCache.Add(id.Uint64(), c)
	return c
}

func (e *Election) GetEndFruitNumber(snail *big.Int) *big.Int {
	fruits := e.snailchain.GetFruitsHead(snail.Uint64())
	return fruits[len(fruits)-1].FastNumber
}

// FinalizeCommittee upddate current committee state
func (e *Election) FinalizeCommittee(block *types.Block) error {
	return nil
}