package main

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/internal/trueapi"
)

type Blocks []map[string]interface{}


func main() {

	var (
		//path = "./data/getrue/chaindata"
		//cache = 768
		//handles = 1024
		snailblocks = 1
		//engine = minerva.NewFaker()
		//db, _ = etruedb.NewLDBDatabase(path, cache, handles)
		fastBlocks  Blocks
	 )

	genesis := core.DefaultGenesisBlock()
	blockchain, fastChain := snailchain.MakeChain(60, 1, genesis, minerva.NewFaker())

	//fastChain, _ := core.NewBlockChain(db, nil, params.AllMinervaProtocolChanges, engine, vm.Config{})
	//blockchain, _ := snailchain.NewSnailBlockChain(db, params.TestChainConfig, engine, vm.Config{}, fastChain)


	for i := 0 ; i < snailblocks ; i++ {

		sblock_json := make(map[string]interface{})

		//Gets the slow column block
		sblock := blockchain.GetBlockByNumber(1)
		sbyt ,_:= rlp.EncodeToBytes(sblock)
		sstr ,_ := trueapi.RPCMarshalSnailBlock(sblock,true)

		sblock_json["blockHeader"] = sblock.Header()
		sblock_json["blocknumber"] = sblock.NumberU64()
		sblock_json["chainname"] = "A"
		sblock_json["rlp"] = hexutil.Bytes(sbyt)
		//sblock_json["sstr"] = sstr

		sString2,_ := json.Marshal(sblock_json)
		fmt.Println(string(sString2))

		beginFruitNumber := sstr["beginFruitNumber"].(*hexutil.Big)
		endFruitNumber := sstr["endFruitNumber"].(*hexutil.Big)

		//
		for j := beginFruitNumber.ToInt().Uint64() ; j <= endFruitNumber.ToInt().Uint64() ; j++ {

			fblock_json := make(map[string]interface{})


			//fmt.Println(i)
			block := fastChain.GetBlockByNumber(j)
			byt ,_:= rlp.EncodeToBytes(block)

			//fmt.Println(hexutil.Bytes(byt))
			//str ,_ := trueapi.RPCMarshalBlock(block,true,true)
			//str["rlp"] = hexutil.Bytes(byt)
			head := block.Header()

			fields := map[string]interface{}{
				"number":           (*hexutil.Big)(head.Number),
				"hash":             block.Hash(),
				"parentHash":       head.ParentHash,
				"CommitteeHash":    head.CommitteeHash,
				"logsBloom":        head.Bloom,
				"stateRoot":        head.Root,
				"SnailHash":        head.SnailHash,
				"SnailNumber":      (*hexutil.Big)(head.SnailNumber),
				"extraData":        hexutil.Bytes(head.Extra),
				"size":             hexutil.Uint64(block.Size()),
				"gasLimit":         hexutil.Uint64(head.GasLimit),
				"gasUsed":          hexutil.Uint64(head.GasUsed),
				"timestamp":        (*hexutil.Big)(head.Time),
				"transactionsRoot": head.TxHash,
				"receiptsRoot":     head.ReceiptHash,
			}

			fblock_json["blockHeader"] = fields
			fblock_json["blocknumber"] = block.Number().String()
			fblock_json["chainname"] = "A"
			fblock_json["rlp"] = hexutil.Bytes(byt)



			//sString2,_ := json.Marshal(fblock_json)
			//fmt.Println(string(sString2))
			fastBlocks = append(fastBlocks,fblock_json)
		}

	}

	fjson,_ :=json.Marshal(fastBlocks)
	fString :=string(fjson)
	fmt.Println(fString)

}


