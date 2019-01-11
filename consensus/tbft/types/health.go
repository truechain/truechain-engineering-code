package types

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
	"sync/atomic"
	"time"
)

const (
	//HealthOut peer time out
	HealthOut = 60 * 60 * 24 * 7 //* 10
	//MixValidator min committee count
	MixValidator   = 2
	BlackDoorCount = 4

	SwitchPartWork = 0
	SwitchPartBack = 1
	SwitchPartSeed = 2

	EnableHealthMgr = false
)

//Health struct
type Health struct {
	ID    tp2p.ID
	IP    string
	Port  uint
	Tick  int32
	State int32
	HType int32
	Val   *Validator
	Self  bool
}

//NewHealth new
func NewHealth(id tp2p.ID, t, state int32, val *Validator, Self bool) *Health {
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
	return h.ID == other.ID && bytes.Equal(h.Val.PubKey.Bytes(), other.Val.PubKey.Bytes())
}

//SwitchValidator struct
type SwitchValidator struct {
	Remove    *Health
	Add       *Health
	Infos     *ctypes.SwitchInfos
	Resion    string
	From      int // 0-- add ,1-- resore
	DoorCount int
	Round 	  int 		// -1 not exc,no lock
	ID        uint64
}

func (s *SwitchValidator) String() string {
	if s == nil {
		return "switch-validator-nil"
	}
	return fmt.Sprintf("switch-validator:[ID:%v,Round:%d,R:%s,A:%s,Info:%s,Resion:%s,From:%d,Door:%d]",
		s.ID,s.Round,s.Remove, s.Add, s.Infos, s.Resion, s.From, s.DoorCount)
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
			s.Add.Equal(other.Add) && s.Infos.Equal(other.Infos)
}
// EqualWithoutID return true they are same id or both nil otherwise return false 
func (s *SwitchValidator) EqualWithoutID(other *SwitchValidator) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	return s.Remove.Equal(other.Remove) && s.Add.Equal(other.Add) && s.Infos.Equal(other.Infos)
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
	uid 		   uint64
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
		healthTick:     nil,
	}
	h.BaseService = *help.NewBaseService("HealthMgr", h)
	hi,lo := cid << 32,uint64(100)
	h.uid = hi | lo
	log.Info("HealthMgr init","cid",cid,"hi",hi,"lo",lo,"uid",h.uid)	
	return h
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
	help.CheckAndPrintError(h.Stop())
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
	sshift,islog,cnt := true,true,0
	for {
		select {
		case <-h.healthTick.C:
			sshift,cnt = h.isShiftSV()
			h.work(sshift)
			if !sshift && islog {
				log.Info("Stop Shift Switch Validator, because minimum SV","Count",cnt,"CID",h.cid)
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
	if !EnableHealthMgr { return }

	for _, v := range h.Work {
		h.checkSwitchValidator(v,sshift)
	}
	for _, v := range h.Back {
		h.checkSwitchValidator(v,sshift)
	}
}

func (h *HealthMgr) checkSwitchValidator(v *Health,sshift bool) {
	if v.State == ctypes.StateUsedFlag && v.HType != ctypes.TypeFixed && !v.Self {
		val := atomic.AddInt32(&v.Tick, 1)
		log.Info("Health", "info:", fmt.Sprintf("id:%s val:%d state:%d", v.ID, val, v.State))

		if sshift && val > HealthOut && v.State == ctypes.StateUsedFlag &&
			!v.Self && len(h.curSwitch) == 0 {
			log.Info("Health", "Change", true)
			back := h.pickUnuseValidator()
			cur := h.makeSwitchValidators(v, back, "Switch", 0)
			h.curSwitch = append(h.curSwitch, cur)
			go h.Switch(cur)
			atomic.StoreInt32(&v.State, int32(ctypes.StateSwitchingFlag))
		}	

		if len(h.curSwitch) > 0 {
			sv0 := h.curSwitch[0]
			val0 := atomic.LoadInt32(&sv0.Remove.Tick)
			if val0 < HealthOut && sv0.From == 0 {
				sv1 := *sv0
				sv1.From = 1
				go h.Switch(&sv1)
			}
		}
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
	uid := h.uid
	h.uid++
	return &SwitchValidator{
		Infos:     infos,
		Resion:    resion,
		From:      from,
		DoorCount: 0,
		Remove:    remove,
		Add:       add,
		Round:	   -1,
		ID:        uid, // for tmp
	}
}

func (h *HealthMgr) isShiftSV() (bool,int) {
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
	return cnt > MixValidator,cnt
}

//switchResult handle the sv after consensus and the result removed from self 
func (h *HealthMgr) switchResult(res *SwitchValidator) {
	if !EnableHealthMgr { return }
	
	// remove sv in curSwitch if can
	if len(h.curSwitch) > 0 {
		cur := h.curSwitch[0] 
		if (res.From == 1 && cur.Equal(res)) || cur.EqualWithoutID(res) {
			h.curSwitch = append(h.curSwitch[:0], h.curSwitch[1:]...)
		}
	}

	ss := "failed"
	if res.From == 0 {
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
	log.Info("switch", "result:", ss, "res", res)
}

//pickUnuseValidator get a back committee
func (h *HealthMgr) pickUnuseValidator() *Health {
	for _, v := range h.Back {
		if s := atomic.CompareAndSwapInt32(&v.State, int32(ctypes.StateUnusedFlag), int32(ctypes.StateSwitchingFlag)); s {
			return v
		}
	}
	for _, v := range h.seed {
		if swap := atomic.CompareAndSwapInt32(&v.State, int32(ctypes.StateUnusedFlag), int32(ctypes.StateSwitchingFlag)); swap {
			return v
		}
	}
	return nil
}

//Update tick
func (h *HealthMgr) Update(id tp2p.ID) {
	if v, ok := h.Work[id]; ok {
		if v.HType != ctypes.TypeFixed {
			val := atomic.LoadInt32(&v.Tick)
			atomic.AddInt32(&v.Tick, -val)
			return
		}
	}
	for _, v := range h.Back {
		if v.ID == id {
			if v.HType != ctypes.TypeFixed {
				val := atomic.LoadInt32(&v.Tick)
				atomic.AddInt32(&v.Tick, -val)
			}
			return
		}
	}
}

func (h *HealthMgr) getHealthFromPart(pk []byte, part int) *Health {
	if part == SwitchPartBack { // back
		for _, v := range h.Back {
			if bytes.Equal(pk, v.Val.PubKey.Bytes()) {
				return v
			}
		}
	} else if part == SwitchPartWork { // work
		for _, v := range h.Work {
			if bytes.Equal(pk, v.Val.PubKey.Bytes()) {
				return v
			}
		}
	} else if part == SwitchPartSeed {
		for _, v := range h.seed {
			if bytes.Equal(pk, v.Val.PubKey.Bytes()) {
				return v
			}
		}
	}
	return nil
}

//GetHealth get a Health for mgr
func (h *HealthMgr) GetHealth(pk []byte) *Health {
	enter := h.getHealthFromPart(pk, SwitchPartWork)
	if enter == nil {
		enter = h.getHealthFromPart(pk, SwitchPartBack)
	}
	if enter == nil {
		enter = h.getHealthFromPart(pk, SwitchPartSeed)
	}
	return enter
}

//VerifySwitch verify remove and add switchEnter
func (h *HealthMgr) VerifySwitch(sv *SwitchValidator) error {
	
	if len(h.curSwitch) > 0 {
		sv0 := h.curSwitch[0] 
		if sv0.Equal(sv) {
			return nil 	// proposal is self?
		}	
	}
	return h.verifySwitchEnter(sv.Remove,sv.Add)
}

func (h *HealthMgr) verifySwitchEnter(remove, add *Health) error {
	if !EnableHealthMgr {
		err := fmt.Errorf("healthMgr not enable")
		log.Error("VerifySwitch","err",err) 
		return err
	}
	r := remove
	rRes := false

	if r == nil {
		return errors.New("not found the remove:" + remove.String())
	}

	rTick := atomic.LoadInt32(&r.Tick)
	if r.State >= ctypes.StateUsedFlag && r.State <= ctypes.StateSwitchingFlag && rTick >= HealthOut {
		rRes = true
	}
	res := r.SimpleString()

	aRes := false
	if add != nil {
		if add.State != ctypes.StateRemovedFlag && add.State != ctypes.StateUsedFlag {
			aRes = true
		}
		res += add.SimpleString()
	} else {
		aRes = true
	}
	if rRes && aRes {
		return nil
	}
	return errors.New("Wrang state:" + res + "Remove:" + remove.String() + ",add:" + add.String())
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
