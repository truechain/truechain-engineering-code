package tbft

import (
	"fmt"
	"math/big"
	"time"
	"github.com/ethereum/go-ethereum/log"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"testing"
)

func makeBlock() *types.Block {
	header := new(types.Header)
	header.Number = common.Big1
	header.Time = big.NewInt(time.Now().Unix())
	block := types.NewBlock(header, nil, nil, nil, nil)
	return block
}

func makePartSet(block *types.Block) (*ttypes.PartSet,error) {
	return ttypes.MakePartSet(ttypes.BlockPartSizeBytes, block)
}

func TestBlock(t *testing.T) {
	block := makeBlock()
	partset,_ := makePartSet(block)
	index := uint(0)
	part := partset.GetPart(index)
	msg := &BlockPartMessage{
		Height:		1,
		Round:		0,
		Part:		part,
	}
	data := cdc.MustMarshalBinaryBare(msg)
	msg2, err := decodeMsg(data)
	if err != nil {
		log.Error("Error decoding message", "bytes", data)
		return
	}
	log.Debug("Receive", "msg", msg2)
	msg3 := msg2.(*BlockPartMessage)
	fmt.Println(msg3)
}

func TestRlp(t *testing.T) {
	header := new(types.Header)
	header.Number = common.Big1
	header.Time = big.NewInt(time.Now().Unix())
	block := types.NewBlock(header, nil, nil, nil, nil)
	bzs, err := rlp.EncodeToBytes(block)
	if err != nil {
	   fmt.Println(err.Error())
	}
 
	var btmp types.Block
 
	err = rlp.DecodeBytes(bzs, &btmp)
	if err != nil {
	   fmt.Println(err.Error())
	}
 }