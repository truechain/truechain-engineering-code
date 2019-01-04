package tbft

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"testing"
)

type A struct {
	B []byte
	C common.Hash
	D int
	E int
}

func TestNoteBuild(t *testing.T) {
	var F []*A

	F = append(F, &A{D: 1})
	for _, v := range F {
		v.E = 999
	}
	fmt.Println(F[0].E)
}
