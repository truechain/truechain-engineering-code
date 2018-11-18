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

package snailchain

import (
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/params"
	"fmt"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	//"github.com/truechain/truechain-engineering-code/etrue"

)

// BlockGen creates blocks for testing.
// See GenerateChain for a detailed explanation.
type BlockGen struct {
	i           int
	parent      *types.SnailBlock
	chain       []*types.SnailBlock
	chainReader consensus.SnailChainReader
	header      *types.SnailHeader

	//gasPool *GasPool
	uncles []*types.SnailHeader

	fruits []*types.SnailBlock
	signs  []*types.PbftSign

	config *params.ChainConfig
	engine consensus.Engine
}

// SetCoinbase sets the coinbase of the generated block.
// It can be called at most once.
func (b *BlockGen) SetCoinbase(addr common.Address) {
	if len(b.fruits) > 0 {
		panic("coinbase must be set before adding fruits")
	}
	b.header.Coinbase = addr
	//TODO not gaslimit 20180804
	//b.gasPool = new(GasPool).AddGas(b.header.GasLimit)
}

func (b *BlockGen) AddFruit(block *types.SnailBlock) {
	b.fruits = append(b.fruits, block)
}

// SetExtra sets the extra data field of the generated block.
func (b *BlockGen) SetExtra(data []byte) {
	b.header.Extra = data
}

// Number returns the block number of the block being generated.
func (b *BlockGen) Number() *big.Int {
	return new(big.Int).Set(b.header.Number)
}

// AddUncle adds an uncle header to the generated block.
func (b *BlockGen) AddUncle(h *types.SnailHeader) {
	b.uncles = append(b.uncles, h)
}

// PrevBlock returns a previously generated block by number. It panics if
// num is greater or equal to the number of the block being generated.
// For index -1, PrevBlock returns the parent block given to GenerateChain.
func (b *BlockGen) PrevBlock(index int) *types.SnailBlock {
	if index >= b.i {
		panic("block index out of range")
	}
	if index == -1 {
		return b.parent
	}
	return b.chain[index]
}

// OffsetTime modifies the time instance of a block, implicitly changing its
// associated difficulty. It's useful to test scenarios where forking is not
// tied to chain length directly.
func (b *BlockGen) OffsetTime(seconds int64) {
	b.header.Time.Add(b.header.Time, new(big.Int).SetInt64(seconds))
	if b.header.Time.Cmp(b.parent.Header().Time) <= 0 {
		panic("block time out of range")
	}

	b.header.Difficulty = b.engine.CalcSnailDifficulty(b.chainReader, b.header.Time.Uint64(), []*types.SnailHeader{b.parent.Header()})
}

// GenerateChain creates a chain of n blocks. The first block's
// parent will be the provided parent. db is used to store
// intermediate states and should contain the parent's state trie.
//
// The generator function is called with a new block generator for
// every block. Any transactions and uncles added to the generator
// become part of the block. If gen is nil, the blocks will be empty
// and their coinbase will be the zero address.
//
// Blocks created by GenerateChain do not contain valid proof of work
// values. Inserting them into BlockChain requires use of FakePow or
// a similar non-validating proof of work implementation.
func GenerateChain(config *params.ChainConfig, fastChain *core.BlockChain, parent *types.SnailBlock, engine consensus.Engine, db ethdb.Database, n int, gen func(int, *BlockGen)) []*types.SnailBlock {
	if config == nil {
		config = params.TestChainConfig
	}
	blocks := make(types.SnailBlocks, n)
	genblock := func(i int, parent *types.SnailBlock) *types.SnailBlock {
		// TODO(karalabe): This is needed for clique, which depends on multiple blocks.
		// It's nonetheless ugly to spin up a blockchain here. Get rid of this somehow.
		blockchain, _ := NewSnailBlockChain(db, nil, config, engine, vm.Config{})
		blockchain.SetValidator(NewBlockValidator(config, fastChain, blockchain, engine))
		defer blockchain.Stop()
		blocks := make(types.SnailBlocks, 2)
		blocks[0] = blockchain.genesisBlock
		blocks[1] = parent
		blockchain.InsertChain(blocks)
		b := &BlockGen{i: i, parent: parent, chain: blocks, chainReader: blockchain, config: config, engine: engine}
		b.header = makeHeader(b.chainReader, parent, b.engine)

		// Execute any user modifications to the block and finalize it
		if gen != nil {
			gen(i, b)
		}

		if b.engine != nil {
			// TODO: add fruits support
			block, error := MakeSnailBlockFruit(blockchain, fastChain, i, params.MinimumFruits, blockchain.genesisBlock.PublicKey(), blockchain.genesisBlock.Coinbase(), true, blockchain.genesisBlock.BlockDifficulty())
			if error != nil {
				panic(error)
			}

			//block, _ := b.engine.FinalizeSnail(b.chainReader, b.header, b.uncles, b.fruits, b.signs)
			return block
		}
		return nil
	}
	for i := 0; i < n; i++ {
		block := genblock(i, parent)
		blocks[i] = block
		parent = block
	}
	return blocks
}

func makeHeader(chain consensus.SnailChainReader, parent *types.SnailBlock, engine consensus.Engine) *types.SnailHeader {

	var time *big.Int
	if parent.Time() == nil {
		time = big.NewInt(10)
	} else {
		time = new(big.Int).Add(parent.Time(), big.NewInt(10)) // block time is fixed at 10 seconds
	}

	return &types.SnailHeader{
		ParentHash: parent.Hash(),
		Coinbase:   parent.Coinbase(),
		Difficulty: engine.CalcSnailDifficulty(chain, time.Uint64(), []*types.SnailHeader{{
			Number:     parent.Number(),
			Time:       new(big.Int).Sub(time, big.NewInt(10)),
			Difficulty: parent.BlockDifficulty(),
			UncleHash:  parent.UncleHash(),
		}}),
		Number: new(big.Int).Add(parent.Number(), common.Big1),
		Time:   time,
	}
}

// makeHeaderChain creates a deterministic chain of headers rooted at parent.
func makeHeaderChain(fastChain *core.BlockChain, parent *types.SnailHeader, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.SnailHeader {
	blocks := makeBlockChain(fastChain, types.NewSnailBlockWithHeader(parent), n, engine, db, seed)
	headers := make([]*types.SnailHeader, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	return headers
}

// makeBlockChain creates a deterministic chain of blocks rooted at parent.
func makeBlockChain(fastChain *core.BlockChain, parent *types.SnailBlock, n int, engine consensus.Engine, db ethdb.Database, seed int) []*types.SnailBlock {
	if fastChain.CurrentBlock().NumberU64() == 0 {
		fastblocks, _ := core.GenerateChain(params.TestChainConfig, fastChain.CurrentBlock(), engine, db, n*params.MinimumFruits, func(i int, b *core.BlockGen) {
			b.SetCoinbase(common.Address{0: byte(1), 19: byte(i)})
		})

		fastChain.InsertChain(fastblocks)
	}
	blocks := GenerateChain(params.TestChainConfig, fastChain, parent, engine, db, n, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})

	return blocks
}

//create block,fruit
// chain: for snail chain
// fastchian: for fast chain
// makeStartFastNum,makeFruitSize :if you create  a block the fruitset  startnumber and size this is fastblock number
//pubkey : for election
// coinbaseAddr: for coin
func MakeSnailBlockFruit(chain *SnailBlockChain, fastchain *core.BlockChain, makeBlockNum int, makeFruitSize int,
	pubkey []byte, coinbaseAddr common.Address, isBlock bool, diff *big.Int) (*types.SnailBlock, error) {

	var parent = chain.CurrentBlock()
	var fruitsetCopy []*types.SnailBlock
	var pointerHashFresh = big.NewInt(7)
	var snailFruitsLastFastNumber *big.Int

	if chain == nil {
		return nil, fmt.Errorf("chain is nil")
	}

	chain.SetValidator(NewBlockValidator(nil, fastchain, chain, chain.Engine()))

	// create head
	if parent.Fruits() != nil && len(parent.Fruits()) != 0 {
		snailFruitsLastFastNumber = parent.Fruits()[len(parent.Fruits())-1].FastNumber()
	} else {
		snailFruitsLastFastNumber = new(big.Int).SetUint64(0)
	}

	//parentNum := parent.Number()
	makeStartFastNum := int(new(big.Int).Add(snailFruitsLastFastNumber, big.NewInt(1)).Int64());
	if isBlock {
		if makeFruitSize < params.MinimumFruits || snailFruitsLastFastNumber.Int64() >= int64(makeStartFastNum) {
			return nil, fmt.Errorf("fruitSet is nill or size less then 60, %d, %d", snailFruitsLastFastNumber, makeStartFastNum)
		}
	}

	makeHead := func(chain *SnailBlockChain, pubkey []byte, coinbaseAddr common.Address, fastNumber *big.Int, isFruit bool) (*types.SnailHeader) {
		//num := parent.Number()
		var fruitDiff  *big.Int
		if isFruit{
			fruitDiff=diff
		}
		var tstamp *big.Int
		if parent.Time() == nil {
			tstamp = big.NewInt(10)
		} else {
			tstamp = new(big.Int).Add(parent.Time(), big.NewInt(10)) // block time is fixed at 10 seconds
		}
		header := &types.SnailHeader{
			ParentHash: parent.Hash(),
			Publickey:  pubkey,
			Number:     new(big.Int).SetUint64(uint64(makeBlockNum)),
			Time:       tstamp,
			Coinbase:   coinbaseAddr,
			Fruit:      isFruit,
			FastNumber: fastNumber,
			Difficulty: diff,
			FruitDifficulty:fruitDiff,
			FastHash:fastchain.GetBlockByNumber(fastNumber.Uint64()).Hash(),
		}

		pointerNum := new(big.Int).Sub(parent.Number(), pointerHashFresh)
		if pointerNum.Cmp(common.Big0) < 0 {
			pointerNum = new(big.Int).Set(common.Big0)
		}
		pointer := chain.GetBlockByNumber(pointerNum.Uint64())
		header.PointerHash = pointer.Hash()
		header.PointerNumber = pointer.Number()
		if !isFruit {
			header.Difficulty = minerva.CalcDifficulty(chain.Config(), header.Time.Uint64(), minerva.GetParents(chain, header))
		}

		return header
	}

	copySignsByFastNum := func(fc *core.BlockChain, fNumber *big.Int) ([]*types.PbftSign, error) {

		if fc.CurrentBlock().Number().Cmp(fNumber) < 0 {
			return nil, fmt.Errorf("fastblocknumber highter then fast chain hight")
		}

		fastSigns := fc.GetBlockByNumber(fNumber.Uint64()).Signs()
		return fastSigns, nil

	}

	makeFruit := func(chain *SnailBlockChain, fChain *core.BlockChain, fastNumber *big.Int, pubkey []byte, coinbaseAddr common.Address) (*types.SnailBlock, error) {

		head := makeHead(chain, pubkey, coinbaseAddr, fastNumber, true)
		pointer := chain.GetHeader(head.PointerHash, head.PointerNumber.Uint64())
		fastBlock := fChain.GetBlockByNumber(fastNumber.Uint64())
		head.FastHash = fastBlock.Hash()
		head.FruitDifficulty = minerva.CalcFruitDifficulty(chain.chainConfig, head.Time.Uint64(), fastBlock.Header().Time.Uint64(), pointer)

		fSign, err := copySignsByFastNum(fChain, fastNumber)
		if err != nil {
			return nil, err
		}

		fruit := types.NewSnailBlock(
			head,
			nil,
			fSign,
			nil,
		)
		return fruit, nil
	}

	// creat fruits
	if isBlock {
		for i := makeStartFastNum; i < makeStartFastNum+makeFruitSize; i++ {
			fruit, err := makeFruit(chain, fastchain, new(big.Int).SetInt64(int64(i)), pubkey, coinbaseAddr)
			if err != nil {
				return nil, err
			}
			fruitsetCopy = append(fruitsetCopy, fruit)
		}
		if len(fruitsetCopy) != makeFruitSize {
			return nil, fmt.Errorf("fruits make fail the length less then makeFruitSize")
		}

		fSign, err := copySignsByFastNum(fastchain, new(big.Int).SetUint64(uint64(makeStartFastNum)))
		if err != nil {
			return nil, err
		}

		block := types.NewSnailBlock(
			makeHead(chain, pubkey, coinbaseAddr, new(big.Int).SetInt64(int64(makeStartFastNum)), false),
			fruitsetCopy,
			fSign,
			nil,
		)
		return block, nil

	} else {
		fruit, err := makeFruit(chain, fastchain, new(big.Int).SetInt64(int64(makeStartFastNum)), pubkey, coinbaseAddr)
		if err != nil {
			return nil, err
		}
		return fruit, nil
	}

	return nil, nil

}

func MakeSnailBlockFruits(chain *SnailBlockChain, fastchain *core.BlockChain, makeStarblockNumber int, makeblockSize int,
	makeStartFastNum int, makeFruitSize int, pubkey []byte, coinbaseAddr common.Address, isBlock bool, diff *big.Int) ([]*types.SnailBlock, error) {

	var blocks types.SnailBlocks
	//var snailFruitsLastFastNumber *big.Int
	//blocks = make(types.SnailBlock,makeblockSize)

	j := 1
	//parent := chain.genesisBlock
	for i := makeStarblockNumber; i < makeblockSize+makeStarblockNumber; i++ {

		block, err := MakeSnailBlockFruit(chain, fastchain, i, params.MinimumFruits, pubkey, coinbaseAddr, true, diff)
		if err != nil {
			return nil, err
		}
		var blocks types.SnailBlocks

		blocks = append(blocks, block)
		if _, error := chain.InsertChain(blocks); error != nil {
			panic(error)
		}
		//parent = block
		//blocks = append(blocks, block)
		j++
	}

	return blocks, nil
}

func MakeChain(fastBlockNumbers int, snailBlockNumbers int) (*SnailBlockChain, *core.BlockChain) {
	var (
		testdb = ethdb.NewMemDatabase()
		genesis = core.DefaultGenesisBlock()
		engine  = minerva.NewFaker()
		fruitnumbers int
	)
	cache := &core.CacheConfig{
		//TrieNodeLimit: etrue.DefaultConfig.TrieCache,
		//TrieTimeLimit: etrue.DefaultConfig.TrieTimeout,
	}

	if fastBlockNumbers < snailBlockNumbers*params.MinimumFruits {
		return nil, nil
	}

	fastGenesis := genesis.MustFastCommit(testdb)
	fastchain, _ := core.NewBlockChain(testdb, cache, params.AllMinervaProtocolChanges, engine, vm.Config{})
	//fastblocks := makeFast(fastGenesis, n * params.MinimumFruits, engine, testdb, canonicalSeed)

	//engine.SetElection(core.NewFakeElection())
	fastblocks, _ := core.GenerateChain(params.TestChainConfig, fastGenesis, engine, testdb, fastBlockNumbers, func(i int, b *core.BlockGen) {
		b.SetCoinbase(common.Address{0: byte(1), 19: byte(i)})
	})

	fastchain.InsertChain(fastblocks)

	snailGenesis := genesis.MustSnailCommit(testdb)
	snailChain, _ := NewSnailBlockChain(testdb, nil, params.TestChainConfig, engine, vm.Config{})
	snailChain.SetValidator(NewBlockValidator(nil, fastchain, snailChain, engine))

	if fastBlockNumbers > snailBlockNumbers * params.MinimumFruits{
		fruitnumbers = snailBlockNumbers * params.MinimumFruits
	}

	_, err := MakeSnailBlockFruits(snailChain, fastchain, 1, snailBlockNumbers, 1, fruitnumbers, snailGenesis.PublicKey(), snailGenesis.Coinbase(), true, big.NewInt(20000))
	if err != nil {
		panic(err)
		return nil, nil
	}

	return snailChain, fastchain
}
