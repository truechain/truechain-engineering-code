package help

import (
	"fmt"
	"testing"
)

type TStruct struct {
	Id string
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
	a := TStruct{Id: "ac", T2: t2}
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
	a := TStruct{Id: "ac", T2: t2}
	var tOut TStruct

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
