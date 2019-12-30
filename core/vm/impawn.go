package vm

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/rlp"
)

var (
	baseUnit           = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fbaseUnit          = new(big.Float).SetFloat64(float64(baseUnit.Int64()))
	mixImpawn          = new(big.Int).Mul(big.NewInt(1000), baseUnit)
	base               = new(big.Int).SetUint64(10000)
	CountInEpoch       = 31
	MaxRedeemHeight    = 1000
	MixEpochCount      = 2
	EpochElectionPoint = 500
	DposForkPoint      = uint64(20)
	PreselectionPeriod = uint64(2000)
	EpochLength        = uint64(10000)
)

var (
	errInvalidParam      = errors.New("Invalid Param")
	errOverEpochID       = errors.New("Over epoch id")
	errNotSequential     = errors.New("epoch id not sequential")
	errInvalidEpochInfo  = errors.New("Invalid epoch info")
	errNullImpawnInEpoch = errors.New("null impawn in the epoch")
	errInvalidStaking    = errors.New("Invalid staking account")
	errMatchEpochID      = errors.New("wrong match epoch id in a reward block")
	errNotStaking        = errors.New("Not match the staking account")
	errNotDelegation     = errors.New("Not match the delegation account")
	errNotMatchEpochInfo = errors.New("the epoch info is not match with accounts")
)

const (
	// StateStakingOnce can be election only once
	StateStakingOnce uint8 = 1 << iota
	// StateStakingAuto can be election in every epoch
	StateStakingAuto
	// StateRedeem can be redeem real time (after MaxRedeemHeight block)
	StateRedeem
	// StateRedeeming flag the asset which is staking in the height is redeeming
	StateRedeeming
	// StateRedeemed flag the asset which is staking in the height is redeemed
	StateRedeemed
)

type RewardInfo struct {
	Address common.Address
	Amount  *big.Int
}
type SARewardInfos struct {
	items []*RewardInfo
}
type EpochIDInfo struct {
	EpochID     uint64
	BeginHeight uint64
	EndHeight   uint64
}

func (e *EpochIDInfo) isValid() bool {
	if e.EpochID < 0 {
		return false
	}
	if e.EpochID == 0 && DposForkPoint+1 != e.BeginHeight {
		return false
	}
	if e.BeginHeight < 0 || e.EndHeight <= 0 || e.EndHeight <= e.BeginHeight {
		return false
	}
	return true
}
func (e *EpochIDInfo) String() string {
	return fmt.Sprintf("[id:%v,begin:%v,end:%v]", e.EpochID, e.BeginHeight, e.EndHeight)
}

func toReward(val *big.Float) *big.Int {
	val = val.Mul(val, fbaseUnit)
	ii, _ := val.Int64()
	return big.NewInt(ii)
}
func fromBlock(block *types.SnailBlock) (begin, end uint64) {
	begin, end = 0, 0
	l := len(block.Fruits())
	if l > 0 {
		begin, end = block.Fruits()[0].NumberU64(), block.Fruits()[l-1].NumberU64()
	}
	return
}
func getfirstEpoch() *EpochIDInfo {
	return &EpochIDInfo{
		EpochID:     0,
		BeginHeight: DposForkPoint + 1,
		EndHeight:   DposForkPoint + PreselectionPeriod + EpochLength,
	}
}
func GetEpochFromHeight(hh uint64) *EpochIDInfo {
	if hh <= DposForkPoint {
		return nil
	}
	first := getfirstEpoch()
	if hh <= first.EndHeight {
		return first
	}
	eid := (hh-first.EndHeight)/EpochLength + 1
	return &EpochIDInfo{
		EpochID:     eid,
		BeginHeight: first.EndHeight + (eid-1)*EpochLength + 1,
		EndHeight:   first.EndHeight + eid*EpochLength,
	}
}
func GetEpochFromID(eid uint64) *EpochIDInfo {
	first := getfirstEpoch()
	if first.EpochID == 0 {
		return first
	}
	return &EpochIDInfo{
		EpochID:     eid,
		BeginHeight: first.EndHeight + (eid-1)*EpochLength + 1,
		EndHeight:   first.EndHeight + eid*EpochLength,
	}
}
func GetEpochFromRange(begin, end uint64) []*EpochIDInfo {
	if end == 0 || begin > end || (begin < DposForkPoint && end < DposForkPoint) {
		return nil
	}
	var ids []*EpochIDInfo
	e1 := GetEpochFromHeight(begin)
	e := uint64(0)

	if e1 != nil {
		ids = append(ids, e1)
		e = e1.EndHeight
	} else {
		e = DposForkPoint
	}
	for e < end {
		e2 := GetEpochFromHeight(e + 1)
		if e1.EpochID != e2.EpochID {
			ids = append(ids, e2)
		}
		e = e2.EndHeight
	}

	if len(ids) == 0 {
		return nil
	}
	return ids
}
func copyVotePk(pk []byte) []byte {
	cc := make([]byte, len(pk))
	copy(cc, pk)
	return cc
}

/////////////////////////////////////////////////////////////////////////////////
type PairstakingValue struct {
	amount *big.Int
	height *big.Int
	state  uint8
}

func (v *PairstakingValue) isElection() bool {
	return v.state&StateStakingOnce != 0 || v.state&StateStakingAuto != 0
}

type RewardItem struct {
	Amount *big.Int
	Height *big.Int
}
type RedeemItem struct {
	Amount *big.Int
	Height *big.Int
	State  uint8
	Last   []*PairstakingValue // the redeem info of the epochs before current epoch
}

type impawnUnit struct {
	address    common.Address
	value      []*PairstakingValue // sort by height
	rewardInfo []*RewardItem
	redeemInof *RedeemItem
}

func (s *impawnUnit) getAllStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, v := range s.value {
		if v.height.Uint64() <= hh {
			all = all.Add(all, v.amount)
		} else {
			break
		}
	}
	return all
}
func (s *impawnUnit) getValidStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, v := range s.value {
		if v.height.Uint64() <= hh {
			if v.isElection() {
				all = all.Add(all, v.amount)
			}
		} else {
			break
		}
	}
	return all
}
func (s *impawnUnit) GetRewardAddress() common.Address {
	return s.address
}
func (s *impawnUnit) stopStakingInfo(amount, lastHeight *big.Int) {
	if s.redeemInof == nil {
		s.redeemInof = &RedeemItem{
			Amount: new(big.Int).Set(amount),
			Height: new(big.Int).Set(lastHeight),
			State:  StateRedeem,
		}
	} else {
		s.redeemInof.Amount = new(big.Int).Add(s.redeemInof.Amount, amount)
		s.redeemInof.Height = new(big.Int).Set(lastHeight)
	}
	// can be optimize
	redeem := big.NewInt(0)
	for _, v := range s.value {
		redeem = redeem.Add(redeem, v.amount)
		res := redeem.Cmp(s.redeemInof.Amount)

		if res == 0 {
			v.state &= ^(StateStakingAuto | StateStakingOnce)
			break
		} else if res > 0 {
			v.state &= ^StateStakingAuto // election on current epoch and cancel in next epoch
			break
		} else {
			v.state &= ^(StateStakingAuto | StateStakingOnce)
		}
	}
}
func (s *impawnUnit) redeeming() (common.Address, *big.Int) {
	all := big.NewInt(0)
	pos := 0
	for i, v := range s.value {
		all = all.Add(all, v.amount)
		v.state |= StateRedeeming
		res := all.Cmp(s.redeemInof.Amount)
		if res == 0 {
			pos = i
		} else if res > 0 {
			pos, v.amount = i, new(big.Int).Sub(all, s.redeemInof.Amount)
			all.Set(s.redeemInof.Amount)
			break
		}
	}
	s.value = s.value[pos:]
	return s.address, all
}
func (s *impawnUnit) clearRedeemed(all *big.Int) {
	if s.redeemInof.Amount.Cmp(all) < 0 {
		panic("redeem all amount exception")
	}
	s.redeemInof.Amount = new(big.Int).Sub(s.redeemInof.Amount, all)
	s.redeemInof.State |= StateRedeemed
}
func (s *impawnUnit) isRedeemed() bool {
	if s.redeemInof != nil {
		return s.redeemInof.State&StateRedeem != 0
	}
	return false
}

// merge for move from prev to next epoch,move the staking who was to be voted
// merge all staking to one staking with the new height(the beginning of next epoch)
// called by move function
func (s *impawnUnit) merge(hh uint64) {
	all := big.NewInt(0)
	redeem := false
	state := uint8(0)
	for _, v := range s.value {
		all = all.Add(all, v.amount)
		redeem = v.state&StateStakingAuto != 0 // the last flag
	}
	if redeem && all.Cmp(s.redeemInof.Amount) > 0 {
		state |= StateStakingAuto
	}
	var val []*PairstakingValue
	left := all.Sub(all, s.redeemInof.Amount)
	if left.Sign() > 0 {
		val = append(val, &PairstakingValue{
			amount: left,
			height: new(big.Int).SetUint64(hh),
			state:  state,
		})
	}
	s.value = val
}
func (s *impawnUnit) update(unit *impawnUnit, move bool) {
	sorter := valuesByHeight(s.value)
	for _, v := range unit.value {
		sorter = sorter.update(v)
	}
	s.value = sorter

	if s.redeemInof == nil {
		s.redeemInof = &RedeemItem{
			Amount: big.NewInt(0),
			Height: big.NewInt(0),
			State:  0,
			Last:   make([]*PairstakingValue, 0, 0),
		}
	}
	if move {
		tmp := &PairstakingValue{
			amount: unit.redeemInof.Amount,
			height: unit.redeemInof.Height,
			state:  unit.redeemInof.State,
		}
		s.redeemInof.Last = append(s.redeemInof.Last, tmp)
	} else {
		s.redeemInof.Amount = new(big.Int).Add(s.redeemInof.Amount, unit.redeemInof.Amount)
		s.redeemInof.State |= unit.redeemInof.State
		if len(unit.redeemInof.Last) != 0 {
			s.redeemInof.Last = append(s.redeemInof.Last, unit.redeemInof.Last...)
		}
	}

}
func (s *impawnUnit) clone() *impawnUnit {
	return nil
}
func (s *impawnUnit) sort() {
	sort.Sort(valuesByHeight(s.value))
}

/////////////////////////////////////////////////////////////////////////////////

type DelegationAccount struct {
	deleAddress common.Address
	unit        *impawnUnit
}

func (d *DelegationAccount) update(da *DelegationAccount, move bool) {
	d.unit.update(da.unit, move)
}
func (s *DelegationAccount) getAllStaking(hh uint64) *big.Int {
	return s.unit.getAllStaking(hh)
}
func (s *DelegationAccount) getValidStaking(hh uint64) *big.Int {
	return s.unit.getValidStaking(hh)
}
func (s *DelegationAccount) stopStakingInfo(amount, lastHeight *big.Int) {
	s.unit.stopStakingInfo(amount, lastHeight)
}
func (s *DelegationAccount) redeeming() (common.Address, *big.Int) {
	return s.unit.redeeming()
}
func (s *DelegationAccount) clearRedeemed(all *big.Int) {
	s.unit.clearRedeemed(all)
}
func (s *DelegationAccount) merge(hh uint64) {
	s.unit.merge(hh)
}
func (s *DelegationAccount) clone() *DelegationAccount {
	return &DelegationAccount{
		deleAddress: s.deleAddress,
		unit:        s.unit.clone(),
	}
}

type StakingAccount struct {
	unit       *impawnUnit
	votepubkey []byte
	fee        *big.Int
	committee  bool
	delegation []*DelegationAccount
	modify     *AlterableInfo
}
type AlterableInfo struct {
	fee        *big.Int
	votePubkey []byte
}

func (s *StakingAccount) isInCommittee() bool {
	return s.committee
}
func (s *StakingAccount) update(sa *StakingAccount, hh uint64, next, move bool) {
	s.unit.update(sa.unit, move)
	dirty := false
	for _, v := range sa.delegation {
		da := s.getDA(v.unit.GetRewardAddress())
		if da == nil {
			s.delegation = append(s.delegation, v)
			dirty = true
		} else {
			da.update(v, move)
		}
	}

	if hh > s.getMaxHeight() {
		s.modify = sa.modify
	}
	if next {
		s.changeAlterableInfo()
	}
	if dirty && hh != 0 {
		tmp := toDelegationByHeight(hh, false, s.delegation)
		sort.Sort(tmp)
		s.delegation, _ = fromDelegationByHeight(tmp)
	}
}
func (s *StakingAccount) stopStakingInfo(amount, lastHeight *big.Int) {
	s.unit.stopStakingInfo(amount, lastHeight)
}
func (s *StakingAccount) redeeming() (common.Address, *big.Int) {
	return s.unit.redeeming()
}
func (s *StakingAccount) clearRedeemed(all *big.Int) {
	s.unit.clearRedeemed(all)
}
func (s *StakingAccount) getAllStaking(hh uint64) *big.Int {
	all := s.unit.getAllStaking(hh)
	for _, v := range s.delegation {
		all = all.Add(all, v.getAllStaking(hh))
	}
	return all
}
func (s *StakingAccount) getValidStaking(hh uint64) *big.Int {
	all := s.unit.getValidStaking(hh)
	for _, v := range s.delegation {
		all = all.Add(all, v.getValidStaking(hh))
	}
	return all
}
func (s *StakingAccount) merge(hh uint64) {
	s.unit.merge(hh)
	for _, v := range s.delegation {
		v.merge(hh)
	}
}
func (s *StakingAccount) getDA(addr common.Address) *DelegationAccount {
	for _, v := range s.delegation {
		if bytes.Equal(v.unit.address.Bytes(), addr.Bytes()) {
			return v
		}
	}
	return nil
}
func (s *StakingAccount) getMaxHeight() uint64 {
	l := len(s.unit.value)
	return s.unit.value[l-1].height.Uint64()
}
func (s *StakingAccount) changeAlterableInfo() {
	if s.modify != nil {
		if s.modify.fee != nil {
			// s.fee = new(big.Int).Set(s.modify.fee)
			s.fee = s.modify.fee
		}
		if s.modify.votePubkey != nil {
			s.votepubkey = s.modify.votePubkey
		}
	}
}
func (s *StakingAccount) clone() *StakingAccount {
	ss := &StakingAccount{
		votepubkey: copyVotePk(s.votepubkey),
		unit:       s.unit.clone(),
		fee:        new(big.Int).Set(s.fee),
		committee:  s.committee,
		delegation: make([]*DelegationAccount, 0),
	}
	for _, v := range s.delegation {
		ss.delegation = append(ss.delegation, v.clone())
	}
	if s.modify != nil {
		ss.modify = &AlterableInfo{
			fee:        new(big.Int).Set(s.modify.fee),
			votePubkey: copyVotePk(s.modify.votePubkey),
		}
	}
	return ss
}

type SAImpawns []*StakingAccount

func (s *SAImpawns) getAllStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, val := range *s {
		all = all.Add(all, val.getAllStaking(hh))
	}
	return all
}
func (s *SAImpawns) getValidStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, val := range *s {
		all = all.Add(all, val.getValidStaking(hh))
	}
	return all
}
func (s *SAImpawns) sort(hh uint64, valid bool) {
	for _, v := range *s {
		tmp := toDelegationByHeight(hh, valid, v.delegation)
		sort.Sort(tmp)
		v.delegation, _ = fromDelegationByHeight(tmp)
	}
	tmp := toStakingByHeight(hh, valid, *s)
	sort.Sort(tmp)
	*s, _ = fromStakingByHeight(tmp)
}
func (s *SAImpawns) getSA(addr common.Address) *StakingAccount {
	for _, val := range *s {
		if bytes.Equal(val.unit.address.Bytes(), addr.Bytes()) {
			return val
		}
	}
	return nil
}
func (s *SAImpawns) update(sa1 *StakingAccount, hh uint64, next, move bool) {
	sa := s.getSA(sa1.unit.address)
	if sa == nil {
		*s = append(*s, sa1)
		s.sort(hh, false)
	} else {
		sa.update(sa1, hh, next, move)
	}
}

type SimpleElectionInfo struct {
	Eid     uint64
	Address []common.Address
}

/////////////////////////////////////////////////////////////////////////////////
// be thread-safe for caller locked
type ImpawnImpl struct {
	lock       sync.RWMutex
	accounts   map[uint64]SAImpawns
	curEpochID uint64
	lastReward uint64 // the curnent reward height block
}

func NewImpawnImpl() *ImpawnImpl {
	return &ImpawnImpl{
		curEpochID: 0,
		lastReward: 0,
		accounts:   make(map[uint64]SAImpawns),
	}
}

/////////////////////////////////////////////////////////////////////////////////
///////////  auxiliary function ////////////////////////////////////////////
func (i *ImpawnImpl) getCurrentEpoch() uint64 {
	return i.curEpochID
}
func (i *ImpawnImpl) setCurrentEpoch(eid uint64) {
	i.curEpochID = eid
}
func (i *ImpawnImpl) getCurrentEpochInfo() []*EpochIDInfo {
	var epochs []*EpochIDInfo
	var eids []float64
	for k, _ := range i.accounts {
		eids = append(eids, float64(k))
	}
	sort.Float64s(eids)
	for _, v := range eids {
		e := GetEpochFromID(uint64(v))
		if e != nil {
			epochs = append(epochs, e)
		}
	}
	return epochs
}
func (i *ImpawnImpl) GetStakingAccount(epochid uint64, addr common.Address) (*StakingAccount, error) {
	if v, ok := i.accounts[epochid]; !ok {
		return nil, errInvalidStaking
	} else {
		for _, val := range v {
			if bytes.Equal(val.unit.address.Bytes(), addr.Bytes()) {
				return val, nil
			}
		}
	}
	return nil, errInvalidStaking
}
func (i *ImpawnImpl) getDAfromSA(sa *StakingAccount, addr common.Address) (*DelegationAccount, error) {
	for _, ii := range sa.delegation {
		if bytes.Equal(ii.unit.address.Bytes(), addr.Bytes()) {
			return ii, nil
		}
	}
	return nil, nil
}
func (i *ImpawnImpl) getElections(epochid uint64) []common.Address {
	if accounts, ok := i.accounts[epochid]; !ok {
		return nil
	} else {
		var addrs []common.Address
		for _, v := range accounts {
			if v.isInCommittee() {
				addrs = append(addrs, v.unit.GetRewardAddress())
			}
		}
		return addrs
	}
}
func (i *ImpawnImpl) fetchAccountsInEpoch(epochid uint64, addrs []common.Address) SAImpawns {
	if len(addrs) == 0 {
		return nil
	}
	if accounts, ok := i.accounts[epochid]; !ok {
		return nil
	} else {
		find := func(addrs []common.Address, addr common.Address) bool {
			for _, v := range addrs {
				if bytes.Equal(v.Bytes(), addr.Bytes()) {
					return true
				}
			}
			return false
		}
		var items SAImpawns
		for _, val := range accounts {
			if val.isInCommittee() && find(addrs, val.unit.GetRewardAddress()) {
				items = append(items, val)
			}
		}
		return items
	}
}
func (i *ImpawnImpl) redeemBySa(sa *StakingAccount, height, epochEnd uint64) {
	// can be redeem in the SA
	if height > epochEnd+uint64(MaxRedeemHeight) {
		addr, all := sa.redeeming()
		err := i.redeemPrincipal(addr, all)
		if err == nil {
			sa.clearRedeemed(all)
		}
		fmt.Println("SA redeemed amount:[", all.String(), "],addr:[", addr.String(), "],err:", err)
	}
}
func (i *ImpawnImpl) redeemByDa(da *DelegationAccount, height, epochEnd uint64) {
	// can be redeem in the DA
	if height > epochEnd+uint64(MaxRedeemHeight) {
		addr, all := da.redeeming()
		err := i.redeemPrincipal(addr, all)
		if err == nil {
			da.clearRedeemed(all)
		}
		fmt.Println("DA redeemed amount:[", all.String(), "],addr:[", addr.String(), "],err:", err)
	}
}
func (i *ImpawnImpl) calcRewardInSa(target uint64, sa *StakingAccount, allReward, allStaking *big.Int, item *RewardInfo) ([]*RewardInfo, error) {
	if sa == nil || allReward == nil || item == nil || allStaking == nil {
		return nil, errInvalidParam
	}
	var items []*RewardInfo
	fee := new(big.Int).Quo(new(big.Int).Mul(allReward, sa.fee), base)
	all, left := new(big.Int).Sub(allReward, fee), big.NewInt(0)
	for _, v := range sa.delegation {
		daAll := v.getAllStaking(target)
		v1 := new(big.Int).Quo(new(big.Int).Mul(all, daAll), allStaking)
		left = left.Add(left, v1)
		var ii RewardInfo
		ii.Address, ii.Amount = v.unit.GetRewardAddress(), new(big.Int).Set(v1)
		items = append(items, &ii)
	}
	item.Amount = new(big.Int).Add(new(big.Int).Sub(all, left), fee)
	return items, nil
}
func (i *ImpawnImpl) calcReward(target uint64, allAmount *big.Int, einfo *EpochIDInfo) ([]*SARewardInfos, error) {
	if _, ok := i.accounts[einfo.EpochID]; !ok {
		return nil, errInvalidParam
	} else {
		addrs := i.getElections(einfo.EpochID)
		impawns := i.fetchAccountsInEpoch(einfo.EpochID, addrs)
		if impawns == nil || len(addrs) == 0 {
			return nil, errNullImpawnInEpoch
		}
		var res []*SARewardInfos
		allValidatorStaking := impawns.getAllStaking(target)

		for _, v := range impawns {
			var info SARewardInfos
			var item RewardInfo
			item.Address = v.unit.GetRewardAddress()
			allStaking := v.getAllStaking(target)

			v2 := new(big.Int).Quo(new(big.Int).Mul(allStaking, allAmount), allValidatorStaking)

			if ii, err := i.calcRewardInSa(target, v, v2, allStaking, &item); err != nil {
				return nil, err
			} else {
				info.items = append(info.items, &item)
				info.items = append(info.items, ii[:]...)
			}
			res = append(res, &info)
		}
		return res, nil
	}
}

///////////auxiliary function ////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////////
// 1. keep the minimum epoch count
// 2. release the item of redeemed
func (i *ImpawnImpl) shuffle() {
	epochs := i.getCurrentEpochInfo()
	if len(epochs) == 0 {
		return
	}
	min, max := epochs[0].EpochID, epochs[len(epochs)-1].EpochID
	if max-min < uint64(MixEpochCount) {
		return
	}
	for _, epoch := range epochs {
		if max-epoch.EpochID > uint64(MixEpochCount) && MixEpochCount >= 2 {
			i.move(epoch.EpochID, epoch.EpochID+1)
		}
	}
}

// move the accounts from prev to next epoch and keeps the prev account still here
func (i *ImpawnImpl) move(prev, next uint64) error {
	nextEpoch := GetEpochFromID(next)
	if nextEpoch == nil {
		return errOverEpochID
	}
	prevInfos, ok := i.accounts[prev]
	nextInfos, ok2 := i.accounts[next]
	if !ok {
		return errors.New(fmt.Sprintln("the epoch is nil", prev, "err:", errNotMatchEpochInfo))
	}
	if !ok2 {
		nextInfos = SAImpawns{}
	}
	for _, v := range prevInfos {
		vv := v.clone()
		vv.merge(nextEpoch.BeginHeight)
		nextInfos.update(vv, nextEpoch.BeginHeight, true, true)
	}
	return nil
}
func (i *ImpawnImpl) redeemPrincipal(addr common.Address, amount *big.Int) error {
	return nil
}

/////////////////////////////////////////////////////////////////////////////////
////////////// external function //////////////////////////////////////////

// DoElections called by consensus while it closer the end of epoch,have 500~1000 fast block
func (i *ImpawnImpl) DoElections(epochid, begin, end uint64) ([]*StakingAccount, error) {
	if epochid != i.getCurrentEpoch()+1 {
		return nil, errOverEpochID
	}
	if val, ok := i.accounts[epochid-1]; ok {
		val.sort(end, true)
		var ee []*StakingAccount
		for i, v := range val {
			v.committee = true
			ee = append(ee, v)
			if i == CountInEpoch-1 {
				break
			}
		}
		return ee, nil
	} else {
		return nil, errMatchEpochID
	}
}

// Shift will move the staking account which has election flag to the next epoch
func (i *ImpawnImpl) Shift(epochid uint64) error {
	if epochid != i.getCurrentEpoch()+1 {
		return errOverEpochID
	}
	i.setCurrentEpoch(epochid)
	prev := epochid - 1
	return i.move(prev, epochid)
}

// RedeemSAccount redeem amount of asset for staking account,it will be done in next epoch
func (i *ImpawnImpl) RedeemSAccount(curHeight uint64, addr common.Address, amount *big.Int) error {
	curEpoch := GetEpochFromHeight(curHeight)
	if curEpoch == nil {
		return errInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addr)
	if err != nil {
		return err
	}
	sa.stopStakingInfo(amount, new(big.Int).SetUint64(curHeight))
	fmt.Println("[SA]insert a redeem,address:[", addr.String(), "],amount:[", amount.String(), "],height:", curHeight)
	return nil
}

// RedeemDAccount redeem amount of asset for delegation account,it will be done in next epoch
func (i *ImpawnImpl) RedeemDAccount(curHeight uint64, addrSA, addrDA common.Address, amount *big.Int) error {
	curEpoch := GetEpochFromHeight(curHeight)
	if curEpoch == nil {
		return errInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addrSA)
	if err != nil {
		return err
	}
	da, err2 := i.getDAfromSA(sa, addrDA)
	if err2 != nil {
		return err
	}
	da.stopStakingInfo(amount, new(big.Int).SetUint64(curHeight))
	fmt.Println("[DA]insert a redeem,address:[", addrSA.String(), "],DA address:[", addrDA.String(), "],amount:[", amount.String(), "],height:", curHeight)
	return nil
}
func (i *ImpawnImpl) insertDAccount(epochID uint64, da *DelegationAccount) error {
	if da == nil {
		return errInvalidParam
	}
	if epochID > i.getCurrentEpoch() {
		return errOverEpochID
	}
	sa, err := i.GetStakingAccount(epochID, da.deleAddress)
	if err != nil {
		return err
	}
	if da, err := i.getDAfromSA(sa, da.unit.address); err != nil {
		return err
	} else {
		if da == nil {
			sa.delegation = append(sa.delegation, da)
		} else {
			da.update(da, false)
		}
	}
	return nil
}
func (i *ImpawnImpl) InsertDAccount2(height uint64, addr, deleAddr common.Address, val *big.Int) error {

	da := &DelegationAccount{
		deleAddress: deleAddr,
		unit: &impawnUnit{
			address: addr,
			value: []*PairstakingValue{&PairstakingValue{
				amount: new(big.Int).Set(val),
				height: new(big.Int).SetUint64(height),
				state:  0,
			}},
		},
	}
	return i.insertDAccount(height, da)
}
func (i *ImpawnImpl) insertSAccount(height uint64, sa *StakingAccount) error {
	if sa == nil {
		return errInvalidParam
	}
	epochInfo := GetEpochFromHeight(height)
	if epochInfo == nil || epochInfo.EpochID > i.getCurrentEpoch() {
		return errOverEpochID
	}
	if val, ok := i.accounts[epochInfo.EpochID]; !ok {
		var accounts []*StakingAccount
		accounts = append(accounts, sa)
		i.accounts[epochInfo.EpochID] = SAImpawns(accounts)
	} else {
		for _, ii := range val {
			if bytes.Equal(ii.unit.address.Bytes(), sa.unit.address.Bytes()) {
				ii.update(sa, height, false, false)
				return nil
			}
		}
		val = append(val, sa)
	}
	return nil
}
func (i *ImpawnImpl) InsertSAccount2(height uint64, addr common.Address, pk []byte, val *big.Int, fee *big.Int, auto bool) error {
	state := uint8(0)
	if auto {
		state |= StateStakingAuto
	}
	sa := &StakingAccount{
		votepubkey: append([]byte{}, pk...),
		fee:        new(big.Int).Set(fee),
		unit: &impawnUnit{
			address: addr,
			value: []*PairstakingValue{&PairstakingValue{
				amount: new(big.Int).Set(val),
				height: new(big.Int).SetUint64(height),
				state:  state,
			}},
		},
		modify: &AlterableInfo{},
	}
	return i.insertSAccount(height, sa)
}

// doing in every 200 fast block produced by consensus
// 1. redeem while not be in committee
// 2. set auto=false when the redeem amount is equal or greater than staking amount in the account
// 3. judge the block height
// 4. all redeem for the staking in last epoch will be done
func (i *ImpawnImpl) DoRedeem(curHeight uint64) error {
	curEID := i.getCurrentEpoch()
	epochs := i.getCurrentEpochInfo()
	// make sure the epochInfo sort by EpochID
	for _, epoch := range epochs {
		if val, ok := i.accounts[epoch.EpochID]; ok {
			for _, v := range val {
				if epoch.EpochID < curEID {
					for _, vv := range v.delegation {
						i.redeemByDa(vv, curHeight, epoch.EndHeight)
					}
					i.redeemBySa(v, curHeight, epoch.EndHeight)
				} else {
					if !v.isInCommittee() {
						i.redeemBySa(v, curHeight, epoch.EndHeight)
					}
				}
			}
		}
	}
	i.shuffle()
	return nil
}

func (i *ImpawnImpl) Reward(block *types.SnailBlock, allAmount *big.Int) ([]*SARewardInfos, error) {
	begin, end := fromBlock(block)
	ids := GetEpochFromRange(begin, end)
	if ids == nil || len(ids) > 2 {
		return nil, errMatchEpochID
	}

	if len(ids) == 2 {
		tmp := new(big.Int).Quo(new(big.Int).Mul(allAmount, new(big.Int).SetUint64(ids[0].EndHeight-ids[0].BeginHeight)), new(big.Int).SetUint64(end-begin))
		amount1, amount2 := tmp, new(big.Int).Sub(allAmount, tmp)
		if items, err := i.calcReward(ids[0].EndHeight, amount1, ids[0]); err != nil {
			return nil, err
		} else {
			if items1, err2 := i.calcReward(ids[1].EndHeight, amount2, ids[1]); err != nil {
				return nil, err2
			} else {
				items = append(items, items1[:]...)
			}
			return items, nil
		}
	} else {
		return i.calcReward(end, allAmount, ids[0])
	}
}

/////////////////////////////////////////////////////////////////////////////////
// storage layer
func (i *ImpawnImpl) GetRoot() common.Hash {
	return common.Hash{}
}
func (i *ImpawnImpl) Save(state StateDB, preAddress common.Address) error {
	key := common.BytesToHash(preAddress[:])
	data, err := rlp.EncodeToBytes(i)
	if err != nil {
		log.Crit("Failed to RLP encode ImpawnImpl", "err", err)
	}
	state.SetPOSState(preAddress, key, data)
	return nil
}
func (i *ImpawnImpl) Load(state StateDB, preAddress common.Address) error {
	key := common.BytesToHash(preAddress[:])
	data := state.GetPOSState(preAddress, key)
	if len(data) == 0 {
		return errors.New("Load data = 0")
	}
	var temp ImpawnImpl
	if err := rlp.DecodeBytes(data, &temp); err != nil {
		log.Error("Invalid ImpawnImpl entry RLP", "err", err)
		return errors.New(fmt.Sprintf("Invalid ImpawnImpl entry RLP %s", err.Error()))
	}
	i.curEpochID, i.accounts = temp.curEpochID, temp.accounts
	return nil
}
func (i *ImpawnImpl) commit() error {
	return nil
}

/////////////////////////////////////////////////////////////////////////////////
type valuesByHeight []*PairstakingValue

func (vs valuesByHeight) Len() int {
	return len(vs)
}
func (vs valuesByHeight) Less(i, j int) bool {
	return vs[i].height.Cmp(vs[j].height) == -1
}
func (vs valuesByHeight) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}
func (vs valuesByHeight) find(hh uint64) (*PairstakingValue, int) {
	low, height := 0, len(vs)-1
	mid := 0
	for low <= height {
		mid = (height + low) / 2
		if hh == vs[mid].height.Uint64() {
			return vs[mid], mid
		} else if hh > vs[mid].height.Uint64() {
			low = mid + 1
			if low > height {
				return nil, low
			}
		} else {
			height = mid - 1
		}
	}
	return nil, mid
}
func (vs valuesByHeight) update(val *PairstakingValue) valuesByHeight {
	item, pos := vs.find(val.height.Uint64())
	if item != nil {
		item.amount = item.amount.Add(item.amount, val.amount)
		item.state |= val.state
	} else {
		rear := append([]*PairstakingValue{}, vs[pos:]...)
		vs = append(append(vs[:pos], val), rear...)
	}
	return vs
}

type stakingItem struct {
	item   *StakingAccount
	height uint64
	valid  bool
}

func (s *stakingItem) getAll() *big.Int {
	if s.valid {
		return s.item.getValidStaking(s.height)
	} else {
		return s.item.getAllStaking(s.height)
	}
}

type stakingByHeight []*stakingItem

func toStakingByHeight(hh uint64, valid bool, items []*StakingAccount) stakingByHeight {
	var tmp []*stakingItem
	for _, v := range items {
		v.unit.sort()
		tmp = append(tmp, &stakingItem{
			item:   v,
			height: hh,
			valid:  valid,
		})
	}
	return stakingByHeight(tmp)
}
func fromStakingByHeight(items stakingByHeight) ([]*StakingAccount, uint64) {
	var tmp []*StakingAccount
	var vv uint64
	for _, v := range items {
		tmp = append(tmp, v.item)
		vv = v.height
	}
	return tmp, vv
}
func (vs stakingByHeight) Len() int {
	return len(vs)
}
func (vs stakingByHeight) Less(i, j int) bool {
	return vs[i].getAll().Cmp(vs[j].getAll()) == -1
}
func (vs stakingByHeight) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}

type delegationItem struct {
	item   *DelegationAccount
	height uint64
	valid  bool
}

func (d *delegationItem) getAll() *big.Int {
	if d.valid {
		return d.item.getValidStaking(d.height)
	} else {
		return d.item.getAllStaking(d.height)
	}
}

type delegationItemByHeight []*delegationItem

func toDelegationByHeight(hh uint64, valid bool, items []*DelegationAccount) delegationItemByHeight {
	var tmp []*delegationItem
	for _, v := range items {
		v.unit.sort()
		tmp = append(tmp, &delegationItem{
			item:   v,
			height: hh,
			valid:  valid,
		})
	}
	return delegationItemByHeight(tmp)
}
func fromDelegationByHeight(items delegationItemByHeight) ([]*DelegationAccount, uint64) {
	var tmp []*DelegationAccount
	var vv uint64
	for _, v := range items {
		tmp = append(tmp, v.item)
		vv = v.height
	}
	return tmp, vv
}
func (vs delegationItemByHeight) Len() int {
	return len(vs)
}
func (vs delegationItemByHeight) Less(i, j int) bool {
	return vs[i].getAll().Cmp(vs[j].getAll()) == -1
}
func (vs delegationItemByHeight) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}
