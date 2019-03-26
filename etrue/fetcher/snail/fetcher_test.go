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
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/election"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

var (
	testdb       = etruedb.NewMemDatabase()
	testKey, _   = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddress  = crypto.PubkeyToAddress(testKey.PublicKey)
	genesis      = core.GenesisSnailBlockForTesting(testdb, testAddress, big.NewInt(1000000000))
	unknownBlock = types.NewSnailBlock(&types.SnailHeader{Number: big.NewInt(1), Difficulty: big.NewInt(150), FruitDifficulty: big.NewInt(3), FastNumber: big.NewInt(2)}, nil, nil, nil)

	gspec = &core.Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{testAddress: {Balance: big.NewInt(1000000)}},
	}
	fastgenesis = gspec.MustFastCommit(testdb)
)

// makeChain creates a chain of n blocks starting at and including parent.
// the returned hash chain is ordered head->parent. In addition, every 3rd block
// contains a transaction and every 5th an uncle to allow testing correct block
// reassembly.
//func makeChain(n int, seed byte, parent *types.SnailBlock) ([]common.Hash, map[common.Hash]*types.SnailBlock) {
//
//	blockchain, _ := core.NewBlockChain(testdb, nil, gspec.Config, ethash.NewFaker(), vm.Config{})
//	chain, _ := core.GenerateChain(gspec.Config, fastgenesis, ethash.NewFaker(), testdb, n, nil)
//	if _, err := blockchain.InsertChain(chain); err != nil {
//		panic(err)
//	}
//
//	blocks := snailchain.GenerateChain(params.TestChainConfig, blockchain,parent, ethash.NewFaker(), testdb, n, func(i int, block *snailchain.BlockGen) {
//		block.SetCoinbase(common.Address{seed})
//
//		// If the block number is multiple of 3, send a bonus transaction to the miner
//		if parent == genesis && i%3 == 0 {
//		}
//		// If the block number is a multiple of 5, add a bonus uncle to the block
//		if i%5 == 0 {
//		}
//	})
//	hashes := make([]common.Hash, n+1)
//	hashes[len(hashes)-1] = parent.Hash()
//	blockm := make(map[common.Hash]*types.SnailBlock, n+1)
//	blockm[parent.Hash()] = parent
//	for i, b := range blocks {
//		hashes[len(hashes)-i-2] = b.Hash()
//		blockm[b.Hash()] = b
//	}
//	return hashes, blockm
//}

var (
	canonicalSeed = 1
	forkSeed      = 2
)

// makeBlockChain creates a deterministic chain of blocks rooted at parent.
func makeFast(parent *types.Block, n int, engine consensus.Engine, db etruedb.Database, seed int) []*types.Block {
	engine.SetElection(election.NewFakeElection())
	blocks, _ := core.GenerateChain(params.TestChainConfig, parent, engine, db, n, func(i int, b *core.BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})

	return blocks
}

// makeChain creates a chain of n blocks starting at and including parent.
// the returned hash chain is ordered head->parent. In addition, every 3rd block
// contains a transaction and every 5th an uncle to allow testing correct block
// reassembly.
func makeChain(n int, seed byte, parent *types.SnailBlock) ([]common.Hash, map[common.Hash]*types.SnailBlock) {
	var (
		testdb  = etruedb.NewMemDatabase()
		genesis = core.DefaultGenesisBlock()
		engine  = minerva.NewFaker()
		// genesis = new(core.Genesis).MustSnailCommit(testdb)

	)

	//blocks := make(types.SnailBlocks, 2)
	cache := &core.CacheConfig{}
	fastGenesis := genesis.MustFastCommit(testdb)
	fastchain, _ := core.NewBlockChain(testdb, cache, params.AllMinervaProtocolChanges, engine, vm.Config{})
	fastblocks := makeFast(fastGenesis, n*params.MinimumFruits, engine, testdb, canonicalSeed)
	fastchain.InsertChain(fastblocks)

	snailGenesis := genesis.MustSnailCommit(testdb)
	snailChain, _ := snailchain.NewSnailBlockChain(testdb, params.TestChainConfig, engine, vm.Config{}, nil)

	_, _ = snailchain.MakeSnailBlockFruitsWithoutInsert(snailChain, fastchain, 1, n, 1, n*params.MinimumFruits, snailGenesis.PublicKey(), snailGenesis.Coinbase(), true, nil)

	return nil, nil
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

// verifyFetchingEvent verifies that one single event arrive on a fetching channel.
func verifyFetchingEvent(t *testing.T, fetching chan []common.Hash, arrive bool) {
	if arrive {
		select {
		case <-fetching:
		case <-time.After(time.Second):
			t.Fatalf("fetching timeout")
		}
	} else {
		select {
		case <-fetching:
			t.Fatalf("fetching invoked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// verifyCompletingEvent verifies that one single event arrive on an completing channel.
func verifyCompletingEvent(t *testing.T, completing chan []common.Hash, arrive bool) {
	if arrive {
		select {
		case <-completing:
		case <-time.After(time.Second):
			t.Fatalf("completing timeout")
		}
	} else {
		select {
		case <-completing:
			t.Fatalf("completing invoked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// verifyImportEvent verifies that one single event arrive on an import channel.
func verifyImportEvent(t *testing.T, imported chan *types.SnailBlock, arrive bool) {
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
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// verifyImportCount verifies that exactly count number of events arrive on an
// import hook channel.
func verifyImportCount(t *testing.T, imported chan *types.SnailBlock, count int) {
	for i := 0; i < count; i++ {
		select {
		case <-imported:
		case <-time.After(time.Second * 3):
			t.Fatalf("block %d: import timeout", i+1)
		}
	}
	verifyImportDone(t, imported)
}

// verifyImportDone verifies that no more events are arriving on an import channel.
func verifyImportDone(t *testing.T, imported chan *types.SnailBlock) {
	select {
	case <-imported:
		t.Fatalf("extra block imported")
	case <-time.After(50 * time.Millisecond):
	}
}

// Tests that blocks with numbers much lower or higher than out current head get
// discarded to prevent wasting resources on useless blocks from faulty peers.
func TestDistantPropagationDiscarding(t *testing.T) {
	// Create a long chain to import and define the discard boundaries
	hashes, blocks := makeChain(3*maxQueueDist, 0, genesis)
	head := hashes[len(hashes)/2]

	low, high := len(hashes)/2+maxUncleDist+1, len(hashes)/2-maxQueueDist-1

	// Create a tester and simulate a head block being the middle of the above chain
	tester := newTester()

	tester.lock.Lock()
	tester.hashes = []common.Hash{head}
	tester.blocks = map[common.Hash]*types.SnailBlock{head: blocks[head]}
	tester.lock.Unlock()

	// Ensure that a block with a lower number than the threshold is discarded
	tester.fetcher.Enqueue("lower", blocks[hashes[low]])
	time.Sleep(10 * time.Millisecond)
	if !tester.fetcher.queue.Empty() {
		t.Fatalf("fetcher queued stale block")
	}
	// Ensure that a block with a higher number than the threshold is discarded
	tester.fetcher.Enqueue("higher", blocks[hashes[high]])
	time.Sleep(10 * time.Millisecond)
	if !tester.fetcher.queue.Empty() {
		t.Fatalf("fetcher queued future block")
	}
}

// Tests that blocks sent to the fetcher (either through propagation or via hash
// announces and retrievals) don't pile up indefinitely, exhausting available
// system memory.
func TestBlockMemoryExhaustionAttack(t *testing.T) {
	// Create a tester with instrumented import hooks
	tester := newTester()

	imported, enqueued := make(chan *types.SnailBlock), int32(0)
	tester.fetcher.importedHook = func(block *types.SnailBlock) { imported <- block }
	tester.fetcher.queueChangeHook = func(hash common.Hash, added bool) {
		if added {
			atomic.AddInt32(&enqueued, 1)
		} else {
			atomic.AddInt32(&enqueued, -1)
		}
	}
	// Create a valid chain and a batch of dangling (but in range) blocks
	targetBlocks := 2 * maxQueueDist
	hashes, blocks := makeChain(targetBlocks, 0, genesis)
	attack := make(map[common.Hash]*types.SnailBlock)
	for i := byte(0); len(attack) < blockLimit+2*maxQueueDist; i++ {
		hashes, blocks := makeChain(maxQueueDist-1, i, unknownBlock)
		for _, hash := range hashes[:maxQueueDist-2] {
			attack[hash] = blocks[hash]
		}
	}
	// Try to feed all the attacker blocks make sure only a limited batch is accepted
	for _, block := range attack {
		tester.fetcher.Enqueue("attacker", block)
	}
	time.Sleep(200 * time.Millisecond)
	queued := atomic.LoadInt32(&enqueued)
	fmt.Println("TestBlockMemoryExhaustionAttack", "queued", queued, "attack", len(attack), "hashes", len(hashes))
	log.Trace("TestBlockMemoryExhaustionAttack", "queued", queued, "attack", len(attack), "hashes", len(hashes))
	if queued != blockLimit {
		t.Fatalf("queued block count mismatch: have %d, want %d", queued, blockLimit)
	}
	// Queue up a batch of valid blocks, and check that a new peer is allowed to do so
	for i := 0; i < maxQueueDist-1; i++ {
		tester.fetcher.Enqueue("valid", blocks[hashes[len(hashes)-3-i]])
	}
	time.Sleep(100 * time.Millisecond)
	if queued := atomic.LoadInt32(&enqueued); queued != blockLimit+maxQueueDist-1 {
		t.Fatalf("queued block count mismatch: have %d, want %d", queued, blockLimit+maxQueueDist-1)
	}
	// Insert the missing piece (and sanity check the import)
	tester.fetcher.Enqueue("valid", blocks[hashes[len(hashes)-2]])
	verifyImportCount(t, imported, maxQueueDist)

	// Insert the remaining blocks in chunks to ensure clean DOS protection
	for i := maxQueueDist; i < len(hashes)-1; i++ {
		tester.fetcher.Enqueue("valid", blocks[hashes[len(hashes)-2-i]])
		verifyImportEvent(t, imported, true)
	}
	verifyImportDone(t, imported)
}
