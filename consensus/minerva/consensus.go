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

package minerva

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/math"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/misc"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/params"
	"gopkg.in/fatih/set.v0"
)

// Ethash proof-of-work protocol constants.
var (
	FrontierBlockReward  *big.Int = big.NewInt(5e+18) // Block reward in wei for successfully mining a block
	ByzantiumBlockReward *big.Int = big.NewInt(3e+18) // Block reward in wei for successfully mining a block upward from Byzantium

	FruitReward *big.Int = big.NewInt(3.333e+16)
	BlockReward *big.Int = new(big.Int).Mul(big.NewInt(2e+18), big10)

	maxUncles              = 2                // Maximum number of uncles allowed in a single block
	allowedFutureBlockTime = 15 * time.Second // Max time from current time allowed for blocks, before they're considered future blocks

	FruitBlockRatio *big.Int = big.NewInt(64) // difficulty ratio between fruit and block
)

// Various error messages to mark blocks invalid. These should be private to
// prevent engine specific errors from being referenced in the remainder of the
// codebase, inherently breaking if the engine is swapped out. Please put common
// error types into the consensus package.
var (
	errLargeBlockTime    = errors.New("timestamp too big")
	errZeroBlockTime     = errors.New("timestamp equals parent's")
	errTooManyUncles     = errors.New("too many uncles")
	errDuplicateUncle    = errors.New("duplicate uncle")
	errUncleIsAncestor   = errors.New("uncle is ancestor")
	errDanglingUncle     = errors.New("uncle's parent is not ancestor")
	errInvalidDifficulty = errors.New("non-positive difficulty")
	errInvalidMixDigest  = errors.New("invalid mix digest")
	errInvalidPoW        = errors.New("invalid proof-of-work")
)

// Author implements consensus.Engine, returning the header's coinbase as the
// proof-of-work verified author of the block.
func (m *Minerva) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}
func (m *Minerva) AuthorSnail(header *types.SnailHeader) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules of the
// stock Ethereum m engine.
func (m *Minerva) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return nil
}

func (m *Minerva) VerifySnailHeader(chain consensus.SnailChainReader, header *types.SnailHeader, seal bool) error {
	// If we're running a full engine faking, accept any input as valid
	if m.config.PowMode == ModeFullFake {
		return nil
	}

	// Short circuit if the header is known, or it's parent not
	number := header.Number.Uint64()

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	//TODO for fruit

	if header.Fruit {
		return m.verifySnailHeader(chain, header, parent, false, seal)
	}

	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}

	// Sanity checks passed, do a proper verification
	return m.verifySnailHeader(chain, header, parent, false, seal)
}

// VerifyFastHeader checks whether a fast chain header conforms to the consensus rules of the
// stock Ethereum ethash engine.
func (m *Minerva) VerifyFastHeader(chain consensus.ChainFastReader, header *types.Header, seal bool) error {
	// Short circuit if the header is known, or it's parent not
	number := header.Number.Uint64()

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}

	return m.verifyFastHeader(chain, header, parent)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications.
func (m *Minerva) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header,
	seals []bool) (chan<- struct{}, <-chan error) {
	return nil, nil
}
func (m *Minerva) VerifySnailHeaders(chain consensus.SnailChainReader, headers []*types.SnailHeader,
	seals []bool) (chan<- struct{}, <-chan error) {
	// If we're running a full engine faking, accept any input as valid
	if m.config.PowMode == ModeFullFake || len(headers) == 0 {
		abort, results := make(chan struct{}), make(chan error, len(headers))
		for i := 0; i < len(headers); i++ {
			results <- nil
		}
		return abort, results
	}

	// Spawn as many workers as allowed threads
	workers := runtime.GOMAXPROCS(0)
	if len(headers) < workers {
		workers = len(headers)
	}

	// Create a task channel and spawn the verifiers
	var (
		inputs = make(chan int)
		done   = make(chan int, workers)
		errors = make([]error, len(headers))
		abort  = make(chan struct{})
	)
	for i := 0; i < workers; i++ {
		go func() {
			for index := range inputs {
				errors[index] = m.verifySnailHeaderWorker(chain, headers, seals, index)
				done <- index
			}
		}()
	}

	errorsOut := make(chan error, len(headers))
	go func() {
		defer close(inputs)
		var (
			in, out = 0, 0
			checked = make([]bool, len(headers))
			inputs  = inputs
		)
		for {
			select {
			case inputs <- in:
				if in++; in == len(headers) {
					// Reached end of headers. Stop sending to workers.
					inputs = nil
				}
			case index := <-done:
				for checked[index] = true; checked[out]; out++ {
					errorsOut <- errors[out]
					if out == len(headers)-1 {
						return
					}
				}
			case <-abort:
				return
			}
		}
	}()
	return abort, errorsOut
}

func (m *Minerva) verifyHeaderWorker(chain consensus.ChainReader, headers []*types.Header,
	seals []bool, index int) error {
	return nil
}
func (m *Minerva) verifySnailHeaderWorker(chain consensus.SnailChainReader, headers []*types.SnailHeader,
	seals []bool, index int) error {
	var parent *types.SnailHeader

	if index == 0 {
		parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
	} else if headers[index-1].Hash() == headers[index].ParentHash {
		parent = headers[index-1]
	}
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	if chain.GetHeader(headers[index].Hash(), headers[index].Number.Uint64()) != nil {
		return nil // known block
	}

	return m.verifySnailHeader(chain, headers[index], parent, false, seals[index])
}

func (m *Minerva) VerifyFastHeaders(chain consensus.ChainFastReader, headers []*types.Header,
	seals []bool) (chan<- struct{}, <-chan error) {
	// If we're running a full engine faking, accept any input as valid
	if m.config.PowMode == ModeFullFake || len(headers) == 0 {
		abort, results := make(chan struct{}), make(chan error, len(headers))
		for i := 0; i < len(headers); i++ {
			results <- nil
		}
		return abort, results
	}

	// Spawn as many workers as allowed threads
	workers := runtime.GOMAXPROCS(0)
	if len(headers) < workers {
		workers = len(headers)
	}

	// Create a task channel and spawn the verifiers
	var (
		inputs = make(chan int)
		done   = make(chan int, workers)
		errors = make([]error, len(headers))
		abort  = make(chan struct{})
	)
	for i := 0; i < workers; i++ {
		go func() {
			for index := range inputs {
				errors[index] = m.verifyFastHeaderWorker(chain, headers, seals, index)
				done <- index
			}
		}()
	}

	errorsOut := make(chan error, len(headers))
	go func() {
		defer close(inputs)
		var (
			in, out = 0, 0
			checked = make([]bool, len(headers))
			inputs  = inputs
		)
		for {
			select {
			case inputs <- in:
				if in++; in == len(headers) {
					// Reached end of headers. Stop sending to workers.
					inputs = nil
				}
			case index := <-done:
				for checked[index] = true; checked[out]; out++ {
					errorsOut <- errors[out]
					if out == len(headers)-1 {
						return
					}
				}
			case <-abort:
				return
			}
		}
	}()
	return abort, errorsOut
}

func (m *Minerva) verifyFastHeaderWorker(chain consensus.ChainFastReader,
	headers []*types.Header, seals []bool, index int) error {
	var parent *types.Header
	if index == 0 {
		parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
	} else if headers[index-1].Hash() == headers[index].ParentHash {
		parent = headers[index-1]
	}
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	if chain.GetHeader(headers[index].Hash(), headers[index].Number.Uint64()) != nil {
		return nil // known block
	}
	return m.verifyFastHeader(chain, headers[index], parent)
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of the stock Ethereum ethash engine.
func (m *Minerva) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	return nil
}
func (m *Minerva) VerifySnailUncles(chain consensus.SnailChainReader, block *types.SnailBlock) error {

	// If we're running a full engine faking, accept any input as valid
	if m.config.PowMode == ModeFullFake {
		return nil
	}
	// Verify that there are at most 2 uncles included in this block
	//TODO snail chain not uncles
	/*
		if len(block.Uncles()) > maxUncles {
			return errTooManyUncles
		}
	*/
	// Gather the set of past uncles and ancestors
	uncles, ancestors := set.New(), make(map[common.Hash]*types.SnailHeader)

	number, parent := block.NumberU64()-1, block.ParentHash()
	for i := 0; i < 7; i++ {
		ancestor := chain.GetBlock(parent, number)
		if ancestor == nil {
			break
		}
		ancestors[ancestor.Hash()] = ancestor.Header()
		//TODO Snail chain not uncles
		/*
			for _, uncle := range ancestor.Uncles() {
				uncles.Add(uncle.Hash())
			}
		*/
		parent, number = ancestor.ParentHash(), number-1
	}
	ancestors[block.Hash()] = block.Header()
	uncles.Add(block.Hash())

	// Verify each of the uncles that it's recent, but not an ancestor
	//TODO Snail chain not uncles
	/*
		for _, uncle := range block.Uncles() {
			// Make sure every uncle is rewarded only once
			hash := uncle.Hash()
			if uncles.Has(hash) {
				return errDuplicateUncle
			}
			uncles.Add(hash)

			// Make sure the uncle has a valid ancestry
			if ancestors[hash] != nil {
				return errUncleIsAncestor
			}
			if ancestors[uncle.ParentHash] == nil || uncle.ParentHash == block.ParentHash() {
				return errDanglingUncle
			}
			if err := ethash.verifyHeader(chain, uncle, ancestors[uncle.ParentHash], true, true); err != nil {
				return err
			}
		}
	*/
	return nil
}

// verifyHeader checks whether a header conforms to the consensus rules of the
// stock Ethereum ethash engine.
// See YP section 4.3.4. "Block Header Validity"
func (m *Minerva) verifyHeader(chain consensus.ChainReader, header, parent *types.Header,
	uncle bool, seal bool) error {
	return nil
}
func (m *Minerva) verifySnailHeader(chain consensus.SnailChainReader, header, parent *types.SnailHeader,
	uncle bool, seal bool) error {
	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}
	// Verify the header's timestamp
	if uncle {
		if header.Time.Cmp(math.MaxBig256) > 0 {
			return errLargeBlockTime
		}
	} else {
		if header.Time.Cmp(big.NewInt(time.Now().Add(allowedFutureBlockTime).Unix())) > 0 {
			return consensus.ErrFutureBlock
		}
	}
	if header.Time.Cmp(parent.Time) <= 0 {
		return errZeroBlockTime
	}

	// Verify the block's difficulty based in it's timestamp and parent's difficulty
	expected := m.CalcSnailDifficulty(chain, header.Time.Uint64(), parent)

	if expected.Cmp(header.Difficulty) != 0 {
		return fmt.Errorf("invalid difficulty: have %v, want %v", header.Difficulty, expected)
	}

	// Verify that the gas limit is <= 2^63-1

	//TODO snail chian gaslimit
	/*
		cap := uint64(0x7fffffffffffffff)
		if header.GasLimit > cap {
			return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, cap)
		}
		// Verify that the gasUsed is <= gasLimit
		if header.GasUsed > header.GasLimit {
			return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
		}

		// Verify that the gas limit remains within allowed bounds
		diff := int64(parent.GasLimit) - int64(header.GasLimit)
		if diff < 0 {
			diff *= -1
		}
		limit := parent.GasLimit / params.GasLimitBoundDivisor

		if uint64(diff) >= limit || header.GasLimit < params.MinGasLimit {
			return fmt.Errorf("invalid gas limit: have %d, want %d += %d", header.GasLimit, parent.GasLimit, limit)
		}
		// Verify that the block number is parent's +1
		if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
			return consensus.ErrInvalidNumber
		}
	*/

	// Verify the engine specific seal securing the block
	if seal {
		if err := m.VerifySnailSeal(chain, header); err != nil {
			return err
		}
	}
	// If all checks passed, validate any special fields for hard forks
	if err := misc.VerifyDAOSnailHeaderExtraData(chain.Config(), header); err != nil {
		return err
	}
	if err := misc.VerifySnailForkHashes(chain.Config(), header, uncle); err != nil {
		return err
	}
	return nil
}

// verifyFastHeader checks whether a header conforms to the consensus rules of the
// stock Ethereum ethash engine.
// See YP section 4.3.4. "Fast Block Header Validity"
func (m *Minerva) verifyFastHeader(chain consensus.ChainFastReader,
	header, parent *types.Header) error {
	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}
	// Verify the header's timestamp
	if header.Time.Cmp(big.NewInt(time.Now().Add(allowedFutureBlockTime).Unix())) > 0 {
		return consensus.ErrFutureBlock
	}

	if header.Time.Cmp(parent.Time) <= 0 {
		return errZeroBlockTime
	}

	// Verify that the gas limit is <= 2^63-1
	cap := uint64(0x7fffffffffffffff)
	if header.GasLimit > cap {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, cap)
	}
	// Verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}

	// Verify that the gas limit remains within allowed bounds
	diff := int64(parent.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}
	limit := parent.GasLimit / params.GasLimitBoundDivisor

	if uint64(diff) >= limit || header.GasLimit < params.MinGasLimit {
		return fmt.Errorf("invalid gas limit: have %d, want %d += %d", header.GasLimit, parent.GasLimit, limit)
	}
	// Verify that the block number is parent's +1
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return consensus.ErrInvalidNumber
	}

	return nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time
// given the parent block's time and difficulty.
func (m *Minerva) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	return nil
}
func (m *Minerva) CalcSnailDifficulty(chain consensus.SnailChainReader, time uint64, parent *types.SnailHeader) *big.Int {
	return CalcDifficulty(chain.Config(), time, parent)
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time
// given the parent block's time and difficulty.
func CalcDifficulty(config *params.ChainConfig, time uint64, parent *types.SnailHeader) *big.Int {
	next := new(big.Int).Add(parent.Number, big1)
	switch {
	case config.IsByzantium(next):
		return calcDifficultyByzantium(time, parent)
	case config.IsHomestead(next):
		return calcDifficultyHomestead(time, parent)
	default:
		return calcDifficultyFrontier(time, parent)
	}
}

// Some weird constants to avoid constant memory allocs for them.
var (
	expDiffPeriod = big.NewInt(100000)
	big1          = big.NewInt(1)
	big2          = big.NewInt(2)
	big8          = big.NewInt(8)
	big9          = big.NewInt(9)
	big10         = big.NewInt(10)
	big32         = big.NewInt(32)
	bigMinus99    = big.NewInt(-99)
	big2999999    = big.NewInt(2999999)
)

// calcDifficultyByzantium is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time given the
// parent block's time and difficulty. The calculation uses the Byzantium rules.
func calcDifficultyByzantium(time uint64, parent *types.SnailHeader) *big.Int {
	// https://github.com/ethereum/EIPs/issues/100.
	// algorithm:
	// diff = (parent_diff +
	//         (parent_diff / 2048 * max((2 if len(parent.uncles) else 1) - ((timestamp - parent.timestamp) // 9), -99))
	//        ) + 2^(periodCount - 2)

	bigTime := new(big.Int).SetUint64(time)
	bigParentTime := new(big.Int).Set(parent.Time)

	// holds intermediate values to make the algo easier to read & audit
	x := new(big.Int)
	y := new(big.Int)

	// (2 if len(parent_uncles) else 1) - (block_timestamp - parent_timestamp) // 9
	x.Sub(bigTime, bigParentTime)
	x.Div(x, big9)
	if parent.UncleHash == types.EmptyUncleHash {
		x.Sub(big1, x)
	} else {
		x.Sub(big2, x)
	}
	// max((2 if len(parent_uncles) else 1) - (block_timestamp - parent_timestamp) // 9, -99)
	if x.Cmp(bigMinus99) < 0 {
		x.Set(bigMinus99)
	}
	// parent_diff + (parent_diff / 2048 * max((2 if len(parent.uncles) else 1) - ((timestamp - parent.timestamp) // 9), -99))
	y.Div(parent.Difficulty, params.DifficultyBoundDivisor)
	x.Mul(y, x)
	x.Add(parent.Difficulty, x)

	// minimum difficulty can ever be (before exponential factor)
	if x.Cmp(params.MinimumDifficulty) < 0 {
		x.Set(params.MinimumDifficulty)
	}
	// calculate a fake block number for the ice-age delay:
	//   https://github.com/ethereum/EIPs/pull/669
	//   fake_block_number = min(0, block.number - 3_000_000
	fakeBlockNumber := new(big.Int)
	if parent.Number.Cmp(big2999999) >= 0 {
		fakeBlockNumber = fakeBlockNumber.Sub(parent.Number, big2999999) // Note, parent is 1 less than the actual block number
	}
	// for the exponential factor
	periodCount := fakeBlockNumber
	periodCount.Div(periodCount, expDiffPeriod)

	// the exponential factor, commonly referred to as "the bomb"
	// diff = diff + 2^(periodCount - 2)
	if periodCount.Cmp(big1) > 0 {
		y.Sub(periodCount, big2)
		y.Exp(big2, y, nil)
		x.Add(x, y)
	}
	return x
}

// calcDifficultyHomestead is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time given the
// parent block's time and difficulty. The calculation uses the Homestead rules.
func calcDifficultyHomestead(time uint64, parent *types.SnailHeader) *big.Int {
	// https://github.com/ethereum/EIPs/blob/master/EIPS/eip-2.md
	// algorithm:
	// diff = (parent_diff +
	//         (parent_diff / 2048 * max(1 - (block_timestamp - parent_timestamp) // 10, -99))
	//        ) + 2^(periodCount - 2)

	bigTime := new(big.Int).SetUint64(time)
	bigParentTime := new(big.Int).Set(parent.Time)

	// holds intermediate values to make the algo easier to read & audit
	x := new(big.Int)
	y := new(big.Int)

	// 1 - (block_timestamp - parent_timestamp) // 10
	x.Sub(bigTime, bigParentTime)
	x.Div(x, big10)
	x.Sub(big1, x)

	// max(1 - (block_timestamp - parent_timestamp) // 10, -99)
	if x.Cmp(bigMinus99) < 0 {
		x.Set(bigMinus99)
	}
	// (parent_diff + parent_diff // 2048 * max(1 - (block_timestamp - parent_timestamp) // 10, -99))
	y.Div(parent.Difficulty, params.DifficultyBoundDivisor)
	x.Mul(y, x)
	x.Add(parent.Difficulty, x)

	// minimum difficulty can ever be (before exponential factor)
	if x.Cmp(params.MinimumDifficulty) < 0 {
		x.Set(params.MinimumDifficulty)
	}
	// for the exponential factor
	periodCount := new(big.Int).Add(parent.Number, big1)
	periodCount.Div(periodCount, expDiffPeriod)

	// the exponential factor, commonly referred to as "the bomb"
	// diff = diff + 2^(periodCount - 2)
	if periodCount.Cmp(big1) > 0 {
		y.Sub(periodCount, big2)
		y.Exp(big2, y, nil)
		x.Add(x, y)
	}
	return x
}

// calcDifficultyFrontier is the difficulty adjustment algorithm. It returns the
// difficulty that a new block should have when created at time given the parent
// block's time and difficulty. The calculation uses the Frontier rules.
func calcDifficultyFrontier(time uint64, parent *types.SnailHeader) *big.Int {
	diff := new(big.Int)
	adjust := new(big.Int).Div(parent.Difficulty, params.DifficultyBoundDivisor)
	bigTime := new(big.Int)
	bigParentTime := new(big.Int)

	bigTime.SetUint64(time)
	bigParentTime.Set(parent.Time)

	if bigTime.Sub(bigTime, bigParentTime).Cmp(params.DurationLimit) < 0 {
		diff.Add(parent.Difficulty, adjust)
	} else {
		diff.Sub(parent.Difficulty, adjust)
	}
	if diff.Cmp(params.MinimumDifficulty) < 0 {
		diff.Set(params.MinimumDifficulty)
	}

	periodCount := new(big.Int).Add(parent.Number, big1)
	periodCount.Div(periodCount, expDiffPeriod)
	if periodCount.Cmp(big1) > 0 {
		// diff = diff + 2^(periodCount - 2)
		expDiff := periodCount.Sub(periodCount, big2)
		expDiff.Exp(big2, expDiff, nil)
		diff.Add(diff, expDiff)
		diff = math.BigMax(diff, params.MinimumDifficulty)
	}
	return diff
}

// VerifySeal implements consensus.Engine, checking whether the given block satisfies
// the PoW difficulty requirements.
func (m *Minerva) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return nil
}
func (m *Minerva) VerifySnailSeal(chain consensus.SnailChainReader, header *types.SnailHeader) error {
	// If we're running a fake PoW, accept any seal as valid
	if m.config.PowMode == ModeFake || m.config.PowMode == ModeFullFake {
		time.Sleep(m.fakeDelay)
		if m.fakeFail == header.Number.Uint64() {
			return errInvalidPoW
		}
		return nil
	}
	// If we're running a shared PoW, delegate verification to it
	if m.shared != nil {
		return m.shared.VerifySnailSeal(chain, header)
	}
	// Ensure that we have a valid difficulty for the block
	if header.Difficulty.Sign() <= 0 {
		return errInvalidDifficulty
	}
	// Recompute the digest and PoW value and verify against the header
	number := header.Number.Uint64()

	cache := m.cache(number)
	size := datasetSize(number)
	if m.config.PowMode == ModeTest {
		size = 32 * 1024
	}
	digest, result := hashimotoLight(size, cache.cache, header.HashNoNonce().Bytes(), header.Nonce.Uint64())
	// Caches are unmapped in a finalizer. Ensure that the cache stays live
	// until after the call to hashimotoLight so it's not unmapped while being used.
	runtime.KeepAlive(cache)

	if !bytes.Equal(header.MixDigest[:], digest) {
		return errInvalidMixDigest
	}

	//TODO for fruit

	target := new(big.Int).Div(maxUint256, header.Difficulty)
	fruitDifficulty := new(big.Int).Div(header.Difficulty, FruitBlockRatio)
	fruitTarget := new(big.Int).Div(maxUint128, fruitDifficulty)

	//if header.Fruit {
	// TODO need know how to get fruits
	if header.Number.Uint64() > 0 {
		last := result[16:]
		if new(big.Int).SetBytes(last).Cmp(fruitTarget) > 0 {
			return errInvalidPoW
		}
	} else if new(big.Int).SetBytes(result).Cmp(target) > 0 {
		return errInvalidPoW
	}

	return nil
}

// Prepare implements consensus.Engine, initializing the difficulty field of a
// header to conform to the ethash protocol. The changes are done inline.
func (m *Minerva) Prepare(chain consensus.ChainReader, header *types.Header) error {
	return nil
}
func (m *Minerva) PrepareSnail(chain consensus.SnailChainReader, header *types.SnailHeader) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = m.CalcSnailDifficulty(chain, header.Time.Uint64(), parent)
	return nil
}

// PrepareFast implements consensus.Engine, initializing the difficulty field of a
// header to conform to the ethash protocol. The changes are done inline.
func (m *Minerva) PrepareFast(chain consensus.ChainFastReader, header *types.Header) error {
	if parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1); parent == nil {
		return consensus.ErrUnknownAncestor
	}
	return nil
}

// Finalize implements consensus.Engine, accumulating the block fruit and uncle rewards,
// setting the final state and assembling the block.
func (m *Minerva) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB,
	txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt, fruits []*types.Block) (*types.Block, error) {
	return nil, nil
}
func (m *Minerva) FinalizeSnail(chain consensus.SnailChainReader, header *types.SnailHeader, state *state.StateDB,
	uncles []*types.SnailHeader, fruits []*types.SnailBlock, signs []*types.PbftSign) (*types.SnailBlock, error) {

	//header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	// Header seems complete, assemble into a block and return
	//TODO need creat a snail block body,the teamper mather is fruits[1].Body()
	return types.NewSnailBlock(header, fruits, signs, uncles), nil
}

// FinalizeFast implements consensus.Engine, accumulating the block fruit and uncle rewards,
// setting the final state and assembling the block.
// Please pass in the parameter block when reward distribution is required.
func (m *Minerva) FinalizeFast(chain consensus.ChainFastReader, header *types.Header, state *state.StateDB,
	txs []*types.Transaction, receipts []*types.Receipt) (*types.Block, error) {

	if header != nil && len(header.SnailHash) > 0 && header.SnailHash != *new(common.Hash) && header.SnailNumber != nil {
		sBlock := m.sbc.GetBlock(header.SnailHash, header.SnailNumber.Uint64())
		if sBlock == nil {
			return nil, consensus.ErrInvalidNumber
		}
		accumulateRewardsFast(state, header, sBlock)
	}
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	return types.NewBlock(header, txs, receipts, nil), nil
}

//gas allocation
func (m *Minerva) FinalizeFastGas(state *state.StateDB, fastNumber *big.Int, fastHash common.Hash, gasLimit *big.Int) error {
	committee := m.election.GetCommittee(fastNumber)
	committeeGas := big.NewInt(0)
	if len(committee) != 0 {
		committeeGas = new(big.Int).Div(gasLimit, big.NewInt(int64(len(committee))))
	}
	for _, v := range committee {
		state.AddBalance(v.Coinbase, committeeGas)
	}
	return nil
}

// AccumulateRewardsFast credits the coinbase of the given block with the mining
// reward. The total reward consists of the static block reward and rewards for
// included uncles. The coinbase of each uncle block is also rewarded.
func accumulateRewardsFast(state *state.StateDB, header *types.Header, sBlock *types.SnailBlock) error {
	//get snail block
	//if sc == nil {
	//	return nil
	//}

	//Get snailBlock current -12
	minerCoin, committeeCoin := getCurrentBlockCoin(sBlock.Number())

	//miner's award
	state.AddBalance(sBlock.Coinbase(), minerCoin)

	//committee's award
	blockFruits := sBlock.Body().Fruits
	blockFruitsLen := big.NewInt(int64(len(blockFruits)))
	committeeCoinFruit := new(big.Int).Div(committeeCoin, blockFruitsLen)

	//all fail committee coinBase
	failAddr := make(map[common.Address]bool)
	var ce consensus.CommitteeElection

	for _, fruit := range blockFruits {
		signs := fruit.Body().Signs

		addr, err := ce.VerifySigns(signs)
		if len(addr) != len(err) {
			return consensus.ErrInvalidSingsLength
		}
		for i := 0; i < len(addr); i++ {
			if err[i] != nil {
				failAddr[addr[i].Coinbase] = false
			}
		}

		//Effective and not evil
		var fruitOkAddr []common.Address
		for i := 0; i < len(addr); i++ {
			v := addr[i]
			if signs[i].Result == 0 {
				if _, ok := failAddr[v.Coinbase]; !ok {
					fruitOkAddr = append(fruitOkAddr, v.Coinbase)
				}
			} else {
				failAddr[v.Coinbase] = false
			}
		}

		// Equal by fruit
		committeeCoinFruitMember := new(big.Int).Div(committeeCoinFruit, big.NewInt(int64(len(fruitOkAddr))))
		for _, v := range fruitOkAddr {
			state.AddBalance(v, committeeCoinFruitMember)
		}
	}

	//miners add all fruit 10%
	state.AddBalance(sBlock.Coinbase(),
		new(big.Int).Div(new(big.Int).SetInt64(int64(len(sBlock.Body().Fruits))),
			big10))
	return nil
}

//Get current revenue value for miner or committee
//Committee miners distribution method  Committee a/a+n  miners  n/a+n
//parameter num:  snail chain header number
func getCurrentBlockCoin(num *big.Int) (minerCoin, committeeCoin *big.Int) {
	currentBlockCoinCount := new(big.Int).Div(SnailBlockRewardsInitial,
		new(big.Int).Exp(new(big.Int).SetInt64(2),
			new(big.Int).Div(num, new(big.Int).SetInt64(5000)),
			nil))

	currentBlockCoinMean := new(big.Int).Div(currentBlockCoinCount, new(big.Int).Add(MinerCount, CommitteesCount))

	minerCoin = new(big.Int).Mul(currentBlockCoinMean, MinerCount)
	committeeCoin = new(big.Int).Mul(currentBlockCoinMean, CommitteesCount)
	return
}
