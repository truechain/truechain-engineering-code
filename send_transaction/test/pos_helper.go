package test

import (
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	first        = false
	firstNumber  = uint64(0)
	sb           map[common.Address]*types.StakingValue
	dbb          map[common.Address]*types.StakingValue
	epoch        = uint64(0)
	send         = false
	delegateNum  = 0
	delegateKey  []*ecdsa.PrivateKey
	delegateAddr []common.Address
	seed         = new(big.Int).SetInt64(0)
)

//epoch  [id:1,begin:1,end:2000]   [id:2,begin:2001,end:4000]   [id:3,begin:4001,end:6000]   [id:4,begin:6001,end:8000]   [id:5,begin:8001,end:10000]
func SendTX(header *types.Header, propagate bool, blockchain *core.BlockChain, tx txPool, config *params.ChainConfig, gen *core.BlockGen, statedb *state.StateDB, comKey *ecdsa.PrivateKey) {
	if !propagate {
		return
	}
	var stateDb *state.StateDB
	if statedb == nil {
		stateDb, _ = blockchain.StateAt(blockchain.CurrentFastBlock().Root())
	} else {
		stateDb = statedb
	}
	if !first {
		first = true
		if gen == nil {
			if comKey != nil {
				skey1 = comKey
			} else {
				skey1, _ = crypto.GenerateKey()
			}
			saddr1 = crypto.PubkeyToAddress(skey1.PublicKey)
			dkey1, _ = crypto.GenerateKey()
			daddr1 = crypto.PubkeyToAddress(dkey1.PublicKey)
		}
		firstNumber = header.Number.Uint64()
		signer = types.NewTIP1Signer(config.ChainID)
		impawn := vm.NewImpawnImpl()
		impawn.Load(stateDb, types.StakingAddress)
		sb = impawn.GetLockedAsset(saddr1)
		dbb = impawn.GetLockedAsset(daddr1)
		epoch = types.GetEpochFromHeight(firstNumber).EpochID
		send = true
		delegateKey = make([]*ecdsa.PrivateKey, delegateNum)
		delegateAddr = make([]common.Address, delegateNum)
		for i := 0; i < delegateNum; i++ {
			delegateKey[i], _ = crypto.GenerateKey()
			delegateAddr[i] = crypto.PubkeyToAddress(delegateKey[i].PublicKey)
		}
		seed, _ = rand.Int(rand.Reader, big.NewInt(9))
		printTest("seed ", seed, "cancel height ", types.GetEpochFromID(epoch+1).BeginHeight, " withdraw height ", types.MinCalcRedeemHeight(epoch+1))
	}
	number := header.Number.Uint64()
	diff := number - firstNumber - seed.Uint64()

	cEpoch := types.GetEpochFromHeight(number).EpochID
	if cEpoch != epoch && cEpoch-epoch == 4 {
		send = true
		firstNumber = types.GetEpochFromID(cEpoch).BeginHeight
		seed, _ = rand.Int(rand.Reader, big.NewInt(8))
		epoch = cEpoch
		printTest("firstNumber", firstNumber, "cancel height ", types.GetEpochFromID(epoch+1).BeginHeight, " withdraw height ", types.MinCalcRedeemHeight(epoch+1))
	}
	//printTest("send ",send,"number ",number,"diff ",diff,"cEpoch ",cEpoch,"epoch ",epoch,"firstNumber ",firstNumber,"sb",sb)
	if send {
		if sb != nil {
			for addr, value := range sb {
				for i, val := range value.Value {
					printTest("epoch ", i, " value ", val, " addr ", addr.String())
					sendNWithdrawTransaction(i, number, gen, saddr1, val, skey1, signer, stateDb, blockchain, abiStaking, tx)
				}
			}
		} else {
			sendTranction(diff, gen, stateDb, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, tx, header)

			sendDepositTransaction(diff, gen, saddr1, big.NewInt(4000000000000000000), skey1, signer, stateDb, blockchain, abiStaking, tx)
			sendCancelTransaction(diff-types.GetEpochFromID(epoch+1).BeginHeight+firstNumber, gen, saddr1, big.NewInt(2000000000000000000), skey1, signer, stateDb, blockchain, abiStaking, tx)
			sendWithdrawTransaction(diff-types.MinCalcRedeemHeight(epoch+1)+firstNumber, gen, saddr1, big.NewInt(2000000000000000000), skey1, signer, stateDb, blockchain, abiStaking, tx)
		}

		if dbb != nil {
			for addr, value := range dbb {
				for i, val := range value.Value {
					printTest("epoch ", i, " value ", val, " addr ", addr.String())
					sendNWithdrawDelegateTransaction(i, number, gen, daddr1, saddr1, val, dkey1, signer, stateDb, blockchain, abiStaking, tx)
				}
			}
		} else {
			sendTranction(diff-2, gen, stateDb, mAccount, daddr1, big.NewInt(6000000000000000000), priKey, signer, tx, header)

			sendDelegateTransaction(diff, gen, daddr1, saddr1, big.NewInt(4000000000000000000), dkey1, signer, stateDb, blockchain, abiStaking, tx)
			sendUnDelegateTransaction(diff-types.GetEpochFromID(epoch+1).BeginHeight+firstNumber, gen, daddr1, saddr1, big.NewInt(2000000000000000000), dkey1, signer, stateDb, blockchain, abiStaking, tx)
			sendWithdrawDelegateTransaction(diff-types.MinCalcRedeemHeight(epoch+1)+firstNumber, gen, daddr1, saddr1, big.NewInt(2000000000000000000), dkey1, signer, stateDb, blockchain, abiStaking, tx)
			for i := 0; i < delegateNum; i++ {
				if i%(int(seed.Uint64())+2) == 0 {
					sendTranction(diff-2-uint64(i), gen, stateDb, mAccount, delegateAddr[i], big.NewInt(6000000000000000000), priKey, signer, tx, header)

					sendDelegateTransaction(diff-uint64(i), gen, delegateAddr[i], saddr1, big.NewInt(4000000000000000000), delegateKey[i], signer, stateDb, blockchain, abiStaking, tx)
					sendUnDelegateTransaction(diff-types.GetEpochFromID(epoch+1).BeginHeight+firstNumber-uint64(i), gen, delegateAddr[i], saddr1, big.NewInt(2000000000000000000), delegateKey[i], signer, stateDb, blockchain, abiStaking, tx)
					sendWithdrawDelegateTransaction(diff-types.MinCalcRedeemHeight(epoch+1)+firstNumber-uint64(i), gen, delegateAddr[i], saddr1, big.NewInt(2000000000000000000), delegateKey[i], signer, stateDb, blockchain, abiStaking, tx)
				}
			}

			if diff-types.MinCalcRedeemHeight(epoch+1)+uint64(delegateNum) == 20 {
				send = false
			}
		}

		sb = nil
		dbb = nil
	}
}

type txPool interface {
	// AddRemotes should add the given transactions to the pool.
	AddRemotes([]*types.Transaction) []error
}

func DefaulGenesisBlock() *core.Genesis {
	i, _ := new(big.Int).SetString("10000000000000000000000", 10)
	key1 := hexutil.MustDecode("0x04d341c94a16b02cee86a627d0f6bc6e814741af4cab5065637aa013c9a7d9f26051bb6546030cd67e440d6df741cb65debaaba6c0835579f88a282193795ed369")
	key2 := hexutil.MustDecode("0x0496e0f18d4bf38e0b0de161edd2aa168adaf6842706e5ebf31e1d46cb79fe7b720c750a9e7a3e1a528482b0da723b5dfae739379e555a2893e8693747559f83cd")
	key3 := hexutil.MustDecode("0x0418196ee090081bdec01e8840941b9f6a141a713dd3461b78825edf0d8a7f8cdf3f612832dc9d94249c10c72629ea59fbe0bdd09bea872ddab2799748964c93a8")
	key4 := hexutil.MustDecode("0x04c4935993a3ce206318ab884871fbe2d4dce32a022795c674784f58e7faf3239631b6952b82471fe1e93ef999108a18d028e5d456cd88bb367d610c5e57c7e443")

	return &core.Genesis{
		Config:     params.DevnetChainConfig,
		Nonce:      928,
		ExtraData:  nil,
		GasLimit:   88080384,
		Difficulty: big.NewInt(20000),
		Alloc: map[common.Address]types.GenesisAccount{
			common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69"): {Balance: i},
			common.HexToAddress("0x6d348e0188Cc2596aaa4046a1D50bB3BA50E8524"): {Balance: i},
			common.HexToAddress("0xE803895897C3cCd35315b2E41c95F817543811A5"): {Balance: i},
			common.HexToAddress("0x3F739ffD8A59965E07e1B8d7CCa938125BCe8CFb"): {Balance: i},
		},
		Committee: []*types.CommitteeMember{
			{Coinbase: common.HexToAddress("0x3f9061bf173d8f096c94db95c40f3658b4c7eaad"), Publickey: key1},
			{Coinbase: common.HexToAddress("0x2cdac3658f85b5da3b70223cc3ad3b2dfe7c1930"), Publickey: key2},
			{Coinbase: common.HexToAddress("0x41acde8dd7611338c2a30e90149e682566716e9d"), Publickey: key3},
			{Coinbase: common.HexToAddress("0x0ffd116a3bf97a7112ff8779cc770b13ea3c66a5"), Publickey: key4},
		},
	}
}

type POSManager struct {
	blockchain  *core.BlockChain
	snailchain  *snailchain.SnailBlockChain
	chainconfig *params.ChainConfig
	GetBalance  func(addr common.Address) *big.Int
}

var (
	engine        = minerva.NewFaker()
	db            = etruedb.NewMemDatabase()
	gspec         = DefaulGenesisBlock()
	abiStaking, _ = abi.JSON(strings.NewReader(vm.StakeABIJSON))
	signer        = types.NewTIP1Signer(gspec.Config.ChainID)
	priKey, _     = crypto.HexToECDSA("0260c952edc49037129d8cabbe4603d15185d83aa718291279937fb6db0fa7a2")
	mAccount      = common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69")
	skey1, _      = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	saddr1        = crypto.PubkeyToAddress(skey1.PublicKey)
	dkey1, _      = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	daddr1        = crypto.PubkeyToAddress(dkey1.PublicKey)
)

func newTestPOSManager(sBlocks int, executableTx func(uint64, *core.BlockGen, *core.BlockChain, *types.Header, *state.StateDB)) *POSManager {

	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)

	genesis := gspec.MustFastCommit(db)
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{})

	snailGenesis := gspec.MustSnailCommit(db)
	snailChainTest, _ := snailchain.NewSnailBlockChain(db, gspec.Config, engine, blockchain)
	engine.SetSnailChainReader(snailChainTest)

	parentFast := genesis
	parentSnail := []*types.SnailBlock{snailGenesis}
	for i := 0; i < sBlocks; i++ {

		chain, _ := core.GenerateChain(gspec.Config, parentFast, engine, db, 60, func(i int, gen *core.BlockGen) {

			header := gen.GetHeader()
			stateDB := gen.GetStateDB()
			switch i {
			case 0:
				rewardSnailBlock(snailChainTest, blockchain, header)
			}
			//if gspec.Config.TIP8.FastNumber != nil && gspec.Config.TIP8.FastNumber.Sign() > 0 {
			//	executableTx(header.Number.Uint64()-gspec.Config.TIP8.FastNumber.Uint64()+9600, gen, blockchain, header, stateDB)
			//}
			executableTx(header.Number.Uint64(), gen, blockchain, header, stateDB)
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

	consensus.InitTIP8(gspec.Config, snailChainTest)
	fmt.Println("first ", types.GetFirstEpoch())
	// Create the pos manager with the base fields
	manager := &POSManager{
		snailchain:  snailChainTest,
		blockchain:  blockchain,
		chainconfig: gspec.Config,
	}

	GetBalance := func(addr common.Address) *big.Int {
		//mecMark how to get ChainFastReader
		statedb, _ := blockchain.State()
		return statedb.GetBalance(addr)
	}
	manager.GetBalance = GetBalance
	return manager
}

//generate rewardSnailHegiht
func rewardSnailBlock(chain consensus.SnailChainReader, fastChain *core.BlockChain, header *types.Header) {
	rewardSnailHegiht := fastChain.NextSnailNumberReward()
	space := new(big.Int).Sub(chain.CurrentHeader().Number, rewardSnailHegiht).Int64()
	if space >= params.SnailRewardInterval.Int64() {
		header.SnailNumber = rewardSnailHegiht
		sb := chain.GetHeaderByNumber(rewardSnailHegiht.Uint64())
		if sb != nil {
			header.SnailHash = sb.Hash()
		} else {
			log.Error("cannot find snailBlock by rewardSnailHegiht.", "snailHeight", rewardSnailHegiht.Uint64())
		}
	}
}

func sendDepositTransaction(height uint64, gen *core.BlockGen, from common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == 40 {
		nonce, _ := getNonce(gen, from, state, "sendDepositTransaction")
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		input := packInput(abiStaking, "deposit", "sendDepositTransaction", pub, new(big.Int).SetInt64(5000))
		addTx(gen, blockchain, nonce, value, input, txPool, priKey, signer)
	}
}

func sendDelegateTransaction(height uint64, gen *core.BlockGen, from, toAddress common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == 60 {
		nonce, _ := getNonce(gen, from, state, "sendDelegateTransaction")
		input := packInput(abiStaking, "delegate", "sendDelegateTransaction", toAddress)
		addTx(gen, blockchain, nonce, value, input, txPool, priKey, signer)
	}
}

func sendCancelTransaction(height uint64, gen *core.BlockGen, from common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == 10 {
		nonce, _ := getNonce(gen, from, state, "sendCancelTransaction")
		input := packInput(abiStaking, "cancel", "sendCancelTransaction", value)
		addTx(gen, blockchain, nonce, big.NewInt(0), input, txPool, priKey, signer)
	}
}

func sendGetDepositTransaction(height uint64, gen *core.BlockGen, from common.Address, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == 10 {
		input := packInput(abiStaking, "getDeposit", "sendGetDepositTransaction", from)
		args := struct {
			Staked   *big.Int
			Locked   *big.Int
			Unlocked *big.Int
		}{}
		readTx(gen, blockchain, 0, big.NewInt(0), input, txPool, priKey, signer, "getDeposit", &args)
		printTest("Staked ", args.Staked, "Locked ", args.Locked, "Unlocked ", args.Unlocked)
	}
}

func sendUnDelegateTransaction(height uint64, gen *core.BlockGen, from common.Address, toAddress common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == 20 {
		nonce, _ := getNonce(gen, from, state, "sendUnDelegateTransaction")
		input := packInput(abiStaking, "undelegate", "sendUnDelegateTransaction", toAddress, value)
		addTx(gen, blockchain, nonce, big.NewInt(0), input, txPool, priKey, signer)
	}
}

func sendWithdrawTransaction(height uint64, gen *core.BlockGen, from common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == 10 {
		nonce, _ := getNonce(gen, from, state, "sendWithdrawTransaction")
		input := packInput(abiStaking, "withdraw", "sendWithdrawTransaction", value)
		addTx(gen, blockchain, nonce, big.NewInt(0), input, txPool, priKey, signer)
	}
}

func sendWithdrawDelegateTransaction(height uint64, gen *core.BlockGen, from, toAddress common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == 20 {
		nonce, _ := getNonce(gen, from, state, "sendWithdrawDelegateTransaction")
		input := packInput(abiStaking, "withdrawDelegate", "sendWithdrawDelegateTransaction", toAddress, value)
		addTx(gen, blockchain, nonce, big.NewInt(0), input, txPool, priKey, signer)
	}
}

func sendNWithdrawDelegateTransaction(epoch uint64, height uint64, gen *core.BlockGen, from, toAddress common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == types.MinCalcRedeemHeight(epoch) {
		nonce, _ := getNonce(gen, from, state, "sendNWithdrawDelegateTransaction")
		input := packInput(abiStaking, "withdrawDelegate", "sendNWithdrawDelegateTransaction", toAddress, value)
		addTx(gen, blockchain, nonce, big.NewInt(0), input, txPool, priKey, signer)
	}
}

func sendNWithdrawTransaction(epoch uint64, height uint64, gen *core.BlockGen, from common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, state *state.StateDB, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool) {
	if height == types.MinCalcRedeemHeight(epoch) {
		nonce, _ := getNonce(gen, from, state, "sendNWithdrawTransaction")
		input := packInput(abiStaking, "withdraw", "sendNWithdrawTransaction", value)
		addTx(gen, blockchain, nonce, big.NewInt(0), input, txPool, priKey, signer)
	}
}

func sendTranction(height uint64, gen *core.BlockGen, state *state.StateDB, from, to common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, signer types.TIP1Signer, txPool txPool, header *types.Header) {
	if height == 20 {
		nonce, statedb := getNonce(gen, from, state, "sendTranction")
		balance := statedb.GetBalance(to)
		remaining := new(big.Int).Sub(value, balance)
		printTest("sendTranction ", balance.Uint64(), " remaining ", remaining.Uint64(), " height ", height, " current ", header.Number.Uint64())
		if remaining.Sign() > 0 {
			tx, _ := types.SignTx(types.NewTransaction(nonce, to, remaining, params.TxGas, new(big.Int).SetInt64(1000000), nil), signer, privateKey)
			if gen != nil {
				gen.AddTx(tx)
			} else {
				txPool.AddRemotes([]*types.Transaction{tx})
			}
		} else {
			printTest("to ", to.String(), " have balance ", balance.Uint64(), " height ", height, " current ", header.Number.Uint64())
		}
	}
}

func getNonce(gen *core.BlockGen, from common.Address, state1 *state.StateDB, method string) (uint64, *state.StateDB) {
	var nonce uint64
	var stateDb *state.StateDB
	if gen != nil {
		nonce = gen.TxNonce(from)
		stateDb = gen.GetStateDB()
	} else {
		stateDb = state1
		nonce = stateDb.GetNonce(from)
	}
	printBalance(stateDb, from, method)
	return nonce, stateDb
}

func addTx(gen *core.BlockGen, blockchain *core.BlockChain, nonce uint64, value *big.Int, input []byte, txPool txPool, priKey *ecdsa.PrivateKey, signer types.TIP1Signer) {
	tx, _ := types.SignTx(types.NewTransaction(nonce, types.StakingAddress, value, 57200, big.NewInt(1000000), input), signer, priKey)
	if gen != nil {
		gen.AddTxWithChain(blockchain, tx)
	} else {
		txPool.AddRemotes([]*types.Transaction{tx})
	}
}

func readTx(gen *core.BlockGen, blockchain *core.BlockChain, nonce uint64, value *big.Int, input []byte, txPool txPool, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, abiMethod string, result interface{}) {
	tx, _ := types.SignTx(types.NewTransaction(nonce, types.StakingAddress, value, 57200, big.NewInt(1000000), input), signer, priKey)
	if gen != nil {
		output, gas := gen.ReadTxWithChain(blockchain, tx)
		err := abiStaking.Unpack(result, abiMethod, output)
		if err != nil {
			printTest(abiMethod, " error ", err)
		}
		printTest("readTx gas ", gas)
	} else {
		txPool.AddRemotes([]*types.Transaction{tx})
	}
}

func printBalance(stateDb *state.StateDB, from common.Address, method string) {
	balance := stateDb.GetBalance(types.StakingAddress)
	fbalance := new(big.Float)
	fbalance.SetString(balance.String())
	StakinValue := new(big.Float).Quo(fbalance, big.NewFloat(math.Pow10(18)))

	printTest(method, " from ", types.ToTrue(stateDb.GetBalance(from)), " Staking fbalance ", fbalance, " StakinValue ", StakinValue, "from ", from.String())
}

func packInput(abiStaking abi.ABI, abiMethod, method string, params ...interface{}) []byte {
	input, err := abiStaking.Pack(abiMethod, params...)
	if err != nil {
		printTest(method, " error ", err)
	}
	return input
}

func printTest(a ...interface{}) {
	log.Info("test", "SendTX", a)
}
