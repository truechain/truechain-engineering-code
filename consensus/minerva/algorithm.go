// Copyright 2018 The truechain-engineering-code Authors
// This file is part of the truechain-engineering-code library.
//
// The truechain-engineering-code library is free software: you can
// redistribute it and/or modify it under the terms of the GNU Lesser
// General Public License as published by the Free Software Foundation,
// either version 3 of the License, or (at your option) any later version.
//
// The truechain-engineering-code library is distributed in the hope
// that it will be useful, but WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See the GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the truechain-engineering-code library.
// If not, see <http://www.gnu.org/licenses/>.

package minerva

import (
	"encoding/binary"
	"hash"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/log"
)

const (
	datasetInitBytes   = 1 << 30 // Bytes in dataset at genesis
	datasetGrowthBytes = 1 << 23 // Dataset growth per epoch
	cacheInitBytes     = 1 << 24 // Bytes in cache at genesis
	cacheGrowthBytes   = 1 << 17 // Cache growth per epoch
	epochLength        = 12000   // Blocks per epoch
	mixBytes           = 128     // Width of mix
	hashBytes          = 64      // Hash length in bytes
	hashWords          = 16      // Number of 32 bit ints in a hash
	datasetParents     = 256     // Number of parents of each dataset element
	cacheRounds        = 3       // Number of rounds in cache production
	loopAccesses       = 64      // Number of accesses in hashimoto loop
	dataBytes          = 256     // Number of absorbing data in mining loop
)

//var trueInit int = 0;
//var tableLookup [16 * 2048 * 32 * 4]uint64




// hasher is a repetitive hasher allowing the same hash data structures to be
// reused between hash runs instead of requiring new ones to be created.
type hasher func(dest []byte, data []byte)

// makeHasher creates a repetitive hasher, allowing the same hash data structures
// to be reused between hash runs instead of requiring new ones to be created.
// The returned function is not thread safe!
func makeHasher(h hash.Hash) hasher {
	return func(dest []byte, data []byte) {
		h.Write(data)
		h.Sum(dest[:0])
		h.Reset()
	}
}

// seedHash is the seed to use for generating a verification cache and the mining
// dataset.
func seedHash(block uint64) []byte {
	seed := make([]byte, 32)
	if block < epochLength {
		return seed
	}
	sha256 := makeHasher(sha3.New256())
	for i := 0; i < int(block/epochLength); i++ {
		sha256(seed, seed)
	}
	return seed
}

// swap changes the byte order of the buffer assuming a uint32 representation.
func swap(buffer []byte) {
	for i := 0; i < len(buffer); i += 4 {
		binary.BigEndian.PutUint32(buffer[i:], binary.LittleEndian.Uint32(buffer[i:]))
	}
}


// fnv is an algorithm inspired by the FNV hash, which in some cases is used as
// a non-associative substitute for XOR. Note that we multiply the prime with
// the full 32-bit input, in contrast with the FNV-1 spec which multiplies the
// prime with one byte (octet) in turn.
func fnv(a, b uint32) uint32 {
	return a*0x01000193 ^ b
}

// fnvHash mixes in data into mix using the ethash fnv method.
func fnvHash(mix []uint32, data []uint32) {
	for i := 0; i < len(mix); i++ {
		mix[i] = mix[i]*0x01000193 ^ data[i]
	}
}

// generateDatasetItem combines data from 256 pseudorandomly selected cache nodes,
// and hashes that to compute a single dataset node.
func generateDatasetItem(cache []uint32, index uint32, sha512 hasher) []byte {
	// Calculate the number of theoretical rows (we use one buffer nonetheless)
	rows := uint32(len(cache) / hashWords)

	// Initialize the mix
	mix := make([]byte, hashBytes)

	binary.LittleEndian.PutUint32(mix, cache[(index%rows)*hashWords]^index)
	for i := 1; i < hashWords; i++ {
		binary.LittleEndian.PutUint32(mix[i*4:], cache[(index%rows)*hashWords+uint32(i)])
	}
	sha512(mix, mix)

	// Convert the mix to uint32s to avoid constant bit shifting
	intMix := make([]uint32, hashWords)
	for i := 0; i < len(intMix); i++ {
		intMix[i] = binary.LittleEndian.Uint32(mix[i*4:])
	}
	// fnv it with a lot of random cache nodes based on index
	for i := uint32(0); i < datasetParents; i++ {
		parent := fnv(index^i, intMix[i%16]) % rows
		fnvHash(intMix, cache[parent*hashWords:])
	}
	// Flatten the uint32 mix into a binary one and return
	for i, val := range intMix {
		binary.LittleEndian.PutUint32(mix[i*4:], val)
	}
	sha512(mix, mix)
	return mix
}

// generateDataset generates the entire truehash dataset for mining.
// This method places the result into dest in machine byte order.
func generateDataset(dest []uint32, epoch uint64, cache []uint32) {
	// Print some debug logs to allow analysis on low end devices
	logger := log.New("epoch", epoch)

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)

		logFn := logger.Debug
		if elapsed > 3*time.Second {
			logFn = logger.Info
		}
		logFn("Generated truehash verification cache", "elapsed", common.PrettyDuration(elapsed))
	}()

	// Figure out whether the bytes need to be swapped for the machine
	swapped := !isLittleEndian()

	// Convert our destination slice to a byte buffer
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&dest))
	header.Len *= 4
	header.Cap *= 4
	dataset := *(*[]byte)(unsafe.Pointer(&header))

	// Generate the dataset on many goroutines since it takes a while
	threads := runtime.NumCPU()
	size := uint64(len(dataset))

	var pend sync.WaitGroup
	pend.Add(threads)

	var progress uint32
	for i := 0; i < threads; i++ {
		go func(id int) {
			defer pend.Done()

			// Create a hasher to reuse between invocations
			sha512 := makeHasher(sha3.New512())

			// Calculate the data segment this thread should generate
			batch := uint32((size + hashBytes*uint64(threads) - 1) / (hashBytes * uint64(threads)))
			first := uint32(id) * batch
			limit := first + batch
			if limit > uint32(size/hashBytes) {
				limit = uint32(size / hashBytes)
			}
			// Calculate the dataset segment
			percent := uint32(size / hashBytes / 100)
			for index := first; index < limit; index++ {
				item := generateDatasetItem(cache, index, sha512)
				if swapped {
					swap(item)
				}
				copy(dataset[index*hashBytes:], item)

				if status := atomic.AddUint32(&progress, 1); status%percent == 0 {
					logger.Info("Generating DAG in progress", "percentage", uint64(status*100)/(size/hashBytes), "elapsed", common.PrettyDuration(time.Since(start)))
				}
			}
		}(i)
	}
	// Wait for all the generators to finish and return
	pend.Wait()
}

// truehash aggregates data from the full dataset in order to produce our final
// value for a particular header hash and nonce.
func truehash(plookup []uint64, hash []byte, nonce uint64) ([]byte, []byte) {
	// Calculate the number of theoretical rows (we use one buffer nonetheless)
	return fchainmining(plookup[:], hash[:], nonce)
}

// truehashLight aggregates data from the full dataset (using only a small
// in-memory cache) in order to produce our final value for a particular header
// hash and nonce.
func truehashLight(dataset []uint64, hash []byte, nonce uint64) ([]byte, []byte) {
	return truehash(dataset[:], hash[:], nonce)
}

// truehashFull aggregates data from the full dataset (using the full in-memory
// dataset) in order to produce our final value for a particular header hash and
// nonce.

func truehashFull(dataset []uint64, hash []byte, nonce uint64) ([]byte, []byte) {

	return truehash(dataset[:], hash[:], nonce)
}




const maxEpoch = 2048

