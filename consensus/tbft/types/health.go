package types

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	"sort"
	"sync/atomic"
	"time"
)

const (
	//HealthOut peer time out
	HealthOut = 60 //* 10
	//MixValidator min committee count
	MixValidator   = 2
	BlackDoorCount = 4
)

//Health struct
type Health struct {
	ID    tp2p.ID
	IP    string
	Port  uint
	Tick  int32
	State int32
	Val   *Validator
	Self  bool
}

//NewHealth new
func NewHealth(id tp2p.ID, state int32, val *Validator, Self bool) *Health {
	return &Health{
		ID:    id,
		State: state,
		Val:   val,
		Tick:  0,
		Self:  Self,
	}
}

func (h *Health) String() string {
	if h == nil {
		return "health-nil"
	}
	return fmt.Sprintf("id:%s,ip:%s,port:%d,tick:%d,state:%d,addr:%s", h.ID, h.IP, h.Port, h.Tick, h.State,
		common.ToHex(h.Val.Address))
}

//SimpleString string
func (h *Health) SimpleString() string {
	s := atomic.LoadInt32(&h.State)
	t := atomic.LoadInt32(&h.Tick)
	return fmt.Sprintf("state:%d,tick:%d", s, t)
}

// Equal return true they are same id or both nil otherwise return false
func (h *Health) Equal(other *Health) bool {
	if h == nil && other == nil {
		return true
	}
	if h == nil || other == nil {
		return false
	}
	return h.ID == other.ID && bytes.Equal(h.Val.PubKey.Bytes(),other.Val.PubKey.Bytes())
}

//SwitchValidator struct
type SwitchValidator struct {
	Remove    *Health
	Add       *Health
	Infos     *ctypes.SwitchInfos
	Resion    string
	From      int // 0-- add ,1--remove
	DoorCount int
}

func (s *SwitchValidator) String() string {
	if s == nil {
		return "switch-validator-nil"
	}
	return fmt.Sprintf("switch-validator:[R:%s,A:%s,Info:%s,Resion:%s,From:%d,Door:%d]",
		s.Remove, s.Add, s.Infos, s.Resion, s.From, s.DoorCount)
}

//HealthMgr struct
type HealthMgr struct {
	help.BaseService
	Sum            int64
	Work           map[tp2p.ID]*Health
	Back           []*Health
	switchChanTo   chan *SwitchValidator
	switchChanFrom chan *SwitchValidator
	healthTick     *time.Ticker
	switchBuffer   []*SwitchValidator
	cid            uint64
}

//NewHealthMgr func
func NewHealthMgr(cid uint64) *HealthMgr {
	h := &HealthMgr{
		Work:           make(map[tp2p.ID]*Health, 0),
		Back:           make([]*Health, 0, 0),
		switchBuffer:	make([]*SwitchValidator,0,0),
		switchChanTo:   make(chan *SwitchValidator),
		switchChanFrom: make(chan *SwitchValidator),
		Sum:            0,
		cid:            cid,
		healthTick:     nil,
	}
	h.BaseService = *help.NewBaseService("HealthMgr", h)
	return h
}

//SetBackValidators set back committee
func (h *HealthMgr) SetBackValidators(hh []*Health) {
	h.Back = hh
	sort.Sort(HealthsByAddress(h.Back))
}
func (h *HealthMgr) verifySeedNode() error {
	return nil
}
//UpdataHealthInfo update one health
func (h *HealthMgr) UpdataHealthInfo(id tp2p.ID, ip string, port uint, pk []byte) {
	enter := h.GetHealth(pk)
	if enter != nil && enter.ID != "" {
		enter.ID, enter.IP, enter.Port = id, ip, port
		log.Info("UpdataHealthInfo", "info", enter)
	}
}

//ChanFrom get switchChanTo for recv from state
func (h *HealthMgr) ChanFrom() chan *SwitchValidator {
	return h.switchChanFrom
}

//ChanTo get switchChanTo for send to state
func (h *HealthMgr) ChanTo() chan *SwitchValidator {
	return h.switchChanTo
}

//OnStart mgr start
func (h *HealthMgr) OnStart() error {
	if h.healthTick == nil {
		h.healthTick = time.NewTicker(1 * time.Second)
		go h.healthGoroutine()
	}
	return nil
}

//OnStop mgr stop
func (h *HealthMgr) OnStop() {
	if h.healthTick != nil {
		h.healthTick.Stop()
	}
	h.Stop()
}

//PutWorkHealth add a *health to work
func (h *HealthMgr) PutWorkHealth(he *Health) {
	h.Work[he.ID] = he
}

//PutBackHealth add a *health to back
func (h *HealthMgr) PutBackHealth(he *Health) {
	h.Back = append(h.Back, he)
}

//Switch send switch
func (h *HealthMgr) Switch(s *SwitchValidator) {
	if s == nil {
		return
	}
	select {
	case h.ChanTo() <- s:
	default:
		log.Info("h.switchChanTo already close")
	}
}
func (h *HealthMgr) healthGoroutine() {
	for {
		select {
		case <-h.healthTick.C:
			h.work()
		case s := <-h.ChanFrom():
			h.switchResult(s)
		case <-h.Quit():
			log.Info("healthMgr is quit")
			return
		}
	}
}
func (h *HealthMgr) work() {
	for _, v := range h.Work {
		if v.State == ctypes.StateUsedFlag && v.State != ctypes.StateFixedFlag && !v.Self {
			atomic.AddInt32(&v.Tick, 1)
			h.checkSwitchValidator(v)
		}
	}
	for _, v := range h.Back {
		if v.State == ctypes.StateUsedFlag && v.State != ctypes.StateFixedFlag && !v.Self {
			atomic.AddInt32(&v.State, 1)
			h.checkSwitchValidator(v)
		}
	}
}

func (h *HealthMgr) checkSwitchValidator(v *Health) {
	val := atomic.LoadInt32(&v.Tick)
	log.Info("Health", "info:", fmt.Sprintf("id:%s val:%d state:%d", v.ID, val, v.State))
	cnt := h.getUsedValidCount()
	if cnt > MixValidator && val > HealthOut && v.State == ctypes.StateUsedFlag && !v.Self {
		log.Info("Health", "Change", true)
		back := h.pickUnuseValidator()
		go h.Switch(h.makeSwitchValidators(v, back, "Switch", 0))
		atomic.StoreInt32(&v.State, int32(ctypes.StateSwitchingFlag))
	}
}

func (h *HealthMgr) makeSwitchValidators(remove, add *Health, resion string, from int) *SwitchValidator {
	vals := make([]*ctypes.SwitchEnter, 0, 0)
	if add != nil {
		vals = append(vals, &ctypes.SwitchEnter{
			Pk:   add.Val.PubKey.Bytes(),
			Flag: ctypes.StateAppendFlag,
		})
	}
	vals = append(vals, &ctypes.SwitchEnter{
		Pk:   remove.Val.PubKey.Bytes(),
		Flag: ctypes.StateRemovedFlag,
	})
	for _, v := range h.Work {
		if !v.Equal(remove) && v.State == ctypes.StateUsedFlag {
			vals = append(vals, &ctypes.SwitchEnter{
				Pk:   v.Val.PubKey.Bytes(),
				Flag: uint32(atomic.LoadInt32(&v.State)),
			})
		}
	}
	for _, v := range h.Back {
		if !v.Equal(remove) && !v.Equal(add) && v.State == ctypes.StateUsedFlag {
			vals = append(vals, &ctypes.SwitchEnter{
				Pk:   v.Val.PubKey.Bytes(),
				Flag: uint32(atomic.LoadInt32(&v.State)),
			})
		}
	}
	// will need check vals with validatorSet
	infos := &ctypes.SwitchInfos{
		CID:  h.cid,
		Vals: vals,
	}
	return &SwitchValidator{
		Infos:     infos,
		Resion:    resion,
		From:      from,
		DoorCount: BlackDoorCount,
		Remove:	   remove,
		Add:	   add,
	}
}

func (h *HealthMgr) getUsedValidCount() int {
	cnt := 0
	for _, v := range h.Work {
		if v.State == ctypes.StateUsedFlag {
			cnt++
		}
	}
	for _, v := range h.Back {
		if v.State == ctypes.StateUnusedFlag || v.State == ctypes.StateUsedFlag {
			cnt++
		}
	}
	return cnt
}

//switchResult run switch
func (h *HealthMgr) switchResult(res *SwitchValidator) {
	ss := "failed"
	if res.Resion == "" {
		if len(res.Infos.Vals) > 2 {
			enter1, enter2 := res.Infos.Vals[0], res.Infos.Vals[1]
			var add, remove *Health
			if enter1.Flag == ctypes.StateAppendFlag {
				add = h.GetHealth(enter1.Pk)
				if enter2.Flag == ctypes.StateRemovedFlag {
					remove = h.GetHealth(enter2.Pk)
				}
			} else if enter1.Flag == ctypes.StateRemovedFlag {
				remove = h.GetHealth(enter1.Pk)
			}
			if !remove.Equal(res.Remove) || !add.Equal(res.Add) {
				log.Error("switchResult item not match", "remove", remove, "Remove", res.Remove, "add", add, "Add", res.Add)
			}
			if remove != nil {
				atomic.StoreInt32(&remove.State, int32(ctypes.StateRemovedFlag))
				ss = "Success"
			}
			if add != nil {
				atomic.StoreInt32(&add.State, int32(ctypes.StateUsedFlag))
			}
		}
	}
	log.Info("switch", "result:", ss, "infos", res.Infos)
}

//pickUnuseValidator get a back committee
func (h *HealthMgr) pickUnuseValidator() *Health {
	sum := len(h.Back)
	for i := 0; i < sum; i++ {
		v := h.Back[i]
		if s := atomic.CompareAndSwapInt32(&v.State, int32(ctypes.StateUnusedFlag), int32(ctypes.StateSwitchingFlag)); s {
			return v
		}
	}
	return nil
}

//Update tick
func (h *HealthMgr) Update(id tp2p.ID) {
	if v, ok := h.Work[id]; ok {
		val := atomic.LoadInt32(&v.Tick)
		atomic.AddInt32(&v.Tick, -val)
		return
	}
	for _, v := range h.Back {
		if v.ID == id {
			val := atomic.LoadInt32(&v.Tick)
			atomic.AddInt32(&v.Tick, -val)
			return
		}
	}
}

//GetHealthFormWork get worker for address
func (h *HealthMgr) GetHealthFormWork(address []byte) *Health {
	for _, v := range h.Work {
		if bytes.Equal(address, v.Val.Address) {
			return v
		}
	}
	return nil
}

func (h *HealthMgr) getHealthFromPart(pk []byte, part int) *Health {
	if part == 1 { // back
		for _, v := range h.Back {
			if bytes.Equal(pk, v.Val.PubKey.Bytes()) {
				return v
			}
		}
	} else { // work
		for _, v := range h.Work {
			if bytes.Equal(pk, v.Val.PubKey.Bytes()) {
				return v
			}
		}
	}
	return nil
}

//GetHealth get a Health for mgr
func (h *HealthMgr) GetHealth(pk []byte) *Health {
	enter := h.getHealthFromPart(pk, 0)
	if enter == nil {
		enter = h.getHealthFromPart(pk, 1)
	}
	return enter
}

//VerifySwitch verify remove and add switchEnter
func (h *HealthMgr) VerifySwitch(remove, add *ctypes.SwitchEnter) error {
	r := h.GetHealth(remove.Pk)
	rRes := false

	if r == nil {
		return errors.New("not found the remove:" + remove.String())
	}

	rTick := atomic.LoadInt32(&r.Tick)
	if r.State >= ctypes.StateUsedFlag && rTick >= HealthOut {
		rRes = true
	}
	res := r.SimpleString()

	var a *Health
	if add != nil {
		a = h.GetHealth(add.Pk)
	}
	aRes := false

	if a != nil {
		if a.State != ctypes.StateRemovedFlag {
			aRes = true
		}
		res += a.SimpleString()
	} else {
		aRes = true
	}
	if rRes && aRes {
		return nil
	}
	addStr := ""
	if add != nil {
		addStr = add.String()
	}
	return errors.New("Wrang state:" + res + "Remove:" + remove.String() + ",add:" + addStr)
}

//UpdateFromCommittee agent put member and back, update flag
func (h *HealthMgr) UpdateFromCommittee(member, backMember ctypes.CommitteeMembers) {
	for _, v := range member {
		for k, v2 := range h.Work {
			pk := crypto.PubKeyTrue(*v.Publickey)
			if bytes.Equal(pk.Address(), v2.Val.Address) {
				atomic.StoreInt32(&h.Work[k].State, v.Flag)
				break
			}
		}
	}
	for _, v := range backMember {
		for k, v2 := range h.Back {
			pk := crypto.PubKeyTrue(*v.Publickey)
			if bytes.Equal(pk.Address(), v2.Val.Address) {
				atomic.StoreInt32(&h.Back[k].State, v.Flag)
				break
			}
		}
	}
}

//-------------------------------------------------
// Implements sort for sorting Healths by address.

// HealthsByAddress Sort Healths by address
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
