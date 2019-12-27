package vm

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/rlp"
	"math/big"
	"testing"
)

//RLP
/////////////////////////////////////////////////////////////////////
func TestRlpImpawnImpl(t *testing.T) {
	impl := makeImpawnImpl()
	bzs, err := rlp.EncodeToBytes(impl)
	if err != nil {
		fmt.Println(err.Error())
	}

	var tmp ImpawnImpl
	err = rlp.DecodeBytes(bzs, &tmp)
	if err != nil {
		fmt.Println(err.Error())
	}

	for m, sas := range tmp.accounts {
		for n, account := range sas {
			fmt.Printf("account %d %d %v %f \n", m, n, account, account.fee)
		}
	}
	fmt.Printf("%v \n", tmp.epochInfo)
}

func makeImpawnImpl() *ImpawnImpl {
	epochInfos := make([]*EpochIDInfo, MixEpochCount)
	accounts := make(map[uint64]SAImpawns)
	for i := range epochInfos {
		epochInfos[i] = &EpochIDInfo{
			EpochID:     uint64(i),
			BeginHeight: uint64(i*60 + 1),
			EndHeight:   uint64(i*60 + 60),
		}
	}

	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)

	for i := 0; i < MixEpochCount; i++ {

		accounts[uint64(i)] = make(SAImpawns, 3)
		sas := []*StakingAccount{}
		for j := 0; j < 3; j++ {
			unit := &impawnUnit{
				address: coinbase,
				value: append(make([]*PairstakingValue, 0), &PairstakingValue{
					amount: new(big.Int).SetUint64(1000),
					height: new(big.Int).SetUint64(epochInfos[i].BeginHeight),
					state:  StateRedeem,
				}),
				rewardInfo: append(make([]*RewardItem, 0), &RewardItem{
					Amount: new(big.Int).SetUint64(1000),
					Height: new(big.Int).SetUint64(epochInfos[i].BeginHeight),
				}),
				redeemInof: &RedeemItem{
					Amount: new(big.Int).SetUint64(1000),
					Height: new(big.Int).SetUint64(epochInfos[i].BeginHeight),
					State:  StateRedeem,
				},
			}
			das := []*DelegationAccount{}
			for k := 0; k < 3; k++ {
				da := &DelegationAccount{
					deleAddress: coinbase,
					unit:        unit,
				}
				das = append(das, da)
			}

			saccount := &StakingAccount{
				unit:       unit,
				votepubkey: crypto.FromECDSAPub(&priKey.PublicKey),
				fee:        new(big.Float).SetFloat64(float64(baseUnit.Int64())),
				committee:  true,
				delegation: das,
				modify: &AlterableInfo{
					fee:        new(big.Float).SetFloat64(float64(baseUnit.Int64())),
					votePubkey: crypto.FromECDSAPub(&priKey.PublicKey),
				},
			}
			sas = append(sas, saccount)
		}
		copy(accounts[uint64(i)], sas)
	}

	impl := &ImpawnImpl{
		epochInfo: epochInfos,
		accounts:  accounts,
	}
	return impl
}

func TestRlpDelegationAccount(t *testing.T) {

	mint := makeDelegationAccount()
	bzs, err := rlp.EncodeToBytes(mint)
	if err != nil {
		fmt.Println(err.Error())
	}

	var tmp DelegationAccount
	err = rlp.DecodeBytes(bzs, &tmp)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func makeDelegationAccount() *DelegationAccount {
	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)
	iMunit := &impawnUnit{
		address: coinbase,
		value: append(make([]*PairstakingValue, 0), &PairstakingValue{
			amount: new(big.Int).SetUint64(1000),
			height: new(big.Int).SetUint64(101),
			state:  StateRedeem,
		}),
		rewardInfo: append(make([]*RewardItem, 0), &RewardItem{
			Amount: new(big.Int).SetUint64(1000),
			Height: new(big.Int).SetUint64(101),
		}),
		redeemInof: &RedeemItem{
			Amount: new(big.Int).SetUint64(1000),
			Height: new(big.Int).SetUint64(101),
			State:  StateRedeem,
		},
	}
	da := &DelegationAccount{
		deleAddress: coinbase,
		unit:        iMunit,
	}
	return da
}

type SAccount struct {
	Fee    *big.Float
	Amount *big.Int
}

type DAccount struct {
	fee    *big.Float
	Amount *big.Int
}

type sAccount struct {
	Fee    *big.Float
	Amount *big.Int
}

func TestRlpAccount(t *testing.T) {
	SA := &SAccount{
		Fee:    new(big.Float).SetFloat64(101.1),
		Amount: new(big.Int).SetUint64(1000),
	}
	bzs, err := rlp.EncodeToBytes(SA)
	if err != nil {
		fmt.Println(err.Error())
	}

	var tmp SAccount
	err = rlp.DecodeBytes(bzs, &tmp)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("11 ", tmp.Fee, " ", tmp.Amount)

	DA := &DAccount{
		fee:    new(big.Float).SetFloat64(101.1),
		Amount: new(big.Int).SetUint64(1000),
	}
	dzs, err := rlp.EncodeToBytes(DA)
	if err != nil {
		fmt.Println(err.Error())
	}

	var dmp DAccount
	err = rlp.DecodeBytes(dzs, &dmp)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("22 ", dmp.fee, " ", dmp.Amount)

	sA := &sAccount{
		Fee:    new(big.Float).SetFloat64(101.1),
		Amount: new(big.Int).SetUint64(1000),
	}
	szs, err := rlp.EncodeToBytes(sA)
	if err != nil {
		fmt.Println(err.Error())
	}

	var smp sAccount
	err = rlp.DecodeBytes(szs, &smp)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("33 ", tmp.Fee, " ", smp.Amount)
}

func TestRlpimpawnUnit(t *testing.T) {

	mint := makeUnit()
	bzs, err := rlp.EncodeToBytes(mint)
	if err != nil {
		fmt.Println(err.Error())
	}

	var tmp impawnUnit
	err = rlp.DecodeBytes(bzs, &tmp)
	if err != nil {
		fmt.Println(err.Error())
	}

	for i, ri := range tmp.rewardInfo {
		fmt.Printf("account %d %v \n", i, ri)
	}
	for i, ri := range tmp.value {
		fmt.Printf("account %d %v \n", i, ri)
	}
}

func makeUnit() *impawnUnit {
	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)
	iMunit := &impawnUnit{
		address: coinbase,
		value: append(make([]*PairstakingValue, 0), &PairstakingValue{
			amount: new(big.Int).SetUint64(1000),
			height: new(big.Int).SetUint64(101),
			state:  StateRedeem,
		}),
		rewardInfo: append(make([]*RewardItem, 0), &RewardItem{
			Amount: new(big.Int).SetUint64(1000),
			Height: new(big.Int).SetUint64(101),
		}),
		redeemInof: &RedeemItem{
			Amount: new(big.Int).SetUint64(1000),
			Height: new(big.Int).SetUint64(101),
			State:  StateRedeem,
		},
	}
	return iMunit
}

/////////////////////////////////////////////////////////////////////
