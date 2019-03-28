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
	"github.com/ethereum/go-ethereum/log"
	"math/big"

	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
	//"github.com/truechain/truechain-engineering-code/etrue"
)

// BlockGen creates blocks for testing.
// See GenerateChain for a detailed explanation.
type BlockGen struct {
	i         int
	fastChain *core.BlockChain
	parent    *types.SnailBlock
	chain     []*types.SnailBlock
	header    *types.SnailHeader

	//gasPool *GasPool
	uncles []*types.SnailHeader

	fruits []*types.SnailBlock

	config    *params.ChainConfig
	chainRead consensus.SnailChainReader
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

//AddFruit add a given fruit into the fruit array
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

	b.header.Difficulty = minerva.CalcDifficulty(b.config, b.header.Time.Uint64(), minerva.GetParents(b.chainRead, b.header))
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
func GenerateChain(config *params.ChainConfig, fastChain *core.BlockChain, parents []*types.SnailBlock, n int, freshPoint int, gen func(int, *BlockGen)) []*types.SnailBlock {
	if config == nil {
		config = params.TestChainConfig
	}
	if int(fastChain.CurrentBlock().NumberU64())/params.MinimumFruits < len(parents) {
		log.Info("GenerateChain fast block already use over", "parents", len(parents), "number", fastChain.CurrentBlock().Number(), "n", n)
		return nil
	}

	var blocks []*types.SnailBlock
	blocks = append(blocks, parents...)
	parent := parents[len(parents)-1]
	log.Info("GenerateChain", "blocks", len(blocks), "number", parent.Number(), "n", n)

	genblock := func(i int, parent *types.SnailBlock, chain []*types.SnailBlock) *types.SnailBlock {
		var fruitSet []*types.SnailBlock
		var fruitparent *types.SnailBlock
		chainreader := &fakeChainReader{config, chain}
		b := &BlockGen{i: i, fastChain: fastChain, parent: parent, chain: blocks, config: config, chainRead: chainreader}
		fast := fastChain.GetBlockByNumber(parent.FastNumber().Uint64() + 1)
		b.header = makeHeader(chainreader, parent, fast)

		// Execute any user modifications to the block and finalize it
		if gen != nil {
			gen(i, b)
		}

		if len(parent.Fruits()) > 0 {
			fruitparent = parent.Fruits()[len(parent.Fruits())-1]
		}

		var fastNumber *big.Int
		for i := 0; i < params.MinimumFruits; i++ {
			if fruitparent != nil {
				fastNumber = new(big.Int).Add(fruitparent.FastNumber(), common.Big1)
			} else {
				fastNumber = new(big.Int).Add(parent.Number(), common.Big1)
			}
			fast := fastChain.GetBlockByNumber(fastNumber.Uint64())
			fruit, err := makeFruit(chainreader, fast, parent, freshPoint)
			if err != nil {
				return nil
			}
			fruitparent = fruit
			fruitSet = append(fruitSet, fruit)
		}

		if len(fruitSet) != params.MinimumFruits {
			log.Warn("fruits make fail the length less then makeFruitSize")
			return nil
		}

		return types.NewSnailBlock(b.header, fruitSet, nil, nil)
	}
	for i := 0; i < n; i++ {
		if int(fastChain.CurrentBlock().NumberU64())/params.MinimumFruits < i+len(parents) {
			break
		}
		block := genblock(i, parent, blocks)
		blocks = append(blocks, block)
		parent = block
		log.Info("Make snail block", "blocks", len(blocks[1:]), "number", parent.Number(), "i", i)
	}
	return blocks[len(parents):]
}

func makeHeader(chain consensus.SnailChainReader, parent *types.SnailBlock, fast *types.Block) *types.SnailHeader {
	var time *big.Int
	if parent.Time() == nil {
		time = big.NewInt(10)
	} else {
		time = new(big.Int).Add(parent.Time(), big.NewInt(10)) // block time is fixed at 10 seconds
	}

	header := &types.SnailHeader{
		ParentHash: parent.Hash(),
		Coinbase:   parent.Coinbase(),
		Publickey:  parent.PublicKey(),
		Number:     new(big.Int).Add(parent.Number(), common.Big1),
		Time:       time,
		FastNumber: fast.Number(),
		FastHash:   fast.Hash(),
	}
	header.Difficulty = minerva.CalcDifficulty(chain.Config(), header.Time.Uint64(), minerva.GetParents(chain, header))

	log.Info("makeBlockHead", "parent", parent.Number(), "fastNumber", fast.Number())
	return header
}

func makeFruit(chain consensus.SnailChainReader, fast *types.Block, parent *types.SnailBlock, fresh int) (*types.SnailBlock, error) {

	head := makeFruitHead(chain, fast, parent, fresh)
	head.FastHash = fast.Hash()
	pointer := chain.GetHeader(head.PointerHash, head.PointerNumber.Uint64())
	head.FruitDifficulty = minerva.CalcFruitDifficulty(chain.Config(), head.Time.Uint64(), fast.Header().Time.Uint64(), pointer)

	log.Info("makeFruit", "parent", parent.Number(), "pointer", pointer.Number, "fastNumber", fast.NumberU64())

	fruit := types.NewSnailBlock(
		head,
		nil,
		fast.Signs(),
		nil,
	)
	return fruit, nil
}

func makeFruitHead(chain consensus.SnailChainReader, fastBlock *types.Block, parent *types.SnailBlock, fresh int) *types.SnailHeader {
	var time *big.Int
	if parent.Time() == nil {
		time = big.NewInt(10)
	} else {
		time = new(big.Int).Add(parent.Time(), big.NewInt(10)) // block time is fixed at 10 seconds
	}

	header := &types.SnailHeader{
		ParentHash:      parent.Hash(),
		Publickey:       parent.PublicKey(),
		Number:          new(big.Int).Add(parent.Number(), common.Big1),
		Time:            time,
		Coinbase:        parent.Coinbase(),
		FastNumber:      fastBlock.Number(),
		FruitDifficulty: parent.Difficulty(),
		FastHash:        fastBlock.Hash(),
	}

	pointerNum := new(big.Int).Sub(parent.Number(), new(big.Int).SetInt64(int64(fresh)))
	if pointerNum.Cmp(common.Big0) < 0 {
		pointerNum = new(big.Int).Set(common.Big0)
	}

	pointerHeader := chain.GetHeaderByNumber(pointerNum.Uint64())
	header.PointerHash = pointerHeader.Hash()
	header.PointerNumber = pointerHeader.Number
	return header
}

type fakeChainReader struct {
	config *params.ChainConfig
	chain  []*types.SnailBlock
}

// Config returns the chain configuration.
func (cr *fakeChainReader) Config() *params.ChainConfig {
	return cr.config
}

// CurrentHeader retrieves the current header from the local chain.
func (cr *fakeChainReader) CurrentHeader() *types.SnailHeader {
	return cr.chain[len(cr.chain)-1].Header()
}

// GetHeader retrieves a block header from the database by hash and number.
func (cr *fakeChainReader) GetHeader(hash common.Hash, number uint64) *types.SnailHeader {
	return cr.chain[number].Header()
}

// GetHeaderByNumber retrieves a block header from the database by number.
func (cr *fakeChainReader) GetHeaderByNumber(number uint64) *types.SnailHeader {
	return cr.chain[number].Header()
}

// GetHeaderByHash retrieves a block header from the database by its hash.
func (cr *fakeChainReader) GetHeaderByHash(hash common.Hash) *types.SnailHeader { return nil }

// GetBlock retrieves a block from the database by hash and number.
func (cr *fakeChainReader) GetBlock(hash common.Hash, number uint64) *types.SnailBlock {
	return cr.chain[number]
}

//MakeChain return snailChain and fastchain by given fastBlockNumbers and snailBlockNumbers
func MakeChain(fastBlockNumbers int, snailBlockNumbers int, genesis *core.Genesis, engine consensus.Engine) (*SnailBlockChain, *core.BlockChain) {
	var (
		testdb = etruedb.NewMemDatabase()
	)
	cache := &core.CacheConfig{
		//TrieNodeLimit: etrue.DefaultConfig.TrieCache,
		//TrieTimeLimit: etrue.DefaultConfig.TrieTimeout,
	}

	if fastBlockNumbers < snailBlockNumbers*params.MinimumFruits {
		return nil, nil
	}
	log.Info("Make fastchain", "number", snailBlockNumbers, "fast number", fastBlockNumbers)

	fastGenesis := genesis.MustFastCommit(testdb)
	fastchain, _ := core.NewBlockChain(testdb, cache, params.AllMinervaProtocolChanges, engine, vm.Config{})

	fastblocks, _ := core.GenerateChain(params.TestChainConfig, fastGenesis, engine, testdb, fastBlockNumbers, func(i int, b *core.BlockGen) {
		b.SetCoinbase(common.Address{0: byte(1), 19: byte(i)})
	})

	fastchain.InsertChain(fastblocks)
	log.Info("Make SnailBlockChain", "number", fastchain.CurrentBlock().Number(), "fast number", len(fastblocks))

	snailGenesis := genesis.MustSnailCommit(testdb)
	snailChain, _ := NewSnailBlockChain(testdb, params.TestChainConfig, engine, vm.Config{}, fastchain)

	log.Info("MakeChain MakeSnailBlockBlockChain", "number", snailChain.CurrentBlock().Number(), "fast number", snailChain.CurrentFastBlock().Number())

	_, err := MakeSnailBlockBlockChain(snailChain, fastchain, snailGenesis, snailBlockNumbers, big.NewInt(20000))
	if err != nil {
		panic(err)
		return nil, nil
	}

	return snailChain, fastchain
}

//MakeSnailBlockFruits return fruits or blocks by given params and insert these in the chain
func MakeSnailBlockBlockChain(chain *SnailBlockChain, fastchain *core.BlockChain, parent *types.SnailBlock, n int, diff *big.Int) ([]*types.SnailBlock, error) {
	var blocks types.SnailBlocks

	blocks = append(blocks, parent)

	//parent := chain.genesisBlock
	log.Info("MakeSnailBlockBlockChain", "makeblockSize", n)
	for i := 0; i < n; i++ {
		block, err := MakeSnailBlock(chain, fastchain, parent, params.MinimumFruits, diff, blocks)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, block)
		log.Info("Make InsertChain", "blocks", len(blocks), "i", i, "fruit", len(block.Fruits()), "sign", len(block.Signs()), "PointNumber", block.PointNumber(), "FastNumber", block.FastNumber(), "Number", block.Number())

		parent = block
	}
	if _, error := chain.InsertChain(blocks[1:]); error != nil {
		panic(error)
	}
	return blocks, nil
}

//MakeSnailBlockFruit retrieves a snailblock or fruit by given parameter
//create block,fruit
// chain: for snail chain
// fastchian: for fast chain
// makeStartFastNum,makeFruitSize :if you create  a block the fruitset  startnumber and size this is fastblock number
//pubkey : for election
// coinbaseAddr: for coin
func MakeSnailBlock(chain *SnailBlockChain, fastchain *core.BlockChain, parent *types.SnailBlock, makeFruitSize int, diff *big.Int, blocks []*types.SnailBlock) (*types.SnailBlock, error) {
	var fruitSet []*types.SnailBlock
	var fruitparent *types.SnailBlock

	if len(parent.Fruits()) > 0 {
		fruitparent = parent.Fruits()[len(parent.Fruits())-1]
	}

	//var parentFruit *types.SnailBlock
	log.Info("MakeSnailBlock", "diff", diff, "parent", parent.Number(), "FastNumber", parent.FastNumber(), "PointNumber", parent.PointNumber())
	var fastNumber *big.Int
	for i := 0; i < makeFruitSize; i++ {
		if fruitparent != nil {
			fastNumber = new(big.Int).Add(fruitparent.FastNumber(), common.Big1)
		} else {
			fastNumber = new(big.Int).Add(parent.Number(), common.Big1)
		}
		fast := fastchain.GetBlockByNumber(fastNumber.Uint64())
		fruit, err := makeFruit(chain, fast, parent, 7)
		if err != nil {
			return nil, err
		}
		fruitparent = fruit
		fruitSet = append(fruitSet, fruit)
	}

	block := types.NewSnailBlock(
		makeBlockHead(chain, fastchain, parent, blocks),
		fruitSet,
		nil,
		nil,
	)
	return block, nil
}

func makeBlockHead(chain *SnailBlockChain, fastchain *core.BlockChain, parent *types.SnailBlock, blocks []*types.SnailBlock) *types.SnailHeader {
	//num := parent.Number()
	var headers []*types.SnailHeader

	var time *big.Int
	if parent.Time() == nil {
		time = big.NewInt(10)
	} else {
		time = new(big.Int).Add(parent.Time(), big.NewInt(10)) // block time is fixed at 10 seconds
	}
	fastNumber := new(big.Int).Add(parent.FastNumber(), common.Big1)

	header := &types.SnailHeader{
		ParentHash: parent.Hash(),
		Publickey:  parent.PublicKey(),
		Number:     new(big.Int).Add(parent.Number(), common.Big1),
		Time:       time,
		Coinbase:   parent.Coinbase(),
		FastNumber: fastNumber,
		FastHash:   fastchain.GetBlockByNumber(fastNumber.Uint64()).Hash(),
	}

	for _, block := range blocks {
		headers = append(headers, block.Header())
	}
	header.Difficulty = minerva.CalcDifficulty(chain.Config(), header.Time.Uint64(), headers)

	log.Info("makeBlockHead", "parent", parent.Number(), "fastNumber", fastNumber)
	return header
}

// makeHeaderChain creates a deterministic chain of headers rooted at parent.
func makeHeaderChain(fastChain *core.BlockChain, parent *types.SnailHeader, n int, engine consensus.Engine, db etruedb.Database, seed int) []*types.SnailHeader {
	blocks := makeBlockChain(fastChain, []*types.SnailBlock{types.NewSnailBlockWithHeader(parent)}, n, engine, db, seed)
	headers := make([]*types.SnailHeader, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	return headers
}

// makeBlockChain creates a deterministic chain of blocks rooted at parent.
func makeBlockChain(fastChain *core.BlockChain, parents []*types.SnailBlock, n int, engine consensus.Engine, db etruedb.Database, seed int) []*types.SnailBlock {
	if fastChain.CurrentBlock().NumberU64() == 0 {
		fastblocks, _ := core.GenerateChain(params.TestChainConfig, fastChain.CurrentBlock(), engine, db, n*params.MinimumFruits, func(i int, b *core.BlockGen) {
			b.SetCoinbase(common.Address{0: byte(1), 19: byte(i)})
		})

		fastChain.InsertChain(fastblocks)
	}
	blocks := GenerateChain(params.TestChainConfig, fastChain, parents, n, 7, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{0: byte(seed), 19: byte(i)})
	})

	return blocks
}

//MakeSnailBlockFruit retrieves a snailblock or fruit by given parameter
func MakeSnailBlockFruit(chain *SnailBlockChain, fastchain *core.BlockChain, makeBlockNum int, makeFruitSize int,
	pubkey []byte, coinbaseAddr common.Address, isBlock bool, diff *big.Int) (*types.SnailBlock, error) {
	return makeSnailBlockFruitInternal(chain, fastchain, makeBlockNum, 0, makeFruitSize, pubkey, coinbaseAddr, isBlock, diff)
}

//create block,fruit
// chain: for snail chain
// fastchian: for fast chain
// makeStartFastNum,makeFruitSize :if you create  a block the fruitset  startnumber and size this is fastblock number
//pubkey : for election
// coinbaseAddr: for coin
func makeSnailBlockFruitInternal(chain *SnailBlockChain, fastchain *core.BlockChain, makeBlockNum int, makeStartFastNum int, makeFruitSize int,
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

	if isBlock {
		makeStartFastNum = int(new(big.Int).Add(snailFruitsLastFastNumber, big.NewInt(1)).Int64())
	}
	if isBlock {
		if makeFruitSize < params.MinimumFruits || snailFruitsLastFastNumber.Int64() >= int64(makeStartFastNum) {
			return nil, fmt.Errorf("fruitSet is nill or size less then 60, %d, %d", snailFruitsLastFastNumber, makeStartFastNum)
		}
	}

	makeHead := func(chain *SnailBlockChain, pubkey []byte, coinbaseAddr common.Address, fastNumber *big.Int, isFruit bool) *types.SnailHeader {
		//num := parent.Number()
		var fruitDiff *big.Int
		if isFruit {
			fruitDiff = diff
		}
		var tstamp *big.Int
		if parent.Time() == nil {
			tstamp = big.NewInt(10)
		} else {
			tstamp = new(big.Int).Add(parent.Time(), big.NewInt(10)) // block time is fixed at 10 seconds
		}
		header := &types.SnailHeader{
			ParentHash:      parent.Hash(),
			Publickey:       pubkey,
			Number:          new(big.Int).SetUint64(uint64(makeBlockNum)),
			Time:            tstamp,
			Coinbase:        coinbaseAddr,
			FastNumber:      fastNumber,
			FruitDifficulty: fruitDiff,
			FastHash:        fastchain.GetBlockByNumber(fastNumber.Uint64()).Hash(),
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

	}
	fruit, err := makeFruit(chain, fastchain, new(big.Int).SetInt64(int64(makeStartFastNum)), pubkey, coinbaseAddr)
	if err != nil {
		return nil, err
	}
	return fruit, nil
}

//MakeSnailBlockFruits return fruits or blocks by given params and insert these in the chain
func MakeSnailBlockFruits(chain *SnailBlockChain, fastchain *core.BlockChain, makeStarblockNumber int, makeblockSize int,
	makeStartFastNum int, makeFruitSize int, pubkey []byte, coinbaseAddr common.Address, isBlock bool, diff *big.Int) ([]*types.SnailBlock, error) {
	var blocks types.SnailBlocks

	for i := makeStarblockNumber; i < makeblockSize+makeStarblockNumber; i++ {
		var blocks2 types.SnailBlocks
		block, err := MakeSnailBlockFruit(chain, fastchain, i, params.MinimumFruits, pubkey, coinbaseAddr, true, diff)
		if err != nil {
			return nil, err
		}

		blocks2 = append(blocks2, block)
		blocks = append(blocks, block)
		if _, error := chain.InsertChain(blocks2); error != nil {
			panic(error)
		}
	}

	return blocks, nil
}

//MakeSnailBlockFruitsWithoutInsert return fruits or blocks by given params
func MakeSnailBlockFruitsWithoutInsert(chain *SnailBlockChain, fastchain *core.BlockChain, makeStarblockNumber int, makeblockSize int,
	makeStartFastNum int, makeFruitSize int, pubkey []byte, coinbaseAddr common.Address, isBlock bool, diff *big.Int) ([]*types.SnailBlock, error) {
	var blocks types.SnailBlocks

	//parent := chain.genesisBlock
	for i := makeStarblockNumber; i < makeblockSize+makeStarblockNumber; i++ {
		var blocks2 types.SnailBlocks
		block, err := MakeSnailBlockFruit(chain, fastchain, i, params.MinimumFruits, pubkey, coinbaseAddr, true, diff)
		if err != nil {
			return nil, err
		}

		blocks2 = append(blocks2, block)
		blocks = append(blocks, block)
	}

	return blocks, nil
}

//MakeSnailChain return snailChain and fastchain by given snailBlockNumbers and a default fastBlockNumbers(60)
func MakeSnailChain(snailBlockNumbers int, genesis *core.Genesis, engine consensus.Engine) (*SnailBlockChain, *core.BlockChain) {
	return MakeChain(snailBlockNumbers*params.MinimumFruits, snailBlockNumbers, genesis, engine)
}
