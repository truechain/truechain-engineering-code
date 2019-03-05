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
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/sha3"
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

func makeDatasetHash(dataset []uint64) {
	var datas []byte
	tmp := make([]byte, 8)
	for _, v := range dataset {
		binary.LittleEndian.PutUint64(tmp, v)
		datas = append(datas, tmp...)
	}
	fmt.Println("datalen:", len(datas), "256:", datas[256])
	sha512 := makeHasher(sha3.New512())
	var sha512_out [64]byte
	sha512(sha512_out[:], datas[:])
	fmt.Println("seedhash:", hexutil.Encode(sha512_out[:]))
}
func makeTestSha3() {
	datas := []byte{1, 2, 3, 4, 5, 6}

	sha512 := makeHasher(sha3.New512())
	var sha512_out [64]byte

	sha512(sha512_out[:], datas[:])
	fmt.Println("seedhash:", hexutil.Encode(sha512_out[:]))
}
func TestTrueHash2(t *testing.T) {
	makeTestSha3()
	dataset := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	var table [TBLSIZE * DATALENGTH * PMTSIZE]uint32

	for k := 0; k < TBLSIZE; k++ {
		for x := 0; x < DATALENGTH*PMTSIZE; x++ {
			table[k*DATALENGTH*PMTSIZE+x] = tableOrg[k][x]
		}
	}
	genLookupTable(dataset[:], table[:])
	makeDatasetHash(dataset)

	hash := hexutil.MustDecode("0x645cf20198c2f3861e947d4f67e3ab63b7b2e24dcc9095bd9123e7b33371f6cc")
	nonce := uint64(79183) // mining for windows

	tt := new(big.Int).Div(maxUint128, big.NewInt(100000))
	tb := tt.Bytes()
	tmp_target := make([]byte, 16-len(tb))
	tmp_target = append(tmp_target, tb...)
	fmt.Println("target:", hexutil.Encode(tb))

	target := new(big.Int).SetBytes(tmp_target)
	target2 := new(big.Int).SetBytes(tb)
	if target.Cmp(target2) == 0 {
		fmt.Println("target equal...")
	}
	//fruitTarget := new(big.Int).SetBytes([]byte{0, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})

	for {
		_, result := truehashLight(dataset, hash, nonce)
		headResult := result[:16]
		if new(big.Int).SetBytes(headResult).Cmp(target) <= 0 {
			// get block
			break

		} else {
			lastResult := result[16:]
			if new(big.Int).SetBytes(lastResult).Cmp(target) <= 0 {
				break
			}
		}
		nonce++
	}

}
