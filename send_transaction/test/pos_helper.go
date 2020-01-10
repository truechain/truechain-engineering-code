package test

import (
	"crypto/ecdsa"
	"fmt"
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
	"math"
	"math/big"
	"strings"
)

var (
	first       = false
	firstNumber = uint64(0)
)

func SendTX(block *types.Block, propagate bool, blockchain *core.BlockChain, tx txPool) {
	if !propagate {
		return
	}
	if !first {
		first = true
		firstNumber = block.Number().Uint64()
	}
	number := block.Number().Uint64()
	diff := number - firstNumber

	snedTranction(diff, nil, blockchain, mAccount, saddr1, big.NewInt(6000000000000000000), priKey, signer, tx, block.Header())
	snedTranction(diff, nil, blockchain, mAccount, daddr1, big.NewInt(6000000000000000000), priKey, signer, tx, block.Header())

	sendDepositTransaction(diff, nil, saddr1, skey1, signer, blockchain, abiStaking, tx, block.Header())
	sendDelegateTransaction(diff, nil, daddr1, saddr1, dkey1, signer, blockchain, abiStaking, tx, block.Header())
	sendCancelTransaction(diff, nil, saddr1, big.NewInt(1000000000000000000), skey1, signer, blockchain, abiStaking, tx, block.Header())
	sendUnDelegateTransaction(diff, nil, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, blockchain, abiStaking, tx, block.Header())
	sendWithdrawTransaction(diff, nil, saddr1, big.NewInt(1000000000000000000), skey1, signer, blockchain, abiStaking, tx, block.Header())
	sendWithdrawDelegateTransaction(diff, nil, daddr1, saddr1, big.NewInt(1000000000000000000), dkey1, signer, blockchain, abiStaking, tx, block.Header())
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
		Config:     params.TestChainConfig,
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
	blockchain *core.BlockChain
	snailchain *snailchain.SnailBlockChain
	GetBalance func(addr common.Address) *big.Int
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

func newTestPOSManager(sBlocks int, executableTx func(uint64, *core.BlockGen, *core.BlockChain, *types.Header)) *POSManager {

	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)

	gspec.Config.TIP8 = &params.BlockConfig{FastNumber: big.NewInt(0)}
	gspec.Config.TIP9 = &params.BlockConfig{SnailNumber: big.NewInt(20)}

	genesis := gspec.MustFastCommit(db)
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{})

	snailGenesis := gspec.MustSnailCommit(db)
	snailChainTest, _ := snailchain.NewSnailBlockChain(db, gspec.Config, engine, blockchain)
	engine.SetSnailChainReader(snailChainTest)

	parentFast := genesis
	parentSnail := []*types.SnailBlock{snailGenesis}
	for i := 1; i < sBlocks; i++ {

		chain, _ := core.GenerateChain(gspec.Config, parentFast, engine, db, 60, func(i int, gen *core.BlockGen) {

			header := gen.GetHeader()
			switch i {
			case 0:
				rewardSnailBlock(snailChainTest, blockchain, header)
			}
			executableTx(header.Number.Uint64(), gen, blockchain, header)
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

	// Create the pos manager with the base fields
	manager := &POSManager{
		snailchain: snailChainTest,
		blockchain: blockchain,
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

func sendDepositTransaction(height uint64, gen *core.BlockGen, from common.Address, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool, header *types.Header) {
	if height == 40 {
		nonce := getNonce(gen, from, blockchain, header)
		pub := crypto.FromECDSAPub(&priKey.PublicKey)
		input, err := abiStaking.Pack("deposit", pub)
		if err != nil {
			fmt.Println("sendDepositTransaction ", "error", err)
		}
		addTx(gen, blockchain, nonce, input, txPool, priKey, signer)
	}
}

func sendDelegateTransaction(height uint64, gen *core.BlockGen, from common.Address, toAddress common.Address, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool, header *types.Header) {
	if height == 60 {
		nonce := getNonce(gen, from, blockchain, header)
		input, err := abiStaking.Pack("delegate", toAddress)
		if err != nil {
			fmt.Println("sendDelegateTransaction ", "error", err)
		}
		addTx(gen, blockchain, nonce, input, txPool, priKey, signer)
	}
}

func sendCancelTransaction(height uint64, gen *core.BlockGen, from common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool, header *types.Header) {
	if height == 80 {
		nonce := getNonce(gen, from, blockchain, header)
		input, err := abiStaking.Pack("cancel", value)
		if err != nil {
			fmt.Println("sendCancelTransaction ", "error", err)
		}
		addTx(gen, blockchain, nonce, input, txPool, priKey, signer)
	}
}

func sendUnDelegateTransaction(height uint64, gen *core.BlockGen, from common.Address, toAddress common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool, header *types.Header) {
	if height == 120 {
		nonce := getNonce(gen, from, blockchain, header)
		input, err := abiStaking.Pack("undelegate", toAddress, value)
		if err != nil {
			fmt.Println("sendUnDelegateTransaction ", "error", err)
		}
		addTx(gen, blockchain, nonce, input, txPool, priKey, signer)
	}
}

func sendWithdrawTransaction(height uint64, gen *core.BlockGen, from common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool, header *types.Header) {
	if height == types.MinCalcRedeemHeight(types.GetEpochFromHeight(160).EpochID) {
		nonce := getNonce(gen, from, blockchain, header)
		input, err := abiStaking.Pack("withdraw", value)
		if err != nil {
			fmt.Println("sendWithdrawTransaction ", "error", err)
		}
		addTx(gen, blockchain, nonce, input, txPool, priKey, signer)
	}
}

func sendWithdrawDelegateTransaction(height uint64, gen *core.BlockGen, from, toAddress common.Address, value *big.Int, priKey *ecdsa.PrivateKey, signer types.TIP1Signer, blockchain *core.BlockChain, abiStaking abi.ABI, txPool txPool, header *types.Header) {
	if height == types.MinCalcRedeemHeight(types.GetEpochFromHeight(180).EpochID) {
		nonce := getNonce(gen, from, blockchain, header)
		input, err := abiStaking.Pack("withdrawDelegate", toAddress, value)
		if err != nil {
			fmt.Println("sendWithdrawDelegateTransaction ", "error", err)
		}
		addTx(gen, blockchain, nonce, input, txPool, priKey, signer)
	}
}

func snedTranction(height uint64, gen *core.BlockGen, blockchain *core.BlockChain, from, to common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, signer types.TIP1Signer, txPool txPool, header *types.Header) {
	if height == 20 {
		nonce := getNonce(gen, from, blockchain, header)
		tx, _ := types.SignTx(types.NewTransaction(nonce, to, value, params.TxGas, nil, nil), signer, privateKey)
		if gen != nil {
			gen.AddTx(tx)
		} else {
			txPool.AddRemotes([]*types.Transaction{tx})
		}
	}
}

func getNonce(gen *core.BlockGen, from common.Address, blockchain *core.BlockChain, header *types.Header) uint64 {
	var nonce uint64
	var stateDb *state.StateDB
	if gen != nil {
		nonce = gen.TxNonce(from)
		stateDb = gen.GetStateDB()
	} else {
		stateDb, _ := blockchain.StateAt(header.Root)
		nonce = stateDb.GetNonce(from)
	}
	printBalance(stateDb, from)
	return nonce
}

func addTx(gen *core.BlockGen, blockchain *core.BlockChain, nonce uint64, input []byte, txPool txPool, priKey *ecdsa.PrivateKey, signer types.TIP1Signer) {
	tx, _ := types.SignTx(types.NewTransaction(nonce, vm.StakingAddress, big.NewInt(1000000000000000000), 47200, big.NewInt(1000000), input), signer, priKey)
	if gen != nil {
		gen.AddTxWithChain(blockchain, tx)
	} else {
		txPool.AddRemotes([]*types.Transaction{tx})
	}
}

func printBalance(stateDb *state.StateDB, from common.Address) {
	fmt.Println(" printBalance ", " ", types.ToTrue(stateDb.GetBalance(from)))
	balance := stateDb.GetBalance(vm.StakingAddress)
	fbalance := new(big.Float)
	fbalance.SetString(balance.String())
	StakinValue := new(big.Float).Quo(fbalance, big.NewFloat(math.Pow10(18)))

	fmt.Println(" fbalance ", fbalance, " Value ", StakinValue)
}
