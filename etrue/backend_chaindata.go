package etrue

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/node"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"time"
)

/**
var datas = []string{"chaindata1", "chaindata2", "chaindata3"}
var (
		fullNode *etrue.Truechain
		err      error
	)
	for _, path := range datas {
		fullNode, err = etrue.NewChain(ctx, cfg, path, 0)
		etrue.CutContent(path, 0)

		fullNode, err = etrue.NewChain(ctx, cfg, path, 1)
		etrue.CutContent(path, 1)
	}
*/

func NewChain(ctx *node.ServiceContext, config *Config, path string, blockType int) (*Truechain, error) {
	etrue, err := New(ctx, config)
	if err != nil {
		log.Error("newMethod", "err", err)
	}
	VerifySign(etrue)
	if blockType == 0 {
		PrintSequenceFastBlock(etrue)
	} else {
		PrintSequenceSnailBlock(etrue)
	}
	return etrue, err
}

func CutContent(path string, blockType int) {
	src, err := os.OpenFile("f:/block.txt", os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("src error", err)
	}
	var fName string
	if blockType == 0 {
		fName = "f:/" + path + "_fast.txt"
	} else {
		fName = "f:/" + path + "_snail.txt"
	}
	dst, err := os.OpenFile(fName, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("dst error", err)
	}
	CopyFile(dst.Name(), src.Name()) // os.Args[1]为目标文件，os.Args[2]为源文件

	err = ioutil.WriteFile(src.Name(), []byte{}, 0666)
	if err != nil {
		fmt.Println("write error", err)
	}
	//fmt.Println("复制完成")
}

func CopyFile(dstName, srcName string) (written int64, err error) {
	src, err := os.Open(srcName)
	if err != nil {
		return
	}
	defer src.Close()
	dst, err := os.OpenFile(dstName, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return
	}
	defer dst.Close()
	return io.Copy(dst, src)
}

func PrintSequenceFastBlock(etrue *Truechain) {
	bc := etrue.blockchain
	fb := bc.GetBlockByNumber(0)
	var (
		preBlock *types.Block
		number   uint64
	)
	for {
		preBlock = fb
		number = fb.NumberU64() + 1
		fb = bc.GetBlockByNumber(number)
		if fb == nil {
			break
		}
		fmt.Println("number", "isConsistence", bytes.Equal(preBlock.Hash().Bytes(), fb.ParentHash().Bytes()),
			fb.NumberU64(), "hash", hex.EncodeToString(fb.Hash().Bytes()),
			"parentHash", hex.EncodeToString(fb.ParentHash().Bytes()))

	}
}

func PrintSequenceSnailBlock(etrue *Truechain) {
	bc := etrue.snailblockchain
	fb := bc.GetBlockByNumber(0)
	var (
		preBlock *types.SnailBlock
		number   uint64
	)
	for {
		preBlock = fb
		number = fb.NumberU64() + 1
		fb = bc.GetBlockByNumber(number)
		if fb == nil {
			break
		}
		fmt.Println("snailNum", "isConsistence", bytes.Equal(preBlock.Hash().Bytes(), fb.ParentHash().Bytes()),
			fb.NumberU64(), "hash", hex.EncodeToString(fb.Hash().Bytes()),
			"parentHash", hex.EncodeToString(fb.ParentHash().Bytes()))
	}
}

func VerifySign(etrue *Truechain) {
	timer2 := time.NewTimer(time.Second * 120)
	go func() {
		<-timer2.C
		CheckSign(etrue)
	}()
}

func CheckSign(etrue *Truechain) {
	fb := etrue.blockchain.CurrentBlock()
	signs := fb.Signs()
	members := etrue.election.GetCurrentCommittee().Members()
	fmt.Println("fb.Number", fb.Number())
	fmt.Println("committee", etrue.election.GetCurrentCommittee())
	fmt.Println("len(members) :", len(members), "len(signs) :", len(signs))

	t := 0
	for _, sign := range signs {
		fmt.Println("")
		fmt.Println("signInfo", "Result", sign.Result)
		pubkey, _ := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
		for _, member := range members {
			isSelf := bytes.Equal(crypto.FromECDSAPub(member.Publickey), crypto.FromECDSAPub(pubkey))
			if isSelf {
				t++
				fmt.Println("pubKey: ", hex.EncodeToString(crypto.FromECDSAPub(member.Publickey)))
				fmt.Println("pubKey: ", crypto.FromECDSAPub(member.Publickey))
				fmt.Println("  verify true", strconv.Itoa(t))
			}
		}
	}
}

func CheckSelf(etrue *Truechain) {
	fb, _ := etrue.agent.FetchFastBlock(nil, false)
	fmt.Println(fb.Number())
	voteSign, err := etrue.agent.GenerateSign(fb)
	if err != nil {
		fmt.Println("generateSign error:", err)
	}
	pubkey, err := crypto.SigToPub(voteSign.HashWithNoSign().Bytes(), voteSign.Sign)
	isSelf := bytes.Equal(crypto.FromECDSAPub(pubkey), etrue.agent.committeeNode.Publickey)
	fmt.Println("isSelf", isSelf)
}
