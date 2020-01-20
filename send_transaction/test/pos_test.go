package test

import (
	"fmt"
	"math/big"
	"os"
	"testing"

	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

///////////////////////////////////////////////////////////////////////
func TestGetLockedAsset(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, fastChain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
		if number == 90 {
			stateDb := gen.GetStateDB()
			impawn := vm.NewImpawnImpl()
			impawn.Load(stateDb, types.StakingAddress)
			arr := impawn.GetLockedAsset(saddr1)
			for addr, value := range arr {
				fmt.Println("value ", value.Value, " addr ", addr.String())
			}
		}

		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, fastChain, abiStaking, nil)
	}
	manager := newTestPOSManager(55, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1))
	//epoch  [id:1,begin:1,end:2000]   [id:2,begin:2001,end:4000]   [id:3,begin:4001,end:6000]
	//epoch  [id:2,begin:2001,end:4000]   5002
}

///////////////////////////////////////////////////////////////////////
func TestDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, blockchain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)
		sendTranction(number, gen, statedb, mAccount, daddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendDelegateTransaction(number-60, gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendUnDelegateTransaction(number-types.GetEpochFromID(2).BeginHeight-10, gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		if number == 130 {
			stateDb := gen.GetStateDB()
			impawn := vm.NewImpawnImpl()
			impawn.Load(stateDb, types.StakingAddress)
			arr := impawn.GetLockedAsset(saddr1)
			for addr, value := range arr {
				fmt.Println("value ", value.Value, " addr ", addr.String())
			}
			arr1 := impawn.GetLockedAsset(daddr1)
			for addr, value := range arr1 {
				fmt.Println("value D ", value.Value, " addr ", addr.String())
			}
		}
		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendWithdrawDelegateTransaction(number-types.MinCalcRedeemHeight(2)-10, gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
	}
	manager := newTestPOSManager(55, executable)
	fmt.Println(" saddr1 ", types.ToTrue(manager.GetBalance(saddr1)), " StakingAddress ", manager.GetBalance(types.StakingAddress), " ", types.ToTrue(manager.GetBalance(types.StakingAddress)))
}

///////////////////////////////////////////////////////////////////////
func TestSendTX(t *testing.T) {
	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)
	gspec.Config.TIP8 = &params.BlockConfig{CID: big.NewInt(-1)}
	gspec.Config.TIP9 = &params.BlockConfig{SnailNumber: big.NewInt(20)}

	genesis := gspec.MustFastCommit(db)
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{})
	chain, _ := core.GenerateChain(gspec.Config, genesis, engine, db, 20000, func(i int, gen *core.BlockGen) {
		header := gen.GetHeader()
		statedb := gen.GetStateDB()
		if i > 60 {
			SendTX(header, true, blockchain, nil, gspec.Config, gen, statedb, nil)
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		panic(err)
	}
}
