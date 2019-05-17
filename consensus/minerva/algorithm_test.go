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
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"golang.org/x/crypto/sha3"
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
	sha512 := makeHasher(sha3.New256())
	output := make([]byte, 32)
	sha512(output, datas[:])
	//fmt.Println("**seedhash:", output)
	fmt.Println("**seedhash:", hexutil.Encode(output))
}
func makeDatasetHash2(dataset []int) []byte {
	var datas []byte
	tmp := make([]byte, 4)
	for _, v := range dataset {
		binary.LittleEndian.PutUint32(tmp, uint32(v))
		datas = append(datas, tmp...)
	}
	sha512 := makeHasher(sha3.New256())
	output := make([]byte, 32)
	sha512(output, datas[:])
	return output
}
func makeDatasetHash3(dataset []uint64) []byte {
	var datas []byte
	tmp := make([]byte, 8)
	for _, v := range dataset {
		binary.LittleEndian.PutUint64(tmp, v)
		datas = append(datas, tmp...)
	}
	sha512 := makeHasher(sha3.New256())
	output := make([]byte, 32)
	sha512(output, datas[:])
	return output
}
func makeDatasetHash4(dataset []common.Hash) []byte {
	var datas []byte
	// tmp := make([]byte, 0)
	for _, v := range dataset {
		// binary.LittleEndian.PutUint64(tmp, v)
		hash := v[:16]
		datas = append(datas, hash[:]...)
	}
	sha512 := makeHasher(sha3.New256())
	output := make([]byte, 32)
	sha512(output, datas[:16])
	return output
}
func makeTestSha3() {
	datas := []byte{1, 2, 3, 4, 5, 6}

	sha512 := makeHasher(sha3.New512())
	var sha512_out [64]byte

	sha512(sha512_out[:], datas[:])
	fmt.Println("seedhash:", hexutil.Encode(sha512_out[:]))
}
func makeTestsha256() {
	data := []byte{1, 2, 3, 4, 5, 6}
	fmt.Println("datalen:", len(data))
	sha256 := makeHasher(sha3.New256())
	var sha256_out [32]byte

	sha256(sha256_out[:], data[:])
	fmt.Println("seedhash:", hexutil.Encode(sha256_out[:]))
	fmt.Println("seedhash:", sha256_out[:])
}
func readLine(fileName string, handler func(string)) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	buf := bufio.NewReader(f)
	for {
		line, err := buf.ReadString('\n')
		line = strings.TrimSpace(line)
		handler(line)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
func fetchhashfromFile(fileName string) []common.Hash {
	heads := make([]common.Hash, 0)
	readLine(fileName, func(line string) {
		if hash, err := hex.DecodeString(line); err == nil {
			heads = append(heads, common.BytesToHash(hash))
		}
	})
	return heads
}
func TestMakeDatasetHash(t *testing.T) {
	dataset1 := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	var table [TBLSIZE * DATALENGTH * PMTSIZE]uint32

	for k := 0; k < TBLSIZE; k++ {
		for x := 0; x < DATALENGTH*PMTSIZE; x++ {
			table[k*DATALENGTH*PMTSIZE+x] = tableOrg[k][x]
		}
	}
	genLookupTable(dataset1[:], table[:])
	makeDatasetHash(dataset1)

	filename := "d:\\1.txt"
	aheads := fetchhashfromFile(filename)
	heads := aheads[:10240]
	fmt.Println("test head:", len(heads))
	headhash := makeDatasetHash4(heads)
	fmt.Println("headhash:", hexutil.Encode(headhash))
	fmt.Println("test 2 dataset")
	dataset2 := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	dataset2 = updateLookupTBL(dataset2, heads)
	makeDatasetHash(dataset2)
	fmt.Println("finish2...")
}
func updateLookupTBL(plookupTbl []uint64, heads []common.Hash) []uint64 {
	const offsetCnst = 0x7
	const skipCnst = 0x3
	var offset [OFF_SKIP_LEN]int
	var skip [OFF_SKIP_LEN]int

	//get offset cnst  8192 lenght
	for i := 0; i < OFF_CYCLE_LEN; i++ {
		hash := heads[i]
		val := hash.Bytes()
		// if i == 0 {
		// 	fmt.Println("hash0:", val)
		// } else if i == OFF_CYCLE_LEN-1 {
		// 	fmt.Println("hash", OFF_CYCLE_LEN, ":", val)
		// }

		offset[i*4] = (int(val[0]) & offsetCnst) - 4
		offset[i*4+1] = (int(val[1]) & offsetCnst) - 4
		offset[i*4+2] = (int(val[2]) & offsetCnst) - 4
		offset[i*4+3] = (int(val[3]) & offsetCnst) - 4
	}
	ohash := makeDatasetHash2(offset[:])
	fmt.Println("ohash:", hexutil.Encode(ohash))
	//get skip cnst 2048 lenght
	for i := 0; i < SKIP_CYCLE_LEN; i++ {
		hash := heads[i+OFF_CYCLE_LEN]
		val := hash.Bytes()
		// if i == 0 {
		// 	fmt.Println("2hash0:", val)
		// } else if i == SKIP_CYCLE_LEN-1 {
		// 	fmt.Println("2hash", SKIP_CYCLE_LEN, ":", val)
		// }
		for k := 0; k < 16; k++ {
			skip[i*16+k] = (int(val[k]) & skipCnst) + 1
		}
	}
	shash := makeDatasetHash2(skip[:])
	fmt.Println("shash:", hexutil.Encode(shash))
	phash := makeDatasetHash3(plookupTbl)
	fmt.Println("phash:", hexutil.Encode(phash))
	ds := updateTBL(offset, skip, plookupTbl)
	return ds
}
func getdataset(pos int) []uint64 {
	dataset1 := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	var table [TBLSIZE * DATALENGTH * PMTSIZE]uint32

	for k := 0; k < TBLSIZE; k++ {
		for x := 0; x < DATALENGTH*PMTSIZE; x++ {
			table[k*DATALENGTH*PMTSIZE+x] = tableOrg[k][x]
		}
	}
	genLookupTable(dataset1[:], table[:])
	if pos == 0 {
		makeDatasetHash(dataset1)
		return dataset1
	}
	filename := "d:\\1.txt"
	aheads := fetchhashfromFile(filename)
	heads := aheads[:10240]
	dataset2 := make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	dataset2 = updateLookupTBL(dataset2, heads)
	makeDatasetHash(dataset2)
	return dataset2
}

func TestTrueHash3(t *testing.T) {
	dataset := getdataset(1)
	hash := hexutil.MustDecode("0xf99f2b620894fee2d313937eb2492a571df0db77aa48f7f51ec99cd9f96c2a6d")
	nonce := uint64(11) // mining for windows

	btarget, _ := hexutil.Decode("0x0ccccccccccccccccccccccccccccccc")
	target := new(big.Int).SetBytes(btarget)

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
func TestTrueHash2(t *testing.T) {
	makeTestsha256()
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
	nonce := uint64(80408) // mining for windows

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
func truehashTableInit(tableLookup []uint64) {

	log.Debug("truehashTableInit start ")
	var table [TBLSIZE * DATALENGTH * PMTSIZE]uint32

	for k := 0; k < TBLSIZE; k++ {
		for x := 0; x < DATALENGTH*PMTSIZE; x++ {
			table[k*DATALENGTH*PMTSIZE+x] = tableOrg[k][x]
		}
		//fmt.Printf("%d,", k+1)
	}
	genLookupTable(tableLookup[:], table[:])
}

func updateTBL(offset [OFF_SKIP_LEN]int, skip [OFF_SKIP_LEN]int, plookupTbl []uint64) []uint64 {

	// fmt.Println("aaaaa:", plookupTbl[990], plookupTbl[991], plookupTbl[992], plookupTbl[993])
	lktWz := uint32(DATALENGTH / 64)
	lktSz := uint32(DATALENGTH) * lktWz

	for k := 0; k < TBLSIZE; k++ {

		plkt := uint32(k) * lktSz

		for x := 0; x < DATALENGTH; x++ {
			idx := k*DATALENGTH + x
			pos := offset[idx] + x
			sk := skip[idx]
			y := pos - sk*PMTSIZE/2
			c := 0
			for i := 0; i < PMTSIZE; i++ {
				if y >= 0 && y < SKIP_CYCLE_LEN {
					vI := uint32(y / 64)
					vR := uint32(y % 64)
					plookupTbl[plkt+vI] |= 1 << vR
					c = c + 1
				}
				y = y + sk
			}
			if c == 0 {
				vI := uint32(x / 64)
				vR := uint32(x % 64)
				plookupTbl[plkt+vI] |= 1 << vR
			}
			plkt += lktWz
		}
	}
	fmt.Println("ddddd:", plookupTbl[990], plookupTbl[991], plookupTbl[992], plookupTbl[993])
	return plookupTbl
}
