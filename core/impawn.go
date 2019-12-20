package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/types"
)

var (
	baseUnit     = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fbaseUnit    = new(big.Float).SetFloat64(float64(baseUnit.Int64()))
	mixImpawn    = new(big.Int).Mul(big.NewInt(1000), baseUnit)
	CountInEpoch = 31
)

var (
	errInvalidParam      = errors.New("Invalid Param")
	errOverEpochID       = errors.New("Over epoch id")
	errNotSequential     = errors.New("epoch id not sequential")
	errInvalidEpochInfo  = errors.New("Invalid epoch info")
	errNullImpawnInEpoch = errors.New("null impawn in the epoch")
	errInvalidStaking    = errors.New("Invalid staking account")
	errMatchEpochID      = errors.New("wrong match epoch id in a reward block")
	errClosed            = errors.New("socket closed")
	errVersion           = errors.New("error version number")
)

type ImpawnAccount interface {
	GetRewardAddress() common.Address
	GetBorn() *big.Int
	GetAmount() *big.Int
}
type RewardItem struct {
	Address common.Address
	Amount  *big.Int
}
type SARewardInfo struct {
	items []*RewardItem
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

/////////////////////////////////////////////////////////////////////////////////
type PairstakingValue struct {
	amount *big.Int
	height *big.Int
}

type impawnUnit struct {
	address common.Address
	value   []*PairstakingValue // sort by height
	auto    bool
	redeem  *big.Int
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
	return common.Address{}
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
func (s *StakingAccount) getAllStaking(hh uint64) *big.Int {
	all := s.unit.getAllStaking(hh)
	for _, v := range s.delegation {
		all = all.Add(all, v.getAllStaking(hh))
	}
	return all
}

type SAImpawns []*StakingAccount

func (s *SAImpawns) getAllStaking(hh uint64) *big.Int {
	all := big.NewInt(0)
	for _, val := range *s {
		all = all.Add(all, val.getAllStaking(hh))
	}
	return all
}

/////////////////////////////////////////////////////////////////////////////////

// be thread-safe for caller locked
type impawnImpl struct {
	lock      sync.RWMutex
	epochInfo []*EpochIDInfo // sort by epoch id
	accounts  map[uint64]SAImpawns
}

// Keep the epoch info is sequential
func (i *impawnImpl) SetEpochID(info *EpochIDInfo) error {
	if info == nil {
		return errInvalidParam
	}
	if !info.IsValid() {
		return errInvalidEpochInfo
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
func (i *impawnImpl) getCurrentEpoch() uint64 {
	pos := len(i.epochInfo)
	if pos > 0 {
		return i.epochInfo[pos].EpochID
	}
	return 0
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
func (i *impawnImpl) calcrewardInSa(target uint64, sa *StakingAccount, allReward, allStaking *big.Float, item *RewardItem) ([]*RewardItem, error) {
	if sa == nil || allReward == nil || item == nil || allStaking == nil {
		return nil, errInvalidParam
	}
	var items []*RewardItem
	fee := new(big.Float).Mul(allReward, sa.fee)
	all, left := new(big.Float).Sub(allReward, fee), big.NewFloat(0)
	for _, v := range sa.delegation {
		daAll := new(big.Float).Quo(new(big.Float).SetFloat64(float64(v.getAllStaking(target).Int64())), fbaseUnit)
		rate := new(big.Float).Quo(daAll, allStaking)
		f1 := new(big.Float).Mul(all, rate)
		left = left.Add(left, f1)
		var ii RewardItem
		ii.Address, ii.Amount = v.unit.GetRewardAddress(), toReward(f1)
		items = append(items, &ii)
	}
	f2 := new(big.Float).Add(fee, new(big.Float).Sub(all, left))
	item.Amount = toReward(f2)
	return items, nil
}
func (i *impawnImpl) calcReward(target uint64, allAmount *big.Int, einfo *EpochIDInfo) ([]*SARewardInfo, error) {
	if _, ok := i.accounts[einfo.EpochID]; !ok {
		return nil, errInvalidParam
	} else {
		addrs := i.getElections(einfo.EpochID)
		impawns := i.fetchAccountsInEpoch(einfo.EpochID, addrs)
		if impawns == nil || len(addrs) == 0 {
			return nil, errNullImpawnInEpoch
		}
		var res []*SARewardInfo
		all := impawns.getAllStaking(target)
		allf := new(big.Float).Quo(new(big.Float).SetFloat64(float64(all.Int64())), fbaseUnit)
		amount := new(big.Float).Quo(new(big.Float).SetFloat64(float64(allAmount.Int64())), fbaseUnit)
		left := big.NewFloat(0)
		for index, v := range impawns {
			var info SARewardInfo
			var item RewardItem
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
func (i *impawnImpl) Reward(block *types.SnailBlock, allAmount *big.Int) ([]*SARewardInfo, error) {
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
// storage layer
func (i *impawnImpl) GetRoot() common.Hash {
	return common.Hash{}
}
func (i *impawnImpl) Save() error {
	return nil
}
func (i *impawnImpl) Load() error {
	return nil
}
func (i *impawnImpl) commit() error {
	return nil
}
