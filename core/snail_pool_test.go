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
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/consensus"
)

// testSnailPoolConfig is a fruit pool configuration without stateful disk
// sideeffects used during testing.
var testSnailPoolConfig SnailPoolConfig
var blockchain *BlockChain
var snailblockchain *snailchain.SnailBlockChain
var engine consensus.Engine
var chainConfig *params.ChainConfig

func init() {
	testSnailPoolConfig = DefaultSnailPoolConfig
	chainConfig = params.TestChainConfig
	testSnailPoolConfig.Journal = ""
}

type testSnailChain struct {
	chainHeadFeed *event.Feed
}

func (bc *testSnailChain) CurrentBlock() *types.SnailBlock {
	return types.NewSnailBlock(&types.SnailHeader{
		ToElect: false,
	}, nil, nil, nil)
}

func (bc *testSnailChain) Config() *params.ChainConfig {
	return params.TestnetChainConfig
}

func (bc *testSnailChain) GetBlock(hash common.Hash, number uint64) *types.SnailBlock {
	return bc.CurrentBlock()
}

func (bc *testSnailChain) CurrentHeader() *types.SnailHeader {
	return &types.SnailHeader{
		ToElect: false,
	}
}
func (bc *testSnailChain) GetHeader(hash common.Hash, number uint64) *types.SnailHeader {
	return &types.SnailHeader{
		ToElect: false,
	}
}
func (bc *testSnailChain) GetHeaderByNumber(number uint64) *types.SnailHeader {
	return bc.CurrentHeader()
}
func (bc *testSnailChain) GetHeaderByHash(hash common.Hash) *types.SnailHeader {
	return bc.CurrentHeader()
}
func (bc *testSnailChain) GetBlockByNumber(number uint64) *types.SnailBlock {
	return bc.CurrentBlock()
}
func (bc *testSnailChain) GetBlockByHash(hash common.Hash) *types.SnailBlock {
	return bc.CurrentBlock()
}
func (bc *testSnailChain) SubscribeChainHeadEvent(ch chan<- types.ChainSnailHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func fruit(fastNumber *big.Int, fruitDifficulty *big.Int) *types.SnailBlock {
	//return signFruit(nonce, gaslimit, big.NewInt(1), key)
	var header *types.SnailHeader
	header = &types.SnailHeader{
		header.Hash(),
		header.Hash(),
		common.BytesToAddress([]byte{0}),
		header.Hash(),
		big.NewInt(0),
		header.Hash(),
		header.Hash(),
		fastNumber,
		header.Hash(),
		types.BytesToBloom([]byte{0}),
		big.NewInt(0),
		fruitDifficulty,
		nil,
		[]byte{0},
		false,
		nil,
		[]byte{0},
		header.Hash(),
		[8]byte{},
		true,
	}
	var fruit *types.SnailBlock
	fruit = types.NewSnailBlock(header, nil, nil, nil)
	return fruit
}

func setupSnailPool() (*SnailPool) {
	sv := snailchain.NewBlockValidator(chainConfig, blockchain, snailblockchain, engine)
	snailblockchain := &testSnailChain{new(event.Feed)}
	pool := NewSnailPool(testSnailPoolConfig, blockchain, snailblockchain, engine, sv)
	return pool
}

// validateSnailPoolInternals checks various consistency invariants within the pool.
func validateSnailPoolInternals(pool *SnailPool) error {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	// Ensure the total fruits set is consistent with pending + unVerified
	pending, unVerified := pool.Stats()
	if total := len(pool.allFruits); total != pending+unVerified {
		return fmt.Errorf("total fruits count %d != %d pending + %d unVerified", total, pending, unVerified)
	}
	return nil
}

// validateEvents checks that the correct number of fruit addition events
// were fired on the pool's event feed.
func validateFruitEvents(events chan types.NewFruitsEvent, count int) error {
	var received []*types.SnailBlock

	for len(received) < count {
		select {
		case ev := <-events:
			received = append(received, ev.Fruits...)
		case <-time.After(time.Second):
			return fmt.Errorf("event #%d not fired", received)
		}
	}
	if len(received) > count {
		return fmt.Errorf("more than %d events fired: %v", count, received[count:])
	}
	select {
	case ev := <-events:
		return fmt.Errorf("more than %d events fired: %v", count, ev.Fruits)

	case <-time.After(50 * time.Millisecond):
		// This branch should be "default", but it's a data race between goroutines,
		// reading the event channel and pushing into it, so better wait a bit ensuring
		// really nothing gets injected.
	}
	return nil
}

func TestInvalidFruits(t *testing.T) {
	t.Parallel()
	ft1 := fruit(nil, big.NewInt(0))
	pool := setupSnailPool()
	defer pool.Stop()
	if err := pool.addFruit(ft1); err != ErrNotExist {
		t.Error("expected", ErrNotExist)
	}
}

func TestFruitQueue(t *testing.T) {
	t.Parallel()

	pool := setupSnailPool()
	defer pool.Stop()

	ft := fruit(big.NewInt(1000), big.NewInt(0))
	pool.addFruit(ft)
	//if fastNumber is bigger than pool.fastchain.CurrentBlock().Number() will only add to allFruits
	if len(pool.allFruits) != 1 {
		t.Error("expected allFruits to be 1 is", len(pool.allFruits))
	}
	if len(pool.fruitPending) > 0 {
		t.Error("expected fruit pending to be empty. is", len(pool.fruitPending))
	}

	pool = setupSnailPool()
	defer pool.Stop()

	ft1 := fruit(big.NewInt(2), big.NewInt(0))
	ft2 := fruit(big.NewInt(3), big.NewInt(0))
	ft3 := fruit(big.NewInt(4), big.NewInt(0))

	pool.addFruit(ft1)
	pool.addFruit(ft2)
	pool.addFruit(ft3)

	if len(pool.fruitPending) != 3 {
		t.Error("expected fruitPending to be 3, got", len(pool.fruitPending))
	}
	if len(pool.allFruits) != 3 {
		t.Error("expected len(allFruits) == 3, got", len(pool.allFruits))
	}
}

func TestFruitDropping(t *testing.T) {
	t.Parallel()


	pool := setupSnailPool()
	defer pool.Stop()

	// Add some pending fruits
	var (
		ft0  = fruit(big.NewInt(1), big.NewInt(0))
		ft1  = fruit(big.NewInt(2), big.NewInt(0))
		ft2  = fruit(big.NewInt(3), big.NewInt(0))
		ft10 = fruit(big.NewInt(4), big.NewInt(0))
		ft11 = fruit(big.NewInt(5), big.NewInt(0))
		ft12 = fruit(big.NewInt(6), big.NewInt(0))
	)
	var fruits []*types.SnailBlock
	fruits = append(fruits, ft0)
	fruits = append(fruits, ft1)
	fruits = append(fruits, ft2)
	pool.AddRemoteFruits(fruits)
	pool.addFruit(ft10)
	pool.addFruit(ft11)
	pool.addFruit(ft12)

	pool.RemovePendingFruitByFastHash(ft10.FastHash())
	// Check that pre and post validations leave the pool as is
	if len(pool.fruitPending) != 5 {
		t.Errorf("pending fruit mismatch: have %d, want %d", len(pool.fruitPending), 5)
	}
	if len(pool.allFruits) != 5 {
		t.Errorf("queued fruit mismatch: have %d, want %d", len(pool.allFruits), 5)
	}
}


// Tests that the pool rejects replacement fruits that a new is difficulty
// than old one.
func TestFruitReplacement(t *testing.T) {
	t.Parallel()

	// Create a test account and fund it
	pool := setupSnailPool()
	defer pool.Stop()

	// Add some pending fruits
	var (
		ft0  = fruit(big.NewInt(1),big.NewInt(0))
		ft2 = fruit(big.NewInt(1),big.NewInt(3))
	)

	pool.addFruit(ft0)
	pool.addFruit(ft2)

	if pool.fruitPending[ft0.FastHash()].FruitDifficulty().Cmp(big.NewInt(3)) != 0 {
		t.Errorf("pending fruit's difficulty mismatch: is %d, want %d", pool.fruitPending[ft0.FastHash()].FruitDifficulty(), big.NewInt(3))
	}
	if pool.allFruits[ft0.FastHash()].FruitDifficulty().Cmp(big.NewInt(3)) != 0 {
		t.Errorf("allFruits's difficulty mismatch: is %d, want %d", pool.allFruits[ft0.FastHash()].FruitDifficulty(), big.NewInt(3))
	}
}

// Tests that local fruits are journaled to disk, but remote fruits
// get discarded between restarts.
func TestFruitJournaling(t *testing.T) { testFruitJournaling(t) }

func testFruitJournaling(t *testing.T) {
	t.Parallel()

	// Create a temporary file for the journal
	file, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("failed to create temporary journal: %v", err)
	}
	journal := file.Name()
	defer os.Remove(journal)

	// Clean up the temporary file, we only need the path for now
	file.Close()
	os.Remove(journal)

	// Create the original pool to inject fruit into the journal
	config := testSnailPoolConfig
	config.Journal = journal
	config.Rejournal = time.Second

	pool := setupSnailPool()
	defer pool.Stop()

	// Add three fruits and ensure they are queued up
	if err := pool.addFruit(fruit(big.NewInt(1),big.NewInt(0))); err != nil {
		t.Fatalf("failed to add local fruit: %v", err)
	}
	if err := pool.addFruit(fruit(big.NewInt(2),big.NewInt(0))); err != nil {
		t.Fatalf("failed to add local fruit: %v", err)
	}
	if err := pool.addFruit(fruit(big.NewInt(3),big.NewInt(0))); err != nil {
		t.Fatalf("failed to add local fruit: %v", err)
	}
	if err := pool.addFruit(fruit(big.NewInt(4),big.NewInt(0))); err != nil {
		t.Fatalf("failed to add remote fruit: %v", err)
	}
	pending, unverified := pool.Stats()
	if pending != 4 {
		t.Fatalf("pending fruits mismatched: have %d, want %d", pending, 4)
	}
	if unverified != 0 {
		t.Fatalf("unverified fruits mismatched: have %d, want %d", unverified, 0)
	}
	if err := validateSnailPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	// Terminate the old pool,create a new pool and ensure relevant fruit survive
	pool.Stop()

	sv := snailchain.NewBlockValidator(chainConfig, blockchain, snailblockchain, engine)
	pool = NewSnailPool(testSnailPoolConfig, blockchain, snailblockchain, engine, sv)

	pending, unverified = pool.Stats()
	if unverified != 0 {
		t.Fatalf("unverified fruits mismatched: have %d, want %d", unverified, 0)
	}

	if err := validateSnailPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	time.Sleep(2 * config.Rejournal)
	pool.Stop()

	sv = snailchain.NewBlockValidator(chainConfig, blockchain, snailblockchain, engine)
	pool = NewSnailPool(testSnailPoolConfig, blockchain, snailblockchain, engine, sv)
	pending, unverified = pool.Stats()
	if pending != 0 {
		t.Fatalf("pending fruits mismatched: have %d, want %d", pending, 0)
	}
	if err := validateSnailPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	pool.Stop()
}

// Benchmarks the speed of iterative fruit insertion.
func BenchmarkSnailPoolInsert(b *testing.B) {
	// Generate a batch of fruits to enqueue into the pool
	pool := setupSnailPool()
	defer pool.Stop()

	fruits := make(types.Fruits, b.N)
	for i := 0; i < b.N; i++ {
		fruits[i] = fruit(big.NewInt(int64(i)),big.NewInt(0))
	}
	// Benchmark importing the fruits into the pending and allFruits
	b.ResetTimer()
	for _, tx := range fruits {
		pool.addFruit(tx)
	}
}

// Benchmarks the speed of batched fruit insertion.
func BenchmarkSnailPoolBatchInsert100(b *testing.B)   { benchmarkSnailPoolBatchInsert(b, 100) }
func BenchmarkSnailPoolBatchInsert1000(b *testing.B)  { benchmarkSnailPoolBatchInsert(b, 1000) }
func BenchmarkSnailPoolBatchInsert10000(b *testing.B) { benchmarkSnailPoolBatchInsert(b, 10000) }

func benchmarkSnailPoolBatchInsert(b *testing.B, size int) {
	// Generate a batch of fruits to enqueue into the pool
	pool := setupSnailPool()
	defer pool.Stop()

	batches := make([]types.Fruits, b.N)
	for i := 0; i < b.N; i++ {
		batches[i] = make(types.Fruits, size)
		for j := 0; j < size; j++ {
			batches[i][j] = fruit(big.NewInt(int64(size*i + j)),big.NewInt(0))
		}
	}
	// Benchmark importing the fruits into the queue
	b.ResetTimer()
	for _, batch := range batches {
		pool.AddRemoteFruits(batch)
	}
}
