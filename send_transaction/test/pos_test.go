package test

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/log"
	"math/big"
	"os"
	"testing"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlWarn, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

///////////////////////////////////////////////////////////////////////
func TestOnlyDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header) {
		snedTranction(number, gen, fastChain, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, skey1, signer, fastChain, abiStaking, nil, header)
		sendCancelTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
		sendWithdrawTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
	}
	manager := newTestPOSManager(40, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1))
}

///////////////////////////////////////////////////////////////////////
func TestGetLockedAsset(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header) {
		snedTranction(number, gen, fastChain, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, skey1, signer, fastChain, abiStaking, nil, header)
		sendCancelTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
		if number == 90 {
			stateDb := gen.GetStateDB()
			impawn := vm.NewImpawnImpl()
			impawn.Load(stateDb, vm.StakingAddress)
			arr := impawn.GetLockedAsset(daddr1)
			for addr, value := range arr {
				fmt.Println("value ", value.Value, " addr ", addr.String())
			}
		}

		sendWithdrawTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, fastChain, abiStaking, nil, header)
	}
	manager := newTestPOSManager(40, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1))
}

///////////////////////////////////////////////////////////////////////
func TestDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, blockchain *core.BlockChain, header *types.Header) {
		snedTranction(number, gen, blockchain, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)
		snedTranction(number, gen, blockchain, mAccount, daddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, skey1, signer, blockchain, abiStaking, nil, header)
		sendDelegateTransaction(number, gen, daddr1, saddr1, dkey1, signer, blockchain, abiStaking, nil, header)
		sendCancelTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, blockchain, abiStaking, nil, header)
		sendUnDelegateTransaction(number, gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, blockchain, abiStaking, nil, header)
		sendWithdrawTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, blockchain, abiStaking, nil, header)
		sendWithdrawDelegateTransaction(number, gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, blockchain, abiStaking, nil, header)
	}
	newTestPOSManager(40, executable)
}
