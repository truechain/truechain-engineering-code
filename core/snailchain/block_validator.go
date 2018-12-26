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

package snailchain

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/core"

	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	// ErrInvalidSignHash is returned if the fruit contains an invalid signatures hash.
	ErrInvalidSignHash = errors.New("invalid sign")

	//ErrInvalidFast is returned if the fastchain not have the hash
	ErrInvalidFast = errors.New("invalid fast hash")

	//ErrNoFruits is returned if the block not contains the exact fruit count
	ErrNoFruits = errors.New("invalid fruits count")

	//ErrInvalidFruits is returned if the fruits in block not continuity
	ErrInvalidFruits = errors.New("invalid fruits number")
)

// BlockValidator is responsible for validating block headers, uncles and
// processed state.
//
// BlockValidator implements Validator.
type BlockValidator struct {
	config *params.ChainConfig // Chain configuration options
	bc     *SnailBlockChain    // Canonical block chain

	engine consensus.Engine // Consensus engine used for validating
	//election  consensus.CommitteeElection
	fastchain *core.BlockChain
}

// NewBlockValidator returns a new block validator which is safe for re-use
func NewBlockValidator(config *params.ChainConfig, fc *core.BlockChain, sc *SnailBlockChain, engine consensus.Engine) *BlockValidator {
	validator := &BlockValidator{
		config:    config,
		engine:    engine,
		fastchain: fc,
		bc:        sc,
	}
	return validator
}

// ValidateBody validates the given block's uncles and verifies the the block
// header's transaction and uncle roots. The headers are assumed to be already
// validated at this point.
func (v *BlockValidator) ValidateBody(block *types.SnailBlock) error {
	// Check whether the block's known, and if not, that it's linkable
	if v.bc.HasBlockAndState(block.Hash(), block.NumberU64()) {
		return ErrKnownBlock
	}
	if !v.bc.HasBlockAndState(block.ParentHash(), block.NumberU64()-1) {
		if !v.bc.HasBlock(block.ParentHash(), block.NumberU64()-1) {
			return consensus.ErrUnknownAncestor
		}
		return consensus.ErrPrunedAncestor
	}
	// Header validity is known at this point, check the uncles and transactions
	header := block.Header()
	//if err := v.engine.VerifySnailUncles(v.bc, block); err != nil {
	//	return err
	//}

	count := len(block.Fruits())
	if count == 0 {
		return ErrNoFruits
	}
	if count > params.MaximumFruits || count < params.MinimumFruits {
		return ErrNoFruits
	}

	temp := uint64(0)
	preBlock := v.bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
	if preBlock == nil {
		log.Info("ValidateBody snail get parent block error", "block", block.Number(), "hash", block.Hash(), "parent", block.ParentHash())
		return consensus.ErrUnknownAncestor
	}
	if preBlock.Number().Cmp(common.Big0) > 0 {
		localFruits := preBlock.Fruits()
		temp = localFruits[len(localFruits)-1].FastNumber().Uint64()
	}
	fruits := block.Fruits()
	for _, fruit := range fruits {
		if fruit.FastNumber().Uint64()-temp != 1 {
			log.Info("ValidateBody snail validate fruit error", "block", block.Number(), "first", fruits[0].FastNumber(), "count", len(fruits),
				"fruit", fruit.FastNumber(), "pre", temp)
			return ErrInvalidFruits
		}
		if err := v.ValidateFruit(fruit, block, false); err != nil {
			log.Info("ValidateBody snail validate fruit error", "block", block.Number(), "fruit", fruit.FastNumber(), "err", err)
			return err
		}

		temp = fruit.FastNumber().Uint64()
	}

	if hash := types.DeriveSha(types.Fruits(block.Fruits())); hash != header.FruitsHash {
		return fmt.Errorf("transaction root hash mismatch: have %x, want %x", hash, header.FruitsHash)
	}
	return nil
}

// ValidateState validates the various changes that happen after a state
// transition, such as amount of used gas, the receipt roots and the state root
// itself. ValidateState returns a database batch if the validation was a success
// otherwise nil and an error is returned.
func (v *BlockValidator) ValidateState(block, parent *types.SnailBlock, statedb *state.StateDB, receipts types.Receipts, usedGas uint64) error {
	header := block.Header()

	// Validate the received block's bloom with the one derived from the generated receipts.
	// For valid blocks this should always validate to true.
	rbloom := types.CreateBloom(receipts)
	if rbloom != header.Bloom {
		return fmt.Errorf("invalid bloom (remote: %x  local: %x)", header.Bloom, rbloom)
	}

	return nil
}

//ValidateFruit is to verify if the fruit is legal
func (v *BlockValidator) ValidateFruit(fruit, block *types.SnailBlock, canonical bool) error {
	//check number(fb)
	//
	currentNumber := v.fastchain.CurrentHeader().Number
	if fruit.FastNumber().Cmp(currentNumber) > 0 {
		return consensus.ErrFutureBlock
	}

	fb := v.fastchain.GetBlock(fruit.FastHash(), fruit.FastNumber().Uint64())
	if fb == nil {
		return ErrInvalidFast
	}

	//check integrity
	getSignHash := types.CalcSignHash(fruit.Signs())
	if fruit.Header().SignHash != getSignHash {
		log.Info("valid fruit sign hash failed.")
		return ErrInvalidSignHash
	}

	// check freshness
	var blockHeader *types.SnailHeader
	if block != nil {
		blockHeader = block.Header()
	}
	err := v.engine.VerifyFreshness(v.bc, fruit.Header(), blockHeader, canonical)
	if err != nil {
		log.Debug("ValidateFruit verify freshness error.", "err", err, "fruit", fruit.FastNumber())
		return err
	}

	header := fruit.Header()
	if err := v.engine.VerifySnailHeader(v.bc, v.fastchain, header, true); err != nil {
		log.Info("validate fruit verify failed.", "err", err)
		return err
	}

	// validate the signatures of this fruit
	if err := v.engine.VerifySigns(fruit.FastNumber(), fruit.FastHash(), fruit.Signs()); err != nil {
		log.Info("validate fruit VerifySigns failed.", "err", err)
		return err
	}

	return nil
}
