package types

import (
	"sync/atomic"
	"time"
	"bytes"
	"sort"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/p2p"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/common"
)

// Heartbeat is a simple vote-like structure so validators can
// alert others that they are alive and waiting for transactions.
// Note: We aren't adding ",omitempty" to Heartbeat's
// json field tags because we always want the JSON
// representation to be in its canonical form.
type Heartbeat struct {
	ValidatorAddress help.Address `json:"validator_address"`
	ValidatorIndex   uint         `json:"validator_index"`
	Height           uint64       `json:"height"`
	Round            uint         `json:"round"`
	Sequence         uint         `json:"sequence"`
	Signature        []byte       `json:"signature"`
}

// SignBytes returns the Heartbeat bytes for signing.
// It panics if the Heartbeat is nil.
func (heartbeat *Heartbeat) SignBytes(chainID string) []byte {
	bz, err := cdc.MarshalJSON(CanonicalJSONHeartbeat{
		ChainID:          chainID,
		Type:             "heartbeat",
		Height:           heartbeat.Height,
		Round:            heartbeat.Round,
		Sequence:         heartbeat.Sequence,
		ValidatorAddress: heartbeat.ValidatorAddress,
		ValidatorIndex:   heartbeat.ValidatorIndex,
	})
	if err != nil {
		panic(err)
	}
	signBytes := help.RlpHash([]interface{}{bz,})
	return signBytes[:]
}

// Copy makes a copy of the Heartbeat.
func (heartbeat *Heartbeat) Copy() *Heartbeat {
	if heartbeat == nil {
		return nil
	}
	heartbeatCopy := *heartbeat
	return &heartbeatCopy
}

// String returns a string representation of the Heartbeat.
func (heartbeat *Heartbeat) String() string {
	if heartbeat == nil {
		return "nil-heartbeat"
	}

	return fmt.Sprintf("Heartbeat{%v:%X %v/%02d (%v) %v}",
		heartbeat.ValidatorIndex, help.Fingerprint(heartbeat.ValidatorAddress),
		heartbeat.Height, heartbeat.Round, heartbeat.Sequence,
		fmt.Sprintf("/%X.../", help.Fingerprint(heartbeat.Signature[:])))
}

const (
	HealthOut = 60*10
	MixValidator = 4
	StateUnused = 0
	StateSwitching = 1
	StateUsed = 2
	StateRemoved = 3
)

type Health struct {
	ID      	p2p.ID
	IP      	string
	Port    	uint
	Tick		int32
	State 		int32
	Val			*Validator
}
func (h *Health) String() string {
	return fmt.Sprintf("id:%s,ip:%s,port:%d,tick:%d,state:%d,addr:%s",h.ID,h.IP,h.Port,h.Tick,h.State,
			common.ToHex(h.Val.Address))
}
func (h *Health) SimpleString() string {
	s := atomic.LoadInt32(&h.State)
	t := atomic.LoadInt32(&h.Tick)
	return fmt.Sprintf("state:%d,tick:%d",s,t)
}

type SwitchValidator struct {
	Remove 		*Health
	Add 		*Health
	Resion 		string
	From		int
} 

type HealthMgr struct {
	help.BaseService
	Sum				int64
	Work	 		map[p2p.ID]*Health
	Back			[]*Health
	SwitchChan		chan *SwitchValidator	
	healthTick 		*time.Ticker
}

func NewHealthMgr() *HealthMgr {
	h := &HealthMgr{
		Work:			make(map[p2p.ID]*Health,0),
		Back:			make([]*Health,0,0),
		SwitchChan:		make(chan*SwitchValidator),
		Sum:			0,
		healthTick:		nil,
	}
	h.BaseService = *help.NewBaseService("HealthMgr", h)
	return h
}
func (h *HealthMgr) SetBackValidators(hh []*Health) {
	h.Back = hh
	sort.Sort(HealthsByAddress(h.Back))
}
func (h *HealthMgr) OnStart() error {
	if h.healthTick == nil {
		h.healthTick = time.NewTicker(1*time.Second)
		go h.healthGoroutine()
	}
	return nil
}
func (h *HealthMgr) OnStop() {
	if h.healthTick != nil {
		h.healthTick.Stop()
	}
	h.Stop()
}
func (h *HealthMgr) Switch(s *SwitchValidator) {
	select {
	case h.SwitchChan <- s:
	default:
		log.Info("h.SwitchChan already close")
	}
}
func (h *HealthMgr) healthGoroutine() {
	for {
		select {
		case <- h.healthTick.C:
			h.work()
		case s:=<- h.SwitchChan:
			h.switchResult(s)
		case <- h.Quit():
			log.Info("healthMgr is quit")
			return 
		}
	}
}
func (h *HealthMgr) work() {	
	for _,v:=range h.Work {
		if v.State == StateUsed {
			atomic.AddInt32(&v.Tick,1)
		}
		h.checkSwitchValidator(v)	
	} 
}

func (h *HealthMgr) checkSwitchValidator(v *Health) {
	val := atomic.LoadInt32(&v.Tick)
	cnt := h.getUsedValidCount()
	if cnt > MixValidator && val > HealthOut && v.State == StateUsed {
		back := h.pickUnuseValidator()
		go h.Switch(&SwitchValidator {
			Remove:			v,
			Add:			back,
			Resion:			"Switch",
			From:			0,
		})
		v.State = StateSwitching
	}
}

func (h *HealthMgr) getUsedValidCount() int {
	cnt := 0
	for _,v := range h.Work {
		if v.State == StateUsed {
			cnt++
		}
	}
	for _,v := range h.Back {
		if v.State == StateUnused {
			cnt++
		}
	}
	return cnt
}

func (h *HealthMgr) switchResult(res *SwitchValidator) {
	if res.From == 1 {
		ss := "Switch Validator failed"
		if res.Resion == "" {
			ss = "Switch Validator Success"
			if v,ok := h.Work[res.Remove.ID];ok {
				atomic.StoreInt32(&v.State,int32(StateRemoved))
			}
			for _,v := range h.Back {
				if v.ID == res.Add.ID {
					atomic.StoreInt32(&v.State,int32(StateUsed))
					break
				}
			}
		} 
		log.Info(ss,"resion",res.Resion,"remove",res.Remove.String(),"add",res.Add.String())
	}
}

func (h *HealthMgr) pickUnuseValidator() *Health {
	sum := len(h.Back)
	for i:=0;i<sum;i++ {
		v := h.Back[i]
		if s := atomic.CompareAndSwapInt32(&v.State,int32(StateUnused),int32(StateSwitching)); s {
			return v
		}
	}
	return nil
}

func (h *HealthMgr) Update(id p2p.ID) {
	if v,ok := h.Work[id];ok{
		val := atomic.LoadInt32(&v.Tick)
		atomic.AddInt32(&v.Tick,-val)
	}
}

func (h *HealthMgr) GetHealthFormWork(address []byte) *Health {
	for _,v := range h.Work {
		if bytes.Equal(address,v.Val.Address) {
			return v
		}
	}
	return nil
}

func (h *HealthMgr) getHealth(pk []byte,part int) *Health {
	if part == 1 {	// back 
		for _,v:=range h.Back {
			if bytes.Equal(pk,v.Val.PubKey.Bytes()) {
				return v
			}
		}
	} else {		// work
		for _,v := range h.Work {
			if bytes.Equal(pk,v.Val.PubKey.Bytes()) {
				return v
			}
		}
	}
	return nil
}

func (h *HealthMgr) VerifySwitch(remove,add *ctypes.SwitchEnter) error {
	r := h.getHealth(remove.Pk,0)	
	if r == nil {
		r = h.getHealth(remove.Pk,1)
	}
	rRes := false 

	if r == nil {
		return errors.New("not found the remove:"+remove.String())
	}

	rTick := atomic.LoadInt32(&r.Tick)
	if r.State >= StateUsed && rTick >= HealthOut {
		rRes = true
	}
	res := r.SimpleString()

	a := h.getHealth(add.Pk,0)
	if a == nil {
		a = h.getHealth(add.Pk,1)
	}
	aRes := false
	
	if a != nil {
		if a.State != StateRemoved {
			aRes = true
		}
		res += a.SimpleString()
	} else {
		aRes = true
	}
	if rRes && aRes {
		return nil
	}
	return errors.New("Wrang state:"+res+"Remove:"+remove.String()+",add:"+add.String())
}

//-------------------------------------------------
// Implements sort for sorting Healths by address.

// Sort Healths by address
type HealthsByAddress []*Health

func (hs HealthsByAddress) Len() int {
	return len(hs)
}

func (hs HealthsByAddress) Less(i, j int) bool {
	return bytes.Compare(hs[i].Val.Address, hs[j].Val.Address) == -1
}

func (hs HealthsByAddress) Swap(i, j int) {
	it := hs[i]
	hs[i] = hs[j]
	hs[j] = it
}