package etrue

import (
	"time"
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/crypto"
)

var (
	sendInterval = 5 * time.Second // Time interval to send record

	sendAddrHex = "970e8128ab834e8eac17ab8e3812f010678cf791"
	sendPrivHex = "289c2857d4598e37fb9647507e47a309d6133539bf21a8b9cb6df88fd5232032"

	recvAddrHex = "68f2517b6c597ede0ae7c0559cdd4a84fd08c928"
)

type FastBlockSender struct {
	SnailPool *core.SnailPool
	signer    types.Signer

	sendAccout *ecdsa.PrivateKey
	recvAddr   common.Address
}

func (sender *FastBlockSender) newSendTransaction(nonce uint64, datasize int) *types.Transaction {

	tx := types.NewTransaction(nonce, sender.recvAddr, big.NewInt(1e+18), 100000, big.NewInt(1e+12), make([]byte, datasize))
	tx, _ = types.SignTx(tx, sender.signer, sender.sendAccout)
	return tx
}

func (sender *FastBlockSender) send() {
	var nonce uint64
	ticker := time.NewTicker(sendInterval)
	defer ticker.Stop()

	number := big.NewInt(0)
	var parentHash common.Hash
	var txhash common.Hash
	for {
		select {
		case <-ticker.C:
			//record
			//nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte
			//*types.Transaction
			txs := make([]*types.Transaction, 1)
			for i := range txs {
				tx:= sender.newSendTransaction(nonce, 0)
				txs[i]=tx
				txhash=tx.Hash()
				nonce++
			}
			//header
			number = number.Add(number, common.Big1)
			var header *types.Header
			header = &types.Header{
				parentHash,
				header.Hash(),
				txhash,
				header.Hash(),
				types.BytesToBloom([]byte{0}),
				header.Hash(),
				nil,
				number,
				0,
				0,
				big.NewInt(0),
				nil,
			}
			//fmt.Println(header)
			fb := types.NewBlock(header, txs, nil, nil)
			parentHash = fb.Hash()
			var fastblocks []*types.Block
			fastblocks = append(fastblocks, fb)

			sender.SnailPool.AddRemoteFastBlock(fastblocks)
		}
	}
}
func (sender *FastBlockSender) Start() {
	go sender.send()
}

//this is usead by
func NewSender(SnailPool *core.SnailPool, chainconfig *params.ChainConfig) *FastBlockSender {
	acc, _ := crypto.HexToECDSA(sendPrivHex)

	// TODO: get key and account address to send

	sendFastBlock := &FastBlockSender{
		SnailPool:  SnailPool,
		signer:     types.NewEIP155Signer(chainconfig.ChainID),
		sendAccout: acc,
		recvAddr:   common.HexToAddress(recvAddrHex),
	}

	return sendFastBlock
}
