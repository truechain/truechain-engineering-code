package types

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/crypto"
)

var (
	baseUnit           = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fbaseUnit          = new(big.Float).SetFloat64(float64(baseUnit.Int64()))
	mixImpawn          = new(big.Int).Mul(big.NewInt(1000), baseUnit)
	Base               = new(big.Int).SetUint64(10000)
	CountInEpoch       = 31
	MaxRedeemHeight    = uint64(1000)
	MixEpochCount      = 2
	DposForkPoint      = uint64(0)
	PreselectionPeriod = uint64(0)
	EpochLength        = uint64(500)
	ElectionPoint      = uint64(100)
	FirstEpochID       = uint64(1)
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

type EpochIDInfo struct {
	EpochID     uint64
	BeginHeight uint64
	EndHeight   uint64
}

type StakingItem struct {
	// CoinBase 	common.Address
	Fee         *big.Int
	PK          []byte
	Value       map[uint64]*big.Int
	Delegations map[common.Address]*DelegationItem
}
type DelegationItem struct {
	SA common.Address
	// CoinBase 	common.Address
	Value map[uint64]*big.Int
}
type LockedItem struct {
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
	// if DposForkPoint == 0 {
	// 	return &EpochIDInfo{
	// 		EpochID:     FirstEpochID,
	// 		BeginHeight: 0,
	// 		EndHeight:   DposForkPoint + PreselectionPeriod + EpochLength,
	// 	}
	// } else {
	// 	return &EpochIDInfo{
	// 		EpochID:     FirstEpochID,
	// 		BeginHeight: DposForkPoint + 1,
	// 		EndHeight:   DposForkPoint + PreselectionPeriod + EpochLength,
	// 	}
	// }
	return &EpochIDInfo{
		EpochID:     FirstEpochID,
		BeginHeight: DposForkPoint + PreselectionPeriod + 1,
		EndHeight:   DposForkPoint + PreselectionPeriod + EpochLength,
	}
}
func GetPreFirstEpoch() *EpochIDInfo {
	return &EpochIDInfo{
		EpochID:     FirstEpochID - 1,
		BeginHeight: 0,
		EndHeight:   DposForkPoint + PreselectionPeriod,
	}
}
func GetEpochFromHeight(hh uint64) *EpochIDInfo {
	if hh <= DposForkPoint+PreselectionPeriod {
		return GetPreFirstEpoch()
	}
	first := GetFirstEpoch()
	if hh <= first.EndHeight {
		return first
	}
	var eid uint64
	if (hh-first.EndHeight)%EpochLength == 0 {
		eid = (hh-first.EndHeight)/EpochLength + first.EpochID
	} else {
		eid = (hh-first.EndHeight)/EpochLength + first.EpochID + 1
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
		BeginHeight: first.EndHeight + (eid-first.EpochID-1)*EpochLength + 1,
		EndHeight:   first.EndHeight + (eid-first.EpochID)*EpochLength,
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
func CopyVotePk(pk []byte) []byte {
	cc := make([]byte, len(pk))
	copy(cc, pk)
	return cc
}
func ValidPk(pk []byte) error {
	_, err := crypto.UnmarshalPubkey(pk)
	return err
}
