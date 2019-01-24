package help

import (
	// "fmt"
	// "bytes"
	"time"
	"sync"
	"sync/atomic"
	// "github.com/ethereum/go-ethereum/crypto/sha3"
	// "github.com/ethereum/go-ethereum/rlp"
	// "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

var (
	watchs = newWatchMgr()
	// WatchFinishTime used to handle the end event in every watch
	WatchFinishTime = 10
	// MaxLockTime	used to judge the watch is locked
	MaxLockTime = 60
	// MaxWatchInChan
	MaxWatchInChan = 20000
)

func BeginWatchMgr() {
	if watchs != nil {
		watchs.start()
		log.Info("begin watchMgr")
	}
}
func EndWatchMgr() {
	if watchs != nil {
		watchs.stop()
		log.Info("end watchMgr")
	}
}

//-----------------------------------------------------------------------------
// TWatch watch and output the cost time for exec
type TWatch struct {
	begin  		time.Time
	end    		time.Time
	expect 		float64
	str    		string
	ID			uint64
	res 		chan uint64
	endFlag 	uint32
}

// NewTWatch make the new watch
func NewTWatch(e float64, s string) *TWatch {
	w := &TWatch{
		begin:  	time.Now(),
		end:    	time.Now(),
		expect: 	e,
		str:    	s,
		endFlag:	0,
	}
	w.ID = watchs.getNewNum()
	watchs.setWatch(w)
	return w
}
// EndWatch end the watch
func (in *TWatch) EndWatch() {
	in.end = time.Now()
	atomic.AddUint32(&in.endFlag,1)
	select {
	case watchs.getRes() <- in.ID:
	default:
		// go func() { watchs.getRes() <- in.ID }()
	}
}
func (in *TWatch) getEndFlag() bool {
	v := atomic.LoadUint32(&in.endFlag)
	return v > 0
}
// Finish count the cost time in watch
func (in *TWatch) Finish(comment interface{}) {
	if d := in.end.Sub(in.begin); d.Seconds() > in.expect {
		log.Warn(in.str, "not expecting time", d.Seconds(), "comment", comment,"id",in.ID)
	}
}

type WatchMgr struct {
	watchID		uint64
	watchs		map[uint64]*TWatch
	quit		bool
	lock		*sync.Mutex
	resFinish   chan uint64
}

func newWatchMgr() *WatchMgr {	
	w := &WatchMgr{
		watchID:	0,
		watchs:		make(map[uint64]*TWatch,0),
		lock:		new(sync.Mutex),
		resFinish:	make(chan uint64,MaxWatchInChan),
		quit:		true,
	}
	return w
}

func (w *WatchMgr) getNewNum() uint64 {
	v := atomic.AddUint64(&w.watchID,1)
	return v
}
func (w *WatchMgr) start() {
	w.quit = false
	go w.work()
}
func (w *WatchMgr) stop() {
	w.quit = true
	time.Sleep(2 * time.Second)
}
func (w *WatchMgr) setWatch(watch *TWatch) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if _,ok := w.watchs[watch.ID]; !ok {
		w.watchs[watch.ID] = watch
	}
}
func (w *WatchMgr) getWatch(id uint64) *TWatch {
	w.lock.Lock()
	defer w.lock.Unlock()
	if v,ok := w.watchs[id]; ok {
		return v
	}
	return nil
}
func (w *WatchMgr) remove(id uint64) {
	w.lock.Lock()
	defer w.lock.Unlock()
	delete(w.watchs,id)
}
func (w *WatchMgr) count() uint64 {
	w.lock.Lock()
	defer w.lock.Unlock()
	return uint64(len(w.watchs))
}
func (w *WatchMgr) getRes() chan<-uint64 {
	return w.resFinish
}
func (w *WatchMgr) watchFinish() {
	count := 0
	for {
		select {
		case id := <-w.resFinish:
			w.remove(id)
		case <-time.After(1*time.Second):
			if w.quit {
				return 
			}
			count++
			if count >= WatchFinishTime {
				return
			}
		}
	}
}
func (w *WatchMgr) watchLoop() {
	max := atomic.LoadUint64(&w.watchID)
	count := w.count()
	now := time.Now()

	for i:=max;count>0;count-- {
		if w.quit { return }
		if watch := w.getWatch(i); watch != nil {
			d := now.Sub(watch.begin)
			end := watch.getEndFlag()
			if !end && d.Seconds() > float64(MaxLockTime) && d.Seconds() > watch.expect * 10 {
				go watch.EndWatch()
				log.Warn("cost long time or forget endwatch","function",watch.str, "time", d.Seconds(),"id",watch.ID,"index",i)
			}
			if end {
				w.remove(i)
				continue
			}
		}
	}
}

func (w *WatchMgr) work() {

	for {
		
		if w.quit { return }
		w.watchFinish()
		w.watchLoop()
		time.Sleep(100 * time.Millisecond)	
	}
}