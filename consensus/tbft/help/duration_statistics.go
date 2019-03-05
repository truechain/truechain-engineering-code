package help

import (
	"sync"
	"time"
)

const CacheLen = 20

var DurationStat = newDurationStat()

func newDurationStat() *durationStat {
	return &durationStat{
		StatTimeArray: make(map[uint64]statTime),
		OtherStatInfo: make(map[uint64]map[string]interface{}),
		StatMaxLen:    CacheLen,
		lock:          new(sync.Mutex),
	}
}

type statTime map[string]runTime

type durationStat struct {
	StatTimeArray map[uint64]statTime
	OtherStatInfo map[uint64]map[string]interface{}
	StatMaxLen    uint64
	lock          *sync.Mutex //Being not
}

type runTime struct {
	start time.Time
	end   time.Time
}

func (r runTime) timeDec() float64 {
	var t time.Time
	if r.end == t || r.start == t {
		return 0
	}
	return r.end.Sub(r.start).Seconds()
}

func (t statTime) toMap() map[string]interface{} {
	s := make(map[string]interface{})
	for k, v := range t {
		s[k] = v.timeDec()
	}
	return s
}

func (d *durationStat) PrintDurStat() map[uint64]interface{} {
	s := make(map[uint64]interface{})
	for k, v := range d.StatTimeArray {
		stat := make(map[string]interface{})
		stat["stat"] = v.toMap()
		if other, ok := d.OtherStatInfo[k]; ok {
			stat["other"] = other
		}
		s[k] = stat
	}
	return s
}

func (d *durationStat) addStatTime(flag string, ifBegin bool, round uint64) {
	var tt statTime = make(map[string]runTime)
	if v, ok := d.StatTimeArray[round]; ok {
		tt = v
	}
	var r runTime
	if v, ok := tt[flag]; ok {
		r = v
	}
	if ifBegin {
		r.start = time.Now()
	} else {
		r.end = time.Now()
	}
	tt[flag] = r
	d.StatTimeArray[round] = tt

	if round > d.StatMaxLen {
		delete(d.StatTimeArray, round-d.StatMaxLen)
	}
}

func (d *durationStat) AddStartStatTime(flag string, height uint64) {
	d.addStatTime(flag, true, height)
}

func (d *durationStat) AddEndStatTime(flag string, height uint64) {
	d.addStatTime(flag, false, height)
}

func (d *durationStat) AddOtherStat(k string, v interface{}, height uint64) {
	info := make(map[string]interface{})
	if v, ok := d.OtherStatInfo[height]; ok {
		info = v
	}
	info[k] = v
	d.OtherStatInfo[height] = info
	if height > d.StatMaxLen {
		delete(d.OtherStatInfo, height-d.StatMaxLen)
	}
}
