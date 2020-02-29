package vm

import (
	"encoding/json"
	"errors"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/params"
	"io"
	"math/big"
	"strconv"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/rlp"
)

// "external" ImpawnImpl encoding. used for pos staking.
type extImpawnImpl struct {
	Accounts   []SAImpawns
	CurEpochID uint64
	Array      []uint64
	LastReward uint64
}

func (i *ImpawnImpl) DecodeRLP(s *rlp.Stream) error {
	var ei extImpawnImpl
	if err := s.Decode(&ei); err != nil {
		return err
	}
	accounts := make(map[uint64]SAImpawns)
	for i, account := range ei.Accounts {
		accounts[ei.Array[i]] = account
	}

	i.curEpochID, i.accounts, i.lastReward = ei.CurEpochID, accounts, ei.LastReward
	return nil
}

// EncodeRLP serializes b into the truechain RLP ImpawnImpl format.
func (i *ImpawnImpl) EncodeRLP(w io.Writer) error {
	var accounts []SAImpawns
	var order []uint64
	for i, _ := range i.accounts {
		order = append(order, i)
	}
	for m := 0; m < len(order)-1; m++ {
		for n := 0; n < len(order)-1-m; n++ {
			if order[n] > order[n+1] {
				order[n], order[n+1] = order[n+1], order[n]
			}
		}
	}
	for _, epoch := range order {
		accounts = append(accounts, i.accounts[epoch])
	}
	return rlp.Encode(w, extImpawnImpl{
		CurEpochID: i.curEpochID,
		Accounts:   accounts,
		Array:      order,
		LastReward: i.lastReward,
	})
}

func (i *ImpawnImpl) GetAllStakingAccountRPC(height uint64) map[string]interface{} {
	sas := i.GetAllStakingAccount()
	sasRPC := make(map[string]interface{}, len(sas))
	var attrs []map[string]interface{}
	count := 0
	countCommittee := 0
	for index, sa := range sas {
		attr := make(map[string]interface{})
		attr["id"] = index
		attr["unit"] = unitDisplay(sa.Unit)
		attr["votePubKey"] = hexutil.Bytes(sa.Votepubkey)
		attr["fee"] = sa.Fee.Uint64()
		if countCommittee <= params.CountInEpoch && isCommitteeMember(i, sa.Unit.Address) {
			attr["committee"] = true
			countCommittee++
		} else {
			attr["committee"] = false
		}
		attr["delegation"] = daSDisplay(sa.Delegation, height)
		if sa.Modify != nil {
			ai := make(map[string]interface{})
			if sa.Modify.Fee != nil {
				ai["fee"] = sa.Modify.Fee.Uint64()
			}
			if sa.Modify.VotePubkey != nil {
				ai["votePubKey"] = hexutil.Bytes(sa.Modify.VotePubkey)
			}
			attr["modify"] = ai
		}
		attr["staking"] = weiToTrue(sa.getAllStaking(height))
		attr["validStaking"] = weiToTrue(sa.getValidStaking(height))
		attrs = append(attrs, attr)
		count = count + len(sa.Delegation)
	}
	sasRPC["stakers"] = attrs
	sasRPC["stakerCount"] = len(sas)
	sasRPC["delegateCount"] = count
	return sasRPC
}

func (i *ImpawnImpl) GetStakingAssetRPC(addr common.Address) []StakingAsset {
	msv := i.GetStakingAsset(addr)
	var attrs []StakingAsset
	for key, value := range msv {
		attr := StakingAsset{
			StakingValue: stakingValueDisplay(value),
			Address:      key,
		}
		attrs = append(attrs, attr)
	}
	return attrs
}

type StakingAsset struct {
	StakingValue []*StakingValue `json:"stakingValue"`
	Address      common.Address  `json:"address"`
}

type StakingValue struct {
	Height uint64
	Amount string
}

// MarshalJSON marshals as JSON.
func (s StakingValue) MarshalJSON() ([]byte, error) {
	type StakingValue struct {
		Height hexutil.Uint64 `json:"height"`
		Amount string         `json:"amount"`
	}
	var enc StakingValue
	enc.Height = hexutil.Uint64(s.Height)

	enc.Amount = s.Amount
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (s *StakingValue) UnmarshalJSON(input []byte) error {
	type StakingValue struct {
		Height *hexutil.Uint64 `json:"height"`
		Amount *string         `json:"amount"`
	}
	var dec StakingValue
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Height != nil {
		s.Height = uint64(*dec.Height)
	}
	if dec.Amount != nil {
		s.Amount = *dec.Amount
	}
	return nil
}

// MarshalJSON marshals as JSON.
func (s StakingAsset) MarshalJSON() ([]byte, error) {
	type StakingAsset struct {
		StakingValue []*StakingValue `json:"stakingValue"`
		Address      common.Address  `json:"address"`
	}
	var enc StakingAsset
	enc.StakingValue = s.StakingValue
	enc.Address = s.Address
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (s *StakingAsset) UnmarshalJSON(input []byte) error {
	type StakingAsset struct {
		StakingValue []*StakingValue `json:"stakingValue"`
		Address      *common.Address `json:"address"`
	}
	var dec StakingAsset
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.StakingValue == nil {
		return errors.New("missing required field 'stakingValue' for LockedAsset")
	}
	s.StakingValue = dec.StakingValue
	if dec.Address != nil {
		s.Address = *dec.Address
	}
	return nil
}

func (i *ImpawnImpl) GetLockedAssetRPC(addr common.Address, height uint64) []LockedAsset {
	ls := i.GetLockedAsset2(addr, height)
	var attrs []LockedAsset
	for key, value := range ls {
		attr := LockedAsset{
			LockValue: lockValueDisplay(value),
			Address:   key,
		}
		attrs = append(attrs, attr)
	}
	return attrs
}

type LockedAsset struct {
	LockValue []*LockValue   `json:"lockValue"`
	Address   common.Address `json:"address"`
}

type LockValue struct {
	EpochID uint64
	Amount  string
	Height  *big.Int
	Locked  bool
}

// MarshalJSON marshals as JSON.
func (l LockValue) MarshalJSON() ([]byte, error) {
	type LockValue struct {
		EpochID hexutil.Uint64 `json:"epochID"`
		Amount  string         `json:"amount"`
		Height  *hexutil.Big   `json:"height"`
		Locked  bool           `json:"locked"`
	}
	var enc LockValue
	enc.EpochID = hexutil.Uint64(l.EpochID)

	enc.Amount = l.Amount
	enc.Height = (*hexutil.Big)(l.Height)
	enc.Locked = l.Locked
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (l *LockValue) UnmarshalJSON(input []byte) error {
	type LockValue struct {
		EpochID *hexutil.Uint64 `json:"epochID"`
		Amount  *string         `json:"amount"`
		Height  *hexutil.Big    `json:"height"`
		Locked  *bool           `json:"locked"`
	}
	var dec LockValue
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.EpochID != nil {
		l.EpochID = uint64(*dec.EpochID)
	}
	if dec.Amount != nil {
		l.Amount = *dec.Amount
	}
	if dec.Height != nil {
		l.Height = (*big.Int)(dec.Height)
	}
	if dec.Locked != nil {
		l.Locked = *dec.Locked
	}
	return nil
}

// MarshalJSON marshals as JSON.
func (l LockedAsset) MarshalJSON() ([]byte, error) {
	type LockedAsset struct {
		LockValue []*LockValue   `json:"lockValue"`
		Address   common.Address `json:"address"`
	}
	var enc LockedAsset
	enc.LockValue = l.LockValue
	enc.Address = l.Address
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (l *LockedAsset) UnmarshalJSON(input []byte) error {
	type LockedAsset struct {
		LockValue []*LockValue    `json:"lockValue"`
		Address   *common.Address `json:"address"`
	}
	var dec LockedAsset
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.LockValue == nil {
		return errors.New("missing required field 'lockValue' for LockedAsset")
	}
	l.LockValue = dec.LockValue
	if dec.Address != nil {
		l.Address = *dec.Address
	}
	return nil
}

func (i *ImpawnImpl) GetAllCancelableAssetRPC(addr common.Address) []CancelableAsset {
	assets := i.GetAllCancelableAsset(addr)
	var attrs []CancelableAsset
	for key, value := range assets {
		attr := CancelableAsset{Value: weiToTrue(value), Address: key}
		attrs = append(attrs, attr)
	}
	return attrs
}

type CancelableAsset struct {
	Value   string
	Address common.Address
}

// MarshalJSON marshals as JSON.
func (c CancelableAsset) MarshalJSON() ([]byte, error) {
	type CancelableAsset struct {
		Value   string         `json:"value"`
		Address common.Address `json:"address"`
	}
	var enc CancelableAsset
	enc.Value = c.Value
	enc.Address = c.Address
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (c *CancelableAsset) UnmarshalJSON(input []byte) error {
	type CancelableAsset struct {
		Value   *string         `json:"value"`
		Address *common.Address `json:"address"`
	}
	var dec CancelableAsset
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Value != nil {
		c.Value = *dec.Value
	}
	if dec.Address != nil {
		c.Address = *dec.Address
	}
	return nil
}

func (i *ImpawnImpl) GetStakingAccountRPC(height uint64, address common.Address) map[string]interface{} {
	sas := i.GetAllStakingAccount()
	sa := sas.getSA(address)
	attr := make(map[string]interface{})
	if sa == nil {
		return nil
	}
	attr["id"] = i
	attr["unit"] = unitDisplay(sa.Unit)
	attr["votePubKey"] = hexutil.Bytes(sa.Votepubkey)
	attr["fee"] = sa.Fee.Uint64()
	attr["committee"] = isCommitteeMember(i, sa.Unit.Address)
	attr["delegation"] = daSDisplay(sa.Delegation, height)
	if sa.Modify != nil {
		ai := make(map[string]interface{})
		if sa.Modify.Fee != nil {
			ai["fee"] = sa.Modify.Fee.Uint64()
		}
		if sa.Modify.VotePubkey != nil {
			ai["votePubKey"] = hexutil.Bytes(sa.Modify.VotePubkey)
		}
		attr["modify"] = ai
	}
	attr["staking"] = weiToTrue(sa.getAllStaking(height))
	attr["validStaking"] = weiToTrue(sa.getValidStaking(height))
	return attr
}

func isCommitteeMember(i *ImpawnImpl, address common.Address) bool {
	sas := i.getElections3(i.curEpochID)
	if sas == nil {
		return false
	}
	impawns := SAImpawns(sas)
	sa := impawns.getSA(address)
	if sa == nil {
		return false
	}
	return true
}

func daSDisplay(das []*DelegationAccount, height uint64) map[string]interface{} {
	attrs := make(map[string]interface{}, len(das))
	for i, da := range das {
		attr := make(map[string]interface{})
		attr["saAddress"] = da.SaAddress
		attr["delegate"] = weiToTrue(da.getAllStaking(height))
		attr["validDelegate"] = weiToTrue(da.getValidStaking(height))
		attr["unit"] = unitDisplay(da.Unit)
		attrs[strconv.Itoa(i)] = attr
	}
	return attrs
}

func unitDisplay(uint *impawnUnit) map[string]interface{} {
	attr := make(map[string]interface{})
	attr["address"] = uint.Address
	attr["value"] = pvSDisplay(uint.Value)
	attr["redeemInfo"] = riSDisplay(uint.RedeemInof)
	return attr
}

func pvSDisplay(pvs []*PairstakingValue) map[string]interface{} {
	attrs := make(map[string]interface{}, len(pvs))
	for i, pv := range pvs {
		attr := make(map[string]interface{})
		attr["amount"] = weiToTrue(pv.Amount)
		attr["height"] = pv.Height
		attr["state"] = uint64(pv.State)
		attrs[strconv.Itoa(i)] = attr
	}
	return attrs
}

func riSDisplay(ris []*RedeemItem) map[string]interface{} {
	attrs := make(map[string]interface{}, len(ris))
	for i, ri := range ris {
		attr := make(map[string]interface{})
		attr["amount"] = weiToTrue(ri.Amount)
		attr["epochID"] = ri.EpochID
		attr["state"] = uint64(ri.State)
		attrs[strconv.Itoa(i)] = attr
	}
	return attrs
}

func lockValueDisplay(lv *types.LockedValue) []*LockValue {
	attrs := make([]*LockValue, 0)
	for epoch, value := range lv.Value {
		attrs = append(attrs, &LockValue{
			EpochID: epoch,
			Amount:  weiToTrue(value.Amount),
			Height:  new(big.Int).SetUint64(types.MinCalcRedeemHeight(epoch)),
			Locked:  value.Locked,
		})
	}
	return attrs
}

func stakingValueDisplay(sv *types.StakingValue) []*StakingValue {
	attrs := make([]*StakingValue, 0)
	for height, value := range sv.Value {
		attrs = append(attrs, &StakingValue{
			Height: height,
			Amount: weiToTrue(value),
		})
	}
	return attrs
}

func weiToTrue(val *big.Int) string {
	baseUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	fbaseUnit := new(big.Float).SetFloat64(float64(baseUnit.Int64()))
	return new(big.Float).Quo(new(big.Float).SetInt(val), fbaseUnit).String()
}
