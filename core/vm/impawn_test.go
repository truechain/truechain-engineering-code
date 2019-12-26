package vm

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/rlp"
	"math/big"
	"testing"
)

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
