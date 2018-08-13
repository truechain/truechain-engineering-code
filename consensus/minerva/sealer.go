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
	crand "crypto/rand"
	"math"
	"math/big"
	"math/rand"
	"runtime"
	"sync"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
)

// Seal implements consensus.Engine, attempting to find a nonce that satisfies
// the block's difficulty requirements.
func (m *Minerva) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error) {
	return nil, nil
}
func (m *Minerva) SealSnail(chain consensus.SnailChainReader, block *types.SnailBlock, stop <-chan struct{}) (*types.SnailBlock, error) {
	// If we're running a fake PoW, simply return a 0 nonce immediately
	if m.config.PowMode == ModeFake || m.config.PowMode == ModeFullFake {
		header := block.Header()
		header.Nonce, header.MixDigest = types.BlockNonce{}, common.Hash{}
		return block.WithSeal(header), nil
	}
	// If we're running a shared PoW, delegate sealing to it
	if m.shared != nil {
		return m.shared.SealSnail(chain, block, stop)
	}
	// Create a runner and the multiple search threads it directs
	abort := make(chan struct{})
	found := make(chan *types.SnailBlock)

	m.lock.Lock()
	threads := m.threads
	if m.rand == nil {
		seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			m.lock.Unlock()
			return nil, err
		}
		m.rand = rand.New(rand.NewSource(seed.Int64()))
	}
	m.lock.Unlock()
	if threads == 0 {
		threads = runtime.NumCPU()
	}
	if threads < 0 {
		threads = 0 // Allows disabling local mining without extra logic around local/remote
	}
	var pend sync.WaitGroup
	for i := 0; i < threads; i++ {
		pend.Add(1)
		go func(id int, nonce uint64) {
			defer pend.Done()
			m.mineSnail(block, id, nonce, abort, found)
		}(i, uint64(m.rand.Int63()))
	}
	// Wait until sealing is terminated or a nonce is found
	var result *types.SnailBlock
	select {
	case <-stop:
		// Outside abort, stop all miner threads
		close(abort)
		//TODO found function
		/*
			case result = <-found:
				// One of the threads found a block, abort all others
				close(abort)
		*/
	case <-m.update:
		// Thread count was changed on user request, restart
		close(abort)
		pend.Wait()
		return m.SealSnail(chain, block, stop)
	}
	// Wait for all miners to terminate and return the block
	pend.Wait()
	return result, nil
}

// Seal implements consensus.Engine, attempting to find a nonce that satisfies
// the block's difficulty requirements.
func (m *Minerva) ConSeal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}, send chan *types.Block) {
	//return nil,nil
}
func (m *Minerva) ConSnailSeal(chain consensus.SnailChainReader, block *types.SnailBlock, stop <-chan struct{}, send chan *types.SnailBlock) {
	// If we're running a fake PoW, simply return a 0 nonce immediately
	if m.config.PowMode == ModeFake || m.config.PowMode == ModeFullFake {
		header := block.Header()
		header.Nonce, header.MixDigest = types.BlockNonce{}, common.Hash{}
		send <- block.WithSeal(header)
		//return block.WithSeal(header), nil
	}
	// If we're running a shared PoW, delegate sealing to it
	if m.shared != nil {
		m.shared.ConSnailSeal(chain, block, stop, send)
	}
	// Create a runner and the multiple search threads it directs
	abort := make(chan struct{})
	found := make(chan *types.SnailBlock)

	m.lock.Lock()
	threads := m.threads
	if m.rand == nil {
		seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			m.lock.Unlock()
			send <- nil
			//return nil, err
		}
		m.rand = rand.New(rand.NewSource(seed.Int64()))
	}
	m.lock.Unlock()
	if threads == 0 {
		threads = runtime.NumCPU()
	}
	if threads < 0 {
		threads = 0 // Allows disabling local mining without extra logic around local/remote
	}
	var pend sync.WaitGroup
	for i := 0; i < threads; i++ {
		pend.Add(1)
		go func(id int, nonce uint64) {
			defer pend.Done()
			m.mineSnail(block, id, nonce, abort, found)
		}(i, uint64(m.rand.Int63()))
	}
	// Wait until sealing is terminated or a nonce is found
	var result *types.SnailBlock

mineloop:
	for {
		select {
		case <-stop:
			// Outside abort, stop all miner threads
			close(abort)
			pend.Wait()
			break mineloop
		case result = <-found:
			// One of the threads found a block or fruit return it
			send <- result
			// TODO snail need a flag to distinguish furit and block
			/*
				if !result.IsFruit() {
					// stop threads when get a block, wait for outside abort when result is fruit
					//close(abort)
					pend.Wait()
					break mineloop
				}
			*/
			break
		case <-m.update:
			// Thread count was changed on user request, restart
			close(abort)
			pend.Wait()
			m.ConSnailSeal(chain, block, stop, send)
			break mineloop
		}
	}
	// Wait for all miners to terminate and return the block

	//send <- result
	//return result, nil
}

// mine is the actual proof-of-work miner that searches for a nonce starting from
// seed that results in correct final block difficulty.
func (m *Minerva) mine(block *types.Block, id int, seed uint64, abort chan struct{}, found chan *types.Block) {
	// backup
	/*
		// Extract some data from the header
		var (
			header          = block.Header()
			hash            = header.HashNoNonce().Bytes()
			target          = new(big.Int).Div(maxUint256, header.Difficulty)
			fruitDifficulty = new(big.Int).Div(header.Difficulty, FruitBlockRatio)
			fruitTarget     = new(big.Int).Div(maxUint128, fruitDifficulty)
			number          = header.Number.Uint64()
			dataset         = m.dataset(number)
		)
		// Start generating random nonces until we abort or find a good one
		var (
			attempts = int64(0)
			nonce    = seed
		)
		logger := log.New("miner", id)
		logger.Trace("Started ethash search for new nonces", "seed", seed)
	search:
		for {
			select {
			case <-abort:
				// Mining terminated, update stats and abort
				logger.Trace("Ethash nonce search aborted", "attempts", nonce-seed)
				ethash.hashrate.Mark(attempts)
				break search

			default:
				// We don't have to update hash rate on every nonce, so update after after 2^X nonces
				attempts++
				if (attempts % (1 << 15)) == 0 {
					ethash.hashrate.Mark(attempts)
					attempts = 0
				}
				// Compute the PoW value of this nonce
				digest, result := hashimotoFull(dataset.dataset, hash, nonce)
				if new(big.Int).SetBytes(result).Cmp(target) <= 0 {
					// Correct nonce found, create a new header with it
					header = types.CopyHeader(header)
					header.Nonce = types.EncodeNonce(nonce)
					header.MixDigest = common.BytesToHash(digest)
					header.Fruit = false

					// Seal and return a block (if still needed)
					select {
					case found <- block.WithSeal(header):
						logger.Trace("Ethash nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
					case <-abort:
						logger.Trace("Ethash nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
					}
					break search
				} else {
					lastResult := result[16:]
					if new(big.Int).SetBytes(lastResult).Cmp(fruitTarget) <= 0 {
						// last 128 bit < Dpf, get a fruit
						header = types.CopyHeader(header)
						header.Nonce = types.EncodeNonce(nonce)
						header.MixDigest = common.BytesToHash(digest)
						header.Fruit = true

						// Seal and return a block (if still needed)
						select {
						case found <- block.WithSeal(header):
							logger.Trace("IsFruit nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
						case <-abort:
							logger.Trace("IsFruit nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
						}
					}
				}
				nonce++
			}
		}
		// Datasets are unmapped in a finalizer. Ensure that the dataset stays live
		// during sealing so it's not unmapped while being read.
		runtime.KeepAlive(dataset)
	*/
}
func (m *Minerva) mineSnail(block *types.SnailBlock, id int, seed uint64, abort chan struct{}, found chan *types.SnailBlock) {
	// Extract some data from the header
	var (
		header          = block.Header()
		hash            = header.HashNoNonce().Bytes()
		target          = new(big.Int).Div(maxUint256, header.Difficulty)
		fruitDifficulty = new(big.Int).Div(header.Difficulty, FruitBlockRatio)
		fruitTarget     = new(big.Int).Div(maxUint128, fruitDifficulty)
		number          = header.Number.Uint64()
		dataset         = m.dataset(number)
	)
	// Start generating random nonces until we abort or find a good one
	var (
		attempts = int64(0)
		nonce    = seed
	)
	logger := log.New("miner", id)
	logger.Trace("Started ethash search for new nonces", "seed", seed)
search:
	for {
		select {
		case <-abort:
			// Mining terminated, update stats and abort
			logger.Trace("m nonce search aborted", "attempts", nonce-seed)
			m.hashrate.Mark(attempts)
			break search

		default:
			// We don't have to update hash rate on every nonce, so update after after 2^X nonces
			attempts++
			if (attempts % (1 << 15)) == 0 {
				m.hashrate.Mark(attempts)
				attempts = 0
			}
			// Compute the PoW value of this nonce
			digest, result := hashimotoFull(dataset.dataset, hash, nonce)
			if new(big.Int).SetBytes(result).Cmp(target) <= 0 {
				// Correct nonce found, create a new header with it
				header = types.CopySnailHeader(header)
				header.Nonce = types.EncodeNonce(nonce)
				header.MixDigest = common.BytesToHash(digest)
				//TODO need add fruit flow
				//header.Fruit = false

				// Seal and return a block (if still needed)
				select {
				case found <- block.WithSeal(header):
					logger.Trace("Ethash nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
				case <-abort:
					logger.Trace("Ethash nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
				}
				break search
			} else {
				lastResult := result[16:]
				if new(big.Int).SetBytes(lastResult).Cmp(fruitTarget) <= 0 {
					// last 128 bit < Dpf, get a fruit
					header = types.CopySnailHeader(header)
					header.Nonce = types.EncodeNonce(nonce)
					header.MixDigest = common.BytesToHash(digest)
					//TODO need add fruit flow
					//header.Fruit = true

					// Seal and return a block (if still needed)
					select {
					case found <- block.WithSeal(header):
						logger.Trace("IsFruit nonce found and reported", "attempts", nonce-seed, "nonce", nonce)
					case <-abort:
						logger.Trace("IsFruit nonce found but discarded", "attempts", nonce-seed, "nonce", nonce)
					}
				}
			}
			nonce++
		}
	}
	// Datasets are unmapped in a finalizer. Ensure that the dataset stays live
	// during sealing so it's not unmapped while being read.
	runtime.KeepAlive(dataset)
}
