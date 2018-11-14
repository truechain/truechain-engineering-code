package help

import (
	"github.com/truechain/truechain-engineering-code/log"
	"sync"
	"time"
)

/*
ThrottleTimer fires an event at most "dur" after each .Set() call.
If a short burst of .Set() calls happens, ThrottleTimer fires once.
If a long continuous burst of .Set() calls happens, ThrottleTimer fires
at most once every "dur".
*/
type ThrottleTimer struct {
	Name string
	Ch   chan struct{}
	quit chan struct{}
	dur  time.Duration

	mtx   sync.Mutex
	timer *time.Timer
	isSet bool
}

func NewThrottleTimer(name string, dur time.Duration) *ThrottleTimer {
	var ch = make(chan struct{})
	var quit = make(chan struct{})
	var t = &ThrottleTimer{Name: name, Ch: ch, dur: dur, quit: quit}
	log.Debug("ThrottleTimer", "lock", 2)
	t.mtx.Lock()
	t.timer = time.AfterFunc(dur, t.fireRoutine)
	t.mtx.Unlock()
	log.Debug("ThrottleTimer", "lock", -2)
	t.timer.Stop()
	return t
}

func (t *ThrottleTimer) fireRoutine() {
	log.Debug("ThrottleTimer", "lock", 1)
	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer log.Debug("ThrottleTimer", "lock", -1)
	select {
	case t.Ch <- struct{}{}:
		t.isSet = false
	case <-t.quit:
		// do nothing
	default:
		t.timer.Reset(t.dur)
	}
}

func (t *ThrottleTimer) Set() {
	log.Debug("ThrottleTimer", "lock", 3)
	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer log.Debug("ThrottleTimer", "lock", -3)
	if !t.isSet {
		t.isSet = true
		t.timer.Reset(t.dur)
	}
}

func (t *ThrottleTimer) Unset() {
	log.Debug("ThrottleTimer", "lock", 4)
	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer log.Debug("ThrottleTimer", "lock", -4)
	t.isSet = false
	t.timer.Stop()
}

// For ease of .Stop()'ing services before .Start()'ing them,
// we ignore .Stop()'s on nil ThrottleTimers
func (t *ThrottleTimer) Stop() bool {
	if t == nil {
		return false
	}
	close(t.quit)
	log.Debug("ThrottleTimer", "lock", 5)
	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer log.Debug("ThrottleTimer", "lock", -5)
	return t.timer.Stop()
}
