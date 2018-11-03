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
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"fmt"
	"github.com/truechain/truechain-engineering-code/common/math"
	osMath "math"
)

type diffTest struct {
	ParentTimestamp    uint64
	ParentDifficulty   *big.Int
	CurrentTimestamp   uint64
	CurrentBlocknumber *big.Int
	CurrentDifficulty  *big.Int
}

func (d *diffTest) UnmarshalJSON(b []byte) (err error) {
	var ext struct {
		ParentTimestamp    string
		ParentDifficulty   string
		CurrentTimestamp   string
		CurrentBlocknumber string
		CurrentDifficulty  string
	}
	if err := json.Unmarshal(b, &ext); err != nil {
		return err
	}

	d.ParentTimestamp = math.MustParseUint64(ext.ParentTimestamp)
	d.ParentDifficulty = math.MustParseBig256(ext.ParentDifficulty)
	d.CurrentTimestamp = math.MustParseUint64(ext.CurrentTimestamp)
	d.CurrentBlocknumber = math.MustParseBig256(ext.CurrentBlocknumber)
	d.CurrentDifficulty = math.MustParseBig256(ext.CurrentDifficulty)

	return nil
}

func TestCalcDifficulty(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "tests", "testdata", "BasicTests", "difficulty.json"))
	if err != nil {
		t.Skip(err)
	}
	defer file.Close()

	tests := make(map[string]diffTest)
	err = json.NewDecoder(file).Decode(&tests)
	if err != nil {
		t.Fatal(err)
	}

	//for name, test := range tests {
	//	number := new(big.Int).Sub(test.CurrentBlocknumber, big.NewInt(1))
	//	diff := CalcDifficulty(config, test.CurrentTimestamp, &types.SnailHeader{
	//		Number:     number,
	//		Time:       new(big.Int).SetUint64(test.ParentTimestamp),
	//		Difficulty: test.ParentDifficulty,
	//	})
	//	if diff.Cmp(test.CurrentDifficulty) != 0 {
	//		t.Error(name, "failed. Expected", test.CurrentDifficulty, "and calculated", diff)
	//	}
	//}
}

func TestAccountDiv(t *testing.T) {
	r := new(big.Int)
	println(r.Uint64())
	r = big.NewInt(600077777777777)
	println(r.Uint64())
	r.Div(r, big2999999)
	println(r.Uint64(), FrontierBlockReward.Uint64(), SnailBlockRewardsInitial.Bytes())
	fmt.Printf("%v", new(big.Int).Exp(new(big.Int).SetInt64(2),
		new(big.Int).Div(new(big.Int).Add(new(big.Int).SetInt64(5000), new(big.Int).SetInt64(12)), new(big.Int).SetInt64(5000)), nil))
}

func TestOutSqrt(t *testing.T) {
	var AConstSqrt []ConstSqrt
	var ARR []float64
	for i := 1; i <= 10000; i++ {

		tmp := osMath.Sqrt(float64(i)) / (osMath.Sqrt(float64(i)) + float64(MiningConstant))

		if tmp > 0.8 {
			break
		}

		if tmp < 0.2 {
			ARR = append(ARR, tmp)
			continue
		}
		ARR = append(ARR, tmp)
		AConstSqrt = append(AConstSqrt, ConstSqrt{Num: i, Sqrt: tmp})
	}

	b, _ := json.Marshal(AConstSqrt)
	fmt.Println(ARR)
	fmt.Println(string(b))
}

//Calculate the reward distribution corresponding to the slow block height
//There is a new distribution incentive for every 4,500 blocks.
//The unit of output is wei
//6 bits at the end are cleared
func TestSnailAwardForHeight(t *testing.T) {
	for i := 1; i < 1000; i++ {
		snailBlockNumber := new(big.Int).SetInt64(int64(1 + 4500*(i-1)))
		fmt.Println("snailBlockNumber:", snailBlockNumber, "Award:", getCurrentCoin(snailBlockNumber))
		committeeAward, minerAward, minerFruitAward, _ := getBlockReward(snailBlockNumber)
		fmt.Println("committeeAward:", committeeAward, "minerAward:", minerAward, "minerFruitAward:", minerFruitAward)
	}
}
