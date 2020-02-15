package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"gopkg.in/urfave/cli.v1"
	"math/big"
)

var AppendCommand = cli.Command{
	Name:   "append",
	Usage:  "Append validator deposit staking count",
	Action: utils.MigrateFlags(AppendImpawn),
	Flags:  ImpawnFlags,
}

func AppendImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)

	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	value := trueToWei(ctx, false)

	input := packInput("append")
	txHash := sendContractTransaction(conn, from, types.StakingAddress, value, priKey, input)

	getResult(conn, txHash, true)

	return nil
}

var UpdateFeeCommand = cli.Command{
	Name:   "updatefee",
	Usage:  "Update delegate fee will take effect in next epoch",
	Action: utils.MigrateFlags(UpdateFeeImpawn),
	Flags:  ImpawnFlags,
}

func UpdateFeeImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)

	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	fee = ctx.GlobalUint64(FeeFlag.Name)
	checkFee(new(big.Int).SetUint64(fee))
	fmt.Println("Fee", fee)

	input := packInput("setFee", new(big.Int).SetUint64(fee))

	txHash := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input)

	getResult(conn, txHash, true)
	return nil
}

var cancelCommand = cli.Command{
	Name:   "cancel",
	Usage:  "Call this staking will cancelled at the next epoch",
	Action: utils.MigrateFlags(cancelImpawn),
	Flags:  ImpawnFlags,
}

func cancelImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	value := trueToWei(ctx, false)

	input := packInput("cancel", value)
	txHash := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input)

	getResult(conn, txHash, true)
	return nil
}

var withdrawCommand = cli.Command{
	Name:   "withdraw",
	Usage:  "Call this will instant receive your deposit money",
	Action: utils.MigrateFlags(withdrawImpawn),
	Flags:  ImpawnFlags,
}

func withdrawImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)
	PrintBalance(conn, from)

	value := trueToWei(ctx, false)

	input := packInput("withdraw", value)

	txHash := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input)

	getResult(conn, txHash, true)
	PrintBalance(conn, from)
	return nil
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

	queryStakingInfo(conn)
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
	txHash := sendContractTransaction(conn, from, common.HexToAddress(address), value, priKey, nil)
	getResult(conn, txHash, false)
	return nil
}

var delegateCommand = cli.Command{
	Name:   "delegate",
	Usage:  "Delegate staking on a validator address",
	Action: utils.MigrateFlags(delegateImpawn),
	Flags:  append(ImpawnFlags, AddressFlag),
}

func delegateImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	PrintBalance(conn, from)

	value := trueToWei(ctx, false)

	address := ctx.GlobalString(AddressFlag.Name)
	if !common.IsHexAddress(address) {
		printError("Must input correct address")
	}

	input := packInput("delegate", common.HexToAddress(address))
	txHash := sendContractTransaction(conn, from, types.StakingAddress, value, priKey, input)

	getResult(conn, txHash, true)
	return nil
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
	queryTx(conn, common.HexToHash(txhash), false, true)
	return nil
}
