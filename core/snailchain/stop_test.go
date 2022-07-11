package snailchain

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/log"
	"testing"
)

//func MakeChainAndBlocks(fastBlockNumbers int, snailBlockNumbers int, genesis *core.Genesis, engine consensus.Engine) (*SnailBlockChain, *core.BlockChain) {
//	var (
//		testdb = etruedb.NewMemDatabase()
//	)
//	cache := &core.CacheConfig{
//		//TrieNodeLimit: etrue.DefaultConfig.TrieCache,
//		//TrieTimeLimit: etrue.DefaultConfig.TrieTimeout,
//	}
//
//	if fastBlockNumbers < snailBlockNumbers*params.MinimumFruits {
//		return nil, nil
//	}
//	log.Info("Make fastchain", "number", snailBlockNumbers, "fast number", fastBlockNumbers)
//
//	fastGenesis := genesis.MustFastCommit(testdb)
//	fastchain, _ := core.NewBlockChain(testdb, cache, params.AllMinervaProtocolChanges, engine, vm.Config{})
//
//	fastblocks, _ := core.GenerateChain(params.TestChainConfig, fastGenesis, engine, testdb, fastBlockNumbers, func(i int, b *core.BlockGen) {
//		b.SetCoinbase(common.Address{0: byte(1), 19: byte(i)})
//	})
//
//	fastchain.InsertChain(fastblocks)
//	log.Info("Make SnailBlockChain", "number", fastchain.CurrentBlock().Number(), "fast number", len(fastblocks))
//
//	snailGenesis := genesis.MustSnailCommit(testdb)
//	snailChain, _ := NewSnailBlockChain(testdb, params.TestChainConfig, engine, fastchain)
//
//	log.Info("MakeChain MakeSnailBlockBlockChain", "number", snailChain.CurrentBlock().Number(), "fast number", snailChain.CurrentFastBlock().Number())
//
//	if snailBlockNumbers>2{
//		snailBlockNumbers = 2
//	}
//	_, err := MakeSnailBlockBlockChain(snailChain, fastchain, snailGenesis, snailBlockNumbers, 1)
//	if err != nil {
//		panic(err)
//	}
//
//	return snailChain, fastchain
//}

func TestStopMine(t *testing.T) {
	//var stoppedSnailBlockNumber int = 2
	genesis := core.DefaultGenesisBlock()
	snail_chain, fast_chain := MakeChainAndBlocks(180, 3, genesis, minerva.NewFaker())
	log.Info("TestMakeChain", "number", snail_chain.CurrentBlock().Number(), "fast number", snail_chain.CurrentFastBlock().Number())
	blocks := snail_chain.GetBlocksFromNumber(1)

	for i := uint64(0); i <= fast_chain.CurrentBlock().Number().Uint64(); i++ {
		block := fast_chain.GetBlockByNumber(i)
		if block == nil {
			break
		} else {
			fmt.Printf("fast %d => %x\n", block.Number(), block.Hash())
		}
	}

	for _, block := range blocks {
		fmt.Printf("snail %d => %x\n", block.Number(), block.Hash())
	}

	header := snail_chain.GetHeaderByNumber(1)

	if header == nil {
		fmt.Printf("header is nil\n")
	} else {
		fmt.Printf("hash: %x\n", header.Hash())
	}
}
