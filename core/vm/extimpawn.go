package vm

import (
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/core/types"
	"io"
	"math/big"
	"strconv"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/rlp"
)

// "external" PairstakingValue encoding. used for pos staking.
type extPairstakingValue struct {
	Amount *big.Int
	Height *big.Int
	State  uint8
}

func (p *PairstakingValue) DecodeRLP(s *rlp.Stream) error {
	var ep extPairstakingValue
	if err := s.Decode(&ep); err != nil {
		return err
	}
	p.amount, p.height, p.state = ep.Amount, ep.Height, ep.State
	return nil
}

// EncodeRLP serializes b into the truechain RLP PairstakingValue format.
func (p *PairstakingValue) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extPairstakingValue{
		Amount: p.amount,
		Height: p.height,
		State:  p.state,
	})
}

// "external" impawnUnit encoding. used for pos staking.
type extImpawnUnit struct {
	Address    common.Address
	Value      []*PairstakingValue // sort by height
	RedeemInof []*RedeemItem
}

func (i *impawnUnit) DecodeRLP(s *rlp.Stream) error {
	var ei extImpawnUnit
	if err := s.Decode(&ei); err != nil {
		return err
	}
	i.address, i.value, i.redeemInof = ei.Address, ei.Value, ei.RedeemInof
	return nil
}

// EncodeRLP serializes b into the truechain RLP impawnUnit format.
func (i *impawnUnit) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extImpawnUnit{
		Address:    i.address,
		Value:      i.value,
		RedeemInof: i.redeemInof,
	})
}

// "external" DelegationAccount encoding. used for pos staking.
type extDAccount struct {
	DeleAddress common.Address
	Unit        *impawnUnit
}

func (d *DelegationAccount) DecodeRLP(s *rlp.Stream) error {
	var da extDAccount
	if err := s.Decode(&da); err != nil {
		return err
	}
	d.saAddress, d.unit = da.DeleAddress, da.Unit
	return nil
}

// EncodeRLP serializes b into the truechain RLP DelegationAccount format.
func (i *DelegationAccount) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extDAccount{
		DeleAddress: i.saAddress,
		Unit:        i.unit,
	})
}

// "external" StakingAccount encoding. used for pos staking.
type extSAccount struct {
	Unit       *impawnUnit
	Votepubkey []byte
	Fee        *big.Int
	Committee  bool
	Delegation []*DelegationAccount
	Modify     *AlterableInfo
}

func (sa *StakingAccount) DecodeRLP(s *rlp.Stream) error {
	var es extSAccount
	if err := s.Decode(&es); err != nil {
		return err
	}
	sa.unit, sa.votepubkey, sa.fee, sa.committee, sa.delegation, sa.modify = es.Unit, es.Votepubkey, es.Fee, es.Committee, es.Delegation, es.Modify
	return nil
}

// EncodeRLP serializes b into the truechain RLP StakingAccount format.
func (sa *StakingAccount) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extSAccount{
		Unit:       sa.unit,
		Votepubkey: sa.votepubkey,
		Fee:        sa.fee,
		Committee:  sa.committee,
		Delegation: sa.delegation,
		Modify:     sa.modify,
	})
}

// "external" AlterableInfo encoding. used for pos staking.
type extAlterableInfo struct {
	Fee        *big.Int
	VotePubkey []byte
}

func (a *AlterableInfo) DecodeRLP(s *rlp.Stream) error {
	var ea extAlterableInfo
	if err := s.Decode(&ea); err != nil {
		return err
	}
	a.fee, a.votePubkey = ea.Fee, ea.VotePubkey
	return nil
}

// EncodeRLP serializes b into the truechain RLP AlterableInfo format.
func (a *AlterableInfo) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extAlterableInfo{
		Fee:        a.fee,
		VotePubkey: a.votePubkey,
	})
}

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
	for i, sa := range sas {
		attr := make(map[string]interface{})
		attr["id"] = i
		attr["unit"] = unitDisplay(sa.unit)
		attr["votePubKey"] = hexutil.Bytes(sa.votepubkey)
		attr["fee"] = sa.fee.Uint64()
		attr["committee"] = sa.committee
		attr["delegation"] = daSDisplay(sa.delegation, height)
		ai := make(map[string]interface{})
		ai["fee"] = sa.modify.fee.Uint64()
		ai["votePubKey"] = hexutil.Bytes(sa.modify.votePubkey)
		attr["modify"] = ai
		attr["staking"] = sa.getAllStaking(height)
		attr["validStaking"] = sa.getValidStaking(height)
		attrs = append(attrs, attr)
		count = count + len(sa.delegation)
	}
	sasRPC["stakers"] = attrs
	sasRPC["stakerCount"] = len(sas)
	sasRPC["delegateCount"] = count
	return sasRPC
}

func (i *ImpawnImpl) GetStakingAssetRPC(addr common.Address) []map[string]interface{} {
	msv := i.GetStakingAsset(addr)
	var attrs []map[string]interface{}
	for key, value := range msv {
		attr := make(map[string]interface{})
		attr["stakingValue"] = stakingValueDisplay(value)
		attr["address"] = key
		attrs = append(attrs, attr)
	}
	return attrs
}

func (i *ImpawnImpl) GetLockedAssetRPC(addr common.Address, height uint64) []map[string]interface{} {
	ls := i.GetLockedAsset2(addr, height)
	var attrs []map[string]interface{}
	for key, value := range ls {
		attr := make(map[string]interface{})
		attr["lockValue"] = lockValueDisplay(value)
		attr["address"] = key.String()
		attrs = append(attrs, attr)
	}
	return attrs
}

func (i *ImpawnImpl) GetAllCancelableAssetRPC(addr common.Address) []map[string]interface{} {
	assets := i.GetAllCancelableAsset(addr)
	var attrs []map[string]interface{}
	for key, value := range assets {
		attr := make(map[string]interface{})
		attr["value"] = (*hexutil.Big)(value)
		attr["address"] = key.String()
		attrs = append(attrs, attr)
	}
	return attrs
}

func daSDisplay(das []*DelegationAccount, height uint64) map[string]interface{} {
	attrs := make(map[string]interface{}, len(das))
	for i, da := range das {
		attr := make(map[string]interface{})
		attr["saAddress"] = da.saAddress
		attr["delegate"] = da.getAllStaking(height)
		attr["validDelegate"] = da.getValidStaking(height)
		attr["unit"] = unitDisplay(da.unit)
		attrs[strconv.Itoa(i)] = attr
	}
	return attrs
}

func unitDisplay(uint *impawnUnit) map[string]interface{} {
	attr := make(map[string]interface{})
	attr["address"] = uint.address
	attr["value"] = pvSDisplay(uint.value)
	attr["redeemInfo"] = riSDisplay(uint.redeemInof)
	return attr
}

func pvSDisplay(pvs []*PairstakingValue) map[string]interface{} {
	attrs := make(map[string]interface{}, len(pvs))
	for i, pv := range pvs {
		attr := make(map[string]interface{})
		attr["amount"] = (*hexutil.Big)(pv.amount)
		attr["height"] = (*hexutil.Big)(pv.height)
		attr["state"] = uint64(pv.state)
		attrs[strconv.Itoa(i)] = attr
	}
	return attrs
}

func riSDisplay(ris []*RedeemItem) map[string]interface{} {
	attrs := make(map[string]interface{}, len(ris))
	for i, ri := range ris {
		attr := make(map[string]interface{})
		attr["amount"] = (*hexutil.Big)(ri.Amount)
		attr["epochID"] = ri.EpochID
		attr["state"] = uint64(ri.State)
		attrs[strconv.Itoa(i)] = attr
	}
	return attrs
}

func lockValueDisplay(lv *types.LockedValue) []map[string]interface{} {
	var attrs []map[string]interface{}
	for epoch, value := range lv.Value {
		attrs = append(attrs, map[string]interface{}{
			"epochID": epoch,
			"amount":  value.Amount,
			"locked":  value.Locked,
		})
	}
	return attrs
}

func stakingValueDisplay(sv *types.StakingValue) []map[string]interface{} {
	var attrs []map[string]interface{}
	for epoch, value := range sv.Value {
		attrs = append(attrs, map[string]interface{}{
			"epochID": epoch,
			"amount":  (*hexutil.Big)(value),
		})
	}
	return attrs
}
