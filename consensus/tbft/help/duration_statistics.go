package help

import (
	"time"
)

type RunTime struct {
	start time.Time
	end   time.Time
}

type TbftTime map[string]RunTime

var TbftTimeArray = make(map[uint64]TbftTime)

func (r RunTime) TimeDec() float64 {
	var t time.Time
	if r.end == t || r.start == t {
		return 0
	}
	return r.end.Sub(r.start).Seconds()
}

func (t TbftTime) toMap() map[string]interface{} {
	s := make(map[string]interface{})
	for k, v := range t {
		s[k] = v.TimeDec()
	}
	return s
}

func PrintDurStat() map[uint64]interface{} {
	s := make(map[uint64]interface{})
	for k, v := range TbftTimeArray {
		s[k] = v.toMap()
	}
	return s
}

func addStatTime(flag string, ifBegin bool, round uint64) {
	var tt TbftTime = make(map[string]RunTime)
	if v, ok := TbftTimeArray[round]; ok {
		tt = v
	}
	var r RunTime
	if v, ok := tt[flag]; ok {
		r = v
	}
	if ifBegin {
		r.start = time.Now()
	} else {
		r.end = time.Now()
	}
	tt[flag] = r
	TbftTimeArray[round] = tt

	if round > 10 {
		delete(TbftTimeArray, round-10)
	}
}
func AddStartStatTime(flag string, round uint64) {
	addStatTime(flag, true, round)
}
func AddEndStatTime(flag string, round uint64) {
	addStatTime(flag, false, round)
}
