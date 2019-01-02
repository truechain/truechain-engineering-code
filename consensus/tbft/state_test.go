package tbft

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"testing"
	"time"
)

func TestTimeTicker(t *testing.T) {
	timer := time.NewTimer(time.Second * 3)
	for {
		select {
		case <-timer.C:
			fmt.Println("time C")
			timer.Reset(time.Second * 3)
		default:
			fmt.Println("time default")
		}
		time.Sleep(time.Second)
	}
}

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
