package test

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
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
	executable := func(number uint64, gen *core.BlockGen, blockchain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		if number == 90 {
			stateDb := gen.GetStateDB()
			impawn := vm.NewImpawnImpl()
			impawn.Load(stateDb, types.StakingAddress)
			arr := impawn.GetLockedAsset(saddr1)
			for addr, value := range arr {
				fmt.Println("value ", value.Value, " addr ", addr.String())
			}
		}

		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
	}
	manager := newTestPOSManager(55, executable)
	fmt.Println(" saddr1 ", manager.GetBalance(saddr1))
	//epoch  [id:1,begin:1,end:2000]   [id:2,begin:2001,end:4000]   [id:3,begin:4001,end:6000]
	//epoch  [id:2,begin:2001,end:4000]   5002
}

func TestFeeAndPK(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, blockchain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)

		sendUpdateFeeTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendUpdatePkTransaction(number, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)

		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)

		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
	}
	manager := newTestPOSManager(101, executable)
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

func TestDepositGetDeposit(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, blockchain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)
		sendTranction(number, gen, statedb, mAccount, daddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(4000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDepositTransaction(number-51, gen, saddr1, skey1, signer, statedb, blockchain, abiStaking, nil)
		sendDelegateTransaction(number, gen, daddr1, saddr1, big.NewInt(4000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDelegateTransaction(number-61, gen, daddr1, saddr1, dkey1, signer, statedb, blockchain, abiStaking, nil)

		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(3000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDepositTransaction(number-types.GetEpochFromID(2).BeginHeight-11, gen, saddr1, skey1, signer, statedb, blockchain, abiStaking, nil)

		sendUnDelegateTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, daddr1, saddr1, big.NewInt(3000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDelegateTransaction(number-types.GetEpochFromID(2).BeginHeight-21, gen, daddr1, saddr1, dkey1, signer, statedb, blockchain, abiStaking, nil)

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
		sendGetDepositTransaction(number-types.MinCalcRedeemHeight(2)-11, gen, saddr1, skey1, signer, statedb, blockchain, abiStaking, nil)
		sendWithdrawDelegateTransaction(number-types.MinCalcRedeemHeight(2), gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDelegateTransaction(number-types.MinCalcRedeemHeight(2)-21, gen, daddr1, saddr1, dkey1, signer, statedb, blockchain, abiStaking, nil)
	}
	manager := newTestPOSManager(101, executable)
	fmt.Println(" saddr1 ", types.ToTrue(manager.GetBalance(saddr1)), " StakingAddress ", manager.GetBalance(types.StakingAddress), " ", types.ToTrue(manager.GetBalance(types.StakingAddress)))
}

///////////////////////////////////////////////////////////////////////
func TestSendTX(t *testing.T) {
	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)
	gspec.Config.TIP7 = &params.BlockConfig{FastNumber: big.NewInt(0)}
	gspec.Config.TIP8 = &params.BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(-1)}
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

///////////////////////////////////////////////////////////////////////
func TestRewardTime(t *testing.T) {
	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)
	params.ElectionMinLimitForStaking = new(big.Int).Mul(big.NewInt(1), big.NewInt(1e18))
	gspec.Config.TIP7 = &params.BlockConfig{FastNumber: big.NewInt(0)}
	gspec.Config.TIP8 = &params.BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(-1)}
	gspec.Config.TIP9 = &params.BlockConfig{SnailNumber: big.NewInt(20)}

	genesis := gspec.MustFastCommit(db)
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{})
	snailGenesis := gspec.MustSnailCommit(db)
	snailChainTest, _ := snailchain.NewSnailBlockChain(db, gspec.Config, engine, blockchain)
	engine.SetSnailChainReader(snailChainTest)
	parentFast := genesis
	parentSnail := []*types.SnailBlock{snailGenesis}
	delegateNum = 50000

	for i := 0; i < 505; i++ {

		chain, _ := core.GenerateChain(gspec.Config, parentFast, engine, db, 60, func(i int, gen *core.BlockGen) {
			header := gen.GetHeader()
			stateDB := gen.GetStateDB()
			if header.Number.Uint64() > 60 {
				SendTX(header, true, blockchain, nil, gspec.Config, gen, stateDB, nil)
			}
			switch i {
			case 0:
				rewardSnailBlock(snailChainTest, blockchain, header)
			}
		})
		if _, err := blockchain.InsertChain(chain); err != nil {
			panic(err)
		}
		parentFast = blockchain.CurrentBlock()
		schain := snailchain.GenerateChain(gspec.Config, blockchain, parentSnail, 1, 7, nil)
		if _, err := snailChainTest.InsertChain(schain); err != nil {
			panic(err)
		}
		parentSnail = snailChainTest.GetBlocksFromNumber(0)
	}
}

func TestDelegateReward(t *testing.T) {
	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(number uint64, gen *core.BlockGen, blockchain *core.BlockChain, header *types.Header, statedb *state.StateDB) {
		sendTranction(number, gen, statedb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)
		sendTranction(number, gen, statedb, mAccount, daddr1, big.NewInt(6000000000000000000), priKey, signer, nil, header)

		sendDepositTransaction(number, gen, saddr1, big.NewInt(4000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDepositTransaction(number-51, gen, saddr1, skey1, signer, statedb, blockchain, abiStaking, nil)

		sendCancelTransaction(number-types.GetEpochFromID(2).BeginHeight, gen, saddr1, big.NewInt(3000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDepositTransaction(number-types.GetEpochFromID(2).BeginHeight-11, gen, saddr1, skey1, signer, statedb, blockchain, abiStaking, nil)

		sendWithdrawTransaction(number-types.MinCalcRedeemHeight(2), gen, saddr1, big.NewInt(1000000000000000000), skey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDepositTransaction(number-types.MinCalcRedeemHeight(2)-11, gen, saddr1, skey1, signer, statedb, blockchain, abiStaking, nil)

		sendDelegateTransaction(number-params.NewEpochLength, gen, daddr1, saddr1, big.NewInt(4000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDelegateTransaction(number-61-params.NewEpochLength, gen, daddr1, saddr1, dkey1, signer, statedb, blockchain, abiStaking, nil)

		sendUnDelegateTransaction(number-types.GetEpochFromID(3).BeginHeight, gen, daddr1, saddr1, big.NewInt(3000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDelegateTransaction(number-types.GetEpochFromID(3).BeginHeight-21, gen, daddr1, saddr1, dkey1, signer, statedb, blockchain, abiStaking, nil)

		i := number / params.NewEpochLength
		if number == 130+params.NewEpochLength*i {
			impawn := vm.NewImpawnImpl()
			impawn.Load(statedb, types.StakingAddress)
			arr := impawn.GetLockedAsset(saddr1)
			for addr, value := range arr {
				fmt.Println("value ", value.Value, " addr ", addr.String())
			}
			arr1 := impawn.GetLockedAsset(daddr1)
			for addr, value := range arr1 {
				fmt.Println("value D ", value.Value, " addr ", addr.String(), "balance", statedb.GetBalance(daddr1), "number", number)
			}
		}

		sendWithdrawDelegateTransaction(number-types.MinCalcRedeemHeight(3), gen, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, statedb, blockchain, abiStaking, nil)
		sendGetDelegateTransaction(number-types.MinCalcRedeemHeight(3)-21, gen, daddr1, saddr1, dkey1, signer, statedb, blockchain, abiStaking, nil)
	}
	manager := newTestPOSManager(101, executable)
	fmt.Println(" saddr1 ", types.ToTrue(manager.GetBalance(saddr1)), " StakingAddress ", manager.GetBalance(types.StakingAddress), " ", types.ToTrue(manager.GetBalance(types.StakingAddress)))
}
