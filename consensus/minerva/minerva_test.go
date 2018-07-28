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
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"testing"

	"github.com/truechain/truechain-engineering-code/core/types"
)

// Tests that ethash works correctly in test mode.
func TestTestMode(t *testing.T) {
	head := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1000)}

	ethash := NewTester()
	send := make(chan *types.Block, 10)
	stop := make(chan struct{})
	go ethash.ConSeal(nil, types.NewBlockWithHeader(head), stop, send)

	for block := range send {
		if block == nil {
			t.Fatalf("failed to seal fruit/block")
			close(stop)
			break
		}

		head.Fruit = block.IsFruit()
		head.Nonce = types.EncodeNonce(block.Nonce())
		head.MixDigest = block.MixDigest()

		//t.Log("%v", head)
		if err := ethash.VerifySeal(nil, head); err != nil {
			t.Fatalf("unexpected verification error: %v", err)
		}

		if !block.IsFruit() {
			break
		}
	}
	close(stop)
}

// This test checks that cache lru logic doesn't crash under load.
// It reproduces https://github.com/truechain/truechain-engineering-code/issues/14943
func TestCacheFileEvict(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "ethash-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	e := New(Config{CachesInMem: 3, CachesOnDisk: 10, CacheDir: tmpdir, PowMode: ModeTest})

	workers := 8
	epochs := 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go verifyTest(&wg, e, i, epochs)
	}
	wg.Wait()
}

func verifyTest(wg *sync.WaitGroup, e *Minerva, workerIndex, epochs int) {

	defer wg.Done()
	const wiggle = 4 * epochLength
	r := rand.New(rand.NewSource(int64(workerIndex)))
	for epoch := 0; epoch < epochs; epoch++ {
		block := int64(epoch)*epochLength - wiggle/2 + r.Int63n(wiggle)
		if block < 0 {
			block = 0
		}
		head := &types.Header{Number: big.NewInt(block), Difficulty: big.NewInt(100)}
		e.VerifySeal(nil, head)
	}
}
