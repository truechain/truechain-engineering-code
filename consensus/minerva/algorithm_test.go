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
	"testing"

	"github.com/truechain/truechain-engineering-code/common/hexutil"
)

// Tests whether the truehash lookup works for both light as well as the full
// datasets.
func TestTruehash(t *testing.T) {
	// Create the verification cache and mining dataset
	//cache := make([]uint32, 1024/4)
	//generateCache(cache, 0, make([]byte, 32))

	dataset := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)

	//truehashTableInit(dataset)
	//generateDataset(dataset, 0, cache)

	// Create a block to verify
	hash := hexutil.MustDecode("0xc9149cc0386e689d789a1c2f3d5d169a61a6218ed30e74414dc736e442ef3d1f")
	nonce := uint64(0)

	wantDigest := hexutil.MustDecode("0xe4073cffaef931d37117cefd9afd27ea0f1cad6a981dd2605c4a1ac97c519800")
	wantResult := hexutil.MustDecode("0xd3539235ee2e6f8db665c0a72169f55b7f6c605712330b778ec3944f0eb5a557")

	//wantDigest := hexutil.MustDecode("0x47af990afa74cf47281fe85246e796e7963fce8e05c443d221aaf1ebaf238b1d")
	//wantResult := hexutil.MustDecode("0x47af990afa74cf47281fe85246e796e7963fce8e05c443d221aaf1ebaf238b1d")

	digest, result := truehashLight(dataset, hash, nonce)
	//println("have %x",digest)
	if !bytes.Equal(digest, wantDigest) {
		t.Errorf("light truehash digest mismatch: have %x, want %x", digest, wantDigest)
	}
	if !bytes.Equal(result, wantResult) {
		t.Errorf("light truehash result mismatch: have %x, want %x", result, wantResult)
	}
	digest, result = truehashFull(dataset, hash, nonce)
	if !bytes.Equal(digest, wantDigest) {
		t.Errorf("full truehash digest mismatch: have %x, want %x", digest, wantDigest)
	}
	if !bytes.Equal(result, wantResult) {
		t.Errorf("full truehash result mismatch: have %x, want %x", result, wantResult)
	}
}

// Benchmarks the light verification performance.
func BenchmarkTruehashLight(b *testing.B) {
	//cache := make([]uint32, cacheSize(1)/4)
	//generateCache(cache, 0, make([]byte, 32))

	dataset := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)

	//truehashTableInit(dataset)

	hash := hexutil.MustDecode("0xc9149cc0386e689d789a1c2f3d5d169a61a6218ed30e74414dc736e442ef3d1f")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		truehashLight(dataset, hash, 0)
	}
}

// Benchmarks the full (small) verification performance.
func BenchmarkTruehashFullSmall(b *testing.B) {
	//cache := make([]uint32, 65536/4)
	//generateCache(cache, 0, make([]byte, 32))

	//dataset := make([]uint32, 32*65536/4)
	//generateDataset(dataset, 0, cache)

	dataset := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)

	//truehashTableInit(dataset)

	hash := hexutil.MustDecode("0xc9149cc0386e689d789a1c2f3d5d169a61a6218ed30e74414dc736e442ef3d1f")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		truehashFull(dataset, hash, 0)
	}
}
