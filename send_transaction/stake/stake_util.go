package main

import (
	"crypto/ecdsa"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	"gopkg.in/urfave/cli.v1"
	"math/big"
)

func cancel(conn *etrueclient.Client, value *big.Int) error {
	input := packInput("cancel", value)
	txHash, _ := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input, "cancel")
	getResult(conn, txHash, true, false)
	return nil
}

func withdrawImpawn(conn *etrueclient.Client, value *big.Int) (common.Hash, error) {
	input := packInput("withdraw", value)
	txHash, err := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input, "withdrawImpawn")
	getResult(conn, txHash, true, false)
	return txHash, err
}

var queryStakingCommand = cli.Command{
	Name:   "querystaking",
	Usage:  "Query staking info, can cancel info and can withdraw info",
	Action: utils.MigrateFlags(queryStakingImpawn),
	Flags:  ImpawnFlags,
}

func queryStakingImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	queryStakingInfo(conn, true, false)
	return nil
}

var sendCommand = cli.Command{
	Name:   "send",
	Usage:  "Send general transaction",
	Action: utils.MigrateFlags(sendTX),
	Flags:  append(ImpawnFlags, AddressFlag),
}

func sendTX(ctx *cli.Context) error {
	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)
	PrintBalance(conn, from)

	address := ctx.GlobalString(AddressFlag.Name)
	if !common.IsHexAddress(address) {
		printError("Must input correct address")
	}

	value := trueToWei(ctx, false)
	txHash, _ := sendContractTransaction(conn, from, common.HexToAddress(address), value, priKey, nil, "sendTX")
	getResult(conn, txHash, false, false)
	return nil
}

func delegateImpawn(conn *etrueclient.Client, value *big.Int, address common.Address, key *ecdsa.PrivateKey) error {
	input := packInput("delegate", from, value)
	txHash, err := sendOtherContractTransaction(conn, address, types.StakingAddress, nil, key, input, "delegateImpawn")
	getResult(conn, txHash, true, true)
	return err
}

func cancelDImpawn(conn *etrueclient.Client, value *big.Int, address common.Address, key *ecdsa.PrivateKey) error {
	input := packInput("undelegate", from, value)
	txHash, err := sendOtherContractTransaction(conn, address, types.StakingAddress, new(big.Int).SetInt64(0), key, input, "cancelDImpawn")
	getResult(conn, txHash, true, true)
	return err
}

func withdrawDImpawn(conn *etrueclient.Client, value *big.Int, address common.Address, key *ecdsa.PrivateKey) (common.Hash, error) {
	input := packInput("withdrawDelegate", from, value)
	txHash, err := sendOtherContractTransaction(conn, address, types.StakingAddress, new(big.Int).SetInt64(0), key, input, "withdrawDImpawn")
	getResult(conn, txHash, true, true)
	return txHash, err
}

var queryTxCommand = cli.Command{
	Name:   "querytx",
	Usage:  "Query tx hash, get transaction result",
	Action: utils.MigrateFlags(queryTxImpawn),
	Flags:  append(ImpawnFlags, TxHashFlag),
}

func queryTxImpawn(ctx *cli.Context) error {
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	txhash := ctx.GlobalString(TxHashFlag.Name)
	if txhash == "" {
		printError("Must input tx hash")
	}
	queryTx(conn, common.HexToHash(txhash), false, true, false)
	return nil
}
