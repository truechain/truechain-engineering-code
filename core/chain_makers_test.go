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
	"fmt"
	"math/big"
	"testing"
	"time"
	"crypto/ecdsa"
	"bytes"
	"runtime"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/common"
)

func ExampleGenerateChain() {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
		db      = etruedb.NewMemDatabase()
	)

	// Ensure that key1 has some funds in the genesis block.
	gspec := &Genesis{
		Config: &params.ChainConfig{},
		Alloc:  types.GenesisAlloc{addr1: {Balance: big.NewInt(1000000)}},
	}
	genesis := gspec.MustFastCommit(db)

	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.
	signer := types.NewTIP1Signer(nil)
	chain, _ := GenerateChain(gspec.Config, genesis, minerva.NewFaker(), db, 5, func(i int, gen *BlockGen) {
		switch i {
		case 0:
			// In block 1, addr1 sends addr2 some ether.
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(10000), params.TxGas, big.NewInt(1), nil), signer, key1)
			gen.AddTx(tx)
		case 1:
			// In block 2, addr1 sends some more ether to addr2.
			// addr2 passes it on to addr3.
			tx1, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(1000), params.TxGas, big.NewInt(1), nil), signer, key1)
			tx2, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr2), addr3, big.NewInt(1000), params.TxGas, big.NewInt(1), nil), signer, key2)
			gen.AddTx(tx1)
			gen.AddTx(tx2)
		case 2:
			// Block 3 is empty but was mined by addr3.
			gen.SetCoinbase(addr3)
			gen.SetExtra([]byte("yeehaw"))
		}
	})

	// Import the chain. This runs all block validation rules.
	blockchain, _ := NewBlockChain(db, nil, gspec.Config, minerva.NewFaker(), vm.Config{})
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
	// balance of addr1: 989000
	// balance of addr2: 10000
	// balance of addr3: 19687500000000001000
}

func TestMakeBlock1(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		key4, _ = crypto.HexToECDSA("42c4d734786eedaf5d0c51fd2bc9bbaa6c289ed23710d9381072932456aeca18")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
		addr4   = crypto.PubkeyToAddress(key4.PublicKey)
		db      = ethdb.NewMemDatabase()
	)
	recvAddr := crypto.CreateAddress(common.Address{0},10)
	// Ensure that key1 has some funds in the genesis block.
	gspec := &Genesis{
		Config: &params.ChainConfig{HomesteadBlock: new(big.Int)},
		GasLimit: 105000000,
		Alloc:  GenesisAlloc{
			addr1: {Balance: big.NewInt(5000000000)},
			addr2: {Balance: big.NewInt(5000000000)},
			addr3: {Balance: big.NewInt(5000000000)},
			addr4: {Balance: big.NewInt(5000000000)},
		},
	}
	genesis := gspec.MustCommit(db)

	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.
	signer := types.HomesteadSigner{}
	cnt := 4000/4
	finish := make(chan int)	
	type tmp struct {
		addr common.Address 
		key *ecdsa.PrivateKey
	}
	addrs := []tmp {
		{addr1,key1},
		{addr2,key2},
		{addr3,key3},
		{addr4,key4},
	}
	begin := time.Now()
	chain, _ := GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, 1, func(i int, gen *BlockGen) {
		sum := 0
		for i:=0;i<3;i++ {
			go func(addr common.Address,key *ecdsa.PrivateKey){
				for n:=0;n<cnt;n++ {
					tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr), recvAddr, big.NewInt(1000), params.TxGas, nil, nil), signer, key)
					gen.AddTx(tx)
				}
				finish <- 1
			}(addrs[i].addr,addrs[i].key)
		}
		for {
			<-finish
			sum++
			if sum == 4 {
				return
			}
		}
	})
	d := time.Now().Sub(begin);
	fmt.Println("make block include", cnt, " txs,cost time ",d.Seconds())
	// Import the chain. This runs all block validation rules.
	blockchain, _ := NewBlockChain(db, nil, gspec.Config, ethash.NewFaker(), vm.Config{}, nil)
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
}
func TestMakeBlock2(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		key4, _ = crypto.HexToECDSA("42c4d734786eedaf5d0c51fd2bc9bbaa6c289ed23710d9381072932456aeca18")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
		addr4   = crypto.PubkeyToAddress(key4.PublicKey)
		db      = ethdb.NewMemDatabase()
		tmpDB   = ethdb.NewMemDatabase()
	)
	recvAddr := crypto.CreateAddress(common.Address{0},10)
	recvAddr2 := crypto.CreateAddress(common.Address{0},50)
	// Ensure that key1 has some funds in the genesis block.
	gspec := &Genesis{
		Config: &params.ChainConfig{HomesteadBlock: new(big.Int)},
		GasLimit: 105000000,
		Alloc:  GenesisAlloc{
			addr1: {Balance: big.NewInt(5000000000)},
			addr2: {Balance: big.NewInt(5000000000)},
			addr3: {Balance: big.NewInt(5000000000)},
			addr4: {Balance: big.NewInt(5000000000)},
		},
	}
	genesis := gspec.MustCommit(db)
	genesis2 := gspec.MustCommit(tmpDB)

	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.
	signer := types.HomesteadSigner{}
	cnt := 1
	type tmp struct {
		addr common.Address 
		key *ecdsa.PrivateKey
	}
	addrs := []tmp {
		{addr1,key1},
		{addr2,key2},
		{addr3,key3},
		{addr4,key4},
	}
	ptxs1 := make([]*types.Transaction,0,0) 
	ptxs2 := make([]*types.Transaction,0,0) 
	begin := time.Now()
	block1, recpt1 := GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, 1, func(i int, gen *BlockGen) {
		from,key := addrs[0].addr,addrs[0].key
		for i:=0;i<cnt;i++ {
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(from), recvAddr, big.NewInt(1000), params.TxGas, nil, nil), signer, key)
			ptxs1 = append(ptxs1,tx)
			gen.AddTx(tx)
		}
		from,key = addrs[1].addr,addrs[1].key
		for i:=0;i<cnt;i++ {
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(from), recvAddr2, big.NewInt(1000), params.TxGas, nil, nil), signer, key)
			ptxs2 = append(ptxs2,tx)
			gen.AddTx(tx)
		}
	})
	d := time.Now().Sub(begin);
	fmt.Println("make block include", cnt, " txs,cost time ",d.Seconds())
	sum := len(ptxs1) + len(ptxs2)
	indexs := make([]int,sum,sum)
	// ptxs2 = append(ptxs1,ptxs2...)
	// for i,_ := range ptxs2 {
	// 	indexs[i] = i
	// }
	ptxs2 = append(ptxs2,ptxs1...)
	pos := 0
	for i:=2*cnt-1;i>=cnt;i-- {
		indexs[pos] = i
		pos++ 
	}
	for i:=cnt-1;i>=0;i-- {
		indexs[pos] = i
		pos++ 
	}
	/////////////////////////////////////////////////////////////////////////////////
	block2, recpt2 := GenerateChain(gspec.Config, genesis2, ethash.NewFaker(), tmpDB, 1, func(i int, gen *BlockGen) {
		gen.AddTxWithChain2(nil,block1[0].Transactions(),ptxs2,indexs)
	})
	/////////////////////////////////////////////////////////////////////////////////
	fmt.Println("r1.hash1:",recpt1[0].GetRlp(0),"r1.hash2",recpt1[0].GetRlp(1))
	fmt.Println("r2.hash1:",recpt2[0].GetRlp(0),"r2.hash2",recpt2[0].GetRlp(1))
	if bytes.Equal(recpt1[0].GetRlp(0),recpt2[0].GetRlp(0)) {
		fmt.Println("recpt index 0 equal")
	}
	if bytes.Equal(recpt1[0].GetRlp(0),recpt2[0].GetRlp(1)) {
		fmt.Println("recpt index 0---1 equal")
	}
	if bytes.Equal(recpt1[0].GetRlp(1),recpt2[0].GetRlp(0)) {
		fmt.Println("recpt index 1---0 equal")
	}
	if bytes.Equal(recpt1[0].GetRlp(1),recpt2[0].GetRlp(1)) {
		fmt.Println("recpt index 1 equal")
	}

	fmt.Println("height:",block1[0].NumberU64(),"hash1",block1[0].Hash(),"root:",block1[0].Root(),"rhash:",block1[0].ReceiptHash(),"txhash:",block1[0].TxHash())
	fmt.Println("height:",block2[0].NumberU64(),"hash1",block2[0].Hash(),"root:",block2[0].Root(),"rhash:",block2[0].ReceiptHash(),"txhash:",block2[0].TxHash())

	fmt.Println("finish")
}
func TestVerifySignInBatch(t *testing.T) {
	fmt.Println("begin......,cpu",runtime.NumCPU())
	verifyInBatch(100,1)
	verifyInBatch(100,4)
	verifyInBatch(100,10)
	verifyInBatch(10000,1)
	verifyInBatch(10000,4)
	verifyInBatch(10000,10)
	fmt.Println("finish.....")
}

func verifyInBatch(sum,count int) {
	var (
		db      = ethdb.NewMemDatabase()
	)
	type tmpItem struct {
		sign 	[]byte
		priv	*ecdsa.PrivateKey
		hash 	common.Hash
	}	
	gspec := &Genesis{
		Config: &params.ChainConfig{HomesteadBlock: new(big.Int)},
		GasLimit: 105000000,
		Alloc:  GenesisAlloc{},
	}
	genesis := gspec.MustCommit(db)
	block1, _ := GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, 1, func(i int, gen *BlockGen) {
	})
	block := block1[0]
	hash := block.Hash()

	items := make([]*tmpItem,sum,sum)
	for i:=0;i<sum;i++ {
		p,_ := crypto.GenerateKey()
		sign,_ := crypto.Sign(hash[:],p)
		items[i] = &tmpItem{
			sign:		sign,
			priv:		p,
			hash:		hash,
		}
	}

	finish := make(chan bool,count)
	fmt.Println("begin verify in batch...")
	begin := time.Now()

	execFunc := func(index int,infos []*tmpItem){
		taskbegin := time.Now()
		for _,v := range infos {
			crypto.SigToPub(v.hash[:],v.sign)
		}
		d := time.Now().Sub(taskbegin);
		finish<-true
		fmt.Println("index",index,"verify sign result, cost time: ",d.Seconds())
	}
	batch := sum / count
	for i:=0;i<count;i++ {
		b,e := i*batch,(i+1)*batch-1
		if i == count -1 {
			e = sum
		}
		info := items[b:e]
		go execFunc(i,info)
	}

	calcCount := 0
	for {
		<-finish
		calcCount++
		if calcCount == count {
			break
		}
	}
	d := time.Now().Sub(begin);
	fmt.Println("all task done, sign Sum:",sum,"task count:",count,"cost time: ",d.Seconds())
}
// for test
func (b *BlockGen) AddTxWithChain2(bc *BlockChain, otxs,txs []*types.Transaction,indexs []int) {
	if b.gasPool == nil {
		b.SetCoinbase(common.Address{})
	}
	
	recpts := make([]*types.Receipt,len(otxs),len(otxs))

	for i,tx := range txs {
		b.statedb.Prepare(tx.Hash(), common.Hash{}, indexs[i])
		receipt, _, err := ApplyTransaction(b.config, bc, &b.header.Coinbase, b.gasPool, b.statedb, b.header, tx, &b.header.GasUsed, vm.Config{})
		if err != nil {
			panic(err)
		}
		b.txs = append(b.txs, tx)
		recpts[indexs[i]] = receipt
	}

	b.txs = otxs
	b.receipts = recpts
}
