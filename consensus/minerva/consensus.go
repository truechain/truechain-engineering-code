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

	"github.com/truechain/truechain-engineering-code/log"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/math"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/misc"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/params"
	"gopkg.in/fatih/set.v0"
)

// Minerva protocol constants.
var (
	FrontierBlockReward  *big.Int = big.NewInt(5e+18) // Block reward in wei for successfully mining a block
	ByzantiumBlockReward *big.Int = big.NewInt(3e+18) // Block reward in wei for successfully mining a block upward from Byzantium

	//FruitReward *big.Int = big.NewInt(3.333e+16)
	//BlockReward *big.Int = new(big.Int).Mul(big.NewInt(2e+18), big10)

	maxUncles              = 2                // Maximum number of uncles allowed in a single block
	allowedFutureBlockTime = 15 * time.Second // Max time from current time allowed for blocks, before they're considered future blocks

	//FruitBlockRatio *big.Int = big.NewInt(64) // difficulty ratio between fruit and block
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
	errInvalidFast       = errors.New("invalid fast number")
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
func (m *Minerva) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error { // TODO remove seal
	// Short circuit if the header is known, or it's parent not
	number := header.Number.Uint64()

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}

	return m.verifyHeader(chain, header, parent)
}

func (m *Minerva) getParents(chain consensus.SnailChainReader, header *types.SnailHeader) []*types.SnailHeader {
	number := header.Number.Uint64()
	period := params.DifficultyPeriod.Uint64()
	if number < period {
		period = number
	}
	//log.Info("getParents", "number", header.Number, "period", period)
	parents := make([]*types.SnailHeader, period)
	hash := header.ParentHash
	for i := uint64(1); i <= period; i++ {
		if number-i < 0 {
			break
		}
		parent := chain.GetHeader(hash, number-i)
		if parent == nil {
			log.Warn("getParents get parent failed.", "number", number-i, "hash", hash)
			return nil
		}
		parents[period-i] = parent
		hash = parent.ParentHash
	}

	return parents
}

func (m *Minerva) VerifySnailHeader(chain consensus.SnailChainReader, fastchain consensus.ChainReader, header *types.SnailHeader, seal bool) error {
	// If we're running a full engine faking, accept any input as valid
	if m.config.PowMode == ModeFullFake {
		return nil
	}

	if header.Fruit {
		pointer := chain.GetHeader(header.PointerHash, header.PointerNumber.Uint64())
		if pointer == nil {
			log.Warn("VerifySnailHeader get pointer failed.", "fNumber", header.FastNumber, "pNumber", header.PointerNumber, "pHash", header.PointerHash)
			return consensus.ErrUnknownPointer
		}
		return m.verifySnailHeader(chain, fastchain, header, pointer, nil, false, seal)
	} else {
		// Short circuit if the header is known, or it's parent not
		if chain.GetHeader(header.Hash(), header.Number.Uint64()) != nil {
			return nil
		}
		parents := m.getParents(chain, header)
		if parents == nil {
			return consensus.ErrUnknownAncestor
		}

		// Sanity checks passed, do a proper verification
		return m.verifySnailHeader(chain, fastchain, header, nil, parents, false, seal)
	}
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications.
func (m *Minerva) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header,
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
				errors[index] = m.verifyHeaderWorker(chain, headers, seals, index)
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
		errs   = make([]error, len(headers))
		abort  = make(chan struct{})
	)

	parents := m.getParents(chain, headers[0])
	if parents == nil {
		abort, results := make(chan struct{}), make(chan error, len(headers))
		for i := 0; i < len(headers); i++ {
			results <- errors.New("invalid parents")
		}
		return abort, results
	}
	parents = append(parents, headers...)

	for i := 0; i < workers; i++ {
		//m.verifySnailHeader(chain, nil, nil, par, false, seals[i])
		go func() {
			for index := range inputs {
				errs[index] = m.verifySnailHeaderWorker(chain, headers, parents, seals, index)
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
					errorsOut <- errs[out]
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

	return m.verifyHeader(chain, headers[index], parent)
	//return nil
}

func (m *Minerva) verifySnailHeaderWorker(chain consensus.SnailChainReader, headers, parents []*types.SnailHeader,
	seals []bool, index int) error {
	//var parent *types.SnailHeader

	/*
		if index == 0 {
			parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
		} else if headers[index-1].Hash() == headers[index].ParentHash {
			parent = headers[index-1]
		}
		if parent == nil {
			return consensus.ErrUnknownAncestor
		}
	*/
	if chain.GetHeader(headers[index].Hash(), headers[index].Number.Uint64()) != nil {
		return nil // known block
	}
	count := len(parents) - len(headers) + index
	parentHeaders := parents[:count]

	return m.verifySnailHeader(chain, nil, headers[index], nil, parentHeaders, false, seals[index])
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of the stock Truechain minerva engine.
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

	return nil
}

// verifyHeader checks whether a header conforms to the consensus rules of the
// stock Truechain minerva engine.
func (m *Minerva) verifyHeader(chain consensus.ChainReader, header, parent *types.Header) error {
	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}
	// Verify the header's timestamp
	if header.Time.Cmp(big.NewInt(time.Now().Add(allowedFutureBlockTime).Unix())) > 0 {
		fmt.Println(consensus.ErrFutureBlock.Error(), "header", header.Time, "now", time.Now().Unix(),
			"cmp:", big.NewInt(time.Now().Add(allowedFutureBlockTime).Unix()))
		return consensus.ErrFutureBlock
	}

	if header.Time.Cmp(parent.Time) < 0 {
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
func (m *Minerva) verifySnailHeader(chain consensus.SnailChainReader, fastchain consensus.ChainReader, header, pointer *types.SnailHeader,
	parents []*types.SnailHeader, uncle bool, seal bool) error {
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
		if !header.Fruit {
			if header.Time.Cmp(big.NewInt(time.Now().Add(allowedFutureBlockTime).Unix())) > 0 {
				return consensus.ErrFutureBlock
			}
		}
	}
	if !header.Fruit {
		if header.Time.Cmp(parents[len(parents)-1].Time) <= 0 {
			return errZeroBlockTime
		}

		// Verify the block's difficulty based in it's timestamp and parent's difficulty
		expected := m.CalcSnailDifficulty(chain, header.Time.Uint64(), parents)

		if expected.Cmp(header.Difficulty) != 0 {
			return fmt.Errorf("invalid difficulty: have %v, want %v", header.Difficulty, expected)
		}
	} else {
		fastHeader := fastchain.GetHeader(header.FastHash, header.FastNumber.Uint64())
		if fastHeader == nil {
			log.Warn("verifySnailHeader get fast failed.", "fNumber", header.FastNumber, "fHash", header.FastHash)
			return errInvalidFast
		}
		// Verify the block's difficulty based in it's timestamp and parent's difficulty
		expected := m.CalcFruitDifficulty(chain, header.Time.Uint64(), fastHeader.Time.Uint64(), pointer)

		if expected.Cmp(header.FruitDifficulty) != 0 {
			return fmt.Errorf("invalid difficulty: have %v, want %v", header.FruitDifficulty, expected)
		}
	}

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

// CalcDifficulty is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time
// given the parent block's time and difficulty.
func (m *Minerva) CalcSnailDifficulty(chain consensus.SnailChainReader, time uint64, parents []*types.SnailHeader) *big.Int {
	return CalcDifficulty(chain.Config(), time, parents)
}

func (m *Minerva) CalcFruitDifficulty(chain consensus.SnailChainReader, time uint64, fastTime uint64, pointer *types.SnailHeader) *big.Int {
	return CalcFruitDifficulty(chain.Config(), time, fastTime, pointer)
}

// VerifySigns check the sings included in fast block or fruit
//
func (m *Minerva) VerifySigns(fastnumber *big.Int, signs []*types.PbftSign) error {

	// validate the signatures of this fruit
	members := m.election.GetCommittee(fastnumber)
	if members == nil {
		log.Warn("validate fruit get committee failed.", "number", fastnumber)
		return consensus.ErrInvalidSign
	}
	count := 0
	for _, sign := range signs {
		if sign.Result == types.VoteAgree {
			count++
		}
	}
	if count <= len(members)*2/3 {
		log.Warn("validate fruit signs number error", "signs", len(signs), "agree", count, "members", len(members))
		return consensus.ErrInvalidSign
	}

	_, errs := m.election.VerifySigns(signs)
	for _, err := range errs {
		if err != nil {
			log.Warn("validate fruit VerifySigns error", "err", err)
			return err
		}
	}

	return nil
}

func (m *Minerva) VerifyFreshness(fruit, block *types.SnailBlock) error {
	var header *types.SnailHeader
	if block == nil {
		header = m.sbc.CurrentHeader()
	} else {
		header = block.Header()
	}
	// check freshness
	pointer := m.sbc.GetHeader(fruit.PointerHash(), fruit.PointNumber().Uint64())
	if pointer == nil {
		log.Warn("VerifyFreshness get pointer failed.", "fruit", fruit.Number(), "number", fruit.PointNumber(), "pointer", fruit.PointerHash())
		return consensus.ErrUnknownPointer
	}
	freshNumber := new(big.Int).Sub(header.Number, pointer.Number)
	if freshNumber.Cmp(params.FruitFreshness) > 0 {
		log.Warn("VerifyFreshness failed.", "fruit", fruit.Number(), "poiner", pointer.Number, "current", header.Number)
		return consensus.ErrFreshness
	}

	return nil
}

func (m *Minerva) GetDifficulty(header *types.SnailHeader) (*big.Int, *big.Int) {
	// m.CheckDataSetState(header.Number.Uint64())
	_, result := truehashLight(*m.dataset.dataset, header.HashNoNonce().Bytes(), header.Nonce.Uint64())

	if header.Fruit {
		last := result[16:]
		actDiff := new(big.Int).Div(maxUint128, new(big.Int).SetBytes(last))

		return actDiff, header.FruitDifficulty
	} else {
		actDiff := new(big.Int).Div(maxUint128, new(big.Int).SetBytes(result[:16]))
		return actDiff, header.Difficulty
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
	bigMinus1     = big.NewInt(-1)
	bigMinus99    = big.NewInt(-99)
	big2999999    = big.NewInt(2999999)
)

// CalcDifficulty is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time
// given the parent block's time and difficulty.
func CalcDifficulty(config *params.ChainConfig, time uint64, parents []*types.SnailHeader) *big.Int {
	return calcDifficulty2(time, parents)
	//return calcDifficulty(time, parents[0])
}

func CalcFruitDifficulty(config *params.ChainConfig, time uint64, fastTime uint64, pointer *types.SnailHeader) *big.Int {
	diff := new(big.Int).Div(pointer.Difficulty, params.FruitBlockRatio)

	delta := time - fastTime

	if delta > 20 {
		diff = new(big.Int).Div(diff, big.NewInt(2))
	} else if delta > 10 && delta <= 20 {
		diff = new(big.Int).Mul(diff, big.NewInt(2))
		diff = new(big.Int).Div(diff, big.NewInt(3))
	}

	if diff.Cmp(params.MinimumFruitDifficulty) < 0 {
		diff.Set(params.MinimumFruitDifficulty)
	}

	//log.Debug("CalcFruitDifficulty", "delta", delta, "diff", diff)

	return diff
}

func calcDifficulty2(time uint64, parents []*types.SnailHeader) *big.Int {
	// algorithm:
	// diff = (average_diff +
	//         (average_diff / 32) * (max(86400 - (block_timestamp - parent_timestamp), -86400) // 86400)
	//        )

	period := big.NewInt(int64(len(parents)))
	parentHeaders := parents

	/* get average diff */
	diff := big.NewInt(0)
	if parents[0].Number.Cmp(common.Big0) == 0 {
		period.Sub(period, common.Big1)
		parentHeaders = parents[1:]
	}
	if period.Cmp(common.Big0) == 0 {
		// only have genesis block
		return parents[0].Difficulty
	}

	for _, parent := range parentHeaders {
		diff.Add(diff, parent.Difficulty)
	}
	average_diff := new(big.Int).Div(diff, period)

	durationDivisor := new(big.Int).Mul(params.DurationLimit, period)

	bigTime := new(big.Int).SetUint64(time)
	bigParentTime := new(big.Int).Set(parentHeaders[0].Time)

	// holds intermediate values to make the algo easier to read & audit
	x := new(big.Int)
	y := new(big.Int)

	// 86400 - (block_timestamp - parent_timestamp)
	x.Add(durationDivisor, bigParentTime)
	x.Sub(x, bigTime)

	// (max(86400 - (block_timestamp - parent_timestamp), -86400)
	y.Mul(durationDivisor, bigMinus1)
	if x.Cmp(y) < 0 {
		x.Set(y)
	}

	// (average_diff / 32) * (max(86400 - (block_timestamp - parent_timestamp), -86400) // 86400)
	y.Div(average_diff, params.DifficultyBoundDivisor)
	x.Mul(y, x)

	x.Div(x, durationDivisor)

	x.Add(average_diff, x)

	// minimum difficulty can ever be (before exponential factor)
	if x.Cmp(params.MinimumDifficulty) < 0 {
		x.Set(params.MinimumDifficulty)
	}

	return x
}

// calcDifficulty is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time given the
// parent block's time and difficulty.
func calcDifficulty(time uint64, parent *types.SnailHeader) *big.Int {
	// algorithm:
	// diff = (parent_diff +
	//         (parent_diff / 32 * max(1 - (block_timestamp - parent_timestamp) // 600, -1))
	//        )

	bigTime := new(big.Int).SetUint64(time)
	bigParentTime := new(big.Int).Set(parent.Time)

	// holds intermediate values to make the algo easier to read & audit
	x := new(big.Int)
	y := new(big.Int)

	// 1 - (block_timestamp - parent_timestamp) // 600
	x.Sub(bigTime, bigParentTime)
	x.Div(x, params.DurationLimit)
	x.Sub(big1, x)

	// max(1 - (block_timestamp - parent_timestamp) // 10, -1)
	if x.Cmp(bigMinus1) < 0 {
		x.Set(bigMinus1)
	}
	// (parent_diff + parent_diff // 32 * max(1 - (block_timestamp - parent_timestamp) // 600, -1))
	y.Div(parent.Difficulty, params.DifficultyBoundDivisor)
	x.Mul(y, x)
	x.Add(parent.Difficulty, x)

	// minimum difficulty can ever be (before exponential factor)
	if x.Cmp(params.MinimumDifficulty) < 0 {
		x.Set(params.MinimumDifficulty)
	}

	return x
}

// VerifySeal implements consensus.Engine, checking whether the given block satisfies
// the PoW difficulty requirements.
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
	if header.FruitDifficulty.Sign() <= 0 {
		return errInvalidDifficulty
	}
	// Recompute the digest and PoW value and verify against the header
	//number := header.Number.Uint64()

	//m.CheckDataSetState(header.Number.Uint64())
	digest, result := truehashLight(*m.dataset.dataset, header.HashNoNonce().Bytes(), header.Nonce.Uint64())

	if !bytes.Equal(header.MixDigest[:], digest) {
		return errInvalidMixDigest
	}

	if header.Fruit {
		fruitTarget := new(big.Int).Div(maxUint128, header.FruitDifficulty)

		last := result[16:]
		if new(big.Int).SetBytes(last).Cmp(fruitTarget) > 0 {
			return errInvalidPoW
		}
	} else {
		target := new(big.Int).Div(maxUint128, header.Difficulty)
		last := result[:16]
		if new(big.Int).SetBytes(last).Cmp(target) > 0 {
			return errInvalidPoW
		}
	}

	return nil
}

// Prepare implements consensus.Engine, initializing the difficulty field of a
// header to conform to the minerva protocol. The changes are done inline.
func (m *Minerva) Prepare(chain consensus.ChainReader, header *types.Header) error {
	if parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1); parent == nil {
		return consensus.ErrUnknownAncestor
	}
	return nil
}
func (m *Minerva) PrepareSnail(chain consensus.ChainReader, header *types.SnailHeader) error {
	parents := m.getParents(m.sbc, header)
	//parent := m.sbc.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parents == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = m.CalcSnailDifficulty(m.sbc, header.Time.Uint64(), parents)

	if header.FastNumber == nil {
		header.FruitDifficulty = new(big.Int).Set(params.MinimumFruitDifficulty)
	} else {
		pointer := m.sbc.GetHeader(header.PointerHash, header.PointerNumber.Uint64())
		if pointer == nil {
			return consensus.ErrUnknownPointer
		}
		fast := chain.GetHeader(header.FastHash, header.FastNumber.Uint64())
		if fast == nil {
			return consensus.ErrUnknownFast
		}

		header.FruitDifficulty = m.CalcFruitDifficulty(m.sbc, header.Time.Uint64(), fast.Time.Uint64(), pointer)
	}

	return nil
}

// Finalize implements consensus.Engine, accumulating the block fruit and uncle rewards,
// setting the final state and assembling the block.
func (m *Minerva) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB,
	txs []*types.Transaction, receipts []*types.Receipt, feeAmount *big.Int) (*types.Block, error) {
	if header != nil && len(header.SnailHash) > 0 && header.SnailHash != *new(common.Hash) && header.SnailNumber != nil {
		log.Info("Finalize:", "header.SnailHash", header.SnailHash, "header.SnailNumber", header.SnailNumber)
		sBlock := m.sbc.GetBlock(header.SnailHash, header.SnailNumber.Uint64())
		if sBlock == nil {
			bTm := m.sbc.GetHeaderByNumber(header.SnailNumber.Uint64())
			if bTm != nil {
				log.Info("Finalize:Error GetHeaderByNumber", "header.SnailHash", bTm.Hash(), "header.SnailNumber", bTm.Number)
			}
			return nil, consensus.ErrInvalidNumber
		}
		err := accumulateRewardsFast(m.election, state, header, sBlock)
		if err != nil {
			log.Error("Finalize Error", "accumulateRewardsFast", err.Error())
			return nil, err
		}
	}
	if err := m.finalizeFastGas(state, header.Number, header.Hash(), feeAmount); err != nil {
		return nil, err
	}
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	return types.NewBlock(header, txs, receipts, nil), nil //TODO remove signs
}
func (m *Minerva) FinalizeSnail(chain consensus.SnailChainReader, header *types.SnailHeader,
	uncles []*types.SnailHeader, fruits []*types.SnailBlock, signs []*types.PbftSign) (*types.SnailBlock, error) {

	//header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	// Header seems complete, assemble into a block and return
	//TODO need creat a snail block body,the teamper mather is fruits[1].Body()
	return types.NewSnailBlock(header, fruits, signs, uncles), nil
}

// gas allocation
func (m *Minerva) finalizeFastGas(state *state.StateDB, fastNumber *big.Int, fastHash common.Hash, feeAmount *big.Int) error {
	log.Debug("FinalizeFastGas:", "fastNumber", fastNumber, "feeAmount", feeAmount)
	if feeAmount.Uint64() == 0 {
		return nil
	}
	committee := m.election.GetCommittee(fastNumber)
	committeeGas := big.NewInt(0)
	if len(committee) == 0 {
		return errors.New("not have committee")
	}
	committeeGas = new(big.Int).Div(feeAmount, big.NewInt(int64(len(committee))))
	for _, v := range committee {
		state.AddBalance(v.Coinbase, committeeGas)
		LogPrint("gas", v.Coinbase, committeeGas)
	}
	return nil
}

func LogPrint(info string, addr common.Address, amount *big.Int) {
	log.Debug("[Consensus AddBalance]", "info", info, "CoinBase:", addr.String(), "amount", amount)
}

// AccumulateRewardsFast credits the coinbase of the given block with the mining
// reward. The total reward consists of the static block reward and rewards for
// included uncles. The coinbase of each uncle block is also rewarded.
func accumulateRewardsFast(election consensus.CommitteeElection, state *state.StateDB, header *types.Header, sBlock *types.SnailBlock) error {
	committeeCoin, minerCoin, minerFruitCoin, e := getBlockReward(header.Number)

	if e != nil {
		return e
	}

	//miner's award
	state.AddBalance(sBlock.Coinbase(), minerCoin)
	LogPrint("miner's award", sBlock.Coinbase(), minerCoin)

	//miner fruit award
	blockFruits := sBlock.Body().Fruits
	blockFruitsLen := big.NewInt(int64(len(blockFruits)))
	if len(blockFruits) > 0 {
		minerFruitCoinOne := new(big.Int).Div(minerFruitCoin, blockFruitsLen)
		for _, v := range sBlock.Body().Fruits {
			state.AddBalance(v.Coinbase(), minerFruitCoinOne)
			LogPrint("minerFruit", v.Coinbase(), minerFruitCoinOne)
		}
	} else {
		return consensus.ErrInvalidBlock
	}

	//committee's award
	committeeCoinFruit := new(big.Int).Div(committeeCoin, blockFruitsLen)

	//all fail committee coinBase
	failAddr := make(map[common.Address]bool)

	for _, fruit := range blockFruits {
		signs := fruit.Body().Signs

		committeeMembers, errs := election.VerifySigns(signs)
		if len(committeeMembers) != len(errs) {
			return consensus.ErrInvalidSignsLength
		}

		//Effective and not evil
		var fruitOkAddr []common.Address
		for i, cm := range committeeMembers {
			if errs[i] != nil {
				continue
			}
			if signs[i].Result == types.VoteAgree {
				if _, ok := failAddr[cm.Coinbase]; !ok {
					fruitOkAddr = append(fruitOkAddr, cm.Coinbase)
				}
			} else {
				failAddr[cm.Coinbase] = false
			}
		}

		if len(fruitOkAddr) == 0 {
			return consensus.ErrInvalidSignsLength
		}

		// Equal by fruit
		committeeCoinFruitMember := new(big.Int).Div(committeeCoinFruit, big.NewInt(int64(len(fruitOkAddr))))
		for _, v := range fruitOkAddr {
			state.AddBalance(v, committeeCoinFruitMember)
			LogPrint("committee", v, committeeCoinFruitMember)
		}
	}

	return nil
}

//Reward for block allocation
func getBlockReward(num *big.Int) (committee, minerBlock, minerFruit *big.Int, e error) {
	base := new(big.Int).Div(getCurrentCoin(num), Big1e6).Int64()
	m, c, e := getDistributionRatio(NetworkFragmentsNuber)
	if e != nil {
		return
	}

	committee = new(big.Int).Mul(big.NewInt(int64(c*float64(base))), Big1e6)
	minerBlock = new(big.Int).Mul(big.NewInt(int64(m*float64(base)/3*2)), Big1e6)
	minerFruit = new(big.Int).Mul(big.NewInt(int64(m*float64(base)/3)), Big1e6)
	return
}

// get Distribution ratio for miner and committee
func getDistributionRatio(fragmentation int) (miner, committee float64, e error) {
	if fragmentation <= SqrtMin {
		return 0.8, 0.2, nil
	}
	if fragmentation >= SqrtMax {
		return 0.2, 0.8, nil
	}
	committee = SqrtArray[fragmentation]
	return 1 - committee, committee, nil
}

func powerf(x float64, n int64) float64 {
	if n == 0 {
		return 1
	} else {
		return x * powerf(x, n-1)
	}
}

//Get the total reward for the current block
func getCurrentCoin(h *big.Int) *big.Int {
	d := h.Int64() / int64(SnailBlockRewardsChangeInterval)
	ratio := big.NewInt(int64(powerf(0.98, d) * float64(SnailBlockRewardsBase)))
	return new(big.Int).Mul(ratio, Big1e6)
}
