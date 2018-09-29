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
	"github.com/truechain/truechain-engineering-code/log"
	"math/big"

	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSign = errors.New("invalid sign")

	ErrInvalidPointer = errors.New("invalid pointer block")

	ErrExist = errors.New("already exist")

	ErrNotExist = errors.New("not exist")

	ErrInvalidHash = errors.New("invalid hash")

	ErrInvalidFast = errors.New("invalid fast hash")

	ErrNoFruits = errors.New("no fruits included")
)

// BlockValidator is responsible for validating block headers, uncles and
// processed state.
//
// BlockValidator implements Validator.
type BlockValidator struct {
	config   *params.ChainConfig // Chain configuration options
	bc       *SnailBlockChain    // Canonical block chain

	engine   consensus.Engine    // Consensus engine used for validating
	election consensus.CommitteeElection
	fastchain consensus.ChainReader
}

// freshFruitSize is the freshness of fruit according to the paper
var fruitFreshness *big.Int = big.NewInt(17)

// NewBlockValidator returns a new block validator which is safe for re-use
func NewBlockValidator(config *params.ChainConfig, blockchain *SnailBlockChain, engine consensus.Engine) *BlockValidator {
	validator := &BlockValidator{
		config: config,
		engine: engine,
		bc:     blockchain,
	}
	return validator
}

func (v *BlockValidator) SetElection(e consensus.CommitteeElection, fc consensus.ChainReader) error {
	v.election = e
	v.fastchain = fc

	return nil
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
	//header := block.Header()
	//if err := v.engine.VerifySnailUncles(v.bc, block); err != nil {
	//	return err
	//}

	if len(block.Fruits()) == 0 {
		return ErrNoFruits
	}

	for _, fruit := range block.Fruits() {
		if err := v.ValidateFruit(fruit, block); err != nil {
			log.Info("ValidateBody snail validate fruit error",  "fruit", fruit.FastNumber(), "block", block.Number(), "err", err)
			return err
		}
	}

	// TODO need add uncles or transaction at snail block 20180804
	/*
		if hash := types.CalcUncleHash(block.Uncles()); hash != header.UncleHash {
			return fmt.Errorf("uncle root hash mismatch: have %x, want %x", hash, header.UncleHash)
		}
		if hash := types.DeriveSha(block.Transactions()); hash != header.TxHash {
			return fmt.Errorf("transaction root hash mismatch: have %x, want %x", hash, header.TxHash)
		}*/
	return nil
}

// ValidateState validates the various changes that happen after a state
// transition, such as amount of used gas, the receipt roots and the state root
// itself. ValidateState returns a database batch if the validation was a success
// otherwise nil and an error is returned.

func (v *BlockValidator) ValidateState(block, parent *types.SnailBlock, statedb *state.StateDB, receipts types.Receipts, usedGas uint64) error {
	header := block.Header()
	//TODO need add gas for snail block 20180804
	/*
		if block.GasUsed() != usedGas {
			return fmt.Errorf("invalid gas used (remote: %d local: %d)", block.GasUsed(), usedGas)
		}
	*/
	// Validate the received block's bloom with the one derived from the generated receipts.
	// For valid blocks this should always validate to true.
	rbloom := types.CreateBloom(receipts)
	if rbloom != header.Bloom {
		return fmt.Errorf("invalid bloom (remote: %x  local: %x)", header.Bloom, rbloom)
	}

	return nil
}

// CalcGasLimit computes the gas limit of the next block after parent.
// This is miner strategy, not consensus protocol.


func CalcGasLimit(parent *types.SnailBlock) uint64 {
	// contrib = (parentGasUsed * 3 / 2) / 1024

	// TODO need add gas limit 20180804
	//fmt.Printf("Block_Validator calcGasLimit not function")
	return 0
	/*
		contrib := (parent.GasUsed() + parent.GasUsed()/2) / params.GasLimitBoundDivisor

		// decay = parentGasLimit / 1024 -1
		decay := parent.GasLimit()/params.GasLimitBoundDivisor - 1
	*/
	/*
		strategy: gasLimit of block-to-mine is set based on parent's
		gasUsed value.  if parentGasUsed > parentGasLimit * (2/3) then we
		increase it, otherwise lower it (or leave it unchanged if it's right
		at that usage) the amount increased/decreased depends on how far away
		from parentGasLimit * (2/3) parentGasUsed is.
	*/
	/*
		limit := parent.GasLimit() - decay + contrib
		if limit < params.MinGasLimit {
			limit = params.MinGasLimit
		}
		// however, if we're now below the target (TargetGasLimit) we increase the
		// limit as much as we can (parentGasLimit / 1024 -1)
		if limit < params.TargetGasLimit {
			limit = parent.GasLimit() + decay
			if limit > params.TargetGasLimit {
				limit = params.TargetGasLimit
			}
		}
		return limit
	*/
}

func (v *BlockValidator) ValidateFruit(fruit, block *types.SnailBlock) error {
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
		log.Warn("valid fruit sisn hash failed.")
		return ErrInvalidSign
	}

	// check freshness
	var blockHeader *types.SnailHeader
	if block != nil {
		blockHeader = block.Header()
	}
	err := v.engine.VerifyFreshness(fruit.Header(), blockHeader)
	if err != nil {
		log.Warn("ValidateFruit verify freshness error.", "err", err, "fruit", fruit.FastNumber())
		return err
	}

	header := fruit.Header()
	if err := v.engine.VerifySnailHeader(v.bc, v.fastchain, header, true); err != nil {
		log.Warn("validate fruit verify failed.", "err", err)
		return err
	}

	// validate the signatures of this fruit
	members := v.election.GetCommittee(fruit.FastNumber())
	if members == nil {
		log.Warn("validate fruit get committee failed.", "number", fruit.FastNumber())
		return ErrInvalidSign
	}
	count := 0
	signs := fruit.Signs()
	for _, sign := range signs {
		if sign.Result == types.VoteAgree {
			count ++
		}
	}
	// TODO: a bug to verify PBFT signs should len(members) * 2 / 3
	// will fix this bug at next release version
	if count <= len(members) / 3 * 2 {
		log.Warn("validate fruit signs number error", "signs", len(signs), "agree", count, "members", len(members))
		return ErrInvalidSign
	}

	_, errs := v.election.VerifySigns(signs)
	for _, err := range errs {
		if err != nil {
			log.Warn("validate fruit VerifySigns error", "err", err)
			return err
		}
	}

	return nil
}
