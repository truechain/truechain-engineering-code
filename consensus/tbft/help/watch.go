package help

import (
	// "fmt"
	// "bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	// "github.com/ethereum/go-ethereum/crypto/sha3"
	// "github.com/ethereum/go-ethereum/rlp"
	// "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

var (
	watchs = newWatchMgr()
	// WatchFinishTime used to handle the end event in every watch
	WatchFinishTime = 3
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
func WatchsCountInMgr() uint64 {
	return watchs.count()
}

//-----------------------------------------------------------------------------
// TWatch watch and output the cost time for exec
type TWatch struct {
	begin   time.Time
	end     time.Time
	expect  float64
	str     string
	ID      uint64
	res     chan uint64
	endFlag uint32
}

// NewTWatch make the new watch
func NewTWatch(e float64, s string) *TWatch {
	w := &TWatch{
		begin:   time.Now(),
		end:     time.Now(),
		expect:  e,
		str:     s,
		endFlag: 0,
	}
	w.ID = watchs.getNewNum()
	watchs.setWatch(w)
	w.begin = time.Now()
	return w
}

// EndWatch end the watch
func (in *TWatch) EndWatch() {
	in.endWatchNoNotify()
	// in.end = time.Now()
	// atomic.AddUint32(&in.endFlag, 1)
	// select {
	// case watchs.getRes() <- in.ID:
	// default:
	// 	// go func() { watchs.getRes() <- in.ID }()
	// }
}
func (in *TWatch) endWatchNoNotify() {
	in.end = time.Now()
	atomic.AddUint32(&in.endFlag, 1)
}

func (in *TWatch) getEndFlag() bool {
	v := atomic.LoadUint32(&in.endFlag)
	return v > 0
}

// Finish count the cost time in watch
func (in *TWatch) Finish(comment interface{}) {
	if d := in.end.Sub(in.begin); d.Seconds() > in.expect {
		log.Warn(in.str, "not expecting time", d.Seconds(), "comment", comment, "id", in.ID)
	}
}

type WatchMgr struct {
	watchID   uint64
	watchs    map[uint64]*TWatch
	tmpWatchs []*TWatch
	quit      bool
	lock      *sync.Mutex
	tlock     *sync.Mutex
	resFinish chan uint64
}

func newWatchMgr() *WatchMgr {
	w := &WatchMgr{
		watchID:   0,
		watchs:    make(map[uint64]*TWatch, 0),
		tmpWatchs: make([]*TWatch, 0, 0),
		lock:      new(sync.Mutex),
		tlock:     new(sync.Mutex),
		resFinish: make(chan uint64, MaxWatchInChan),
		quit:      true,
	}
	return w
}

func (w *WatchMgr) getNewNum() uint64 {
	v := atomic.AddUint64(&w.watchID, 1)
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
	w.tlock.Lock()
	defer w.tlock.Unlock()
	w.tmpWatchs = append(w.tmpWatchs, watch)
}

func (w *WatchMgr) getUnsetWatchs() []*TWatch {
	w.tlock.Lock()
	defer w.tlock.Unlock()
	wss := w.tmpWatchs
	w.tmpWatchs = make([]*TWatch, 0, 0)
	return wss
}

func (w *WatchMgr) setWatchs() {
	wss := w.getUnsetWatchs()
	if len(wss) == 0 {
		return
	}
	w.lock.Lock()
	defer w.lock.Unlock()
	for _, watch := range wss {
		if _, ok := w.watchs[watch.ID]; !ok {
			w.watchs[watch.ID] = watch
		}
	}
}
func (w *WatchMgr) getWatch(id uint64) *TWatch {
	w.lock.Lock()
	defer w.lock.Unlock()
	if v, ok := w.watchs[id]; ok {
		return v
	}
	return nil
}
func (w *WatchMgr) remove(id uint64) {
	w.lock.Lock()
	defer w.lock.Unlock()
	delete(w.watchs, id)
}
func (w *WatchMgr) removeByIDs(ids []uint64) {
	if len(ids) == 0 {
		return
	}
	w.lock.Lock()
	defer w.lock.Unlock()
	for _, v := range ids {
		delete(w.watchs, v)
	}
}
func (w *WatchMgr) count() uint64 {
	w.lock.Lock()
	defer w.lock.Unlock()
	return uint64(len(w.watchs))
}
func (w *WatchMgr) getRes() chan<- uint64 {
	return w.resFinish
}
func (w *WatchMgr) getLastWatchs() []*TWatch {
	ws := make([]*TWatch, 0, 0)
	w.lock.Lock()
	defer w.lock.Unlock()
	for _, v := range w.watchs {
		ws = append(ws, v)
	}
	return ws
}
func (w *WatchMgr) watchFinish() {
	ids := make([]uint64, 0, 0)
	begin := time.Now()
	defer func() {
		n := len(ids)
		w.removeByIDs(ids)
		d := time.Now().Sub(begin)
		fmt.Println("watchFinish:", d.Seconds(), "count:", n)

	}()

	for {
		select {
		case id := <-w.resFinish:
			// w.remove(id)
			ids = append(ids, id)
		case <-time.After(1 * time.Second):
			if w.quit {
				return
			}
			return
		}
	}
}
func (w *WatchMgr) watchLoop() {
	ws := w.getLastWatchs()
	now := time.Now()
	ids := make([]uint64, 0, 0)
	defer func() {
		w.removeByIDs(ids)
	}()

	for _, watch := range ws {
		if w.quit {
			return
		}
		d := now.Sub(watch.begin)
		end := watch.getEndFlag()
		if end {
			ids = append(ids, watch.ID)
		} else if d.Seconds() > float64(MaxLockTime) {
			watch.endWatchNoNotify()
			log.Warn("cost long time or forget endwatch", "function", watch.str, "time", d.Seconds(), "id", watch.ID)
			ids = append(ids, watch.ID)
		}
	}
}

func (w *WatchMgr) work() {

	for {
		if w.quit {
			return
		}
		begin := time.Now()

		w.setWatchs()
		n := w.count()
		// w.watchFinish()
		w.watchLoop()

		time.Sleep(1 * time.Second)
		d := time.Now().Sub(begin)
		if d.Seconds() > 5 {
			log.Info("watch work cost time:", d.Seconds(), "count:", n)
		}
	}
}
