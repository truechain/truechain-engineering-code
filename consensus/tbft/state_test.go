package tbft

import (
	"fmt"
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
