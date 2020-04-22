package main

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"os"
	"testing"
	"time"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

func DefaulGenesisBlock() *core.Genesis {
	i, _ := new(big.Int).SetString("900000000000000000000000", 10)
	key1 := hexutil.MustDecode("0x04d341c94a16b02cee86a627d0f6bc6e814741af4cab5065637aa013c9a7d9f26051bb6546030cd67e440d6df741cb65debaaba6c0835579f88a282193795ed369")
	key2 := hexutil.MustDecode("0x0496e0f18d4bf38e0b0de161edd2aa168adaf6842706e5ebf31e1d46cb79fe7b720c750a9e7a3e1a528482b0da723b5dfae739379e555a2893e8693747559f83cd")
	key3 := hexutil.MustDecode("0x0418196ee090081bdec01e8840941b9f6a141a713dd3461b78825edf0d8a7f8cdf3f612832dc9d94249c10c72629ea59fbe0bdd09bea872ddab2799748964c93a8")
	key4 := hexutil.MustDecode("0x04c4935993a3ce206318ab884871fbe2d4dce32a022795c674784f58e7faf3239631b6952b82471fe1e93ef999108a18d028e5d456cd88bb367d610c5e57c7e443")

	return &core.Genesis{
		Config:     params.DevnetChainConfig,
		Nonce:      928,
		ExtraData:  nil,
		GasLimit:   88080384,
		Difficulty: big.NewInt(20000),
		Alloc: map[common.Address]types.GenesisAccount{
			common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69"): {Balance: i},
			common.HexToAddress("0x6d348e0188Cc2596aaa4046a1D50bB3BA50E8524"): {Balance: i},
			common.HexToAddress("0xE803895897C3cCd35315b2E41c95F817543811A5"): {Balance: i},
			common.HexToAddress("0x3F739ffD8A59965E07e1B8d7CCa938125BCe8CFb"): {Balance: i},
		},
		Committee: []*types.CommitteeMember{
			{Coinbase: common.HexToAddress("0x3f9061bf173d8f096c94db95c40f3658b4c7eaad"), Publickey: key1},
			{Coinbase: common.HexToAddress("0x2cdac3658f85b5da3b70223cc3ad3b2dfe7c1930"), Publickey: key2},
			{Coinbase: common.HexToAddress("0x41acde8dd7611338c2a30e90149e682566716e9d"), Publickey: key3},
			{Coinbase: common.HexToAddress("0x0ffd116a3bf97a7112ff8779cc770b13ea3c66a5"), Publickey: key4},
		},
	}
}

var (
	engine    = minerva.NewFaker()
	db        = etruedb.NewMemDatabase()
	gspec     = DefaulGenesisBlock()
	signer    = types.NewTIP1Signer(gspec.Config.ChainID)
	priKey, _ = crypto.HexToECDSA("0260c952edc49037129d8cabbe4603d15185d83aa718291279937fb6db0fa7a2")
	mAccount  = common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69")
)

///////////////////////////////////////////////////////////////////////
func TestParallelTX(t *testing.T) {
	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)
	sendNumber := 4000
	delegateKey := make([]*ecdsa.PrivateKey, sendNumber)
	delegateAddr := make([]common.Address, sendNumber)
	for i := 0; i < sendNumber; i++ {
		delegateKey[i], _ = crypto.GenerateKey()
		delegateAddr[i] = crypto.PubkeyToAddress(delegateKey[i].PublicKey)
	}
	genesis := gspec.MustFastCommit(db)
	chain, _ := core.GenerateChain(gspec.Config, genesis, engine, db, 10, func(i int, gen *core.BlockGen) {
		switch i {
		case 0:
			// In block 1, addr1 sends addr2 some ether.
			fmt.Println("balance ", weiToTrue(gen.GetState().GetBalance(mAccount)))
			nonce := gen.TxNonce(mAccount)
			for _, v := range delegateAddr {
				tx, _ := types.SignTx(types.NewTransaction(nonce, v, trueToWei(2), params.TxGas, nil, nil), signer, priKey)
				gen.AddTx(tx)
				nonce = nonce + 1
			}
		case 1:
			// In block 2, addr1 sends some more ether to addr2.
			nonce := gen.TxNonce(mAccount)
			key, _ := crypto.GenerateKey()
			coinbase := crypto.PubkeyToAddress(key.PublicKey)
			for i := 0; i < sendNumber; i++ {
				tx, _ := types.SignTx(types.NewTransaction(nonce, coinbase, trueToWei(2), params.TxGas, nil, nil), signer, priKey)
				gen.AddTx(tx)
				nonce = nonce + 1
			}
		case 4:
			for k, v := range delegateAddr {
				key, _ := crypto.GenerateKey()
				coinbase := crypto.PubkeyToAddress(key.PublicKey)
				nonce := gen.TxNonce(v)
				for i := 0; i < 1; i++ {
					tx, _ := types.SignTx(types.NewTransaction(nonce, coinbase, new(big.Int).SetInt64(30000), params.TxGas, nil, nil), signer, delegateKey[k])
					gen.AddTx(tx)
					nonce = nonce + 1
				}
			}
		}
	})
	params.ApplytxTime = 0
	params.FinalizeTime = 0
	params.ProcessTime = 0
	params.InsertBlockTime = 0
	repeat := int64(params.RepeatCount)
	for i := 0; i < int(repeat); i++ {
		db1 := etruedb.NewMemDatabase()
		gspec.MustFastCommit(db1)

		blockchain, err := core.NewBlockChain(db1, nil, gspec.Config, engine, vm.Config{})
		if err != nil {
			fmt.Println("NewBlockChain ", err)
		}
		if _, err := blockchain.InsertChain(chain); err != nil {
			panic(err)
		}
	}
	log.Info("Process:",
		"applyTxs", common.PrettyDuration(time.Duration(int64(params.ApplytxTime)/repeat)),
		"finalize", common.PrettyDuration(time.Duration(int64(params.FinalizeTime)/repeat)),
		"Process", common.PrettyDuration(time.Duration(int64(params.ProcessTime)/repeat)),
		"insertblock", common.PrettyDuration(time.Duration(int64(params.InsertBlockTime)/repeat)))
}

func trueToWei(trueValue uint64) *big.Int {
	baseUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	value := new(big.Int).Mul(big.NewInt(int64(trueValue)), baseUnit)
	return value
}

func weiToTrue(value *big.Int) uint64 {
	baseUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	valueT := new(big.Int).Div(value, baseUnit).Uint64()
	return valueT
}
