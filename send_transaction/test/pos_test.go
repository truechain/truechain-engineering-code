package test

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"os"
	"testing"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlWarn, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

///////////////////////////////////////////////////////////////////////
func TestOnlyDeposit(t *testing.T) {

	params.MinimumFruits = 60

	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header, config *params.ChainConfig) {

		if config.TIP8.FastNumber != nil && config.TIP8.FastNumber.Sign() > 0 {
			begin := config.TIP8.FastNumber.Uint64() - 1200
			snedTranction(number-begin, gen, fastChain, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

			sendDepositTransaction(number-begin, gen, saddr1, skey1, signer, fastChain, abiStaking, nil, header)
			sendCancelTransaction(number-begin, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
			sendWithdrawTransaction(number-begin, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
		}
	}
	manager := newTestPOSManager(520, executable)
	fmt.Println("BeginHeight ", types.GetEpochFromID(3).BeginHeight, " ")
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1), " StakingAddress ", manager.GetBalance(vm.StakingAddress), " ", types.ToTrue(manager.GetBalance(vm.StakingAddress)))
}

///////////////////////////////////////////////////////////////////////
func TestGetLockedAsset(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header, config *params.ChainConfig) {
		snedTranction(number, gen, fastChain, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, skey1, signer, fastChain, abiStaking, nil, header)
		sendCancelTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
		if number == 90 {
			stateDb := gen.GetStateDB()
			impawn := vm.NewImpawnImpl()
			impawn.Load(stateDb, vm.StakingAddress)
			arr := impawn.GetLockedAsset(saddr1)
			for addr, value := range arr {
				fmt.Println("value ", value.Value, " addr ", addr.String())
			}
		}

		sendWithdrawTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
	}
	manager := newTestPOSManager(55, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1))
}

///////////////////////////////////////////////////////////////////////
func TestDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, blockchain *core.BlockChain, header *types.Header, config *params.ChainConfig) {
		snedTranction(number, gen, blockchain, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)
		snedTranction(number, gen, blockchain, mAccount, daddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, skey1, signer, blockchain, abiStaking, nil, header)
		sendDelegateTransaction(number, gen, daddr1, saddr1, dkey1, signer, blockchain, abiStaking, nil, header)
		sendCancelTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, blockchain, abiStaking, nil, header)
		sendUnDelegateTransaction(number, gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, blockchain, abiStaking, nil, header)
		if number == 130 {
			stateDb := gen.GetStateDB()
			impawn := vm.NewImpawnImpl()
			impawn.Load(stateDb, vm.StakingAddress)
			arr := impawn.GetLockedAsset(saddr1)
			for addr, value := range arr {
				fmt.Println("value ", value.Value, " addr ", addr.String())
			}
			arr1 := impawn.GetLockedAsset(daddr1)
			for addr, value := range arr1 {
				fmt.Println("value D ", value.Value, " addr ", addr.String())
			}
		}
		sendWithdrawTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, blockchain, abiStaking, nil, header)
		sendWithdrawDelegateTransaction(number, gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, blockchain, abiStaking, nil, header)
	}
	manager := newTestPOSManager(55, executable)
	fmt.Println(" saddr1 ", types.ToTrue(manager.GetBalance(saddr1)), " StakingAddress ", manager.GetBalance(vm.StakingAddress), " ", types.ToTrue(manager.GetBalance(vm.StakingAddress)))
}
