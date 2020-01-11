// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package consensus implements different Ethereum consensus engines.
package consensus

import (
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
)

// ChainReader defines a small collection of methods needed to access the local
// blockchain during header and/or uncle verification.
type ChainReader interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *params.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() *types.Header

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *types.Header

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.Header

	// GetHeaderByHash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash common.Hash) *types.Header

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *types.Block

	GetBlockReward(snumber uint64) *types.BlockReward
}

// SnailChainReader defines a small collection of methods needed to access the local
// block chain during header and/or uncle verification.
// Temporary interface for snail
type SnailChainReader interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *params.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() *types.SnailHeader

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *types.SnailHeader

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.SnailHeader

	// GetHeaderByHash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash common.Hash) *types.SnailHeader

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *types.SnailBlock
}

// Engine is an algorithm agnostic consensus engine.
type Engine interface {
	SetElection(e CommitteeElection)

	GetElection() CommitteeElection

	SetSnailChainReader(scr SnailChainReader)

	SetSnailHeaderHash(db etruedb.Database)
	// Author retrieves the Ethereum address of the account that minted the given
	// block, which may be different from the header's coinbase if a consensus
	// engine is based on signatures.
	Author(header *types.Header) (common.Address, error)
	AuthorSnail(header *types.SnailHeader) (common.Address, error)

	// VerifyHeader checks whether a header conforms to the consensus rules of a
	// given engine. Verifying the seal may be done optionally here, or explicitly
	// via the VerifySeal method.
	VerifyHeader(chain ChainReader, header *types.Header) error
	VerifySnailHeader(chain SnailChainReader, fastchain ChainReader, header *types.SnailHeader, seal bool, isFruit bool) error

	// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
	// concurrently. The method returns a quit channel to abort the operations and
	// a results channel to retrieve the async verifications (the order is that of
	// the input slice).
	VerifyHeaders(chain ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

	// VerifySnailHeaders is similar to VerifySnailHeader, but verifies a batch of headers concurrently.
	// VerifySnailHeaders only verifies snail header rather than fruit header.
	// The method returns a quit channel to abort the operations and
	// a results channel to retrieve the async verifications (the order is that of
	// the input slice).
	VerifySnailHeaders(chain SnailChainReader, headers []*types.SnailHeader, seals []bool) (chan<- struct{}, <-chan error)

	ValidateRewarded(number uint64, hash common.Hash, fastchain ChainReader) error

	ValidateFruitHeader(block *types.SnailHeader, fruit *types.SnailHeader, snailchain SnailChainReader, fastchain ChainReader, checkpoint uint64) error
	// VerifySeal checks whether the crypto seal on a header is valid according to
	// the consensus rules of the given engine.
	VerifySnailSeal(chain SnailChainReader, header *types.SnailHeader, isFruit bool) error

	VerifyFreshness(chain SnailChainReader, fruit *types.SnailHeader, headerNumber *big.Int, canonical bool) error

	VerifySigns(fastnumber *big.Int, fastHash common.Hash, signs []*types.PbftSign) error

	VerifySwitchInfo(fastnumber *big.Int, info []*types.CommitteeMember) error

	// Prepare initializes the consensus fields of a block header according to the
	// rules of a particular engine. The changes are executed inline.
	Prepare(chain ChainReader, header *types.Header) error
	PrepareSnail(chain ChainReader, snailchain SnailChainReader, header *types.SnailHeader) error
	PrepareSnailWithParent(chain ChainReader, snailchain SnailChainReader, header *types.SnailHeader, parents []*types.SnailHeader) error

	// Finalize runs any post-transaction state modifications (e.g. block rewards)
	// and assembles the final block.
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	Finalize(chain ChainReader, header *types.Header, state *state.StateDB,
		txs []*types.Transaction, receipts []*types.Receipt, feeAmount *big.Int) (*types.Block, error)
	FinalizeSnail(chain SnailChainReader, header *types.SnailHeader,
		uncles []*types.SnailHeader, fruits []*types.SnailBlock, signs []*types.PbftSign) (*types.SnailBlock, error)

	FinalizeCommittee(block *types.Block) error

	// Seal generates a new block for the given input block with the local miner's
	Seal(chain SnailChainReader, block *types.SnailBlock, stop <-chan struct{}) (*types.SnailBlock, error)

	// ConSeal generates a new block for the given input block with the local miner's
	// seal place on top.
	ConSeal(chain SnailChainReader, block *types.SnailBlock, stop <-chan struct{}, send chan *types.SnailBlock)

	CalcSnailDifficulty(chain SnailChainReader, time uint64, parents []*types.SnailHeader) *big.Int

	GetDifficulty(header *types.SnailHeader, isFruit bool) (*big.Int, *big.Int)

	// APIs returns the RPC APIs this consensus engine provides.
	APIs(chain ChainReader) []rpc.API

	DataSetHash(epoch uint64) string

	GetRewardContentBySnailNumber(sBlock *types.SnailBlock) *types.SnailRewardContenet
}

//CommitteeElection module implementation committee interface
type CommitteeElection interface {
	// VerifySigns verify the fast chain committee signatures in batches
	VerifySigns(pvs []*types.PbftSign) ([]*types.CommitteeMember, []error)

	// VerifySwitchInfo verify committee members and it's state
	VerifySwitchInfo(fastnumber *big.Int, info []*types.CommitteeMember) error

	FinalizeCommittee(block *types.Block) error

	//Get a list of committee members
	//GetCommittee(FastNumber *big.Int, FastHash common.Hash) (*big.Int, []*types.CommitteeMember)
	GetCommittee(fastNumber *big.Int) []*types.CommitteeMember

	GenerateFakeSigns(fb *types.Block) ([]*types.PbftSign, error)
}

// PoW is a consensus engine based on proof-of-work.
type PoW interface {
	Engine

	// Hashrate returns the current mining hashrate of a PoW consensus engine.
	Hashrate() float64
}

func IsTIP8(fastHeadNumber *big.Int, config *params.ChainConfig, reader SnailChainReader) bool {

	if config.TIP8.FastNumber != nil && config.TIP8.FastNumber.Sign() > 0 {
		return fastHeadNumber.Cmp(config.TIP8.FastNumber) >= 0
	}

	oldID := big.NewInt(0)
	var lastFast *big.Int
	if reader != nil {
		snailHeadNumber := reader.CurrentHeader().Number
		oldID = new(big.Int).Div(snailHeadNumber, params.ElectionPeriodNumber)
		lastFast = getEndOfOldEpoch(oldID, reader)
	}

	if lastFast == nil {
		res := oldID.Cmp(config.TIP8.CID)
		if res <= 0 {
			return false
		} else {
			return true
		}
	} else {
		updateForkedPoint(oldID, lastFast, config)
	}
	return config.IsTIP8(oldID, fastHeadNumber)
}
func getEndOfOldEpoch(eid *big.Int, reader SnailChainReader) *big.Int {

	switchCheckNumber := new(big.Int).Mul(new(big.Int).Add(eid, common.Big1), params.ElectionPeriodNumber)
	snailEndNumber := new(big.Int).Sub(switchCheckNumber, params.SnailConfirmInterval)

	header := reader.GetHeaderByNumber(snailEndNumber.Uint64())
	if header == nil {
		return nil
	}
	block := reader.GetBlock(header.Hash(), snailEndNumber.Uint64())
	if block == nil {
		return nil
	}

	fruits := block.Fruits()
	lastFruitNumber := fruits[len(fruits)-1].FastNumber()
	lastFastNumber := new(big.Int).Add(lastFruitNumber, params.ElectionSwitchoverNumber)

	return lastFastNumber
}
func updateForkedPoint(forkedID, fastNumber *big.Int, config *params.ChainConfig) {
	if config.TIP8.CID.Cmp(forkedID) == 0 && config.TIP8.FastNumber.Sign() == 0 && fastNumber != nil {
		params.DposForkPoint = fastNumber.Uint64()
		config.TIP8.FastNumber = new(big.Int).Set(fastNumber)
	}
}

func InitTIP8(config *params.ChainConfig, reader SnailChainReader) {
	eid := config.TIP8.CID
	params.FirstNewEpochID = new(big.Int).Add(eid, common.Big1).Uint64()
	switchCheckNumber := new(big.Int).Mul(new(big.Int).Add(eid, common.Big1), params.ElectionPeriodNumber)
	curSnailNumber := reader.CurrentHeader().Number
	if curSnailNumber.Cmp(switchCheckNumber) >= 0 {
		snailEndNumber := new(big.Int).Sub(switchCheckNumber, params.SnailConfirmInterval)
		header := reader.GetHeaderByNumber(snailEndNumber.Uint64())
		if header == nil {
			log.Error("InitTIP8 GetHeaderByNumber failed.", "switchCheckNumber", switchCheckNumber, "curSnailNumber", curSnailNumber, "Epochid", eid)
			return
		}
		block := reader.GetBlock(header.Hash(), snailEndNumber.Uint64())
		if block == nil {
			log.Error("InitTIP8 GetBlock failed.", "switchCheckNumber", switchCheckNumber, "curSnailNumber", curSnailNumber, "Epochid", eid)
			return
		}
		fruits := block.Fruits()
		lastFruitNumber := fruits[len(fruits)-1].FastNumber()
		config.TIP8.FastNumber = new(big.Int).Add(lastFruitNumber, params.ElectionSwitchoverNumber)
	}
}
