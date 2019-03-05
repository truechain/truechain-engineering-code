package types

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/params"
	"sync"
	"sync/atomic"
	"time"
)

const (
	//HealthOut peer time out
	HealthOut = 1200

	BlackDoorCount = 4

	SwitchPartWork = 0
	SwitchPartBack = 1
	SwitchPartSeed = 2
)

var EnableHealthMgr = true

//Health struct
type Health struct {
	ID    tp2p.ID
	IP    string
	Port  uint32
	Tick  int32
	State uint32
	HType uint32
	Val   *Validator
	Self  bool
}

//NewHealth new
func NewHealth(id tp2p.ID, t, state uint32, val *Validator, Self bool) *Health {
	return &Health{
		ID:    id,
		State: state,
		HType: t,
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
		hexutil.Encode(h.Val.Address))
}

//SimpleString string
func (h *Health) SimpleString() string {
	s := atomic.LoadUint32(&h.State)
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
	return h.ID == other.ID && bytes.Equal(h.Val.PubKey.Bytes(), other.Val.PubKey.Bytes())
}

//SwitchValidator struct
type SwitchValidator struct {
	Remove    *Health
	Add       *Health
	Infos     []*ctypes.CommitteeMember
	Resion    string
	From      int // 0-- add ,1-- resore
	DoorCount int
	Round     int // -1 not exc,no lock
	ID        uint64
}

func (s *SwitchValidator) String() string {
	if s == nil {
		return "switch-validator-nil"
	}
	return fmt.Sprintf("switch-validator:[ID:%v,Round:%d,From:%d,Door:%d,Resion:%s,R:%s,A:%s,Info:%s]",
		s.ID, s.Round, s.From, s.DoorCount, s.Resion, s.Remove, s.Add, s.Infos)
}

// Equal return true they are same id or both nil otherwise return false
func (s *SwitchValidator) Equal(other *SwitchValidator) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	return s.ID == other.ID && s.Remove.Equal(other.Remove) &&
		s.Add.Equal(other.Add) && EqualCommitteeMemberArray(s.Infos, other.Infos)
}

func EqualCommitteeMemberArray(a, b []*ctypes.CommitteeMember) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	for _, v := range a {
		have := false
		for _, vm := range b {
			if v.Compared(vm) {
				have = true
				break
			}
		}
		if !have {
			return false
		}
	}
	return true
}

// EqualWithoutID return true they are same id or both nil otherwise return false
func (s *SwitchValidator) EqualWithoutID(other *SwitchValidator) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	return s.Remove.Equal(other.Remove) && s.Add.Equal(other.Add) && EqualCommitteeMemberArray(s.Infos, other.Infos)
}

// EqualWithRemove return true they are same id or both nil otherwise return false
func (s *SwitchValidator) EqualWithRemove(other *SwitchValidator) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	return s.Remove.Equal(other.Remove)
}

//HealthMgr struct
type HealthMgr struct {
	help.BaseService
	Work           map[tp2p.ID]*Health
	Back           []*Health
	seed           []*Health
	switchChanTo   chan *SwitchValidator
	switchChanFrom chan *SwitchValidator
	healthTick     *time.Ticker
	curSwitch      []*SwitchValidator
	switchBuffer   []*SwitchValidator
	cid            uint64
	uid            uint64
	lock           *sync.Mutex
}

//NewHealthMgr func
func NewHealthMgr(cid uint64) *HealthMgr {
	h := &HealthMgr{
		Work:           make(map[tp2p.ID]*Health, 0),
		Back:           make([]*Health, 0, 0),
		seed:           make([]*Health, 0, 0),
		curSwitch:      make([]*SwitchValidator, 0, 0),
		switchBuffer:   make([]*SwitchValidator, 0, 0),
		switchChanTo:   make(chan *SwitchValidator),
		switchChanFrom: make(chan *SwitchValidator),
		cid:            cid,
		lock:           new(sync.Mutex),
		healthTick:     nil,
	}
	h.BaseService = *help.NewBaseService("HealthMgr", h)
	hi, lo := cid<<32, uint64(100)
	h.uid = hi | lo
	log.Info("HealthMgr init", "cid", cid, "hi", hi, "lo", lo, "uid", h.uid)
	return h
}

// Sum invoke in the testing, after mgr start
func (h *HealthMgr) Sum() int {
	return len(h.Work) + len(h.Back) + len(h.seed)
}

//PutWorkHealth add a *health to work
func (h *HealthMgr) PutWorkHealth(he *Health) {
	h.Work[he.ID] = he
}

//PutBackHealth add a *health to back
func (h *HealthMgr) PutBackHealth(he *Health) {
	if he != nil {
		if he.HType == ctypes.TypeFixed {
			h.seed = append(h.seed, he)
		} else {
			h.Back = append(h.Back, he)
		}
	}
}

//UpdataHealthInfo update one health
func (h *HealthMgr) UpdataHealthInfo(id tp2p.ID, ip string, port uint32, pk []byte) {
	enter := h.GetHealth(pk)
	if enter != nil && enter.ID != "" {
		enter.ID, enter.IP, enter.Port = id, ip, port
		log.Debug("UpdataHealthInfo", "info", enter)
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
	EnableHealthMgr = true
	if h.healthTick == nil {
		h.healthTick = time.NewTicker(1 * time.Second)
		go h.healthGoroutine()
	}
	return nil
}

//OnStop mgr stop
func (h *HealthMgr) OnStop() {
	EnableHealthMgr = false
	if h.healthTick != nil {
		h.healthTick.Stop()
	}
	help.CheckAndPrintError(h.Stop())
}
func (h *HealthMgr) getCurSV() *SwitchValidator {
	h.lock.Lock()
	defer h.lock.Unlock()
	if len(h.curSwitch) > 0 {
		return h.curSwitch[0]
	}
	return nil
}
func (h *HealthMgr) setCurSV(sv *SwitchValidator) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if len(h.curSwitch) == 0 && sv != nil {
		h.curSwitch = append(h.curSwitch, sv)
	}
}
func (h *HealthMgr) removeCurSV() {
	h.lock.Lock()
	defer h.lock.Unlock()
	if len(h.curSwitch) > 0 {
		h.curSwitch = append(h.curSwitch[:0], h.curSwitch[1:]...)
	}
}

//Switch send switch
func (h *HealthMgr) Switch(s *SwitchValidator) {
	if s == nil {
		return
	}

	h.ChanTo() <- s
}
func (h *HealthMgr) healthGoroutine() {
	sshift, islog, cnt := true, true, 0
	for {
		select {
		case <-h.healthTick.C:
			sshift, cnt = h.isShiftSV()
			h.work(sshift)
			if !sshift && islog {
				log.Info("Stop Shift Switch Validator, because minimum SV", "Count", cnt, "CID", h.cid)
				islog = false
			}
		case s := <-h.ChanFrom():
			h.switchResult(s)
		case <-h.Quit():
			log.Info("healthMgr is quit")
			return
		}
	}
}
func (h *HealthMgr) work(sshift bool) {
	if !EnableHealthMgr {
		return
	}
	for _, v := range h.Work {
		h.checkSwitchValidator(v, sshift)
	}
	for _, v := range h.Back {
		h.checkSwitchValidator(v, sshift)
	}
}

func (h *HealthMgr) checkSwitchValidator(v *Health, sshift bool) {
	if v.State == ctypes.StateUsedFlag && v.HType != ctypes.TypeFixed && !v.Self {
		val := atomic.AddInt32(&v.Tick, 1)
		log.Debug("Health", "id", v.ID, "val", val)
		if sshift && val > HealthOut && v.State == ctypes.StateUsedFlag && !v.Self {
			if sv0 := h.getCurSV(); sv0 == nil {
				log.Warn("Health", "id", v.ID, "val", val)
				back := h.pickUnuseValidator()
				cur := h.makeSwitchValidators(v, back, "Switch", 0)
				atomic.StoreUint32(&v.State, ctypes.StateSwitchingFlag)
				h.setCurSV(cur)
				log.Info("CheckSwitchValidator(remove,add)", "info:", cur, "cid", h.cid)
				go h.Switch(cur)
			}
		}

		if sv0 := h.getCurSV(); sv0 != nil {
			val0 := atomic.LoadInt32(&sv0.Remove.Tick)
			if val0 < HealthOut && sv0.From == 0 {
				sv1 := *sv0
				sv1.From = 1
				log.Info("Restore SwitchValidator", "info", sv1, "cid", h.cid)
				go h.Switch(&sv1)
			}
		}
	}
}

func (h *HealthMgr) makeSwitchValidators(remove, add *Health, resion string, from int) *SwitchValidator {
	vals := make([]*ctypes.CommitteeMember, 0, 0)
	if add != nil {
		vals = append(vals, &ctypes.CommitteeMember{
			CommitteeBase: common.BytesToAddress(add.Val.Address),
			Flag:          ctypes.StateAppendFlag,
		})
	}
	vals = append(vals, &ctypes.CommitteeMember{
		CommitteeBase: common.BytesToAddress(remove.Val.Address),
		Flag:          ctypes.StateRemovedFlag,
	})
	// will need check vals with validatorSet
	uid := h.uid
	h.uid++
	return &SwitchValidator{
		Infos:     vals,
		Resion:    resion,
		From:      from,
		DoorCount: 0,
		Remove:    remove,
		Add:       add,
		Round:     -1,
		ID:        uid, // for tmp
	}
}

func (h *HealthMgr) isShiftSV() (bool, int) {
	cnt := 0
	for _, v := range h.Work {
		if v.State == ctypes.StateUsedFlag {
			cnt++
		}
	}
	for _, v := range h.Back {
		if v.State == ctypes.StateUsedFlag {
			cnt++
		}
	}
	for _, v := range h.seed {
		if v.State == ctypes.StateUsedFlag {
			cnt++
		}
	}
	return cnt > params.MinimumCommitteeNumber, cnt
}

//switchResult handle the sv after consensus and the result removed from self
func (h *HealthMgr) switchResult(res *SwitchValidator) {
	if !EnableHealthMgr {
		return
	}
	ss := "failed"
	// remove sv in curSwitch if can
	if cur := h.getCurSV(); cur != nil {
		if (res.From == 1 && cur.Equal(res)) || cur.EqualWithoutID(res) || cur.EqualWithRemove(res) {
			h.removeCurSV()
			ss = "restore "
		}
	}

	if res.From == 0 {
		if len(res.Infos) > 2 {
			enter1, enter2 := res.Infos[0], res.Infos[1]
			var add, remove *Health
			if enter1.Flag == ctypes.StateAppendFlag {
				add = h.GetHealth(enter1.CommitteeBase.Bytes())
				if enter2.Flag == ctypes.StateRemovedFlag {
					remove = h.GetHealth(enter2.CommitteeBase.Bytes())
				}
			} else if enter1.Flag == ctypes.StateRemovedFlag {

				remove = h.GetHealth(enter1.CommitteeBase.Bytes())
			}
			if !remove.Equal(res.Remove) || !add.Equal(res.Add) {
				log.Error("switchResult item not match", "cid", h.cid, "remove", remove, "Remove", res.Remove, "add", add, "Add", res.Add)
			}
			if remove != nil {

				atomic.StoreUint32(&remove.State, ctypes.StateRemovedFlag)
				atomic.StoreInt32(&remove.Tick, 0) // issues for the sv was in another proposal queue
				ss += "Success"
			}
			if add != nil {

				atomic.StoreUint32(&add.State, ctypes.StateUsedFlag)
				atomic.StoreInt32(&add.Tick, 0)
			}
		}
	}
	log.Info("switchResult", "result:", ss, "res", res, "cid", h.cid)
}

//pickUnuseValidator get a back committee
func (h *HealthMgr) pickUnuseValidator() *Health {
	for _, v := range h.Back {

		if s := atomic.CompareAndSwapUint32(&v.State, ctypes.StateUnusedFlag, ctypes.StateSwitchingFlag); s {
			return v
		}
	}
	for _, v := range h.seed {

		if swap := atomic.CompareAndSwapUint32(&v.State, ctypes.StateUnusedFlag, ctypes.StateSwitchingFlag); swap {
			return v
		}
	}
	return nil
}

//Update tick
func (h *HealthMgr) Update(id tp2p.ID) {
	if v, ok := h.Work[id]; ok {
		if v.HType != ctypes.TypeFixed {
			atomic.StoreInt32(&v.Tick, 0)
			return
		}
	}
	for _, v := range h.Back {
		if v.ID == id {
			if v.HType != ctypes.TypeFixed {
				atomic.StoreInt32(&v.Tick, 0)
			}
			return
		}
	}
}

func (h *HealthMgr) getHealthFromPart(address []byte, part int) *Health {
	if part == SwitchPartBack { // back
		for _, v := range h.Back {

			if bytes.Equal(address, v.Val.Address) {
				return v
			}
		}
	} else if part == SwitchPartWork { // work
		for _, v := range h.Work {

			if bytes.Equal(address, v.Val.Address) {
				return v
			}
		}
	} else if part == SwitchPartSeed {
		for _, v := range h.seed {

			if bytes.Equal(address, v.Val.Address) {
				return v
			}
		}
	}
	return nil
}

//GetHealth get a Health for mgr

func (h *HealthMgr) GetHealth(adress []byte) *Health {
	enter := h.getHealthFromPart(adress, SwitchPartWork)
	if enter == nil {

		enter = h.getHealthFromPart(adress, SwitchPartBack)
	}
	if enter == nil {

		enter = h.getHealthFromPart(adress, SwitchPartSeed)
	}
	return enter
}

//VerifySwitch verify remove and add switchEnter
func (h *HealthMgr) VerifySwitch(sv *SwitchValidator) error {
	if !EnableHealthMgr {
		err := fmt.Errorf("healthMgr not enable")
		log.Error("VerifySwitch", "err", err)
		return err
	}
	if sv0 := h.getCurSV(); sv0 != nil {
		if sv0.Equal(sv) {
			log.Info("HealthMgr verify:sv equal sv0", "info", sv)
			return nil // proposal is self?
		}
	}
	return h.verifySwitchEnter(sv.Remove, sv.Add)
}

func (h *HealthMgr) verifySwitchEnter(remove, add *Health) error {

	rRes := false
	if remove == nil {
		return errors.New("not found the remove:" + remove.String())
	}

	rTick := atomic.LoadInt32(&remove.Tick)

	rState := atomic.LoadUint32(&remove.State)
	if rState >= ctypes.StateUsedFlag && rState <= ctypes.StateSwitchingFlag && rTick >= HealthOut {
		rRes = true
	}
	res := remove.SimpleString()

	aRes := false
	if add != nil {
		aState := atomic.LoadUint32(&add.State)
		if aState != ctypes.StateRemovedFlag && aState != ctypes.StateUsedFlag {
			aRes = true
		}
		res += add.SimpleString()
	} else {
		aRes = true
	}
	if rRes && aRes {
		return nil
	}
	return errors.New("Wrong state:" + res + "Remove:" + remove.String() + ",add:" + add.String())
}

//UpdateFromCommittee agent put member and back, update flag
func (h *HealthMgr) UpdateFromCommittee(member, backMember ctypes.CommitteeMembers) {
	for _, v := range member {
		for k, v2 := range h.Work {
			if bytes.Equal(v.CommitteeBase.Bytes(), v2.Val.Address) {
				atomic.StoreUint32(&h.Work[k].State, v.Flag)
				break
			}
		}
	}
	for _, v := range backMember {
		if v.MType == ctypes.TypeBack {
			for k, v2 := range h.Back {
				if bytes.Equal(v.CommitteeBase.Bytes(), v2.Val.Address) {
					atomic.StoreUint32(&h.Back[k].State, v.Flag)
					break
				}
			}
		} else if v.MType == ctypes.TypeFixed {
			for k, v2 := range h.seed {
				if bytes.Equal(v.CommitteeBase.Bytes(), v2.Val.Address) {
					atomic.StoreUint32(&h.seed[k].State, v.Flag)
					break
				}
			}
		}
	}

	h.checkSaveSwitchValidator(append(member, backMember...))
}

func (h *HealthMgr) checkSaveSwitchValidator(members ctypes.CommitteeMembers) {
	h.lock.Lock()
	defer h.lock.Unlock()

	for i := len(h.curSwitch) - 1; i >= 0; i-- {
		remove := h.curSwitch[i].Remove
		add := h.curSwitch[i].Add
		if remove == nil {
			log.Error("checkSaveSwitchValidator", "msg", "remove is nil")
			continue
		}
		rOK, aOk := false, false
		for _, v := range members {
			if bytes.Equal(v.CommitteeBase.Bytes(), remove.Val.Address) && v.Flag == ctypes.StateUsedFlag {
				rOK = true
			}

			if add == nil || (bytes.Equal(v.CommitteeBase.Bytes(), add.Val.Address) && v.Flag == ctypes.StateUnusedFlag) {
				aOk = true
			}
		}
		if !(rOK && aOk) {
			h.curSwitch = append(h.curSwitch[:i], h.curSwitch[i+1:]...)
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
