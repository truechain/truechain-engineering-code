// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"os"
	"testing"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

func ExampleGenerateChain() {
	var (
		chainId = big.NewInt(3)
		db      = etruedb.NewMemDatabase()

		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
		gspec   = &Genesis{
			Config: &params.ChainConfig{ChainID: chainId,
				TIP7:  &params.BlockConfig{FastNumber: big.NewInt(0)},
				TIP8:  &params.BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(-1)},
				TIP9:  &params.BlockConfig{SnailNumber: big.NewInt(20)},
				TIP13: &params.BlockConfig{FastNumber: big.NewInt(0), SnailNumber: big.NewInt(0)}},
			Alloc: types.GenesisAlloc{addr1: {Balance: big.NewInt(3000000)}},
		}
		genesis = gspec.MustFastCommit(db)
		pow     = minerva.NewFaker()
		signer  = types.NewTIP1Signer(gspec.Config.ChainID)
	)

	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.
	chain, _ := GenerateChain(gspec.Config, genesis, pow, db, 5, func(i int, gen *BlockGen) {
		switch i {
		case 0:
			// In block 1, addr1 sends addr2 some ether.
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(30000), params.TxGas, nil, nil), signer, key1)
			gen.AddTx(tx)
		case 1:
			// In block 2, addr1 sends some more ether to addr2.
			// addr2 passes it on to addr3.
			tx1, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(1000), params.TxGas, nil, nil), signer, key1)
			tx2, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr2), addr3, big.NewInt(1000), params.TxGas, nil, nil), signer, key2)
			gen.AddTx(tx1)
			gen.AddTx(tx2)
		case 2:
			// Block 3 is empty but was mined by addr3.
			gen.SetCoinbase(addr3)
			gen.SetExtra([]byte("yeehaw"))
		}
	})

	// Import the chain. This runs all block validation rules.
	blockchain, _ := NewBlockChain(db, nil, gspec.Config, pow, vm.Config{})
	defer blockchain.Stop()

	if i, err := blockchain.InsertChain(chain); err != nil {
		fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
		return
	}

	state, _ := blockchain.State()
	fmt.Printf("last block: #%d\n", blockchain.CurrentBlock().Number())
	fmt.Println("balance of addr1:", state.GetBalance(addr1))
	fmt.Println("balance of addr2:", state.GetBalance(addr2))
	fmt.Println("balance of addr3:", state.GetBalance(addr3))
	// Output:
	// last block: #5
	// balance of addr1: 2969000
	// balance of addr2: 30000
	// balance of addr3: 1000
}

func TestTransactionCost(t *testing.T) {
	var (
		addresses   []common.Address
		privateKeys []*ecdsa.PrivateKey
		//geneSnailBlockNumber = 3
	)
	for i := 1; i <= 3; i++ {
		key, _ := crypto.GenerateKey()
		privateKeys = append(privateKeys, key)
		address := crypto.PubkeyToAddress(key.PublicKey)
		addresses = append(addresses, address)
	}

	params.MinimumFruits = 1
	var (
		db    = etruedb.NewMemDatabase()
		pow   = minerva.NewFaker()
		gspec = &Genesis{
			Config: params.TestChainConfig,
			Alloc: types.GenesisAlloc{
				addresses[0]: {Balance: big.NewInt(params.Ether)},
				addresses[1]: {Balance: big.NewInt(params.Ether)},
				addresses[2]: {Balance: big.NewInt(params.Ether)},
			},
			Difficulty: big.NewInt(20000),
		}
		genesis    = gspec.MustFastCommit(db)
		fastParent = genesis
		signer     = types.NewTIP1Signer(params.TestChainConfig.ChainID)

		/*balance_given = new(big.Int)
		balance_get   = new(big.Int)*/

		tx_amount = big.NewInt(100000000000000000)
		tx_price  = big.NewInt(1000000)
		tx_fee    = big.NewInt(100000000000000000)
	)

	//generate blockchain
	blockchain, _ := NewBlockChain(db, nil, gspec.Config, pow, vm.Config{})
	defer blockchain.Stop()

	fastBlocks, _ := GenerateChain(gspec.Config, fastParent, pow, db, params.MinimumFruits, func(i int, gen *BlockGen) {
		tx1, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addresses[0]), addresses[1], tx_amount, params.TxGas, tx_price, nil), signer, privateKeys[0])
		gen.AddTx(tx1)
	})
	if i, err := blockchain.InsertChain(fastBlocks); err != nil {
		fmt.Printf("insert error (block %d): %v\n", fastBlocks[i-1].NumberU64(), err)
		return
	}
	fastParent = blockchain.CurrentBlock()
	statedb, _ := state.New(blockchain.CurrentBlock().Root(), state.NewDatabase(db))
	balance_get1, isAverage1 := getCommitteeMemberReward(pow, statedb)
	if isAverage1 == false {
		log.Error("[TestTransactionCost error]:committee member reward is not average.")
	}
	//balance_given = getAddressBalance(addresses, statedb)
	tx1_addr0_balance := statedb.GetBalance(addresses[0])
	tx1_addr1_balance := statedb.GetBalance(addresses[1])
	tx1_gas := new(big.Int).Mul(tx_price, big.NewInt(int64(params.TxGas)))
	tx1_value := new(big.Int).Add(tx_amount, tx1_gas)
	if new(big.Int).Add(tx1_addr0_balance, tx1_value).Cmp(big.NewInt(params.Ether)) != 0 || new(big.Int).Sub(tx1_addr1_balance, tx_amount).Cmp(big.NewInt(params.Ether)) != 0 {
		log.Info("[TestTransactionCost info]:", " balance", tx1_value, "addr0_balance", tx1_addr0_balance, "addr1_balance", tx1_addr1_balance)
		log.Error("[TestTransactionCost error]:tx1 execution error")
	}
	if balance_get1.Cmp(tx1_gas) != 0 {
		log.Info("[TestTransactionCost info]:", "balance_get1", balance_get1, "tx1_gas", tx1_gas)
		log.Error("[TestTransactionCost error]:tx1 gas is not equal")
	}
	//test transaction2  payment
	fastBlocks, _ = GenerateChain(gspec.Config, fastParent, pow, db, params.MinimumFruits, func(i int, gen *BlockGen) {
		signTx_sender, _ := types.SignTx(types.NewTransaction_Payment(gen.TxNonce(addresses[0]), addresses[1], tx_amount, tx_fee, params.TxGas, tx_price, nil, addresses[2]), signer, privateKeys[0])
		tx2, _ := types.SignTx_Payment(signTx_sender, signer, privateKeys[2])
		gen.AddTx(tx2)
	})

	if i, err := blockchain.InsertChain(fastBlocks); err != nil {
		fmt.Printf("insert error (block %d): %v\n", fastBlocks[i].NumberU64(), err)
		return
	}
	fastParent = blockchain.CurrentBlock()
	statedb, _ = state.New(blockchain.CurrentBlock().Root(), state.NewDatabase(db))

	balance_get2, isAverage2 := getCommitteeMemberReward(pow, statedb)
	if isAverage2 == false {
		log.Error("[TestTransactionCost error]:committee member reward is not average.")
	}
	tx2_addr0_balance := statedb.GetBalance(addresses[0])
	tx2_addr1_balance := statedb.GetBalance(addresses[1])
	tx2_addr2_balance := statedb.GetBalance(addresses[2])
	tx2_value := new(big.Int).Add(tx_amount, tx_fee)
	tx2_gas := new(big.Int).Mul(tx_price, big.NewInt(int64(params.TxGas)))
	if new(big.Int).Add(tx2_addr0_balance, tx2_value).Cmp(tx1_addr0_balance) != 0 {
		log.Info("[TestTransactionCost info]:", " tx2_value", tx2_value, "addr0_balance", tx2_addr0_balance)
		log.Error("[TestTransactionCost error]:tx2 tx2_addr0_balance execution error")
	}
	if new(big.Int).Sub(tx2_addr1_balance, tx_amount).Cmp(tx1_addr1_balance) != 0 {
		log.Info("[TestTransactionCost info]:", " tx2_value", tx2_value, "tx2_addr1_balance", tx2_addr1_balance, "tx1_addr1_balance", tx1_addr1_balance)
		log.Error("[TestTransactionCost error]:tx2 addr1 execution error")
	}
	if new(big.Int).Add(tx2_addr2_balance, tx2_gas).Cmp(big.NewInt(params.Ether)) != 0 {
		log.Info("[TestTransactionCost info]:", " tx2_value", tx2_value, "addr2_balance", tx2_addr2_balance)
		log.Error("[TestTransactionCost error]:tx2 addr2 execution error")
	}
	gasAndfee := new(big.Int).Add(tx2_gas, tx_fee)
	distance := new(big.Int).Sub(balance_get2, balance_get1)
	members := pow.GetElection().GetCommittee(big.NewInt(1))
	if new(big.Int).Sub(distance, gasAndfee).Uint64() >= uint64(len(members)) {
		log.Info("[TestTransactionCost info]:", "balance_get2", balance_get2, "balance_get1", balance_get1, "tx2_gas", tx2_gas)
		log.Error("[TestTransactionCost error]:tx2 gas is not equal")
	}

	//test all balance
	address_get := getAddressBalance(addresses, statedb)
	all_balance := new(big.Int).Add(address_get, balance_get2)
	diff := new(big.Int).Sub(big.NewInt(3*params.Ether), all_balance).Uint64()
	if diff >= uint64(2*len(members)) {
		log.Info("[TestTransactionCost info]:", "diff", diff)
		log.Error("[TestTransactionCost error]:tx2 addr2 execution error")
	}
}

func getCommitteeMemberReward(pow *minerva.Minerva, statedb *state.StateDB) (balance_get *big.Int, isAverage bool) {
	members := pow.GetElection().GetCommittee(big.NewInt(1))
	if len(members) == 0 {
		return nil, false
	}
	reward := statedb.GetBalance(members[0].Coinbase)
	balance_get = new(big.Int)
	for _, member := range members {
		balance := statedb.GetBalance(member.Coinbase)
		if reward.Cmp(balance) != 0 {
			return nil, false
		}
		balance_get = new(big.Int).Add(balance_get, balance)
		log.Info("getBalance[committe member]", "addr", member.Coinbase, "balance", statedb.GetBalance(member.Coinbase))
	}
	return balance_get, true
}

func getAddressBalance(addresses []common.Address, statedb *state.StateDB) (balance_given *big.Int) {
	balance_given = new(big.Int)
	for _, addr := range addresses {
		balance_given = new(big.Int).Add(balance_given, statedb.GetBalance(addr))
	}
	return balance_given
}

//func TestMakeBlock1(t *testing.T) {
//	var (
//		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
//		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
//		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
//		key4, _ = crypto.HexToECDSA("42c4d734786eedaf5d0c51fd2bc9bbaa6c289ed23710d9381072932456aeca18")
//		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
//		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
//		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
//		addr4   = crypto.PubkeyToAddress(key4.PublicKey)
//		db      = etruedb.NewMemDatabase()
//	)
//	recvAddr := crypto.CreateAddress(common.Address{0}, 10)
//	// Ensure that key1 has some funds in the genesis block.
//	gspec := &Genesis{
//		Config:   &params.ChainConfig{ChainID: big.NewInt(1)},
//		GasLimit: 1050000000,
//		Alloc: types.GenesisAlloc{
//			addr1: {Balance: big.NewInt(5000000000000)},
//			addr2: {Balance: big.NewInt(5000000000000)},
//			addr3: {Balance: big.NewInt(5000000000000)},
//			addr4: {Balance: big.NewInt(5000000000000)},
//		},
//	}
//	genesis := gspec.MustFastCommit(db)
//
//	// This call generates a chain of 5 blocks. The function runs for
//	// each block and adds different features to gen based on the
//	// block index.
//	signer := types.NewTIP1Signer(gspec.Config.ChainID)
//	cnt := 4000 / 4
//	finish := make(chan int)
//	type tmp struct {
//		addr common.Address
//		key  *ecdsa.PrivateKey
//	}
//	addrs := []tmp{
//		{addr1, key1},
//		{addr2, key2},
//		{addr3, key3},
//		{addr4, key4},
//	}
//	begin := time.Now()
//	chain, _ := GenerateChain(gspec.Config, genesis, minerva.NewFaker(), db, 1, func(i int, gen *BlockGen) {
//		sum := 0
//		for i := 0; i < 3; i++ {
//			go func(addr common.Address, key *ecdsa.PrivateKey) {
//				for n := 0; n < cnt; n++ {
//					tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr), recvAddr, big.NewInt(100000), params.TxGas, nil, nil), signer, key)
//					gen.AddTx(tx)
//				}
//				finish <- 1
//			}(addrs[i].addr, addrs[i].key)
//		}
//		for {
//			<-finish
//			sum++
//			if sum == 4 {
//				return
//			}
//		}
//	})
//	d := time.Now().Sub(begin)
//	fmt.Println("make block include", cnt, " txs,cost time ", d.Seconds())
//	// Import the chain. This runs all block validation rules.
//	blockchain, _ := NewBlockChain(db, nil, gspec.Config, minerva.NewFaker(), vm.Config{})
//	defer blockchain.Stop()
//
//	if i, err := blockchain.InsertChain(chain); err != nil {
//		fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
//		return
//	}
//
//	state, _ := blockchain.State()
//	fmt.Printf("last block: #%d\n", blockchain.CurrentBlock().Number())
//	fmt.Println("balance of addr1:", state.GetBalance(addr1))
//	fmt.Println("balance of addr2:", state.GetBalance(addr2))
//	fmt.Println("balance of addr3:", state.GetBalance(addr3))
//}
//func TestMakeBlock2(t *testing.T) {
//	var (
//		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
//		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
//		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
//		key4, _ = crypto.HexToECDSA("42c4d734786eedaf5d0c51fd2bc9bbaa6c289ed23710d9381072932456aeca18")
//		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
//		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
//		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
//		addr4   = crypto.PubkeyToAddress(key4.PublicKey)
//		db      = etruedb.NewMemDatabase()
//		tmpDB   = etruedb.NewMemDatabase()
//	)
//	recvAddr := crypto.CreateAddress(common.Address{0}, 10)
//	recvAddr2 := crypto.CreateAddress(common.Address{0}, 50)
//	// Ensure that key1 has some funds in the genesis block.
//	gspec := &Genesis{
//		Config:   &params.ChainConfig{ChainID: new(big.Int)},
//		GasLimit: 105000000,
//		Alloc: types.GenesisAlloc{
//			addr1: {Balance: big.NewInt(5000000000)},
//			addr2: {Balance: big.NewInt(5000000000)},
//			addr3: {Balance: big.NewInt(5000000000)},
//			addr4: {Balance: big.NewInt(5000000000)},
//		},
//	}
//	genesis := gspec.MustFastCommit(db)
//	genesis2 := gspec.MustFastCommit(tmpDB)
//
//	// This call generates a chain of 5 blocks. The function runs for
//	// each block and adds different features to gen based on the
//	// block index.
//	signer := types.MakeSigner(gspec.Config, nil)
//	cnt := 1
//	type tmp struct {
//		addr common.Address
//		key  *ecdsa.PrivateKey
//	}
//	addrs := []tmp{
//		{addr1, key1},
//		{addr2, key2},
//		{addr3, key3},
//		{addr4, key4},
//	}
//	ptxs1 := make([]*types.Transaction, 0, 0)
//	ptxs2 := make([]*types.Transaction, 0, 0)
//	begin := time.Now()
//	block1, recpt1 := GenerateChain(gspec.Config, genesis, minerva.NewFaker(), db, 1, func(i int, gen *BlockGen) {
//		from, key := addrs[0].addr, addrs[0].key
//		for i := 0; i < cnt; i++ {
//			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(from), recvAddr, big.NewInt(1000), params.TxGas, nil, nil), signer, key)
//			ptxs1 = append(ptxs1, tx)
//			gen.AddTx(tx)
//		}
//		from, key = addrs[1].addr, addrs[1].key
//		for i := 0; i < cnt; i++ {
//			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(from), recvAddr2, big.NewInt(1000), params.TxGas, nil, nil), signer, key)
//			ptxs2 = append(ptxs2, tx)
//			gen.AddTx(tx)
//		}
//	})
//	d := time.Now().Sub(begin)
//	fmt.Println("make block include", cnt, " txs,cost time ", d.Seconds())
//	sum := len(ptxs1) + len(ptxs2)
//	indexs := make([]int, sum, sum)
//	// ptxs2 = append(ptxs1,ptxs2...)
//	// for i,_ := range ptxs2 {
//	// 	indexs[i] = i
//	// }
//	ptxs2 = append(ptxs2, ptxs1...)
//	pos := 0
//	for i := 2*cnt - 1; i >= cnt; i-- {
//		indexs[pos] = i
//		pos++
//	}
//	for i := cnt - 1; i >= 0; i-- {
//		indexs[pos] = i
//		pos++
//	}
//	/////////////////////////////////////////////////////////////////////////////////
//	block2, recpt2 := GenerateChain(gspec.Config, genesis2, minerva.NewFaker(), tmpDB, 1, func(i int, gen *BlockGen) {
//		gen.AddTxWithChain2(nil, block1[0].Transactions(), ptxs2, indexs)
//	})
//	/////////////////////////////////////////////////////////////////////////////////
//	fmt.Println("r1.hash1:", recpt1[0].GetRlp(0), "r1.hash2", recpt1[0].GetRlp(1))
//	fmt.Println("r2.hash1:", recpt2[0].GetRlp(0), "r2.hash2", recpt2[0].GetRlp(1))
//	if bytes.Equal(recpt1[0].GetRlp(0), recpt2[0].GetRlp(0)) {
//		fmt.Println("recpt index 0 equal")
//	}
//	if bytes.Equal(recpt1[0].GetRlp(0), recpt2[0].GetRlp(1)) {
//		fmt.Println("recpt index 0---1 equal")
//	}
//	if bytes.Equal(recpt1[0].GetRlp(1), recpt2[0].GetRlp(0)) {
//		fmt.Println("recpt index 1---0 equal")
//	}
//	if bytes.Equal(recpt1[0].GetRlp(1), recpt2[0].GetRlp(1)) {
//		fmt.Println("recpt index 1 equal")
//	}
//
//	fmt.Println("height:", block1[0].NumberU64(), "hash1", block1[0].Hash(), "root:", block1[0].Root(), "rhash:", block1[0].ReceiptHash(), "txhash:", block1[0].TxHash())
//	fmt.Println("height:", block2[0].NumberU64(), "hash1", block2[0].Hash(), "root:", block2[0].Root(), "rhash:", block2[0].ReceiptHash(), "txhash:", block2[0].TxHash())
//
//	fmt.Println("finish")
//}
//func TestVerifySignInBatch(t *testing.T) {
//	fmt.Println("begin......,cpu", runtime.NumCPU())
//	verifyInBatch(100, 1)
//	verifyInBatch(100, 4)
//	verifyInBatch(100, 10)
//	verifyInBatch(10000, 1)
//	verifyInBatch(10000, 4)
//	verifyInBatch(10000, 10)
//	fmt.Println("finish.....")
//}
//
//func verifyInBatch(sum, count int) {
//	var (
//		db = etruedb.NewMemDatabase()
//	)
//	type tmpItem struct {
//		sign []byte
//		priv *ecdsa.PrivateKey
//		hash common.Hash
//	}
//	gspec := &Genesis{
//		Config:   &params.ChainConfig{ChainID: new(big.Int)},
//		GasLimit: 105000000,
//		Alloc:    types.GenesisAlloc{},
//	}
//	genesis := gspec.MustFastCommit(db)
//	block1, _ := GenerateChain(gspec.Config, genesis, minerva.NewFaker(), db, 1, func(i int, gen *BlockGen) {
//	})
//	block := block1[0]
//	hash := block.Hash()
//
//	items := make([]*tmpItem, sum, sum)
//	for i := 0; i < sum; i++ {
//		p, _ := crypto.GenerateKey()
//		sign, _ := crypto.Sign(hash[:], p)
//		items[i] = &tmpItem{
//			sign: sign,
//			priv: p,
//			hash: hash,
//		}
//	}
//
//	finish := make(chan bool, count)
//	fmt.Println("begin verify in batch...")
//	begin := time.Now()
//
//	execFunc := func(index int, infos []*tmpItem) {
//		taskbegin := time.Now()
//		for _, v := range infos {
//			crypto.SigToPub(v.hash[:], v.sign)
//		}
//		d := time.Now().Sub(taskbegin)
//		finish <- true
//		fmt.Println("index", index, "verify sign result, cost time: ", d.Seconds())
//	}
//	batch := sum / count
//	for i := 0; i < count; i++ {
//		b, e := i*batch, (i+1)*batch-1
//		if i == count-1 {
//			e = sum
//		}
//		info := items[b:e]
//		go execFunc(i, info)
//	}
//
//	calcCount := 0
//	for {
//		<-finish
//		calcCount++
//		if calcCount == count {
//			break
//		}
//	}
//	d := time.Now().Sub(begin)
//	fmt.Println("all task done, sign Sum:", sum, "task count:", count, "cost time: ", d.Seconds())
//}
//
//// for test
//func (b *BlockGen) AddTxWithChain2(bc *BlockChain, otxs, txs []*types.Transaction, indexs []int) {
//	if b.gasPool == nil {
//		b.SetCoinbase(common.Address{})
//	}
//
//	recpts := make([]*types.Receipt, len(otxs), len(otxs))
//
//	for i, tx := range txs {
//		b.statedb.Prepare(tx.Hash(), common.Hash{}, indexs[i])
//
//		feeAmount := big.NewInt(0)
//		receipt, _, err := ApplyTransaction(b.config, bc, b.gasPool, b.statedb, b.header, tx, &b.header.GasUsed, feeAmount, vm.Config{})
//		if err != nil {
//			panic(err)
//		}
//		b.txs = append(b.txs, tx)
//		recpts[indexs[i]] = receipt
//	}
//
//	b.txs = otxs
//	b.receipts = recpts
//}
