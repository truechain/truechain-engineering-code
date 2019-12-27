package vm

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/rlp"
	"math/big"
	"testing"
)

/////////////////////////////////////////////////////////////////////

func TestImpawnUnit(t *testing.T) {
	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)
	value := make([]*PairstakingValue, 0)
	Reward := make([]*RewardItem, 0)
	for i := 0; i < 3; i++ {
		pv := &PairstakingValue{
			amount: new(big.Int).SetUint64(10*uint64(i) + 10),
			height: new(big.Int).SetUint64(10*uint64(i) + 10),
			state:  StateRedeem,
		}
		value = append(value, pv)
		ri := &RewardItem{
			Amount: new(big.Int).SetUint64(10*uint64(i) + 10),
			Height: new(big.Int).SetUint64(10*uint64(i) + 10),
		}
		Reward = append(Reward, ri)
	}
	iMunit := &impawnUnit{
		address:    coinbase,
		value:      value,
		rewardInfo: Reward,
		redeemInof: &RedeemItem{
			Amount: new(big.Int).SetUint64(15),
			Height: new(big.Int).SetUint64(45),
			State:  StateRedeem,
		},
	}

	iunit := &impawnUnit{
		address: coinbase,
		value: append(make([]*PairstakingValue, 0), &PairstakingValue{
			amount: new(big.Int).SetUint64(15),
			height: new(big.Int).SetUint64(15),
			state:  StateRedeem,
		}),
		rewardInfo: append(make([]*RewardItem, 0), &RewardItem{
			Amount: new(big.Int).SetUint64(15),
			Height: new(big.Int).SetUint64(15),
		}),
		redeemInof: &RedeemItem{
			Amount: new(big.Int).SetUint64(15),
			Height: new(big.Int).SetUint64(45),
			State:  StateRedeem,
		},
	}

	iMunit.update(iunit)
	for i, value := range iMunit.value {
		fmt.Printf("%d %d %d \n", i, value.height, value.amount)
	}
	for i, value := range iMunit.rewardInfo {
		fmt.Printf("rewardInfo %d %v \n", i, value)
	}
	fmt.Printf("redeemInof %d %v  %s \n", iMunit.redeemInof.Amount, iMunit.redeemInof, coinbase.String())

	fmt.Println(iMunit.getAllStaking(30))
	fmt.Println(iMunit.GetRewardAddress().String())

	pv := &PairstakingValue{
		amount: new(big.Int).SetUint64(25),
		height: new(big.Int).SetUint64(25),
		state:  StateRedeem,
	}
	iMunit.value = append(iMunit.value, pv)
	iMunit.sort()

	for i, value := range iMunit.value {
		fmt.Printf("%d %d %d \n", i, value.height, value.amount)
	}

	fmt.Println("isRedeemed ", iMunit.isRedeemed())
	iMunit.insertRedeemInfo(new(big.Int).SetInt64(30), new(big.Int).SetInt64(35))
	for i, value := range iMunit.value {
		fmt.Printf("%d %d %d %d \n", i, value.amount, value.height, value.state)
	}
	fmt.Printf("insertRedeemInfo redeemInof %d %v \n", iMunit.redeemInof.Amount, iMunit.redeemInof)

	_, value1 := iMunit.redeeming()

	for i, value := range iMunit.value {
		fmt.Printf("%d %d %d %d \n", i, value.height, value.amount, value.state)
	}
	fmt.Printf("insertRedeemInfo redeemInof %d %v   %d \n", iMunit.redeemInof.Amount, iMunit.redeemInof, value1)

	iMunit.clearRedeemed(value1)

	fmt.Printf("clearRedeemed redeemInof %d %v  \n", iMunit.redeemInof.Amount, iMunit.redeemInof)

	iMunit.merge(15)
	for i, value := range iMunit.value {
		fmt.Printf("merge %d %d %d  %d \n", i, value.height, value.amount, value.state)
	}
}

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
			fmt.Printf("account %d %d %v %d \n", m, n, account, account.fee)
		}
	}
	fmt.Printf("%v \n", tmp.curEpochID)
}

func makeImpawnImpl() *ImpawnImpl {
	accounts := make(map[uint64]SAImpawns)

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
					height: new(big.Int).SetUint64(1000),
					state:  StateRedeem,
				}),
				rewardInfo: append(make([]*RewardItem, 0), &RewardItem{
					Amount: new(big.Int).SetUint64(1000),
					Height: new(big.Int).SetUint64(1000),
				}),
				redeemInof: &RedeemItem{
					Amount: new(big.Int).SetUint64(1000),
					Height: new(big.Int).SetUint64(1000),
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
				fee:        new(big.Int).SetUint64(1000),
				committee:  true,
				delegation: das,
				modify: &AlterableInfo{
					fee:        new(big.Int).SetUint64(1000),
					votePubkey: crypto.FromECDSAPub(&priKey.PublicKey),
				},
			}
			sas = append(sas, saccount)
		}
		copy(accounts[uint64(i)], sas)
	}

	impl := &ImpawnImpl{
		curEpochID: 1000,
		accounts:   accounts,
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
