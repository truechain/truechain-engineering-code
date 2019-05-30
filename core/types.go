// Copyright 2015 The go-ethereum Authors
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

package core

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/params"
)

// Validator is an interface which defines the standard for block validation. It
// is only responsible for validating block contents, as the header validation is
// done by the specific consensus engines.
//
type Validator interface {
	// ValidateBody validates the given block's content.
	ValidateBody(block *types.Block, validateSign bool) error

	// ValidateState validates the given statedb and optionally the receipts and
	// gas used.
	ValidateState(block, parent *types.Block, state *state.StateDB, receipts types.Receipts, usedGas uint64) error
}

// Processor is an interface for processing blocks using a given initial state.
//
// Process takes the block to be processed and the statedb upon which the
// initial state is based. It should return the receipts generated, amount
// of gas used in the process and return an error if any of the internal rules
// failed.
type Processor interface {
	Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error)
}

// SnailValidator is an interface which defines the standard for block validation. It
// is only responsible for validating block contents, as the header validation is
// done by the specific consensus engines.
//
type SnailValidator interface {
	// SetElection set election
	//SetElection(e consensus.CommitteeElection, fc consensus.ChainReader) error

	// ValidateBody validates the given block's content.
	ValidateBody(block *types.SnailBlock, verifyFruits bool) error

	// ValidateFruit validates the given fruit's content
	ValidateFruit(fruit, block *types.SnailBlock, canonical bool) error
	// VerifySnailSeal checking whether the given block satisfies
	// the PoW difficulty requirements.
	VerifySnailSeal(chain consensus.SnailChainReader, header *types.SnailHeader, isFruit bool) error
	// ValidateRewarded validates the given block if rewarded
	ValidateRewarded(number uint64) error
}

// SnailChain is an interface which defines the standard for snail block.
type SnailChain interface {
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

	// CurrentBlock retrieves the current block from the local chain.
	CurrentBlock() *types.SnailBlock

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *types.SnailBlock

	// GetBlockByNumber retrieves a snail block from the database by number.
	GetBlockByNumber(number uint64) *types.SnailBlock

	// GetBlockByHash retrieves a snail block from the database by its hash.
	GetBlockByHash(hash common.Hash) *types.SnailBlock

	SubscribeChainHeadEvent(ch chan<- types.SnailChainHeadEvent) event.Subscription

	Validator() SnailValidator
}
