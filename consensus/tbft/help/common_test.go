package help

import (
	"fmt"
	"github.com/tendermint/tendermint/types/time"
	types2 "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
	"math/rand"
	"testing"
)

type TStruct struct {
	Id *big.Float
	T2 TStruct2
}

type TStruct2 struct {
	Id string
}

func TestJson(t *testing.T) {
	//json
	TestMap := make(map[string]uint64)
	TestMap2 := make(map[string]uint64)
	TestMap["a"] = 6
	byte, _ := MarshalJSON(TestMap)
	fmt.Println(string(byte))
	if err := UnmarshalJSON(byte, &TestMap2); err == nil {
		fmt.Println(TestMap2)
	}

}

func TestBinaryBare(t *testing.T) {
	//TestBinary
	t2 := TStruct2{Id: "ab"}
	a := TStruct{Id: big.NewFloat(0.001), T2: t2}
	var tOut TStruct
	byte2, err := MarshalBinaryBare(a)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(string(byte2))
	if err := UnmarshalBinaryBare(byte2, &tOut); err == nil {
		fmt.Println(tOut)
	} else {
		fmt.Println(err.Error())
	}
}

func TestMarshalBinary(t *testing.T) {
	t2 := TStruct2{Id: "ab"}
	a := TStruct{Id: big.NewFloat(1.001), T2: t2}
	var tOut TStruct
	fmt.Println(a)
	byte2, err := MarshalBinary(a)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(string(byte2))
	if err := UnmarshalBinary(byte2, &tOut); err == nil {
		fmt.Println(tOut)
	} else {
		fmt.Println(err.Error())
	}
}

func TestReader(t *testing.T) {
	header := &types.Header{}
	header.Time = big.NewInt(time.Now().Unix())
	header.ParentHash = RandHexBytes()
	header.Number = header.Time

	var tr []*types.Transaction

	for i := 0; i < 1000; i++ {
		t := types.NewTransaction(1, RandHexBytes20(), big.NewInt(1), 8888, big.NewInt(1), nil)
		tr = append(tr, t)
	}

	var re []*types.Receipt
	var si []*types.PbftSign

	bTmp := types.NewBlock(header, tr, re, si)

	ps := types2.MakePartSet(64*1024, bTmp)
	pe := types2.NewPartSetFromHeader(ps.Header())

	header2 := &types.Header{}

	UnmarshalBinaryReader(pe.GetReader(), &header2, 1000)

	fmt.Println(header)
	fmt.Println(header2)

}

func RandUint() uint8 {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return uint8(random.Intn(255))
}

func RandHexBytes() [32]byte {
	var b [32]byte
	for i := 0; i < 32; i++ {
		b[i] = RandUint() % 255
	}
	return b
}

func RandHexBytes20() [20]byte {
	var b [20]byte
	for i := 0; i < 20; i++ {
		b[i] = RandUint() % 255
	}
	return b
}

func TestAbc(t *testing.T) {
	var a int = -1

	fmt.Println(uint(a))
	fmt.Println(int(uint(a)))
}
