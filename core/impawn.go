package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/truechain/truechain-engineering-code/core/vm"

	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/types"
)

var (
	baseUnit        = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fbaseUnit       = new(big.Float).SetFloat64(float64(baseUnit.Int64()))
	mixImpawn       = new(big.Int).Mul(big.NewInt(1000), baseUnit)
	CountInEpoch    = 31
	MaxRedeemHeight = 1000
	mixEpochCount   = 2
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
	StateStakingAuto uint8 = 1 << iota
	StateRedeem
	StateRedeeming
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

func (e *EpochIDInfo) IsValid() bool {
	if e.EpochID < 0 {
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

/////////////////////////////////////////////////////////////////////////////////
type PairstakingValue struct {
	amount *big.Int
	height *big.Int
	state  uint8
}
type RewardItem struct {
	Amount *big.Int
	Height *big.Int
}
type RedeemItem struct {
	Amount *big.Int
	Height *big.Int
	State  uint8
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
func (s *impawnUnit) GetRewardAddress() common.Address {
	return s.address
}
func (s *impawnUnit) insertRedeemInfo(amount, lastHeight *big.Int) {
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
		v.state &= (^StateStakingAuto)
		if res <= 0 {
			break
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
			if pos > 0 {
				pos--
			}
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
func (s *impawnUnit) merge(hh uint64) {
	all := big.NewInt(0)
	redeem := false
	state := uint8(0)
	for _, v := range s.value {
		all = all.Add(all, v.amount)
		redeem = (v.state&StateStakingAuto != 0)
	}
	if redeem && all.Cmp(s.redeemInof.Amount) > 0 {
		state |= StateStakingAuto
	}
	var val []*PairstakingValue
	val = append(val, &PairstakingValue{
		amount: all,
		height: new(big.Int).SetUint64(hh),
		state:  state,
	})
	s.value = val
}
func (s *impawnUnit) update(unit *impawnUnit) {

}
func (s *impawnUnit) sort() {
	sort.Sort(valuesByHeight(s.value))
}

/////////////////////////////////////////////////////////////////////////////////

type DelegationAccount struct {
	deleAddress common.Address
	unit        *impawnUnit
}

func (d *DelegationAccount) update(da *DelegationAccount) {

}
func (s *DelegationAccount) getAllStaking(hh uint64) *big.Int {
	return s.unit.getAllStaking(hh)
}
func (s *DelegationAccount) insertRedeemInfo(amount, lastHeight *big.Int) {
	s.unit.insertRedeemInfo(amount, lastHeight)
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

type StakingAccount struct {
	unit       *impawnUnit
	votepubkey []byte
	fee        *big.Float
	committee  bool
	delegation []*DelegationAccount
}

func (s *StakingAccount) isInCommittee() bool {
	return s.committee
}
func (s *StakingAccount) update(sa *StakingAccount) {
}
func (s *StakingAccount) insertRedeemInfo(amount, lastHeight *big.Int) {
	s.unit.insertRedeemInfo(amount, lastHeight)
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
func (s *StakingAccount) merge(hh uint64) {
	s.unit.merge(hh)
	for _, v := range s.delegation {
		v.merge(hh)
	}
}

type SAImpawns []*StakingAccount

func (s *SAImpawns) getAllStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, val := range *s {
		all = all.Add(all, val.getAllStaking(hh))
	}
	return all
}
func (s *SAImpawns) sort(hh uint64) {
	for _, v := range *s {
		tmp := toDelegationByHeight(hh, v.delegation)
		sort.Sort(tmp)
		v.delegation, _ = fromDelegationByHeight(tmp)
	}
	tmp := toStakingByHeight(hh, *s)
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
func (s *SAImpawns) update(sa1 *StakingAccount, hh uint64) {
	sa := s.getSA(sa1.unit.address)
	if sa == nil {
		*s = append(*s, sa1)
		s.sort(hh)
	} else {
		sa.update(sa1)
	}
}

/////////////////////////////////////////////////////////////////////////////////

// be thread-safe for caller locked
type impawnImpl struct {
	lock      sync.RWMutex
	epochInfo []*EpochIDInfo // sort by epoch id
	accounts  map[uint64]SAImpawns
}

func (i *impawnImpl) getCurrentEpoch() uint64 {
	pos := len(i.epochInfo)
	if pos > 0 {
		return i.epochInfo[pos].EpochID
	}
	return 0
}
func (i *impawnImpl) getEpochInfo(epochid uint64) (*EpochIDInfo, error) {
	for _, v := range i.epochInfo {
		if v.EpochID == epochid {
			return v, nil
		}
	}
	return nil, errOverEpochID
}
func (i *impawnImpl) getEpochFromHeight(hh uint64) *EpochIDInfo {
	for _, v := range i.epochInfo {
		if v.BeginHeight <= hh && hh <= v.EndHeight {
			return v
		}
	}
	return nil
}
func (i *impawnImpl) getEpochIDFromHeight(b, e uint64) []*EpochIDInfo {
	var ids []*EpochIDInfo
	if e == 0 || b > e {
		return ids
	}
	for _, v := range i.epochInfo {
		if b >= v.BeginHeight {
			if e <= v.EndHeight {
				ids = append(ids, v)
				break
			} else {
				b = v.EndHeight
			}
		}
	}
	return ids
}
func (i *impawnImpl) getStakingAccount(epochid uint64, addr common.Address) (*StakingAccount, error) {
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
func (i *impawnImpl) getDAfromSA(sa *StakingAccount, addr common.Address) (*DelegationAccount, error) {
	for _, ii := range sa.delegation {
		if bytes.Equal(ii.unit.address.Bytes(), addr.Bytes()) {
			return ii, nil
		}
	}
	return nil, nil
}
func (i *impawnImpl) getElections(epochid uint64) []common.Address {
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
func (i *impawnImpl) fetchAccountsInEpoch(epochid uint64, addrs []common.Address) SAImpawns {
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
func (i *impawnImpl) redeemPrincipal(addr common.Address, amount *big.Int) error {
	return nil
}

// can be redeem in the SA
func (i *impawnImpl) redeemBySa(sa *StakingAccount, height, epochEnd uint64) {
	if height > epochEnd+uint64(MaxRedeemHeight) {
		addr, all := sa.redeeming()
		err := i.redeemPrincipal(addr, all)
		if err == nil {
			sa.clearRedeemed(all)
		}
		fmt.Println("SA redeemed amount:[", all.String(), "],addr:[", addr.String(), "],err:", err)
	}
}

// can be redeem in the DA
func (i *impawnImpl) redeemByDa2(sa *StakingAccount, addr common.Address, height, epochEnd uint64) {
	da, err := i.getDAfromSA(sa, addr)
	if err == nil && height > epochEnd+uint64(MaxRedeemHeight) {
		addr, all := da.redeeming()
		err := i.redeemPrincipal(addr, all)
		if err == nil {
			da.clearRedeemed(all)
		}
		fmt.Println("DA redeemed amount:[", all.String(), "],addr:[", addr.String(), "],err:", err)
	}
}

// can be redeem in the DA
func (i *impawnImpl) redeemByDa(da *DelegationAccount, height, epochEnd uint64) {
	if height > epochEnd+uint64(MaxRedeemHeight) {
		addr, all := da.redeeming()
		err := i.redeemPrincipal(addr, all)
		if err == nil {
			da.clearRedeemed(all)
		}
		fmt.Println("DA redeemed amount:[", all.String(), "],addr:[", addr.String(), "],err:", err)
	}
}

/////////////////////////////////////////////////////////////////////////////////
// 1. keep the minimum epoch count
// 2. release the item of redeemed
func (i *impawnImpl) shuffle() {
	min, max := i.epochInfo[0].EpochID, i.epochInfo[len(i.epochInfo)-1].EpochID
	if max-min < uint64(mixEpochCount) {
		return
	}
	for _, epoch := range i.epochInfo {
		if max-epoch.EpochID > uint64(mixEpochCount) && mixEpochCount >= 2 {
			i.move(epoch.EpochID, epoch.EpochID+1)
		}
	}
}
func (i *impawnImpl) move(prev, next uint64) error {
	nextEpoch, err := i.getEpochInfo(next)
	if err != nil {
		return err
	}
	prevInfos, ok := i.accounts[prev]
	nextInfos, ok2 := i.accounts[next]
	if !ok {
		return errors.New(fmt.Sprintln("the epoch is nil", prev, "err:", errNotMatchEpochInfo))
	}
	if !ok2 {
		return errors.New(fmt.Sprintln("the epoch is nil", next, "err:", errNotMatchEpochInfo))
	}
	for _, v := range prevInfos {
		v.merge(nextEpoch.BeginHeight)
		nextInfos.update(v, nextEpoch.BeginHeight)
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////////
func (i *impawnImpl) InsertDAccount(epochID uint64, da *DelegationAccount) error {
	if da == nil {
		return errInvalidParam
	}
	if epochID > i.getCurrentEpoch() {
		return errOverEpochID
	}
	sa, err := i.getStakingAccount(epochID, da.deleAddress)
	if err != nil {
		return err
	}
	if da, err := i.getDAfromSA(sa, da.unit.address); err != nil {
		return err
	} else {
		if da == nil {
			sa.delegation = append(sa.delegation, da)
		} else {
			da.update(da)
		}
	}
	return nil
}
func (i *impawnImpl) InsertSAccount(epochID uint64, sa *StakingAccount) error {
	if sa == nil {
		return errInvalidParam
	}
	if epochID > i.getCurrentEpoch() {
		return errOverEpochID
	}
	if val, ok := i.accounts[epochID]; !ok {
		var accounts []*StakingAccount
		accounts = append(accounts, sa)
		i.accounts[epochID] = SAImpawns(accounts)
	} else {
		for _, ii := range val {
			if bytes.Equal(ii.unit.address.Bytes(), sa.unit.address.Bytes()) {
				ii.update(sa)
				return nil
			}
		}
		val = append(val, sa)
	}
	return nil
}
func (i *impawnImpl) calcrewardInSa(target uint64, sa *StakingAccount, allReward, allStaking *big.Float, item *RewardInfo) ([]*RewardInfo, error) {
	if sa == nil || allReward == nil || item == nil || allStaking == nil {
		return nil, errInvalidParam
	}
	var items []*RewardInfo
	fee := new(big.Float).Mul(allReward, sa.fee)
	all, left := new(big.Float).Sub(allReward, fee), big.NewFloat(0)
	for _, v := range sa.delegation {
		daAll := new(big.Float).Quo(new(big.Float).SetFloat64(float64(v.getAllStaking(target).Int64())), fbaseUnit)
		rate := new(big.Float).Quo(daAll, allStaking)
		f1 := new(big.Float).Mul(all, rate)
		left = left.Add(left, f1)
		var ii RewardInfo
		ii.Address, ii.Amount = v.unit.GetRewardAddress(), toReward(f1)
		items = append(items, &ii)
	}
	f2 := new(big.Float).Add(fee, new(big.Float).Sub(all, left))
	item.Amount = toReward(f2)
	return items, nil
}
func (i *impawnImpl) calcReward(target uint64, allAmount *big.Int, einfo *EpochIDInfo) ([]*SARewardInfos, error) {
	if _, ok := i.accounts[einfo.EpochID]; !ok {
		return nil, errInvalidParam
	} else {
		addrs := i.getElections(einfo.EpochID)
		impawns := i.fetchAccountsInEpoch(einfo.EpochID, addrs)
		if impawns == nil || len(addrs) == 0 {
			return nil, errNullImpawnInEpoch
		}
		var res []*SARewardInfos
		all := impawns.getAllStaking(target)
		allf := new(big.Float).Quo(new(big.Float).SetFloat64(float64(all.Int64())), fbaseUnit)
		amount := new(big.Float).Quo(new(big.Float).SetFloat64(float64(allAmount.Int64())), fbaseUnit)
		left := big.NewFloat(0)
		for index, v := range impawns {
			var info SARewardInfos
			var item RewardInfo
			item.Address = v.unit.GetRewardAddress()
			allStaking := v.getAllStaking(target)
			f1 := new(big.Float).Quo(new(big.Float).SetFloat64(float64(allStaking.Int64())), fbaseUnit)
			rate := new(big.Float).Quo(f1, allf)
			v1 := new(big.Float).Mul(amount, rate)
			left = left.Add(left, v1)
			if index == len(impawns)-1 {
				v1 = v1.Sub(amount, left)
			}
			if ii, err := i.calcrewardInSa(target, v, v1, f1, &item); err != nil {
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
func (i *impawnImpl) Reward(block *types.SnailBlock, allAmount *big.Int) ([]*SARewardInfos, error) {
	begin, end := fromBlock(block)
	ids := i.getEpochIDFromHeight(begin, end)
	if len(ids) == 0 || len(ids) > 2 {
		return nil, errMatchEpochID
	}

	if len(ids) == 2 {
		a1 := new(big.Float).Quo(new(big.Float).SetFloat64(float64(allAmount.Int64())), fbaseUnit)
		r := float64(ids[0].EndHeight-ids[0].BeginHeight) / float64(end-begin)
		tmp := new(big.Float).Mul(a1, new(big.Float).SetFloat64(r))
		amount1, amount2 := toReward(tmp), toReward(new(big.Float).Quo(a1, tmp))
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
// Keep the epoch info is sequential
func (i *impawnImpl) SetEpochID(info *EpochIDInfo) error {
	if info == nil {
		return errInvalidParam
	}
	if !info.IsValid() {
		return errInvalidEpochInfo
	}
	for _, v := range i.epochInfo {
		if info.EpochID == v.EpochID {
			return nil
		}
	}
	pos := len(i.epochInfo)
	if pos > 0 {
		prev := i.epochInfo[pos-0]
		if !(prev.EpochID+1 == info.EpochID && prev.EndHeight+1 == info.BeginHeight) {
			return errNotSequential
		}
	}
	i.epochInfo = append(i.epochInfo, info)
	return nil
}
func (i *impawnImpl) DoElections(epochid, begin, end uint64) ([]*StakingAccount, error) {
	if err := i.SetEpochID(&EpochIDInfo{
		EpochID:     epochid,
		BeginHeight: begin,
		EndHeight:   end,
	}); err == nil {
		if val, ok := i.accounts[epochid]; ok {
			val.sort(end)
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
	} else {
		return nil, err
	}
}
func (i *impawnImpl) RedeemSAccount(curHeight uint64, addr common.Address, amount *big.Int) error {
	curEpoch := i.getEpochFromHeight(curHeight)
	if curEpoch == nil {
		return errInvalidParam
	}
	sa, err := i.getStakingAccount(curEpoch.EpochID, addr)
	if err != nil {
		return err
	}
	sa.insertRedeemInfo(amount, new(big.Int).SetUint64(curHeight))
	fmt.Println("[SA]insert a redeem,address:[", addr.String(), "],amount:[", amount.String(), "],height:", curHeight)
	return nil
}
func (i *impawnImpl) RedeemDAccount(curHeight uint64, addrSA, addrDA common.Address, amount *big.Int) error {
	curEpoch := i.getEpochFromHeight(curHeight)
	if curEpoch == nil {
		return errInvalidParam
	}
	sa, err := i.getStakingAccount(curEpoch.EpochID, addrSA)
	if err != nil {
		return err
	}
	da, err2 := i.getDAfromSA(sa, addrDA)
	if err2 != nil {
		return err
	}
	da.insertRedeemInfo(amount, new(big.Int).SetUint64(curHeight))
	fmt.Println("[DA]insert a redeem,address:[", addrSA.String(), "],DA address:[", addrDA.String(), "],amount:[", amount.String(), "],height:", curHeight)
	return nil
}

// doing in every fast block produced by consensus
// 1. redeem while not be in committee
// 2. set auto=false when the redeem amount is equal or greater than staking amount in the account
// 3. judge the block height
// 4. all redeem for the staking in last epoch will be done
func (i *impawnImpl) DoRedeem(curHeight uint64) error {
	curEpoch := i.getEpochFromHeight(curHeight)
	if curEpoch == nil {
		return errInvalidParam
	}
	// make sure the epochInfo sort by EpochID
	for _, epoch := range i.epochInfo {
		if val, ok := i.accounts[epoch.EpochID]; ok {
			for _, v := range val {
				if epoch.EpochID < curEpoch.EpochID {
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
func (i *impawnImpl) Shift(epochid uint64) error {
	return nil
}

/////////////////////////////////////////////////////////////////////////////////
// storage layer
func (i *impawnImpl) GetRoot() common.Hash {
	return common.Hash{}
}
func (i *impawnImpl) Save(state vm.StateDB, preAddress common.Address, key common.Hash, value []byte) error {
	state.SetPOSState(preAddress, key, value)
	return nil
}
func (i *impawnImpl) Load(state vm.StateDB, preAddress common.Address, key common.Hash) error {
	state.GetPOSState(preAddress, key)
	return nil
}
func (i *impawnImpl) commit() error {
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
		mid = low + (height-low)/2
		if hh == vs[mid].height.Uint64() {
			return vs[mid], mid
		} else if hh > vs[mid].height.Uint64() {
			height = mid - 1
		} else {
			low = mid + 1
		}
	}
	return nil, mid
}
func (vs valuesByHeight) update(val *PairstakingValue) {
	item, pos := vs.find(val.height.Uint64())
	if item != nil {
		item.amount = new(big.Int).Add(item.amount, val.amount)
		item.state |= val.state
	} else {
		rear := append([]*PairstakingValue{}, vs[pos:]...)
		vs = append(append(vs[:pos], val), rear...)
	}
}

type stakingItem struct {
	item   *StakingAccount
	height uint64
}

type stakingByHeight []*stakingItem

func toStakingByHeight(hh uint64, items []*StakingAccount) stakingByHeight {
	var tmp []*stakingItem
	for _, v := range items {
		v.unit.sort()
		tmp = append(tmp, &stakingItem{
			item:   v,
			height: hh,
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
	return vs[i].item.getAllStaking(vs[i].height).Cmp(vs[j].item.getAllStaking(vs[j].height)) == -1
}
func (vs stakingByHeight) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}

type delegationItem struct {
	item   *DelegationAccount
	height uint64
}

type delegationItemByHeight []*delegationItem

func toDelegationByHeight(hh uint64, items []*DelegationAccount) delegationItemByHeight {
	var tmp []*delegationItem
	for _, v := range items {
		v.unit.sort()
		tmp = append(tmp, &delegationItem{
			item:   v,
			height: hh,
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
	return vs[i].item.getAllStaking(vs[i].height).Cmp(vs[j].item.getAllStaking(vs[j].height)) == -1
}
func (vs delegationItemByHeight) Swap(i, j int) {
	it := vs[i]
	vs[i] = vs[j]
	vs[j] = it
}
