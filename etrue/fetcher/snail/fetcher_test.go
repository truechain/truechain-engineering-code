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

package snailfetcher

import (
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	ethash "github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	testdb       = ethdb.NewMemDatabase()
	testKey, _   = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddress  = crypto.PubkeyToAddress(testKey.PublicKey)
	genesis      = core.GenesisSnailBlockForTesting(testdb, testAddress, big.NewInt(1000000000))
	unknownBlock = types.NewSnailBlock(&types.SnailHeader{}, nil, nil, nil)
)

// makeChain creates a chain of n blocks starting at and including parent.
// the returned hash chain is ordered head->parent. In addition, every 3rd block
// contains a transaction and every 5th an uncle to allow testing correct block
// reassembly.
func makeChain(n int, seed byte, parent *types.SnailBlock) ([]common.Hash, map[common.Hash]*types.SnailBlock) {
	blocks := snailchain.GenerateChain(params.TestChainConfig, parent, ethash.NewFaker(), testdb, n, func(i int, block *snailchain.BlockGen) {
		//block.SetCoinbase(common.Address{seed})

		// If the block number is a multiple of 5, add a bonus uncle to the block
		if i%5 == 0 {
			block.AddUncle(&types.SnailHeader{ParentHash: block.PrevBlock(i - 1).Hash(), Number: big.NewInt(int64(i - 1))})
		}
	})
	hashes := make([]common.Hash, n+1)
	hashes[len(hashes)-1] = parent.Hash()
	blockm := make(map[common.Hash]*types.SnailBlock, n+1)
	blockm[parent.Hash()] = parent
	for i, b := range blocks {
		hashes[len(hashes)-i-2] = b.Hash()
		blockm[b.Hash()] = b
	}
	return hashes, blockm
}

// fetcherTester is a test simulator for mocking out local block chain.
type fetcherTester struct {
	fetcher *Fetcher

	hashes []common.Hash                     // Hash chain belonging to the tester
	blocks map[common.Hash]*types.SnailBlock // Blocks belonging to the tester
	drops  map[string]bool                   // Map of peers dropped by the fetcher

	lock sync.RWMutex
}

// newTester creates a new fetcher test mocker.
func newTester() *fetcherTester {
	tester := &fetcherTester{
		hashes: []common.Hash{genesis.Hash()},
		blocks: map[common.Hash]*types.SnailBlock{genesis.Hash(): genesis},
		drops:  make(map[string]bool),
	}
	tester.fetcher = New(tester.getBlock, tester.verifyHeader, tester.broadcastBlock, tester.chainHeight, tester.insertChain, tester.dropPeer)
	tester.fetcher.Start()

	return tester
}

// getBlock retrieves a block from the tester's block chain.
func (f *fetcherTester) getBlock(hash common.Hash) *types.SnailBlock {
	f.lock.RLock()
	defer f.lock.RUnlock()

	return f.blocks[hash]
}

// verifyHeader is a nop placeholder for the block header verification.
func (f *fetcherTester) verifyHeader(header *types.SnailHeader) error {
	return nil
}

// broadcastBlock is a nop placeholder for the block broadcasting.
func (f *fetcherTester) broadcastBlock(block *types.SnailBlock, propagate bool) {
}

// chainHeight retrieves the current height (block number) of the chain.
func (f *fetcherTester) chainHeight() uint64 {
	f.lock.RLock()
	defer f.lock.RUnlock()

	return f.blocks[f.hashes[len(f.hashes)-1]].NumberU64()
}

// insertChain injects a new blocks into the simulated chain.
func (f *fetcherTester) insertChain(blocks types.SnailBlocks) (int, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for i, block := range blocks {
		// Make sure the parent in known
		if _, ok := f.blocks[block.ParentHash()]; !ok {
			return i, errors.New("unknown parent")
		}
		// Discard any new blocks if the same height already exists
		if block.NumberU64() <= f.blocks[f.hashes[len(f.hashes)-1]].NumberU64() {
			return i, nil
		}
		// Otherwise build our current chain
		f.hashes = append(f.hashes, block.Hash())
		f.blocks[block.Hash()] = block
	}
	return 0, nil
}

// dropPeer is an emulator for the peer removal, simply accumulating the various
// peers dropped by the fetcher.
func (f *fetcherTester) dropPeer(peer string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.drops[peer] = true
}

// verifyImportEvent verifies that one single event arrive on an import channel.
func verifyImportEvent(t *testing.T, imported chan *types.Block, arrive bool) {
	if arrive {
		select {
		case <-imported:
		case <-time.After(time.Second):
			t.Fatalf("import timeout")
		}
	} else {
		select {
		case <-imported:
			t.Fatalf("import invoked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Tests that blocks arriving from various sources (multiple propagations, hash
// announces, etc) do not get scheduled for import multiple times.
func TestImportDeduplication62(t *testing.T) { testImportDeduplication(t, 62) }
func TestImportDeduplication63(t *testing.T) { testImportDeduplication(t, 63) }
func TestImportDeduplication64(t *testing.T) { testImportDeduplication(t, 64) }

func testImportDeduplication(t *testing.T, protocol int) {
	// Create two blocks to import (one for duplication, the other for stalling)
	hashes, blocks := makeChain(1, 0, genesis)

	// Create the tester and wrap the importer with a counter
	tester := newTester()

	counter := uint32(0)
	tester.fetcher.insertChain = func(blocks types.SnailBlocks) (int, error) {
		atomic.AddUint32(&counter, uint32(len(blocks)))
		return tester.insertChain(blocks)
	}

	tester.fetcher.Enqueue("valid", blocks[hashes[0]])
	tester.fetcher.Enqueue("valid", blocks[hashes[0]])
	tester.fetcher.Enqueue("valid", blocks[hashes[0]])

	// Fill the missing block directly as if propagated, and check import uniqueness
	tester.fetcher.Enqueue("valid", blocks[hashes[1]])

	if len(tester.blocks) != 2 {
		t.Fatalf("import invocation count mismatch: have %v, want %v", counter, 2)
	}
}
