package tbft

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"testing"
)

type Test struct {
	A int
}

func TestStructNil(t *testing.T) {
	test := &Test{}
	fmt.Println(test, test == nil)
}

func TestHelp(t *testing.T) {
	h := help.NewHeap()

	h.Push("msg1", 4)
	h.Push("msg3", 3)
	h.Push("msg2", 2)

	fmt.Println(h.Peek())
}

func TestChanDate(t *testing.T) {
	c := make(chan *Test, 2)
	test := &Test{A: 1}
	c <- test
	test.A = 2
	fmt.Println(<-c)
}
