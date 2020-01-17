package test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
)

///////////////////////////////////////////////////////////////////////
func TestOnlyDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(4000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(3000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
	}
	manager := newTestPOSManager(101, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1), " StakingAddress ", manager.GetBalance(types.StakingAddress), " ", types.ToTrue(manager.GetBalance(types.StakingAddress)))
	fmt.Println("epoch ", types.GetEpochFromID(1), " ", types.GetEpochFromID(2), " ", types.GetEpochFromID(3), " ", types.GetEpochFromID(4), " ", types.GetEpochFromID(5))
	fmt.Println("epoch ", types.GetEpochFromID(2), " ", types.MinCalcRedeemHeight(2))
	//epoch  [id:1,begin:1,end:2000]   [id:2,begin:2001,end:4000]   [id:3,begin:4001,end:6000]
	//epoch  [id:2,begin:2001,end:4000]   5002
}

func TestCancelMoreDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(4000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(2000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight-60, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight-120, gen, saddr1, big.NewInt(3000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
	}
	manager := newTestPOSManager(101, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1), " StakingAddress ", manager.GetBalance(types.StakingAddress), " ", types.ToTrue(manager.GetBalance(types.StakingAddress)))
	fmt.Println("epoch ", types.GetEpochFromID(1), " ", types.GetEpochFromID(2), " ", types.GetEpochFromID(3))
	fmt.Println("epoch ", types.GetEpochFromID(2), " ", types.MinCalcRedeemHeight(2))
}

func TestWithdrawMoreDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(4000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(3000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2)-10, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2)-20, gen, saddr1, big.NewInt(2000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
	}
	manager := newTestPOSManager(101, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1), " StakingAddress ", manager.GetBalance(types.StakingAddress), " ", types.ToTrue(manager.GetBalance(types.StakingAddress)))
	fmt.Println("epoch ", types.GetEpochFromID(1), " ", types.GetEpochFromID(2), " ", types.GetEpochFromID(3))
	fmt.Println("epoch ", types.GetEpochFromID(2), " ", types.MinCalcRedeemHeight(2))
}
