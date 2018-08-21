// Copyright 2014 The go-ethereum Authors
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
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	ethash "github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core/fastchain"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"testing"
)

// So we can deterministically seed different blockchains
var (
	canonicalSeed = 1
	forkSeed      = 2
)

// newCanonical creates a chain database, and injects a deterministic canonical
// chain. Depending on the full flag, if creates either a full block chain or a
// header only chain.
func newCanonical(engine consensus.Engine, n int, full bool) (ethdb.Database, *BlockChain, error) {
	var (
		db          = ethdb.NewMemDatabase()
		BaseGenesis = new(Genesis)
	)

	BaseGenesis.Fast = new(fastchain.Genesis)
	genesis := BaseGenesis.Fast.MustCommit(db)
	// Initialize a fresh chain with only a genesis block
	//初始化一个新链
	blockchain, _ := NewBlockChain(db, nil, params.AllEthashProtocolChanges, engine, vm.Config{})
	// Create and inject the requested chain
	if n == 0 {
		return db, blockchain, nil
	}
	if full {
		// Full block-chain requested
		blocks := makeBlockChain(genesis, n, engine, db, canonicalSeed)
		_, err := blockchain.InsertChain(blocks)
		return db, blockchain, err
	}
	// Header-only chain requested
	headers := makeHeaderChain(genesis.Header(), n, engine, db, canonicalSeed)
	_, err := blockchain.InsertHeaderChain(headers, 1)
	return db, blockchain, err
}

//测试块插入到链上
func TestInsertBlock(t *testing.T) {

	_, blockchain, err := newCanonical(ethash.NewFaker(), 0, true)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	blocks := makeBlockChain(blockchain.CurrentBlock(), 1, ethash.NewFullFaker(), blockchain.db, 0)
	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatalf("Failed to insert block: %v", err)
	}

	t.Log("this block number:", blocks[0].NumberU64())
	t.Log("this block hash:", blocks[0].Hash())

	//获取生成块的hash
	thast := rawdb.ReadHeadBlockHash(blockchain.db)
	if blocks[len(blocks)-1].Hash() != thast {
		t.Fatalf("Write/Get HeadBlockHash failed")
	}

	t.Log("this block number:", *rawdb.ReadHeaderNumber(blockchain.db, thast))
	t.Log("this block hash:", thast)
}

func newSendTransaction(nonce uint64,bc *BlockChain, datasize int) *types.Transaction {
	var (
		//sendInterval = 5 * time.Second // Time interval to send record

		//sendAddrHex = "970e8128ab834e8eac17ab8e3812f010678cf791"
		sendPrivHex = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"

		recvAddrHex = "68f2517b6c597ede0ae7c0559cdd4a84fd08c928"
	)

	recvAddr := common.HexToAddress(recvAddrHex)
	signer := types.NewEIP155Signer(bc.chainConfig.ChainID)
	acc, _ := crypto.HexToECDSA(sendPrivHex)

	tx := types.NewTransaction(nonce, recvAddr, big.NewInt(1e+18), 100000, big.NewInt(1e+12), make([]byte, datasize))
	tx, _ = types.SignTx(tx, signer, acc)
	return tx
}

//测试块插入到链上（含交易）
func TestInsertTexBlock(t *testing.T) {

	var(
		db         = ethdb.NewMemDatabase()
		key, _     = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address    = crypto.PubkeyToAddress(key.PublicKey)
		funds      = big.NewInt(1000000000)
		deleteAddr = common.Address{1}
		gspec      = &fastchain.Genesis{
			Config: &params.ChainConfig{ChainID: big.NewInt(1), EIP158Block: big.NewInt(0),HomesteadBlock : big.NewInt(0)},
			Alloc:  types.GenesisAlloc{address: {Balance: funds}, deleteAddr: {Balance: new(big.Int)}},
		}
	)



	//初始化一个新链
	blockchain, err:= NewBlockChain(db, nil, params.AllEthashProtocolChanges, ethash.NewFaker(), vm.Config{})


	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()
	//blocks := makeBlockChain(blockchain.CurrentBlock(), 1, ethash.NewFullFaker(), blockchain.db, 0)
	//blocks, _ := GenerateChain(params.TestChainConfig, parent, engine, db, n, func(i int, b *BlockGen) {
	//	b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	//})


	genesis := gspec.MustCommit(blockchain.db)
	blocks, _ := GenerateChain(params.AllEthashProtocolChanges,genesis, ethash.NewFaker(), blockchain.db, 1, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				return types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, big.NewInt(0), 21000, big.NewInt(0), nil), signer, key)
			}
		)

		switch i {
		case 0:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		case 2:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)

			tx, err = basicTx(types.NewEIP155Signer(gspec.Config.ChainID))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		case 3:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)

			tx, err = basicTx(types.NewEIP155Signer(gspec.Config.ChainID))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		}
	})


	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatalf("Failed to insert block: %v", err)
	}

	t.Log("this block number:", blocks[0].NumberU64())
	t.Log("this block hash:", blocks[0].Hash())


	//获取生成块的hash
	thast := rawdb.ReadHeadBlockHash(blockchain.db)
	if blocks[len(blocks)-1].Hash() != thast {
		t.Fatalf("Write/Get HeadBlockHash failed")
	}

	t.Log("this block number:", *rawdb.ReadHeaderNumber(blockchain.db, thast))
	t.Log("this block hash:", thast)
}
