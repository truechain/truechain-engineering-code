package vm

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"encoding/hex"
	"testing"
	"strings"
	"bytes"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/core/state"
)

/////////////////////////////////////////////////////////////////////

func TestImpawnImplDoElections(t *testing.T) {
	fmt.Println(" epoch 1 ", types.GetEpochFromID(1), " ", params.FirstNewEpochID)
	fmt.Println(" epoch 2 ", types.GetEpochFromID(2))
	fmt.Println(" epoch 3 ", types.GetEpochFromID(3))
	impl := NewImpawnImpl()

	for i := uint64(0); i < 6; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		if i % 2 == 0 {
			amount := new(big.Int).Mul(big.NewInt(20000),big.NewInt(1e18))
			impl.InsertSAccount2(0, from, pub, amount, big.NewInt(50), true)
		} else {
			impl.InsertSAccount2(0, from, pub, value, big.NewInt(50), true)
		}
	}

	_, err := impl.DoElections(1, 0)
	if err != nil {
		log.Error("ToFastBlock DoElections", "error", err)
	}
	err = impl.Shift(1,0)

	for i := uint64(0); i < 6; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(395+i, from, pub, value, big.NewInt(50), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(395+i, daAddress, from, value)
	}

	committee, _ := impl.DoElections(2, 400)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election ", len(impl.getElections3(1)))
	fmt.Println("election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))

	for i := uint64(0); i < 5; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(496+i, from, pub, value, big.NewInt(50), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(496+i, daAddress, from, value)
	}

	fmt.Println(impl.getCurrentEpochInfo(), " election 2 ", len(impl.getElections3(2)))
	impl.Shift(2,0)
	fmt.Println(" Shift 2 ", impl.getCurrentEpochInfo(), " election 3 ", len(impl.getElections3(3)))

	for i := uint64(0); i < 6; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(895+i, from, pub, value, big.NewInt(50), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(895+i, daAddress, from, value)
	}

	fmt.Println(" Shift 3 ", impl.getCurrentEpochInfo(), " election  2 ", len(impl.getElections3(2)), " 3 ", len(impl.getElections3(3)))
	committee, _ = impl.DoElections(3, 900)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))

	impl.Shift(3,0)
	fmt.Println(impl.getCurrentEpochInfo(), " election 3 ", len(impl.getElections3(4)))
}

func TestImpawnImplReward(t *testing.T) {
	fmt.Println(" epoch 1 ", types.GetEpochFromID(1), " ", params.FirstNewEpochID)
	fmt.Println(" epoch 2 ", types.GetEpochFromID(2))
	fmt.Println(" epoch 3 ", types.GetEpochFromID(3))
	impl := NewImpawnImpl()

	for i := uint64(0); i < 4; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(0, from, pub, value, big.NewInt(50), true)
	}

	_, err := impl.DoElections(1, 0)
	if err != nil {
		log.Error("ToFastBlock DoElections", "error", err)
	}
	err = impl.Shift(1,0)

	for i := uint64(0); i < 5; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(396+i, from, pub, value, big.NewInt(50), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(396+i, daAddress, from, value)
	}

	committee, _ := impl.DoElections(2, 400)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election 1 ", len(impl.getElections3(1)), "election 2 ", len(impl.getElections3(2)))

	fruits := make([]*types.SnailBlock, 0)
	for i := uint64(0); i < 60; i++ {
		sh := &types.SnailHeader{
			Number: big.NewInt(int64(341 + i)),
		}
		fruits = append(fruits, types.NewSnailBlockWithHeader(sh))
	}
	sh := &types.SnailHeader{
		Number: big.NewInt(int64(5)),
	}

	sblock := types.NewSnailBlock(sh, fruits, nil, nil, params.TestChainConfig)
	rinfo, _ := impl.Reward(sblock, big.NewInt(int64(100)),1)
	for i, info := range rinfo {
		fmt.Println("i ", i, " info ", len(info.Items), " rinfo ", len(rinfo))
		for j, item := range info.Items {
			fmt.Println("j ", j, " item ", item.Amount, " address ", item.Address)
		}
	}

	for i := uint64(0); i < 5; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(496+i, from, pub, value, big.NewInt(50), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(496+i, daAddress, from, value)
	}

	impl.Shift(2,0)
	fmt.Println(" Shift 2 ", impl.getCurrentEpochInfo(), " election 3 ", len(impl.getElections3(3)))

	for i := uint64(0); i < 6; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(895+i, from, pub, value, big.NewInt(50), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(895+i, daAddress, from, value)
	}

	fmt.Println(" Shift 3 ", " election  2 ", len(impl.getElections3(2)), " 3 ", len(impl.getElections3(3)))
	committee, _ = impl.DoElections(3, 900)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))

	impl.Shift(3,0)
	fmt.Println(impl.getCurrentEpochInfo(), " election 3 ", len(impl.getElections3(4)))
}

func TestImpawnImplRedeem(t *testing.T) {
	params.NewEpochLength = 5
	params.MaxRedeemHeight = 0
	params.ElectionPoint = 10
	params.DposForkPoint = 20
	fmt.Println(" epoch 1 ", types.GetEpochFromID(1), " ", params.FirstNewEpochID)
	fmt.Println(" epoch 2 ", types.GetEpochFromID(2))
	fmt.Println(" epoch 3 ", types.GetEpochFromID(3))
	impl := NewImpawnImpl()

	for i := uint64(0); i < params.NewEpochLength+1; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(20+i, from, pub, value, big.NewInt(50), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(20+i, daAddress, from, value)
	}

	committee, _ := impl.DoElections(1, 17)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election ", len(impl.getElections3(1)))
	fmt.Println("election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))

	impl.CancelDAccount(23, impl.accounts[1][3].Unit.Address, impl.accounts[1][3].Delegation[0].Unit.Address, big.NewInt(int64(70)))
	impl.CancelSAccount(23, impl.accounts[1][3].Unit.Address, big.NewInt(int64(70)))

	fruits := make([]*types.SnailBlock, 0)
	for i := uint64(0); i < params.NewEpochLength; i++ {
		sh := &types.SnailHeader{
			Number: big.NewInt(int64(28 + i)),
		}
		fruits = append(fruits, types.NewSnailBlockWithHeader(sh))
	}
	sh := &types.SnailHeader{
		Number: big.NewInt(int64(21)),
	}
	sblock := types.NewSnailBlock(sh, fruits, nil, nil, params.TestChainConfig)

	fmt.Println(" Shift 2 ", impl.getCurrentEpochInfo(), " election  2 ", len(impl.getElections3(2)), " 3 ", len(impl.getElections3(3)))
	committee, _ = impl.DoElections(2, 17)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))

	impl.Shift(2,0)

	impl.Reward(sblock, big.NewInt(int64(70)),1)

	impl.RedeemSAccount(29, impl.accounts[1][3].Unit.Address, big.NewInt(int64(70)))
	impl.RedeemDAccount(29, impl.accounts[1][3].Unit.Address, impl.accounts[1][3].Delegation[0].Unit.Address, big.NewInt(int64(70)))

	fmt.Println(" Shift 2 ", impl.getCurrentEpochInfo(), " election  2 ", len(impl.getElections3(2)), " 3 ", len(impl.getElections3(3)))
	committee, _ = impl.DoElections(3, 22)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))
	impl.Shift(3,0)

	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(impl.getElections3(1)), " election  2 ", len(impl.getElections3(2)))
	committee, _ = impl.DoElections(4, 27)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election ", len(impl.getElections3(1)))
	fmt.Println(" election ", len(impl.getElections3(1)), " election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))
	impl.Shift(4,0)
	fmt.Println(impl.getCurrentEpochInfo(), " committee ", len(committee), " election ", len(impl.getElections3(1)))
	fmt.Println(" election ", len(impl.getElections3(1)), " election 2 ", len(impl.getElections3(2)), " election 3 ", len(impl.getElections3(3)))

	for i := uint64(0); i < params.NewEpochLength; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		impl.InsertSAccount2(28+i, from, pub, value, big.NewInt(0), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(28+i, daAddress, from, value)
	}
	fmt.Println(impl.getCurrentEpochInfo())
}

func TestImpawnImpl(t *testing.T) {
	params.NewEpochLength = 5
	params.MaxRedeemHeight = 0
	params.ElectionPoint = 10
	params.DposForkPoint = 20
	fmt.Println(" epoch 1 ", types.GetEpochFromID(1))
	fmt.Println(" epoch 2 ", types.GetEpochFromID(2))
	impl := NewImpawnImpl()

	for i := uint64(0); i < params.NewEpochLength+1; i++ {
		value := big.NewInt(100)
		priKey, _ := crypto.GenerateKey()
		from := crypto.PubkeyToAddress(priKey.PublicKey)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		fmt.Println(" ", from.String())
		impl.InsertSAccount2(20+i, from, pub, new(big.Int).Sub(value, big.NewInt(int64(10*i))), big.NewInt(0), true)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		impl.InsertDAccount2(20+i, daAddress, from, new(big.Int).Sub(value, big.NewInt(int64(10*i))))
	}

	committee, _ := impl.DoElections(1, 17)
	fmt.Println(impl.getCurrentEpochInfo(), " ", len(committee))
}

func TestEpoch(t *testing.T) {
	params.DposForkPoint = 20
	fmt.Println(" first  ", types.GetFirstEpoch())
	fmt.Println(" epoch 2 ", types.GetEpochFromID(1))
	fmt.Println(" epoch 2 ", types.GetEpochFromID(2))
	fmt.Println(" epoch 2 ", types.GetEpochFromID(2))
	fmt.Println(" epoch 3 ", types.GetEpochFromID(3))
	fmt.Println(" epoch 4 ", types.GetEpochFromID(4))
	fmt.Println(types.GetEpochFromHeight(0))
	fmt.Println(types.GetEpochFromHeight(12000))
	fmt.Println(types.GetEpochFromHeight(12021))
	fmt.Println(types.GetEpochFromHeight(22021))
	fmt.Println(types.GetEpochFromHeight(32020))
	fmt.Println(types.GetEpochFromHeight(32021))
	fmt.Println(types.GetEpochFromHeight(42020))
	fmt.Println("GetEpochFromRange ", types.GetEpochFromRange(21, 12020))
	fmt.Println("GetEpochFromRange ", types.GetEpochFromRange(21, 12021))
	fmt.Println("GetEpochFromRange ", types.GetEpochFromRange(21, 22020))
	fmt.Println("GetEpochFromRange ", types.GetEpochFromRange(21, 22021))
}

// Underlying data structure
/////////////////////////////////////////////////////////////////////
func TestImpawnUnit(t *testing.T) {
	params.DposForkPoint = 1
	params.NewEpochLength = 50
	params.MaxRedeemHeight = 0
	fmt.Println(" epoch 1 ", types.GetEpochFromID(1))
	fmt.Println(" epoch 2 ", types.GetEpochFromID(2))
	priKey, _ := crypto.GenerateKey()
	fmt.Printf("%x \n", crypto.FromECDSAPub(&priKey.PublicKey))
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)
	value := make([]*PairstakingValue, 0)
	Reward := make([]*RewardItem, 0)
	for i := 3; i > 0; i-- {
		pv := &PairstakingValue{
			Amount: new(big.Int).SetUint64(10*uint64(i) + 10),
			Height: new(big.Int).SetUint64(10*uint64(i) + 60),
			State:  types.StateStakingAuto,
		}
		value = append(value, pv)
		ri := &RewardItem{
			Amount: new(big.Int).SetUint64(10*uint64(i) + 10),
			Height: new(big.Int).SetUint64(10*uint64(i) + 10),
		}
		Reward = append(Reward, ri)
	}
	iMunit := &impawnUnit{
		Address: coinbase,
		Value:   value,
		RedeemInof: append(make([]*RedeemItem, 0), &RedeemItem{
			Amount:  new(big.Int).SetUint64(15),
			EpochID: 1,
			State:   types.StateStakingAuto,
		}),
	}

	iunit := &impawnUnit{
		Address: coinbase,
		Value: append(make([]*PairstakingValue, 0), &PairstakingValue{
			Amount: new(big.Int).SetUint64(15),
			Height: new(big.Int).SetUint64(65),
			State:  types.StateStakingAuto,
		}),
		RedeemInof: append(make([]*RedeemItem, 0), &RedeemItem{
			Amount:  new(big.Int).SetUint64(15),
			EpochID: 1,
			State:   types.StateStakingAuto,
		}),
	}

	iMunit.update(iunit, true)
	for i, value := range iMunit.Value {
		fmt.Printf("%d %d %d %d \n", i, value.Height, value.Amount, value.State)
	}

	fmt.Println(iMunit.getAllStaking(90))
	fmt.Println(iMunit.GetRewardAddress().String())

	pv := &PairstakingValue{
		Amount: new(big.Int).SetUint64(25),
		Height: new(big.Int).SetUint64(60),
		State:  types.StateStakingAuto,
	}
	iMunit.Value = append(iMunit.Value, pv)
	iMunit.sort()

	for i, value := range iMunit.Value {
		fmt.Printf("%d %d %d %d \n", i, value.Height, value.Amount, value.State)
	}

	iMunit.stopStakingInfo(new(big.Int).SetInt64(30), new(big.Int).SetInt64(35))

	iMunit.merge(1, 100)

	_, value1, _ := iMunit.redeeming(90, new(big.Int).SetInt64(60))

	for i, value := range iMunit.Value {
		fmt.Printf("%d %d %d %d \n", i, value.Height, value.Amount, value.State)
	}
	fmt.Printf("insertRedeemInfo redeemInof %d %v %d \n", iMunit.RedeemInof[0].Amount, iMunit.RedeemInof, value1)

	iMunit.finishRedeemed()

	for i, value := range iMunit.Value {
		fmt.Printf("merge %d %d %d  %d \n", i, value.Height, value.Amount, value.State)
	}
}

func initialStakingAccount(n int, m int, stride int, SAaddress common.Address, DAaddress common.Address, priKey *ecdsa.PrivateKey, priKeyDA *ecdsa.PrivateKey) *StakingAccount {
	var das []*DelegationAccount
	for k := 0; k < n; k++ {
		da := &DelegationAccount{
			SaAddress: SAaddress,
			Unit:        initialImpawnUnit(m, stride, DAaddress),
		}
		das = append(das, da)
	}

	da := &DelegationAccount{
		SaAddress: SAaddress,
		Unit:        initialImpawnUnit(m, stride-5, DAaddress),
	}
	das = append(das, da)

	saccount := &StakingAccount{
		Unit:       initialImpawnUnit(m, stride, SAaddress),
		Votepubkey: crypto.FromECDSAPub(&priKey.PublicKey),
		Fee:        new(big.Int).SetUint64(1000),
		Committee:  true,
		Delegation: das,
		Modify: &AlterableInfo{
			Fee:        new(big.Int).SetUint64(1000),
			VotePubkey: crypto.FromECDSAPub(&priKeyDA.PublicKey),
		},
	}
	return saccount
}

func initialImpawnUnit(n int, stride int, address common.Address) *impawnUnit {
	value := make([]*PairstakingValue, 0)
	Reward := make([]*RewardItem, 0)
	for i := 0; i < n; i++ {
		pv := &PairstakingValue{
			Amount: new(big.Int).SetUint64(uint64(stride*i + stride)),
			Height: new(big.Int).SetUint64(uint64(stride*i + stride)),
			State:  types.StateStakingAuto,
		}
		value = append(value, pv)
		ri := &RewardItem{
			Amount: new(big.Int).SetUint64(uint64(stride*i + stride)),
			Height: new(big.Int).SetUint64(uint64(stride*i + stride)),
		}
		Reward = append(Reward, ri)
	}
	iMunit := &impawnUnit{
		Address: address,
		Value:   value,
		RedeemInof: append(make([]*RedeemItem, 0), &RedeemItem{
			Amount:  new(big.Int).SetUint64(1000),
			EpochID: 2,
			State:   types.StateRedeem,
		}),
	}
	return iMunit
}

func TestDelegationAccount(t *testing.T) {

	priKey, _ := crypto.GenerateKey()
	saAddress := crypto.PubkeyToAddress(priKey.PublicKey)
	priKeyDA, _ := crypto.GenerateKey()
	daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
	da := &DelegationAccount{
		SaAddress: saAddress,
		Unit:        initialImpawnUnit(3, 5, daAddress),
	}

	priKey1, _ := crypto.GenerateKey()
	saAddress1 := crypto.PubkeyToAddress(priKey1.PublicKey)
	priKeyDA1, _ := crypto.GenerateKey()
	daAddress1 := crypto.PubkeyToAddress(priKeyDA1.PublicKey)
	da1 := &DelegationAccount{
		SaAddress: saAddress1,
		Unit:        initialImpawnUnit(3, 4, daAddress1),
	}
	da.update(da1, false)
	da.stopStakingInfo(new(big.Int).SetInt64(30), new(big.Int).SetInt64(15))

	da.redeeming(90, new(big.Int).SetInt64(60))

	da.finishRedeemed()

	da.merge(1, 100)
}

func TestStakingAccount(t *testing.T) {

	priKey, _ := crypto.GenerateKey()
	saAddress := crypto.PubkeyToAddress(priKey.PublicKey)
	priKeyDA, _ := crypto.GenerateKey()
	daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
	saccount := initialStakingAccount(3, 1, 100, saAddress, daAddress, priKey, priKeyDA)

	fmt.Println(saccount.getAllStaking(300), " ", saccount.getDA(daAddress).getAllStaking(300), " ", saccount.Unit.getAllStaking(300))

	priKey1, _ := crypto.GenerateKey()
	saAddress1 := crypto.PubkeyToAddress(priKey1.PublicKey)
	priKeyDA1, _ := crypto.GenerateKey()
	daAddress1 := crypto.PubkeyToAddress(priKeyDA1.PublicKey)
	saccount1 := initialStakingAccount(3, 1, 50, saAddress1, daAddress1, priKey1, priKeyDA1)

	saccount.update(saccount1, 300, false, false)
	fmt.Println("Committee ", saccount.isInCommittee())
	saccount.stopStakingInfo(new(big.Int).SetInt64(1000), new(big.Int).SetInt64(300))

	saccount.redeeming(90, new(big.Int).SetInt64(60))

	saccount.finishRedeemed()

	saccount.merge(1, 300,0)
}

func TestSAImpawns(t *testing.T) {
	var sas []*StakingAccount

	sa := common.Address{}

	for i := 0; i < 3; i++ {
		priKey, _ := crypto.GenerateKey()
		saAddress := crypto.PubkeyToAddress(priKey.PublicKey)
		priKeyDA, _ := crypto.GenerateKey()
		daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
		saccount := initialStakingAccount(1, 1, 10, saAddress, daAddress, priKey, priKeyDA)
		sas = append(sas, saccount)
		sa = saAddress
	}
	SAIs := SAImpawns(sas)
	fmt.Println(" sa ", SAIs.getValidStaking(15), " all ", SAIs.getAllStaking(15), " da ", SAIs.getSA(sa))
	priKey, _ := crypto.GenerateKey()
	saAddress := crypto.PubkeyToAddress(priKey.PublicKey)
	priKeyDA, _ := crypto.GenerateKey()
	daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
	saccount := initialStakingAccount(1, 1, 9, saAddress, daAddress, priKey, priKeyDA)
	SAIs.update(saccount, 30, false, false,0)
	saccount = initialStakingAccount(1, 1, 15, saAddress, daAddress, priKey, priKeyDA)
	SAIs.update(saccount, 30, false, false,0)

	SAIs.sort(15, false)
}

// RLP
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
			fmt.Printf("account %d %d %v %d \n", m, n, account, account.Fee)
		}
	}
	fmt.Printf("%v \n", tmp.curEpochID)
}

func makeImpawnImpl() *ImpawnImpl {
	accounts := make(map[uint64]SAImpawns)

	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)

	for i := 0; i < types.MixEpochCount; i++ {

		accounts[uint64(i)] = make(SAImpawns, 3)
		var sas []*StakingAccount
		for j := 0; j < 3; j++ {
			unit := &impawnUnit{
				Address: coinbase,
				Value: append(make([]*PairstakingValue, 0), &PairstakingValue{
					Amount: new(big.Int).SetUint64(1000),
					Height: new(big.Int).SetUint64(1000),
					State:  types.StateRedeem,
				}),
				RedeemInof: append(make([]*RedeemItem, 0), &RedeemItem{
					Amount:  new(big.Int).SetUint64(1000),
					EpochID: 2,
					State:   types.StateRedeem,
				}),
			}
			das := []*DelegationAccount{}
			for k := 0; k < 3; k++ {
				da := &DelegationAccount{
					SaAddress: coinbase,
					Unit:        unit,
				}
				das = append(das, da)
			}

			saccount := &StakingAccount{
				Unit:       unit,
				Votepubkey: crypto.FromECDSAPub(&priKey.PublicKey),
				Fee:        new(big.Int).SetUint64(1000),
				Committee:  true,
				Delegation: das,
				Modify: &AlterableInfo{
					Fee:        new(big.Int).SetUint64(1000),
					VotePubkey: crypto.FromECDSAPub(&priKey.PublicKey),
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
		Address: coinbase,
		Value: append(make([]*PairstakingValue, 0), &PairstakingValue{
			Amount: new(big.Int).SetUint64(1000),
			Height: new(big.Int).SetUint64(101),
			State:  types.StateRedeem,
		}),
		RedeemInof: append(make([]*RedeemItem, 0), &RedeemItem{
			Amount:  new(big.Int).SetUint64(1000),
			EpochID: 2,
			State:   types.StateRedeem,
		}),
	}
	da := &DelegationAccount{
		SaAddress: coinbase,
		Unit:        iMunit,
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

	for i, ri := range tmp.Value {
		fmt.Printf("account %d %v \n", i, ri)
	}
}

func makeUnit() *impawnUnit {
	priKey, _ := crypto.GenerateKey()
	coinbase := crypto.PubkeyToAddress(priKey.PublicKey)
	iMunit := &impawnUnit{
		Address: coinbase,
		Value: append(make([]*PairstakingValue, 0), &PairstakingValue{
			Amount: new(big.Int).SetUint64(1000),
			Height: new(big.Int).SetUint64(101),
			State:  types.StateRedeem,
		}),
		RedeemInof: append(make([]*RedeemItem, 0), &RedeemItem{
			Amount:  new(big.Int).SetUint64(1000),
			EpochID: 2,
			State:   types.StateRedeem,
		}),
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

	fmt.Printf("account %d %d \n", tmp.Fee, tmp.VotePubkey)

}

func makeAlterableInfo() *AlterableInfo {
	priKey, _ := crypto.GenerateKey()

	modify := &AlterableInfo{
		Fee:        nil,
		VotePubkey: crypto.FromECDSAPub(&priKey.PublicKey),
	}
	return modify
}

/////////////////////////////////////////////////////////////////////
func TestCache(t *testing.T) {
	addr := common.Address{'1'}
	fmt.Println(addr)
	fmt.Println(addr.String())
	db := etruedb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	impawn := NewImpawnImpl()
	impawn.curEpochID,impawn.lastReward = 100,99
	impawn.Save(statedb, types.StakingAddress)
	impawn2 := NewImpawnImpl()
	impawn2.Load(statedb,types.StakingAddress)
}
func TestRlp(t *testing.T) {
	infos := types.ChainReward{
		Foundation: &types.RewardInfo{
			Address: 	common.Address{'1'},
			Amount:		big.NewInt(100),
		},
		CoinBase:&types.RewardInfo{
			Address: 	common.Address{'2'},
			Amount:		big.NewInt(200),
		},
		FruitBase: []*types.RewardInfo{
			&types.RewardInfo{
				Address: 	common.Address{'3'},
				Amount:		big.NewInt(300),
			},
			&types.RewardInfo{
				Address: 	common.Address{'4'},
				Amount:		big.NewInt(400),
			},
		},
		CommitteeBase: []*types.SARewardInfos{
			&types.SARewardInfos{
				Items:	[]*types.RewardInfo{
					&types.RewardInfo{
						Address: 	common.Address{'5'},
						Amount:		big.NewInt(500),
					},
					&types.RewardInfo{
						Address: 	common.Address{'6'},
						Amount:		big.NewInt(600),
					},
				},
			},
			&types.SARewardInfos{
				Items:	[]*types.RewardInfo{
					&types.RewardInfo{
						Address: 	common.Address{'7'},
						Amount:		big.NewInt(700),
					},
					&types.RewardInfo{
						Address: 	common.Address{'8'},
						Amount:		big.NewInt(800),
					},
				},
			},
			&types.SARewardInfos{
				Items:	[]*types.RewardInfo{
					&types.RewardInfo{
						Address: 	common.Address{'9'},
						Amount:		big.NewInt(900),
					},
					&types.RewardInfo{
						Address: 	common.Address{'a'},
						Amount:		big.NewInt(1000),
					},
				},
			},
		},
	}
	data, err := rlp.EncodeToBytes(infos)
	if err != nil {
		log.Crit("Failed to RLP encode reward infos", "err", err)
	}

	infos1 := &types.ChainReward{}
	if err := rlp.Decode(bytes.NewReader(data), infos1); err != nil {
		log.Error("Invalid reward infos RLP", "err", err)
	}
	fmt.Println("finish")
}
func TestFunc(t *testing.T) {
	step := 0
	fmt.Println("test insert sa in same address")
	// test_func(step)
	step = 1
	fmt.Println("test update fee sa in same address")
	// test_func(step)
	step = 2
	fmt.Println("test update pk sa in same address")
	test_func(step)
	fmt.Println("finish")
}
func test_func(step int) {
	impawn := NewImpawnImpl()
	effectHeight := uint64(10000)
	priKey, _ := crypto.GenerateKey()
	pk := crypto.FromECDSAPub(&priKey.PublicKey)
	fmt.Println("first insert:")
	err := impawn.InsertSAccount2(0,common.Address{'1'},pk,new(big.Int).Set(params.ElectionMinLimitForStaking),big.NewInt(10),true)
	if err != nil {
		fmt.Println("InsertSAccount1:",err)
		return
	} 
	print_sas(impawn.GetAllStakingAccount())
	err = impawn.InsertSAccount2(0,common.Address{'1'},pk,new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),big.NewInt(500),true)
	if err != nil {
		fmt.Println("InsertSAccount1:",err)
		return
	} 
	print_sas(impawn.GetAllStakingAccount())
	err = impawn.Shift(1,effectHeight)
	if err != nil {
		fmt.Println("Shift1:",err)
		return
	}
	fmt.Println("-----------Shift---------------")
	print_sas(impawn.GetAllStakingAccount())
	if step == 0 {
		fmt.Println("second insert:")
		err = impawn.InsertSAccount2(500,common.Address{'1'},pk,big.NewInt(100),big.NewInt(20),true)
		if err != nil {
			fmt.Println("InsertSAccount2:",err)
			return
		}
	} else if step == 1 {
		fmt.Println("update fee:")
		err = impawn.UpdateSAFee(500,common.Address{'1'},big.NewInt(20))
		if err != nil {
			fmt.Println("UpdateSAFee:",err)
			return
		}
	} else {
		fmt.Println("update pk:")
		priKey1, _ := crypto.GenerateKey()
		pk1 := crypto.FromECDSAPub(&priKey1.PublicKey)
		err = impawn.UpdateSAPK(500,common.Address{'1'},pk1)
		if err != nil {
			fmt.Println("UpdateSAFee:",err)
			return
		}
	}
	print_sas(impawn.GetAllStakingAccount())
	print_election(impawn,2)
	err = impawn.Shift(2,effectHeight)
	if err != nil {
		fmt.Println("Shift2:",err)
		return
	}
	fmt.Println("-----------Shift---------------")
	eid := impawn.getCurrentEpoch()
	accs := impawn.getElections3(eid)
	var vv []*types.CommitteeMember
	for _, v := range accs {
		pubkey, _ := crypto.UnmarshalPubkey(v.Votepubkey)
		vv = append(vv, &types.CommitteeMember{
			CommitteeBase: crypto.PubkeyToAddress(*pubkey),
			Coinbase:      v.Unit.GetRewardAddress(),
			Publickey:     types.CopyVotePk(v.Votepubkey),
			Flag:          types.StateUsedFlag,
			MType:         types.TypeWorked,
		})
	}
	fmt.Println("accs:",accs)
	print_sas(impawn.GetAllStakingAccount())
}
func make_sas(impawn *ImpawnImpl,h uint64,addrs []common.Address,pk[][]byte,values []*big.Int) error{
	l := len(addrs)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.InsertSAccount2(h,addrs[i],pk[i],values[i],big.NewInt(10),true)
		if err != nil {
			fmt.Println("InsertSAccount1:",err,"index:",i)
			return err
		}
	}
	return nil
}
func make_das(impawn *ImpawnImpl,h uint64,saAddrs,daAddrs[]common.Address,values []*big.Int) error {
	l := len(values)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.InsertDAccount2(h,saAddrs[i],daAddrs[i],values[i])
		if err != nil {
			fmt.Println("InsertDAccount1:",err)
			return err
		}
	}
	return nil
}
func make_cancel_das(impawn *ImpawnImpl,h uint64,saAddrs,daAddrs[]common.Address,values []*big.Int) error {
	l := len(values)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.CancelDAccount(h,saAddrs[i],daAddrs[i],values[i])
		if err != nil {
			fmt.Println("CancelDAccount:",err)
			return err
		}
	}
	return nil
}
func make_cancel_sas(impawn *ImpawnImpl,h uint64,saAddrs []common.Address,values []*big.Int) error {
	l := len(values)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.CancelSAccount(h,saAddrs[i],values[i])
		if err != nil {
			fmt.Println("CancelSAccount:",err)
			return err
		}
	}
	return nil
}
func make_redeem_das(impawn *ImpawnImpl,h uint64,saAddrs,daAddrs[]common.Address,values []*big.Int) error {
	l := len(values)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.RedeemDAccount(h,saAddrs[i],daAddrs[i],values[i])
		if err != nil {
			fmt.Println("RedeemDAccount:",err)
			return err
		}
	}
	return nil
}
func make_redeem_sas(impawn *ImpawnImpl,h uint64,saAddrs []common.Address,values []*big.Int) error {
	l := len(values)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.RedeemSAccount(h,saAddrs[i],values[i])
		if err != nil {
			fmt.Println("RedeemSAccount:",err)
			return err
		}
	}
	return nil
}
func make_append(impawn *ImpawnImpl,h uint64,addrs []common.Address,values []*big.Int) error {
	l := len(addrs)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.AppendSAAmount(h,addrs[i],values[i])
		if err != nil {
			fmt.Println("AppendSAAmount:",err,"index:",i)
			return err
		}
	}
	return nil
}
func make_update_fee(impawn *ImpawnImpl,h uint64,addrs []common.Address,fees []*big.Int) error {
	l := len(addrs)
	var err error
	for i:=0;i<l;i++ {
		err = impawn.UpdateSAFee(h,addrs[i],fees[i])
		if err != nil {
			fmt.Println("UpdateSAFee:",err,"index:",i)
			return err
		}
	}
	return nil
}
func getPks(n int) [][]byte {
	res := make([][]byte,0,0)
	for i:=0;i<n;i++ {
		priKey, _ := crypto.GenerateKey()
		pk := crypto.FromECDSAPub(&priKey.PublicKey)
		res = append(res,pk)
	}
	return res
}
func print_reward(impawn *ImpawnImpl,b,e,effectid uint64,rewardAmount *big.Int) {
	fmt.Println("reward [",b,e,types.ToTrue(rewardAmount).Text('f',8),"]")
	res,err2 := impawn.Reward2(b,e,effectid,rewardAmount)
	if err2 != nil {
		fmt.Println("Reward2:",err2)
		return
	}
	fmt.Println("rewardinfo:",res)
}
func print_election(impawn *ImpawnImpl,id uint64) {
	e := types.GetEpochFromID(id-1)
	info,err := impawn.DoElections(id,e.EndHeight-params.ElectionPoint)
	if err != nil {
		fmt.Println("DoElections:",err)
	} else {
		fmt.Println("election id:",id)
		print_sas(info)
	}
}
func TestFetch(t *testing.T) {
	impawn := NewImpawnImpl()
	effectHeight,effectid := uint64(10000),uint64(1)
	rewardAmount := new(big.Int).Mul(big.NewInt(60), big.NewInt(1e18))
	pks := getPks(4)
	saAddrs := []common.Address{
		common.Address{'1'},
		common.Address{'2'},
		common.Address{'3'},
		common.Address{'4'},
	}
	values := []*big.Int{
		new(big.Int).Set(params.ElectionMinLimitForStaking),
		new(big.Int).Set(params.ElectionMinLimitForStaking),
		new(big.Int).Set(params.ElectionMinLimitForStaking),
		big.NewInt(500000),
	}
	err := make_sas(impawn,0,saAddrs,pks,values)
	if err != nil {
		fmt.Println("make_sas:",err)
		return
	}
	acc1,err1 := impawn.DoElections(1,1)
	if err1 != nil {
		fmt.Println("DoElections:",err)
		return
	}
	print_sas(acc1)
	err = impawn.Shift(1,effectHeight)
	if err != nil {
		fmt.Println("Shift1:",err)
		return
	}
	daAddrs := []common.Address{
		common.Address{'8'},
		common.Address{'9'},
	}
	values2 := []*big.Int{
		new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	}
	err = make_das(impawn,10000,saAddrs[0:2],daAddrs,values2)
	if err != nil {
		fmt.Println("make_das:",err)
		return
	}
	print_sas(impawn.GetAllStakingAccount())
	print_reward(impawn,100,10001,effectid,rewardAmount)
	fmt.Println("cancel das")
	values3 := []*big.Int{
		new(big.Int).Mul(big.NewInt(5000), big.NewInt(1e18)),
		new(big.Int).Mul(big.NewInt(5000), big.NewInt(1e18)),
	}
	make_cancel_das(impawn,24890,saAddrs[0:2],daAddrs,values3)
	fmt.Println("update fee")
	fee2 := []*big.Int{
		big.NewInt(5000),
		big.NewInt(5000),
	}
	make_update_fee(impawn,24899,saAddrs[0:2],fee2)
	// fmt.Println("cancel sas")
	// calcelvalues := []*big.Int{
	// 	new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	// 	new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	// 	new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	// }
	// make_cancel_sas(impawn,24900,saAddrs[0:3],calcelvalues)
	print_sas(impawn.GetAllStakingAccount())
	// fmt.Println("append sas")
	// appendvalues := []*big.Int{
	// 	new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	// 	new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	// 	new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	// 	new(big.Int).Mul(big.NewInt(30000), big.NewInt(1e18)),
	// }
	// make_append(impawn,24692,saAddrs,appendvalues)
	// print_sas(impawn.GetAllStakingAccount())
	print_election(impawn,2)
	fmt.Println("cancel sas")
	calcelvalues := []*big.Int{
		new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	}
	make_cancel_sas(impawn,24900,saAddrs[0:3],calcelvalues)
	err = impawn.Shift(2,effectHeight)
	if err != nil {
		fmt.Println("Shift1:",err)
		return
	}
	print_reward(impawn,10000,30000,effectid,rewardAmount)
	
	print_sas(impawn.GetAllStakingAccount())
	print_reward(impawn,45001,70000,effectid,rewardAmount)
	fmt.Println()
}
func TestClear(t *testing.T) {
	params.MaxRedeemHeight = uint64(5000)
	params.NewEpochLength = uint64(10000)
	impawn := NewImpawnImpl()
	effectHeight := uint64(2000)
	effectid := uint64(1)
	rewardAmount := new(big.Int).Mul(big.NewInt(60), big.NewInt(1e18))
	pks := getPks(4)
	saAddrs := []common.Address{
		common.Address{'1'},
		common.Address{'2'},
		common.Address{'3'},
		common.Address{'4'},
	}
	values := []*big.Int{
		new(big.Int).Set(params.ElectionMinLimitForStaking),
		new(big.Int).Set(params.ElectionMinLimitForStaking),
		new(big.Int).Set(params.ElectionMinLimitForStaking),
		big.NewInt(500000),
	}
	err := make_sas(impawn,0,saAddrs,pks,values)
	if err != nil {
		fmt.Println("make_sas:",err)
		return
	}
	acc1,err1 := impawn.DoElections(1,1)
	if err1 != nil {
		fmt.Println("DoElections:",err)
		return
	}
	print_sas(acc1)
	err = impawn.Shift(1,effectHeight)
	if err != nil {
		fmt.Println("Shift1:",err)
		return
	}
	daAddrs := []common.Address{
		common.Address{'8'},
		common.Address{'9'},
	}
	values2 := []*big.Int{
		new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
		new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18)),
	}
	err = make_das(impawn,5000,saAddrs[0:2],daAddrs,values2)
	if err != nil {
		fmt.Println("make_das:",err)
		return
	}
	print_sas(impawn.GetAllStakingAccount())
	print_reward(impawn,100,5001,effectid,rewardAmount)
	fmt.Println("cancel das")
	make_cancel_das(impawn,8000,saAddrs[0:2],daAddrs,values2)
	fmt.Println("cancel sas")
	make_cancel_sas(impawn,8001,saAddrs[0:1],values[0:1])
	print_sas(impawn.GetAllStakingAccount())
	fmt.Println("Shift2.................")
	print_election(impawn,2)
	err = impawn.Shift(2,effectHeight)
	if err != nil {
		fmt.Println("Shift1:",err)
		return
	}
	print_reward(impawn,10001,15000,effectid,rewardAmount)
	fmt.Println("redeem das")
	make_redeem_das(impawn,15002,saAddrs[0:2],daAddrs,values2)
	fmt.Println("redeem sas")
	make_redeem_sas(impawn,15002,saAddrs[0:1],values[0:1])
	print_sas(impawn.GetAllStakingAccount())

	fmt.Println("Shift3..................")
	print_election(impawn,3)
	err = impawn.Shift(3,effectHeight)
	if err != nil {
		fmt.Println("Shift1:",err)
		return
	}
	print_sas(impawn.GetAllStakingAccount())
	print_reward(impawn,45001,70000,effectid,rewardAmount)
	fmt.Println()
}
func print_sas(sas SAImpawns) {
	sasStrings := make([]string, len(sas))
	for i,v := range sas {
		sasStrings[i] = sa_to_string(v)
	}
	fmt.Println("sas:",strings.Join(sasStrings, "\n"))
}
func sa_to_string(sa *StakingAccount) string {
	dasStrings := make([]string, len(sa.Delegation))
	for i, m := range sa.Delegation {
		dasStrings[i] = da_to_string(m)
	}
	return fmt.Sprintf("SA{Unit:%s,pk:{%s},fee:{%s},Committee:%v,das:{%s},Modify:{%s,%s}}",
	unit_to_string(sa.Unit), hex.EncodeToString(sa.Votepubkey),sa.Fee.String(),sa.Committee,
	strings.Join(dasStrings, "\n"),sa.Modify.Fee.String(),hex.EncodeToString(sa.Modify.VotePubkey))
}
func da_to_string(da *DelegationAccount) string {
	return fmt.Sprintf("DA{SaAddr:%s,{%s}}", da.SaAddress.String(), unit_to_string(da.Unit))
}
func unit_to_string(u *impawnUnit) string {
	valstring := make([]string, len(u.Value))
	redeemString := make([]string, len(u.RedeemInof))
	for i, v := range u.Value {
		valstring[i] = fmt.Sprintf("Amount:{%s},Height:{%s}",v.Amount.String(),v.Height.String())
	}
	for i, v := range u.RedeemInof {
		redeemString[i] = fmt.Sprintf("Amount:{%s},EpochID:{%v}",v.Amount.String(),v.EpochID)
	}
	return fmt.Sprintf("Unit{Addr:%s,Values:{%s},RedeemInof:{%s}}", 
	u.Address.String(), strings.Join(valstring, "\n  "),strings.Join(redeemString, "\n  "))
}