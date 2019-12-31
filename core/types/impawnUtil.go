package types

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
)

var (
	baseUnit           = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fbaseUnit          = new(big.Float).SetFloat64(float64(baseUnit.Int64()))
	mixImpawn          = new(big.Int).Mul(big.NewInt(1000), baseUnit)
	Base               = new(big.Int).SetUint64(10000)
	CountInEpoch       = 31
	MaxRedeemHeight    = uint64(1000)
	MixEpochCount      = 2
	EpochElectionPoint = 500
	DposForkPoint      = uint64(20)
	PreselectionPeriod = uint64(2000)
	EpochLength        = uint64(10000)
	ElectionPoint      = uint64(300)
)

var (
	ErrInvalidParam      = errors.New("Invalid Param")
	ErrOverEpochID       = errors.New("Over epoch id")
	ErrNotSequential     = errors.New("epoch id not sequential")
	ErrInvalidEpochInfo  = errors.New("Invalid epoch info")
	ErrNullImpawnInEpoch = errors.New("null impawn in the epoch")
	ErrInvalidStaking    = errors.New("Invalid staking account")
	ErrMatchEpochID      = errors.New("wrong match epoch id in a reward block")
	ErrNotStaking        = errors.New("Not match the staking account")
	ErrNotDelegation     = errors.New("Not match the delegation account")
	ErrNotMatchEpochInfo = errors.New("the epoch info is not match with accounts")
	ErrNotElectionTime   = errors.New("not time to election the next committee")
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
type SARewardInfos struct {
	Items []*RewardInfo
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
func FromBlock(block *SnailBlock) (begin, end uint64) {
	begin, end = 0, 0
	l := len(block.Fruits())
	if l > 0 {
		begin, end = block.Fruits()[0].NumberU64(), block.Fruits()[l-1].NumberU64()
	}
	return
}
func getfirstEpoch() *EpochIDInfo {
	return &EpochIDInfo{
		EpochID:     1,
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
	var eid uint64
	if (hh-first.EndHeight)%EpochLength == 0 {
		eid = (hh-first.EndHeight)/EpochLength + first.EpochID
	} else {
		eid = (hh-first.EndHeight)/EpochLength + first.EpochID + 1
	}
	return GetEpochFromID(eid)
}
func GetEpochFromID(eid uint64) *EpochIDInfo {
	first := getfirstEpoch()
	if first.EpochID == eid {
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
