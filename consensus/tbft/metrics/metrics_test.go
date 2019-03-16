package metrics

import (
	"testing"
	"time"
)

func TestMTimes(t *testing.T) {
	MTimes(PreVoteTime, false)
	time.Sleep(time.Second)
	MTimes(PreVoteTime, true)
}
