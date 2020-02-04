package types

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	baseUnit  = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fbaseUnit = new(big.Float).SetFloat64(float64(baseUnit.Int64()))
	mixImpawn = new(big.Int).Mul(big.NewInt(1000), baseUnit)
	Base      = new(big.Int).SetUint64(10000)
	// StakingAddress is defined as Address('truestaking')
	// i.e. contractAddress = 0x000000000000000000747275657374616b696E67
	StakingAddress = common.BytesToAddress([]byte("truestaking"))
	MixEpochCount  = 2
)

var (
	ErrInvalidParam      = errors.New("Invalid Param")
	ErrOverEpochID       = errors.New("Over epoch id")
	ErrNotSequential     = errors.New("epoch id not sequential")
	ErrInvalidEpochInfo  = errors.New("Invalid epoch info")
	ErrNotFoundEpoch     = errors.New("cann't found the epoch info")
	ErrInvalidStaking    = errors.New("Invalid staking account")
	ErrMatchEpochID      = errors.New("wrong match epoch id in a reward block")
	ErrNotStaking        = errors.New("Not match the staking account")
	ErrNotDelegation     = errors.New("Not match the delegation account")
	ErrNotMatchEpochInfo = errors.New("the epoch info is not match with accounts")
	ErrNotElectionTime   = errors.New("not time to election the next committee")
	ErrAmountOver        = errors.New("the amount more than staking amount")
	ErrDelegationSelf    = errors.New("Cann't delegation myself")
	ErrRedeemAmount      = errors.New("wrong redeem amount")
	ErrForbidAddress     = errors.New("Forbidding Address")
)

const (
	// StateStakingOnce can be election only once
	StateStakingOnce uint8 = 1 << iota
	// StateStakingAuto can be election in every epoch
	StateStakingAuto
	StateStakingCancel
	// StateRedeem can be redeem real time (after MaxRedeemHeight block)
	StateRedeem
	// StateRedeemed flag the asset which is staking in the height is redeemed
	StateRedeemed
)
const (
	OpQueryStaking uint8 = 1 << iota
	OpQueryLocked
	OpQueryCancelable
)

type RewardInfo struct {
	Address common.Address
	Amount  *big.Int
}

func (e *RewardInfo) String() string {
	return fmt.Sprintf("[Address:%v,Amount:%v\n]", e.Address.String(), ToTrue(e.Amount))
}

type SARewardInfos struct {
	Items []*RewardInfo
}

func (s *SARewardInfos) String() string {
	var ss string
	for _, v := range s.Items {
		ss += v.String()
	}
	return ss
}

type ChainReward struct {
	Foundation		*RewardInfo
	CoinBase 		*RewardInfo
	FruitBase 		[]*RewardInfo
	CommitteeBase 	[]*SARewardInfos
}
func NewChainReward(found,coin *RewardInfo,fruits []*RewardInfo,committee []*SARewardInfos) *ChainReward{
	return &ChainReward{
		Foundation: found,
		CoinBase: 	coin,
		FruitBase:fruits,
		CommitteeBase:committee,
	}
}
func ToRewardInfos1(items map[common.Address]*big.Int) []*RewardInfo {
	infos := make([]*RewardInfo,0,0)
	for k,v := range items {
		infos = append(infos,&RewardInfo{
			Address:	k,
			Amount: 	new(big.Int).Set(v),
		})
	}
	return infos
}
func ToRewardInfos2(items map[common.Address]*big.Int) []*SARewardInfos {
	infos := make([]*SARewardInfos,0,0)
	for k,v := range items {
		items := []*RewardInfo{&RewardInfo{
			Address:	k,
			Amount: 	new(big.Int).Set(v),
		}}
		
		infos = append(infos,&SARewardInfos{
			Items:	items,
		})
	}
	return infos
}
func MergeReward(map1,map2 map[common.Address]*big.Int) map[common.Address]*big.Int {
	for k,v := range map2 {
		if vv,ok := map1[k];ok {
			map1[k] = new(big.Int).Add(vv,v)
		} else {
			map1[k] = v
		}
	}
	return map1
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
	if e.EpochID == 0 && params.DposForkPoint+1 != e.BeginHeight {
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


// the key is epochid if StakingValue as a locked asset,otherwise key is block height if StakingValue as a staking asset
type StakingValue struct {
	Value map[uint64]*big.Int
}

type LockedItem struct {
	Amount *big.Int
	Locked 	bool
}
// LockedValue,the key of Value is epochid
type LockedValue struct {
	Value map[uint64]*LockedItem
}

func (s *StakingValue) ToLockedValue(height uint64) *LockedValue {
	res := make(map[uint64]*LockedItem)
	for k,v := range s.Value {
		item := &LockedItem{
			Amount: new(big.Int).Set(v),
			Locked: !IsUnlocked(k,height),
		}
		res[k]=item
	}
	return &LockedValue{
		Value: res,
	}
}

func toReward(val *big.Float) *big.Int {
	val = val.Mul(val, fbaseUnit)
	ii, _ := val.Int64()
	return big.NewInt(ii)
}
func ToTrue(val *big.Int) *big.Float {
	return new(big.Float).Quo(new(big.Float).SetInt(val), fbaseUnit)
}
func FromBlock(block *SnailBlock) (begin, end uint64) {
	begin, end = 0, 0
	l := len(block.Fruits())
	if l > 0 {
		begin, end = block.Fruits()[0].FastNumber().Uint64(), block.Fruits()[l-1].FastNumber().Uint64()
	}
	return
}
func GetFirstEpoch() *EpochIDInfo {
	return &EpochIDInfo{
		EpochID:     params.FirstNewEpochID,
		BeginHeight: params.DposForkPoint + 1,
		EndHeight:   params.DposForkPoint + params.NewEpochLength,
	}
}
func GetPreFirstEpoch() *EpochIDInfo {
	return &EpochIDInfo{
		EpochID:     params.FirstNewEpochID - 1,
		BeginHeight: 0,
		EndHeight:   params.DposForkPoint,
	}
}
func GetEpochFromHeight(hh uint64) *EpochIDInfo {
	if hh <= params.DposForkPoint {
		return GetPreFirstEpoch()
	}
	first := GetFirstEpoch()
	if hh <= first.EndHeight {
		return first
	}
	var eid uint64
	if (hh-first.EndHeight)%params.NewEpochLength == 0 {
		eid = (hh-first.EndHeight)/params.NewEpochLength + first.EpochID
	} else {
		eid = (hh-first.EndHeight)/params.NewEpochLength + first.EpochID + 1
	}
	return GetEpochFromID(eid)
}
func GetEpochFromID(eid uint64) *EpochIDInfo {
	preFirst := GetPreFirstEpoch()
	if preFirst.EpochID == eid {
		return preFirst
	}
	first := GetFirstEpoch()
	if first.EpochID >= eid {
		return first
	}
	return &EpochIDInfo{
		EpochID:     eid,
		BeginHeight: first.EndHeight + (eid-first.EpochID-1)*params.NewEpochLength + 1,
		EndHeight:   first.EndHeight + (eid-first.EpochID)*params.NewEpochLength,
	}
}
func GetEpochFromRange(begin, end uint64) []*EpochIDInfo {
	if end == 0 || begin > end || (begin < params.DposForkPoint && end < params.DposForkPoint) {
		return nil
	}
	var ids []*EpochIDInfo
	e1 := GetEpochFromHeight(begin)
	e := uint64(0)

	if e1 != nil {
		ids = append(ids, e1)
		e = e1.EndHeight
	} else {
		e = params.DposForkPoint
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
func CopyVotePk(pk []byte) []byte {
	cc := make([]byte, len(pk))
	copy(cc, pk)
	return cc
}
func ValidPk(pk []byte) error {
	_, err := crypto.UnmarshalPubkey(pk)
	return err
}
func MinCalcRedeemHeight(eid uint64) uint64 {
	e := GetEpochFromID(eid + 1)
	return e.BeginHeight + params.MaxRedeemHeight + 1
}
func ForbidAddress(addr common.Address) error {
	if bytes.Equal(addr[:], StakingAddress[:]) {
		return errors.New(fmt.Sprint("addr error:", addr, ErrForbidAddress))
	}
	return nil
}
func IsUnlocked(eid,height uint64) bool {
	e := GetEpochFromID(eid + 1)
	return height > e.BeginHeight+params.MaxRedeemHeight
}