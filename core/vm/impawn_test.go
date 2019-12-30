package vm

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/rlp"
	"math/big"
	"testing"
)

/////////////////////////////////////////////////////////////////////
func TestImpawnUnit(t *testing.T) {
	priKey, _ := crypto.GenerateKey()
	fmt.Printf("%x \n", crypto.FromECDSAPub(&priKey.PublicKey))
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)
	value := make([]*PairstakingValue, 0)
	Reward := make([]*RewardItem, 0)
	for i := 3; i > 0; i-- {
		pv := &PairstakingValue{
			amount: new(big.Int).SetUint64(10*uint64(i) + 10),
			height: new(big.Int).SetUint64(10*uint64(i) + 10),
			state:  StateStakingAuto,
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
			State:  StateStakingAuto,
		},
	}

	iunit := &impawnUnit{
		address: coinbase,
		value: append(make([]*PairstakingValue, 0), &PairstakingValue{
			amount: new(big.Int).SetUint64(15),
			height: new(big.Int).SetUint64(15),
			state:  StateStakingAuto,
		}),
		rewardInfo: append(make([]*RewardItem, 0), &RewardItem{
			Amount: new(big.Int).SetUint64(15),
			Height: new(big.Int).SetUint64(15),
		}),
		redeemInof: &RedeemItem{
			Amount: new(big.Int).SetUint64(15),
			Height: new(big.Int).SetUint64(45),
			State:  StateStakingAuto,
		},
	}

	iMunit.update(iunit)
	for i, value := range iMunit.value {
		fmt.Printf("%d %d %d %d \n", i, value.height, value.amount, value.state)
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
		state:  StateStakingAuto,
	}
	iMunit.value = append(iMunit.value, pv)
	iMunit.sort()

	for i, value := range iMunit.value {
		fmt.Printf("%d %d %d %d \n", i, value.height, value.amount, value.state)
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
	fmt.Printf("insertRedeemInfo redeemInof %d %v %d \n", iMunit.redeemInof.Amount, iMunit.redeemInof, value1)

	iMunit.clearRedeemed(value1)

	fmt.Printf("clearRedeemed redeemInof %d %v  \n", iMunit.redeemInof.Amount, iMunit.redeemInof)

	iMunit.merge(15)
	for i, value := range iMunit.value {
		fmt.Printf("merge %d %d %d  %d \n", i, value.height, value.amount, value.state)
	}
}

func TestStakingAccount(t *testing.T) {

	priKey, _ := crypto.GenerateKey()
	saAddress := crypto.PubkeyToAddress(priKey.PublicKey)
	priKeyDA, _ := crypto.GenerateKey()
	daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
	saccount := initialStakingAccount(3, 3, 100, saAddress, daAddress, priKey, priKeyDA)

	fmt.Println(saccount.getAllStaking(300), " ", saccount.getDA(daAddress).getAllStaking(300), " ", saccount.unit.getAllStaking(300))

	priKey1, _ := crypto.GenerateKey()
	saAddress1 := crypto.PubkeyToAddress(priKey1.PublicKey)
	priKeyDA1, _ := crypto.GenerateKey()
	daAddress1 := crypto.PubkeyToAddress(priKeyDA1.PublicKey)
	saccount1 := initialStakingAccount(3, 3, 100, saAddress1, daAddress1, priKey1, priKeyDA1)

	saccount.update(saccount1, 300, false)
	fmt.Println("Committee ", saccount.isInCommittee())
	saccount.insertRedeemInfo(new(big.Int).SetInt64(1000), new(big.Int).SetInt64(300))

	_, value1 := saccount.redeeming()

	saccount.clearRedeemed(value1)

	saccount.merge(300)
}

func initialStakingAccount(n int, m int, stride int, SAaddress common.Address, DAaddress common.Address, priKey *ecdsa.PrivateKey, priKeyDA *ecdsa.PrivateKey) *StakingAccount {
	das := []*DelegationAccount{}
	for k := 0; k < n; k++ {
		da := &DelegationAccount{
			deleAddress: SAaddress,
			unit:        initialImpawnUnit(m, stride, DAaddress),
		}
		das = append(das, da)
	}

	da := &DelegationAccount{
		deleAddress: SAaddress,
		unit:        initialImpawnUnit(m, stride-1, DAaddress),
	}
	das = append(das, da)

	saccount := &StakingAccount{
		unit:       initialImpawnUnit(m, stride, SAaddress),
		votepubkey: crypto.FromECDSAPub(&priKey.PublicKey),
		fee:        new(big.Int).SetUint64(1000),
		committee:  true,
		delegation: das,
		modify: &AlterableInfo{
			fee:        new(big.Int).SetUint64(1000),
			votePubkey: crypto.FromECDSAPub(&priKeyDA.PublicKey),
		},
	}
	return saccount
}

func initialImpawnUnit(n int, stride int, address common.Address) *impawnUnit {
	value := make([]*PairstakingValue, 0)
	Reward := make([]*RewardItem, 0)
	for i := 0; i < n; i++ {
		pv := &PairstakingValue{
			amount: new(big.Int).SetUint64(uint64(stride*i + stride)),
			height: new(big.Int).SetUint64(uint64(stride*i + stride)),
			state:  StateStakingAuto,
		}
		value = append(value, pv)
		ri := &RewardItem{
			Amount: new(big.Int).SetUint64(uint64(stride*i + stride)),
			Height: new(big.Int).SetUint64(uint64(stride*i + stride)),
		}
		Reward = append(Reward, ri)
	}
	iMunit := &impawnUnit{
		address:    address,
		value:      value,
		rewardInfo: Reward,
		redeemInof: &RedeemItem{
			Amount: new(big.Int).SetUint64(uint64(stride + 5)),
			Height: new(big.Int).SetUint64(uint64(stride + 35)),
			State:  StateStakingAuto,
		},
	}
	return iMunit
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

func TestRlpAlterableInfo(t *testing.T) {
	impl := makeAlterableInfo()
	bzs, err := rlp.EncodeToBytes(impl)
	if err != nil {
		fmt.Println(err.Error())
	}

	var tmp AlterableInfo
	err = rlp.DecodeBytes(bzs, &tmp)
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Printf("account %d %d \n", tmp.fee, tmp.votePubkey)

}

func makeAlterableInfo() *AlterableInfo {
	//priKey, _ := crypto.GenerateKey()

	modify := &AlterableInfo{
		fee:        nil,
		votePubkey: nil,
	}
	return modify
}

/////////////////////////////////////////////////////////////////////
