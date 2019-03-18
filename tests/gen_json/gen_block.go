package main

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/internal/trueapi"
	"github.com/truechain/truechain-engineering-code/params"
)

func main() {

	var path = "/Users/shijinyuan/code/GOPATH/src/github.com/truechain/truechain-engineering-code/cmd/getrue/data/getrue/chaindata"
	var cache = 768
	var handles = 1024

	db, _ := etruedb.NewLDBDatabase(path, cache, handles)
	blockchain, _ := core.NewBlockChain(db, nil, params.AllMinervaProtocolChanges, minerva.NewFaker(), vm.Config{})
	block := blockchain.GetBlockByNumber(1)

	byt ,_:= rlp.EncodeToBytes(block)

	fmt.Println(hexutil.Bytes(byt))
	str ,_ := trueapi.RPCMarshalBlock(block,true,true)

	mjson,_ :=json.Marshal(str)
	mString :=string(mjson)
	fmt.Println(mString)

}


