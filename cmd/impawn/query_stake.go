package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/core/types"
	"gopkg.in/urfave/cli.v1"
	"math/big"
)

var AppendCommand = cli.Command{
	Name:   "depositappend",
	Usage:  "Append validator staking count",
	Action: utils.MigrateFlags(AppendImpawn),
	Flags:  ImpawnFlags,
}

func AppendImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)

	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	value := trueToWei(ctx, false)

	input := packInput("append", "")
	txHash := sendContractTransaction(conn, from, types.StakingAddress, value, priKey, input)

	getResult(conn, txHash)

	return nil
}

var UpdateFeeCommand = cli.Command{
	Name:   "updatefee",
	Usage:  "Update delegate fee will take effect in next epoch",
	Action: utils.MigrateFlags(UpdateFeeImpawn),
	Flags:  ImpawnFlags,
}

func UpdateFeeImpawn(ctx *cli.Context) error {
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	fmt.Println("Fee", fee)
	input := packInput("setFee", new(big.Int).SetUint64(fee))

	txHash := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input)

	getResult(conn, txHash)
	return nil
}

var cancelCommand = cli.Command{
	Name:   "cancel",
	Usage:  "Call this staking will cancelled at the next epoch",
	Action: utils.MigrateFlags(cancelImpawn),
	Flags:  ImpawnFlags,
}

func cancelImpawn(ctx *cli.Context) error {
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	value := trueToWei(ctx, false)

	input := packInput("cancel", value)
	txHash := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input)

	getResult(conn, txHash)
	return nil
}

var withdrawCommand = cli.Command{
	Name:   "withdraw",
	Usage:  "Call this will instant receive your deposit money",
	Action: utils.MigrateFlags(withdrawImpawn),
	Flags:  ImpawnFlags,
}

func withdrawImpawn(ctx *cli.Context) error {
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	value := trueToWei(ctx, false)

	input := packInput("withdraw", value)

	txHash := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input)

	getResult(conn, txHash)
	return nil
}
