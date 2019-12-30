package vm

import (
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"testing"
)

func TestDeposit(t *testing.T) {

	priKey, _ := crypto.GenerateKey()
	from := crypto.PubkeyToAddress(priKey.PublicKey)
	pub := crypto.FromECDSAPub(&priKey.PublicKey)
	value := big.NewInt(1000)

	statedb, _ := state.New(common.Hash{}, state.NewDatabase(etruedb.NewMemDatabase()))
	statedb.GetOrNewStateObject(StakingAddress)
	evm := NewEVM(Context{}, statedb, params.TestChainConfig, Config{})

	log.Info("Staking deposit", "address", from, "value", value)
	impawn := NewImpawnImpl()
	impawn.Load(evm.StateDB, StakingAddress)

	impawn.InsertSAccount2(1000, from, pub, value, big.NewInt(0), true)
	impawn.Save(evm.StateDB, StakingAddress)
}
