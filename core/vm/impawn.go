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
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rlp"
)

/////////////////////////////////////////////////////////////////////////////////
type PairstakingValue struct {
	amount *big.Int
	height *big.Int
	state  uint8
}

func (v *PairstakingValue) isElection() bool {
	return v.state&types.StateStakingCancel == 0 && (v.state&types.StateStakingOnce != 0 || v.state&types.StateStakingAuto != 0)
}

type RewardItem struct {
	Amount *big.Int
	Height *big.Int
}
type RedeemItem struct {
	Amount  *big.Int
	EpochID uint64
	State   uint8
}

func (r *RedeemItem) toHeight() *big.Int {
	e := types.GetEpochFromID(r.EpochID + 1)
	return new(big.Int).SetUint64(e.BeginHeight)
}
func (r *RedeemItem) fromHeight(hh *big.Int) {
	e := types.GetEpochFromHeight(hh.Uint64())
	if e != nil {
		r.EpochID = e.EpochID
	}
}
func (r *RedeemItem) update(o *RedeemItem) {
	if r.EpochID == o.EpochID {
		r.Amount = r.Amount.Add(r.Amount, o.Amount)
	}
}
func (r *RedeemItem) isRedeem(target uint64) bool {
	hh := r.toHeight().Uint64()
	return target > hh+params.MaxRedeemHeight
}
func newRedeemItem(eid uint64, amount *big.Int) *RedeemItem {
	return &RedeemItem{
		Amount:  new(big.Int).Set(amount),
		EpochID: eid,
	}
}

type impawnUnit struct {
	address    common.Address
	value      []*PairstakingValue // sort by height
	redeemInof []*RedeemItem
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
func (s *impawnUnit) getValidRedeem(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, v := range s.redeemInof {
		if v.isRedeem(hh) {
			all = all.Add(all, v.Amount)
		}
	}
	return all
}
func (s *impawnUnit) GetRewardAddress() common.Address {
	return s.address
}
func (s *impawnUnit) getRedeemItem(epochID uint64) *RedeemItem {
	for _, v := range s.redeemInof {
		if v.EpochID == epochID {
			return v
		}
	}
	return nil
}

// stopStakingInfo redeem delay in next epoch + MaxRedeemHeight
func (s *impawnUnit) stopStakingInfo(amount, lastHeight *big.Int) error {
	all := s.getValidStaking(lastHeight.Uint64())
	if all.Cmp(amount) < 0 {
		return types.ErrAmountOver
	}
	e := types.GetEpochFromHeight(lastHeight.Uint64())
	if e == nil {
		return types.ErrNotFoundEpoch
	}
	r := s.getRedeemItem(e.EpochID)
	tmp := &RedeemItem{
		Amount:  new(big.Int).Set(amount),
		EpochID: e.EpochID,
		State:   types.StateRedeem,
	}
	if r == nil {
		s.redeemInof = append(s.redeemInof, tmp)
	} else {
		r.update(tmp)
	}
	return nil
}
func (s *impawnUnit) redeeming(hh uint64, amount *big.Int) (common.Address, *big.Int, error) {
	if amount.Cmp(s.getValidRedeem(hh)) > 0 {
		return common.Address{}, nil, types.ErrAmountOver
	}
	allAmount := big.NewInt(0)
	s.sortRedeemItems()
	for _, v := range s.redeemInof {
		if v.isRedeem(hh) {
			allAmount = allAmount.Add(allAmount, v.Amount)
			res := allAmount.Cmp(amount)
			if res <= 0 {
				v.Amount, v.State = v.Amount.Sub(v.Amount, v.Amount), types.StateRedeemed
				if res == 0 {
					break
				}
			} else {
				v.State = types.StateRedeemed
				v.Amount.Set(new(big.Int).Sub(allAmount, amount))
				break
			}
		}
	}
	res := allAmount.Cmp(amount)
	if res >= 0 {
		return s.address, amount, nil
	} else {
		return s.address, allAmount, nil
	}
}

// called by user input and it will be execute without wait for the staking be rewarded
func (s *impawnUnit) finishRedeemed() {
	pos := -1
	for i, v := range s.redeemInof {
		if v.Amount.Sign() == 0 && v.State == types.StateRedeemed {
			pos = i
		}
	}
	if len(s.redeemInof)-1 == pos {
		s.redeemInof = s.redeemInof[0:0]
	} else {
		s.redeemInof = s.redeemInof[pos+1:]
	}
}

// sort the redeemInof by asc with epochid
func (s *impawnUnit) sortRedeemItems() {
	sort.Sort(redeemByID(s.redeemInof))
}

// merge for move from prev to next epoch,move the staking who was to be voted.
// merge all staking to one staking with the new height(the beginning of next epoch).
// it will remove the staking which was canceled in the prev epoch
// called by move function.
func (s *impawnUnit) merge(epochid, hh uint64) {
	redeem := s.getRedeemItem(epochid)
	if redeem == nil {
		return
	}
	all := big.NewInt(0)
	for _, v := range s.value {
		all = all.Add(all, v.amount)
	}
	var val []*PairstakingValue
	left := all.Sub(all, redeem.Amount)
	if left.Sign() > 0 {
		val = append(val, &PairstakingValue{
			amount: left,
			height: new(big.Int).SetUint64(hh),
			state:  types.StateStakingAuto,
		})
	}
	s.value = val
}
func (s *impawnUnit) update(unit *impawnUnit, move bool) {
	sorter := valuesByHeight(s.value)
	sort.Sort(sorter)
	for _, v := range unit.value {
		sorter = sorter.update(v)
	}
	s.value = sorter

	if s.redeemInof == nil {
		s.redeemInof = make([]*RedeemItem, 0)
	}
	if move {
		var tmp []*RedeemItem
		s.redeemInof = append(append(tmp, unit.redeemInof...), s.redeemInof...)
	}
}
func (s *impawnUnit) clone() *impawnUnit {
	tmp := &impawnUnit{
		address:    s.address,
		value:      make([]*PairstakingValue, 0),
		redeemInof: make([]*RedeemItem, 0),
	}
	for _, v := range s.value {
		tmp.value = append(tmp.value, &PairstakingValue{
			amount: new(big.Int).Set(v.amount),
			height: new(big.Int).Set(v.height),
			state:  v.state,
		})
	}
	for _, v := range s.redeemInof {
		tmp.redeemInof = append(tmp.redeemInof, &RedeemItem{
			Amount:  new(big.Int).Set(v.Amount),
			EpochID: v.EpochID,
			State:   v.State,
		})
	}
	return tmp
}
func (s *impawnUnit) sort() {
	sort.Sort(valuesByHeight(s.value))
	s.sortRedeemItems()
}
func (s *impawnUnit) isValid() bool {
	for _, v := range s.value {
		if v.amount.Sign() > 0 {
			return true
		}
	}
	for _, v := range s.redeemInof {
		if v.Amount.Sign() > 0 {
			return true
		}
	}
	return false
}
func (s *impawnUnit) valueToMap() map[uint64]*big.Int {
	res := make(map[uint64]*big.Int)
	for _, v := range s.value {
		res[v.height.Uint64()] = new(big.Int).Set(v.amount)
	}
	return res
}
func (s *impawnUnit) redeemToMap() map[uint64]*big.Int {
	res := make(map[uint64]*big.Int)
	for _, v := range s.redeemInof {
		res[v.EpochID] = new(big.Int).Set(v.Amount)
	}
	return res
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
func (s *DelegationAccount) stopStakingInfo(amount, lastHeight *big.Int) error {
	return s.unit.stopStakingInfo(amount, lastHeight)
}
func (s *DelegationAccount) redeeming(hh uint64, amount *big.Int) (common.Address, *big.Int, error) {
	return s.unit.redeeming(hh, amount)
}
func (s *DelegationAccount) finishRedeemed() {
	s.unit.finishRedeemed()
}
func (s *DelegationAccount) merge(epochid, hh uint64) {
	s.unit.merge(epochid, hh)
}
func (s *DelegationAccount) clone() *DelegationAccount {
	return &DelegationAccount{
		deleAddress: s.deleAddress,
		unit:        s.unit.clone(),
	}
}
func (s *DelegationAccount) isValid() bool {
	return s.unit.isValid()
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
	// ignore the pk param
	if hh > s.getMaxHeight() {
		s.modify.fee = new(big.Int).Set(sa.modify.fee)
		s.modify.votePubkey = types.CopyVotePk(s.votepubkey)
	}
	if next {
		s.changeAlterableInfo()
	}
	if dirty && hh != 0 {
		tmp := toDelegationByAmount(hh, false, s.delegation)
		sort.Sort(tmp)
		s.delegation, _ = fromDelegationByAmount(tmp)
	}
}
func (s *StakingAccount) stopStakingInfo(amount, lastHeight *big.Int) error {
	return s.unit.stopStakingInfo(amount, lastHeight)
}
func (s *StakingAccount) redeeming(hh uint64, amount *big.Int) (common.Address, *big.Int, error) {
	return s.unit.redeeming(hh, amount)
}
func (s *StakingAccount) finishRedeemed() {
	s.unit.finishRedeemed()
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
func (s *StakingAccount) merge(epochid, hh uint64) {
	s.unit.merge(epochid, hh)
	for _, v := range s.delegation {
		v.merge(epochid, hh)
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
		votepubkey: types.CopyVotePk(s.votepubkey),
		unit:       s.unit.clone(),
		fee:        new(big.Int).Set(s.fee),
		committee:  s.committee,
		delegation: make([]*DelegationAccount, 0),
		modify:     &AlterableInfo{},
	}
	for _, v := range s.delegation {
		ss.delegation = append(ss.delegation, v.clone())
	}
	if s.modify != nil {
		if s.modify.fee != nil {
			ss.modify.fee = new(big.Int).Set(s.modify.fee)
		}
		if s.modify.votePubkey != nil {
			ss.modify.votePubkey = types.CopyVotePk(s.modify.votePubkey)
		}
	}
	return ss
}
func (s *StakingAccount) isvalid() bool {
	for _, v := range s.delegation {
		if v.isValid() {
			return true
		}
	}
	return s.unit.isValid()
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
		tmp := toDelegationByAmount(hh, valid, v.delegation)
		sort.Sort(tmp)
		v.delegation, _ = fromDelegationByAmount(tmp)
	}
	tmp := toStakingByAmount(hh, valid, *s)
	sort.Sort(tmp)
	*s, _ = fromStakingByAmount(tmp)
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
		curEpochID: params.FirstNewEpochID - 1,
		lastReward: 0,
		accounts:   make(map[uint64]SAImpawns),
	}
}

/////////////////////////////////////////////////////////////////////////////////
///////////  auxiliary function ////////////////////////////////////////////
func (i *ImpawnImpl) getCurrentEpoch() uint64 {
	return i.curEpochID
}
func (i *ImpawnImpl) SetCurrentEpoch(eid uint64) {
	i.curEpochID = eid
}
func (i *ImpawnImpl) getMinEpochID() uint64 {
	eid := i.curEpochID
	for k, _ := range i.accounts {
		if eid > k {
			eid = k
		}
	}
	return eid
}
func (i *ImpawnImpl) isInCurrentEpoch(hh uint64) bool {
	return i.curEpochID == types.GetEpochFromHeight(hh).EpochID
}
func (i *ImpawnImpl) getCurrentEpochInfo() []*types.EpochIDInfo {
	var epochs []*types.EpochIDInfo
	var eids []float64
	for k, _ := range i.accounts {
		eids = append(eids, float64(k))
	}
	sort.Float64s(eids)
	for _, v := range eids {
		e := types.GetEpochFromID(uint64(v))
		if e != nil {
			epochs = append(epochs, e)
		}
	}
	return epochs
}
func (i *ImpawnImpl) GetStakingAccount(epochid uint64, addr common.Address) (*StakingAccount, error) {
	if v, ok := i.accounts[epochid]; !ok {
		return nil, types.ErrInvalidStaking
	} else {
		for _, val := range v {
			if bytes.Equal(val.unit.address.Bytes(), addr.Bytes()) {
				return val, nil
			}
		}
	}
	return nil, types.ErrInvalidStaking
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
func (i *ImpawnImpl) getElections2(epochid uint64) []*StakingAccount {
	if accounts, ok := i.accounts[epochid]; !ok {
		return nil
	} else {
		var sas []*StakingAccount
		for _, v := range accounts {
			if v.isInCommittee() {
				sas = append(sas, v)
			}
		}
		return sas
	}
}
func (i *ImpawnImpl) getElections3(epochid uint64) []*StakingAccount {
	eid := epochid
	if eid >= params.FirstNewEpochID {
		eid = eid - 1
	}
	return i.getElections2(eid)
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
func (i *ImpawnImpl) redeemBySa(sa *StakingAccount, height uint64, amount *big.Int) error {
	// can be redeem in the SA
	addr, all, err1 := sa.redeeming(height, amount)
	if err1 != nil {
		return err1
	}
	if all.Cmp(amount) != 0 {
		return errors.New(fmt.Sprint(types.ErrRedeemAmount, "request amount", amount, "redeem amount", all))
	}
	sa.finishRedeemed()
	fmt.Println("SA redeemed amount:[", all.String(), "],addr:[", addr.String())
	return nil
}
func (i *ImpawnImpl) redeemByDa(da *DelegationAccount, height uint64, amount *big.Int) error {
	// can be redeem in the DA
	addr, all, err1 := da.redeeming(height, amount)
	if err1 != nil {
		return err1
	}
	if all.Cmp(amount) != 0 {
		return errors.New(fmt.Sprint(types.ErrRedeemAmount, "request amount", amount, "redeem amount", all))
	}
	da.finishRedeemed()
	fmt.Println("DA redeemed amount:[", all.String(), "],addr:[", addr.String())
	return nil
}
func (i *ImpawnImpl) calcRewardInSa(target uint64, sa *StakingAccount, allReward, allStaking *big.Int, item *types.RewardInfo) ([]*types.RewardInfo, error) {
	if sa == nil || allReward == nil || item == nil || allStaking == nil {
		return nil, types.ErrInvalidParam
	}
	var items []*types.RewardInfo
	fee := new(big.Int).Quo(new(big.Int).Mul(allReward, sa.fee), types.Base)
	all, left := new(big.Int).Sub(allReward, fee), big.NewInt(0)
	for _, v := range sa.delegation {
		daAll := v.getAllStaking(target)
		if daAll.Sign() <= 0 {
			continue
		}
		v1 := new(big.Int).Quo(new(big.Int).Mul(all, daAll), allStaking)
		left = left.Add(left, v1)
		var ii types.RewardInfo
		ii.Address, ii.Amount = v.unit.GetRewardAddress(), new(big.Int).Set(v1)
		items = append(items, &ii)
	}
	item.Amount = new(big.Int).Add(new(big.Int).Sub(all, left), fee)
	return items, nil
}
func (i *ImpawnImpl) calcReward(target uint64, allAmount *big.Int, einfo *types.EpochIDInfo) ([]*types.SARewardInfos, error) {
	if _, ok := i.accounts[einfo.EpochID]; !ok {
		return nil, types.ErrInvalidParam
	} else {
		das := i.getElections3(einfo.EpochID)
		if das == nil {
			return nil, errors.New(fmt.Sprint(types.ErrMatchEpochID, "epochid:", einfo.EpochID))
		}
		impawns := SAImpawns(das)
		var res []*types.SARewardInfos
		allValidatorStaking := impawns.getAllStaking(target)

		for _, v := range impawns {
			var info types.SARewardInfos
			var item types.RewardInfo
			item.Address = v.unit.GetRewardAddress()
			allStaking := v.getAllStaking(target)
			if allStaking.Sign() <= 0 {
				continue
			}

			v2 := new(big.Int).Quo(new(big.Int).Mul(allStaking, allAmount), allValidatorStaking)

			if ii, err := i.calcRewardInSa(target, v, v2, allStaking, &item); err != nil {
				return nil, err
			} else {
				info.Items = append(info.Items, &item)
				info.Items = append(info.Items, ii[:]...)
			}
			res = append(res, &info)
		}
		return res, nil
	}
}
func (i *ImpawnImpl) reward(begin, end uint64, allAmount *big.Int) ([]*types.SARewardInfos, error) {
	ids := types.GetEpochFromRange(begin, end)
	if ids == nil || len(ids) > 2 {
		return nil, errors.New(fmt.Sprint(types.ErrMatchEpochID, "more than 2 epochid:", begin, end))
	}

	if len(ids) == 2 {
		tmp := new(big.Int).Quo(new(big.Int).Mul(allAmount, new(big.Int).SetUint64(ids[0].EndHeight-begin)), new(big.Int).SetUint64(end-begin))
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

///////////auxiliary function ////////////////////////////////////////////

/////////////////////////////////////////////////////////////////////////////////
// move the accounts from prev to next epoch and keeps the prev account still here
func (i *ImpawnImpl) move(prev, next uint64) error {
	nextEpoch := types.GetEpochFromID(next)
	if nextEpoch == nil {
		return types.ErrOverEpochID
	}
	prevInfos, ok := i.accounts[prev]
	nextInfos, ok2 := i.accounts[next]
	if !ok {
		return errors.New(fmt.Sprintln("the epoch is nil", prev, "err:", types.ErrNotMatchEpochInfo))
	}
	if !ok2 {
		nextInfos = SAImpawns{}
	}
	for _, v := range prevInfos {
		vv := v.clone()
		vv.merge(prev, nextEpoch.BeginHeight)
		if vv.isvalid() {
			vv.committee = false
			nextInfos.update(vv, nextEpoch.BeginHeight, true, true)
		}
	}
	i.accounts[next] = nextInfos
	return nil
}

/////////////////////////////////////////////////////////////////////////////////
////////////// external function //////////////////////////////////////////

// DoElections called by consensus while it closer the end of epoch,have 500~1000 fast block
func (i *ImpawnImpl) DoElections(epochid, height uint64) ([]*StakingAccount, error) {
	if epochid < params.FirstNewEpochID && epochid != i.getCurrentEpoch()+1 {
		return nil, types.ErrOverEpochID
	}
	if types.DposForkPoint != 0 || height > 0 {
		cur := types.GetEpochFromID(i.curEpochID)
		if cur.EndHeight != height+params.ElectionPoint {
			return nil, types.ErrNotElectionTime
		}
	}
	e := types.GetEpochFromID(epochid)
	eid := epochid
	if eid >= params.FirstNewEpochID {
		eid = eid - 1
	}
	if val, ok := i.accounts[eid]; ok {
		val.sort(e.EndHeight, true)
		var ee []*StakingAccount
		for i, v := range val {
			v.committee = true
			ee = append(ee, v)
			if i == params.CountInEpoch-1 {
				break
			}
		}
		return ee, nil
	} else {
		return nil, types.ErrMatchEpochID
	}
}

// Shift will move the staking account which has election flag to the next epoch
// it will be save the whole state in the current epoch end block after it called by consensus
func (i *ImpawnImpl) Shift(epochid uint64) error {
	minEpoch := types.GetEpochFromHeight(i.lastReward)
	min := i.getMinEpochID()
	fmt.Println("*** move min:", min, "minEpoch:", minEpoch.EpochID, "lastReward:", i.lastReward)
	if minEpoch != nil && min >= 0 && minEpoch.EpochID-1 > min {
		for ii := min; minEpoch.EpochID > 1 && ii < minEpoch.EpochID-1; ii++ {
			delete(i.accounts, ii)
			fmt.Println("delete epoch:", ii)
		}
	}

	if epochid != i.getCurrentEpoch()+1 {
		return types.ErrOverEpochID
	}
	i.SetCurrentEpoch(epochid)
	prev := epochid - 1
	return i.move(prev, epochid)
}

// CancelSAccount cancel amount of asset for staking account,it will be work in next epoch
func (i *ImpawnImpl) CancelSAccount(curHeight uint64, addr common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addr)
	if err != nil {
		return err
	}
	err2 := sa.stopStakingInfo(amount, new(big.Int).SetUint64(curHeight))
	fmt.Println("[SA]insert a redeem,address:[", addr.String(), "],amount:[", amount.String(), "],height:", curHeight, "]err:", err2)
	return err2
}

// CancelDAccount cancel amount of asset for delegation account,it will be work in next epoch
func (i *ImpawnImpl) CancelDAccount(curHeight uint64, addrSA, addrDA common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addrSA)
	if err != nil {
		return err
	}
	da, err2 := i.getDAfromSA(sa, addrDA)
	if err2 != nil {
		return err
	}
	err3 := da.stopStakingInfo(amount, new(big.Int).SetUint64(curHeight))
	fmt.Println("[DA]insert a redeem,address:[", addrSA.String(), "],DA address:[", addrDA.String(), "],amount:[", amount.String(), "],height:", curHeight, "]err:", err3)
	return err3
}

// RedeemSAccount redeem amount of asset for staking account,it will locked for a certain time
func (i *ImpawnImpl) RedeemSAccount(curHeight uint64, addr common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addr)
	if err != nil {
		return err
	}
	return i.redeemBySa(sa, curHeight, amount)
}

// RedeemDAccount redeem amount of asset for delegation account,it will locked for a certain time
func (i *ImpawnImpl) RedeemDAccount(curHeight uint64, addrSA, addrDA common.Address, amount *big.Int) error {
	if amount.Sign() <= 0 || curHeight <= 0 {
		return types.ErrInvalidParam
	}
	curEpoch := types.GetEpochFromHeight(curHeight)
	if curEpoch == nil || curEpoch.EpochID != i.curEpochID {
		return types.ErrInvalidParam
	}
	sa, err := i.GetStakingAccount(curEpoch.EpochID, addrSA)
	if err != nil {
		return err
	}
	da, err2 := i.getDAfromSA(sa, addrDA)
	if err2 != nil {
		return err
	}
	return i.redeemByDa(da, curHeight, amount)
}
func (i *ImpawnImpl) insertDAccount(height uint64, da *DelegationAccount) error {
	if da == nil {
		return types.ErrInvalidParam
	}
	epochInfo := types.GetEpochFromHeight(height)
	if epochInfo == nil || epochInfo.EpochID > i.getCurrentEpoch() {
		return types.ErrOverEpochID
	}
	sa, err := i.GetStakingAccount(epochInfo.EpochID, da.deleAddress)
	if err != nil {
		return err
	}
	if ds, err := i.getDAfromSA(sa, da.unit.address); err != nil {
		return err
	} else {
		if ds == nil {
			sa.delegation = append(sa.delegation, da)
			log.Info("Insert delegation account", "staking account", sa.unit.GetRewardAddress(), "account", da.unit.GetRewardAddress())
		} else {
			ds.update(da, false)
			log.Info("Update delegation account", "staking account", sa.unit.GetRewardAddress(), "account", da.unit.GetRewardAddress())
		}
	}
	return nil
}
func (i *ImpawnImpl) InsertDAccount2(height uint64, addrSA, addrDA common.Address, val *big.Int) error {
	if val.Sign() <= 0 || height < 0 {
		return types.ErrInvalidParam
	}
	if bytes.Equal(addrSA.Bytes(), addrDA.Bytes()) {
		return types.ErrDelegationSelf
	}
	state := uint8(0)
	state |= types.StateStakingAuto
	da := &DelegationAccount{
		deleAddress: addrSA,
		unit: &impawnUnit{
			address: addrDA,
			value: []*PairstakingValue{&PairstakingValue{
				amount: new(big.Int).Set(val),
				height: new(big.Int).SetUint64(height),
				state:  state,
			}},
			redeemInof: make([]*RedeemItem, 0),
		},
	}
	return i.insertDAccount(height, da)
}
func (i *ImpawnImpl) insertSAccount(height uint64, sa *StakingAccount) error {
	if sa == nil {
		return types.ErrInvalidParam
	}
	epochInfo := types.GetEpochFromHeight(height)
	if epochInfo == nil || epochInfo.EpochID > i.getCurrentEpoch() {
		return types.ErrOverEpochID
	}
	if val, ok := i.accounts[epochInfo.EpochID]; !ok {
		var accounts []*StakingAccount
		accounts = append(accounts, sa)
		i.accounts[epochInfo.EpochID] = SAImpawns(accounts)
		log.Info("Insert staking account", "account", sa.unit.GetRewardAddress())
	} else {
		for _, ii := range val {
			if bytes.Equal(ii.unit.address.Bytes(), sa.unit.address.Bytes()) {
				ii.update(sa, height, false, false)
				log.Info("Update staking account", "account", sa.unit.GetRewardAddress())
				return nil
			}
		}
		i.accounts[epochInfo.EpochID] = append(val, sa)
		log.Info("Insert staking account", "account", sa.unit.GetRewardAddress())
	}
	return nil
}
func (i *ImpawnImpl) InsertSAccount2(height uint64, addr common.Address, pk []byte, val *big.Int, fee *big.Int, auto bool) error {
	if val.Sign() <= 0 || height < 0 {
		return types.ErrInvalidParam
	}
	if err := types.ValidPk(pk); err != nil {
		return err
	}
	state := uint8(0)
	if auto {
		state |= types.StateStakingAuto
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
			redeemInof: make([]*RedeemItem, 0),
		},
		modify: &AlterableInfo{},
	}
	return i.insertSAccount(height, sa)
}

func (i *ImpawnImpl) Reward(block *types.SnailBlock, allAmount *big.Int) ([]*types.SARewardInfos, error) {
	begin, end := types.FromBlock(block)
	res, err := i.reward(begin, end, allAmount)
	if err == nil {
		i.lastReward = end
	}
	return res, err
}

/////////////////////////////////////////////////////////////////////////////////
// GetStakings return all staking accounts of the current epoch
func (i *ImpawnImpl) GetAllStakingAccount() SAImpawns {
	if val, ok := i.accounts[i.curEpochID]; ok {
		return val
	} else {
		return nil
	}
}

// GetStakingAccount2 returns a map for all staking amount of the address, the key is the SA address
func (i *ImpawnImpl) GetStakingAsset(addr common.Address) map[common.Address]*types.StakingValue {
	epochid := i.curEpochID
	return i.getAsset(addr, epochid, false)
}
func (i *ImpawnImpl) GetLockedAsset(addr common.Address) map[common.Address]*types.StakingValue {
	epochid := i.curEpochID
	return i.getAsset(addr, epochid, true)
}

func (i *ImpawnImpl) getAsset(addr common.Address, epoch uint64, locked bool) map[common.Address]*types.StakingValue {
	epochid := epoch
	if val, ok := i.accounts[epochid]; ok {
		res := make(map[common.Address]*types.StakingValue)
		for _, v := range val {
			if bytes.Equal(v.unit.address.Bytes(), addr.Bytes()) {
				if _, ok := res[addr]; !ok {
					if locked {
						res[addr] = &types.StakingValue{
							Value: v.unit.redeemToMap(),
						}
					} else {
						res[addr] = &types.StakingValue{
							Value: v.unit.valueToMap(),
						}
					}

				} else {
					log.Error("getAsset", "repeat staking account", addr, "epochid", epochid, "locked", locked)
				}
				continue
			} else {
				for _, vv := range v.delegation {
					if bytes.Equal(vv.unit.address.Bytes(), addr.Bytes()) {
						if _, ok := res[v.unit.address]; !ok {
							if locked {
								res[v.unit.address] = &types.StakingValue{
									Value: vv.unit.redeemToMap(),
								}
							} else {
								res[v.unit.address] = &types.StakingValue{
									Value: vv.unit.valueToMap(),
								}
							}

						} else {
							log.Error("getAsset", "repeat delegation account[sa,da]", v.unit.address, addr, "epochid", epochid, "locked", locked)
						}
						break
					}
				}
			}
		}
		return res
	} else {
		log.Error("getAsset", "wrong epoch in current", epochid)
	}
	return nil
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
	return err
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
	i.curEpochID, i.accounts, i.lastReward = temp.curEpochID, temp.accounts, temp.lastReward
	return nil
}

func GetCurrentValidators(state StateDB) []*types.CommitteeMember {
	i := NewImpawnImpl()
	i.Load(state, StakingAddress)
	eid := i.getCurrentEpoch()
	accs := i.getElections3(eid)
	var vv []*types.CommitteeMember
	for _, v := range accs {
		pubkey, _ := crypto.UnmarshalPubkey(v.votepubkey)
		vv = append(vv, &types.CommitteeMember{
			CommitteeBase: crypto.PubkeyToAddress(*pubkey),
			Coinbase:      v.unit.GetRewardAddress(),
			Publickey:     types.CopyVotePk(v.votepubkey),
			Flag:          types.StateUsedFlag,
			MType:         types.TypeWorked,
		})
	}
	return vv
}

func GetValidatorsByEpoch(state StateDB, eid, hh uint64) []*types.CommitteeMember {
	i := NewImpawnImpl()
	err := i.Load(state, StakingAddress)
	accs := i.getElections3(eid)
	first := types.GetFirstEpoch()
	if hh == first.EndHeight-params.ElectionPoint {
		fmt.Println("****** accounts len:", len(i.accounts), "election:", len(accs), " err ", err)
	}
	var vv []*types.CommitteeMember
	for _, v := range accs {
		pubkey, _ := crypto.UnmarshalPubkey(v.votepubkey)
		vv = append(vv, &types.CommitteeMember{
			CommitteeBase: crypto.PubkeyToAddress(*pubkey),
			Coinbase:      v.unit.GetRewardAddress(),
			Publickey:     types.CopyVotePk(v.votepubkey),
			Flag:          types.StateUsedFlag,
			MType:         types.TypeWorked,
		})
	}
	return vv
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

type redeemByID []*RedeemItem

func (vs redeemByID) Len() int {
	return len(vs)
}
func (vs redeemByID) Less(i, j int) bool {
	return vs[i].EpochID < vs[j].EpochID
}
func (vs redeemByID) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
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

type stakingByAmount []*stakingItem

func toStakingByAmount(hh uint64, valid bool, items []*StakingAccount) stakingByAmount {
	var tmp []*stakingItem
	for _, v := range items {
		v.unit.sort()
		tmp = append(tmp, &stakingItem{
			item:   v,
			height: hh,
			valid:  valid,
		})
	}
	return stakingByAmount(tmp)
}
func fromStakingByAmount(items stakingByAmount) ([]*StakingAccount, uint64) {
	var tmp []*StakingAccount
	var vv uint64
	for _, v := range items {
		tmp = append(tmp, v.item)
		vv = v.height
	}
	return tmp, vv
}
func (vs stakingByAmount) Len() int {
	return len(vs)
}
func (vs stakingByAmount) Less(i, j int) bool {
	return vs[i].getAll().Cmp(vs[j].getAll()) > 0
}
func (vs stakingByAmount) Swap(i, j int) {
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

type delegationItemByAmount []*delegationItem

func toDelegationByAmount(hh uint64, valid bool, items []*DelegationAccount) delegationItemByAmount {
	var tmp []*delegationItem
	for _, v := range items {
		v.unit.sort()
		tmp = append(tmp, &delegationItem{
			item:   v,
			height: hh,
			valid:  valid,
		})
	}
	return delegationItemByAmount(tmp)
}
func fromDelegationByAmount(items delegationItemByAmount) ([]*DelegationAccount, uint64) {
	var tmp []*DelegationAccount
	var vv uint64
	for _, v := range items {
		tmp = append(tmp, v.item)
		vv = v.height
	}
	return tmp, vv
}
func (vs delegationItemByAmount) Len() int {
	return len(vs)
}
func (vs delegationItemByAmount) Less(i, j int) bool {
	return vs[i].getAll().Cmp(vs[j].getAll()) > 0
}
func (vs delegationItemByAmount) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}
