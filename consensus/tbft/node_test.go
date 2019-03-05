package tbft

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"math/rand"
	"testing"
	"time"
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

func TestHex(t *testing.T) {
	var c []byte
	c = append(c, byte(222))
	fmt.Println(common.ToHex(c), hexutil.Encode(c))
}

type Main struct {
	sub map[int]string
}

type Main2 struct {
	sub string
}

func GetRandomString(l int) string {
	str := "0123456789abcdefghijklmnopqrstuvwxyz"
	bytes := []byte(str)
	result := []byte{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < l; i++ {
		result = append(result, bytes[r.Intn(len(bytes))])
	}
	return string(result)
}

func TestStructMapNotLock(t *testing.T) {
	out := make(chan bool)
	tMain := &Main{sub: make(map[int]string)}

	j := 0
	for {
		j++
		if j > 100 {
			break
		}
		go func() {
			i := 0
			for {
				i++
				if i > 1000 {
					i = 0
				}
				tMain.sub[i] = GetRandomString(30)
			}
		}()
	}
	<-out
}

func TestStructNoMapNotLock(t *testing.T) {
	out := make(chan bool)
	tMain := &Main2{sub: ""}

	j := 0
	for {
		j++
		if j > 100 {
			break
		}
		go func() {
			i := 0
			for {
				i++
				if i > 1000 {
					i = 0
				}
				tMain.sub = GetRandomString(30)
			}
		}()
	}
	for {
		time.Sleep(time.Second)
		fmt.Println(tMain.sub)
	}
	<-out
}
