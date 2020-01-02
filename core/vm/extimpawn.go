package vm

import (
	"io"
	"math/big"

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
	d.deleAddress, d.unit = da.DeleAddress, da.Unit
	return nil
}

// EncodeRLP serializes b into the truechain RLP DelegationAccount format.
func (i *DelegationAccount) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extDAccount{
		DeleAddress: i.deleAddress,
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
	Accounts   []*SAImpawns
	CurEpochID uint64
	Array      []uint64
}

func (i *ImpawnImpl) DecodeRLP(s *rlp.Stream) error {
	var ei extImpawnImpl
	if err := s.Decode(&ei); err != nil {
		return err
	}
	accounts := make(map[uint64]SAImpawns)
	for i, account := range ei.Accounts {
		accounts[ei.Array[i]] = *account
	}

	i.curEpochID, i.accounts = ei.CurEpochID, accounts
	return nil
}

// EncodeRLP serializes b into the truechain RLP ImpawnImpl format.
func (i *ImpawnImpl) EncodeRLP(w io.Writer) error {
	var accounts []*SAImpawns
	var arr []uint64
	for i, account := range i.accounts {
		arr = append(arr, i)
		accounts = append(accounts, &account)
	}
	for m := 0; m < len(i.accounts)-1; m++ {
		for n := 0; n < len(i.accounts)-1-m; n++ {
			if arr[n] > arr[n+1] {
				arr[n], arr[n+1] = arr[n+1], arr[n]
			}
		}
	}
	return rlp.Encode(w, extImpawnImpl{
		CurEpochID: i.curEpochID,
		Accounts:   accounts,
		Array:      arr,
	})
}
