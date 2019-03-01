package help

import (
	"sync"
	"time"
)

var dsLocal *durationStat

//DurationStat base
func DurationStat() *durationStat {
	if dsLocal != nil {
		return dsLocal
	}
	dsLocal = &durationStat{
		statTimeArray: make(map[uint64]statTime),
		otherStatInfo: make(map[uint64]map[string]interface{}),
		statMaxLen:    20,
		lock:          new(sync.Mutex),
	}
	return dsLocal
}

type statTime map[string]runTime

type durationStat struct {
	statTimeArray map[uint64]statTime
	otherStatInfo map[uint64]map[string]interface{}
	statMaxLen    uint64
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
	for k, v := range d.statTimeArray {
		stat := make(map[string]interface{})
		stat["stat"] = v.toMap()
		if other, ok := d.otherStatInfo[k]; ok {
			stat["other"] = other
		}
		s[k] = stat
	}
	return s
}

func (d *durationStat) addStatTime(flag string, ifBegin bool, round uint64) {
	var tt statTime = make(map[string]runTime)
	if v, ok := d.statTimeArray[round]; ok {
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
	d.statTimeArray[round] = tt

	if round > d.statMaxLen {
		delete(d.statTimeArray, round-d.statMaxLen)
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
	if v, ok := d.otherStatInfo[height]; ok {
		info = v
	}
	info[k] = v
	d.otherStatInfo[height] = info
	if height > d.statMaxLen {
		delete(d.otherStatInfo, height-d.statMaxLen)
	}
}
