package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"gopkg.in/urfave/cli.v1"
	"os"
	"path/filepath"
	"sort"
)

var (
	// Git SHA1 commit hash of the release (set via linker flags)
	gitCommit = ""
	gitDate   = ""
	// The app that holds all commands and flags.
	app *cli.App

	// Flags needed by abigen
	KeyFlag = cli.StringFlag{
		Name:  "key",
		Usage: "Private key file path",
		Value: "",
	}
	KeyStoreFlag = cli.StringFlag{
		Name:  "keystore",
		Usage: "Keystore file path",
	}
	PrivKeyFlag = cli.StringFlag{
		Name:  "privkey",
		Usage: "Private key hex string",
	}
	TrueValueFlag = cli.Uint64Flag{
		Name:  "value",
		Usage: "Staking value units one true",
		Value: 0,
	}
	DelegateFlag = cli.Uint64Flag{
		Name:  "delegate",
		Usage: "Delegate num",
		Value: 0,
	}
	CountFlag = cli.Uint64Flag{
		Name:  "count",
		Usage: "Delegate count",
		Value: 0,
	}
	FeeFlag = cli.Uint64Flag{
		Name:  "fee",
		Usage: "Staking fee",
		Value: 0,
	}
	IntervalFlag = cli.Uint64Flag{
		Name:  "interval",
		Usage: "Interval transaction time",
		Value: 1,
	}
	AddressFlag = cli.StringFlag{
		Name:  "address",
		Usage: "Transfer address",
		Value: "",
	}
	TxHashFlag = cli.StringFlag{
		Name:  "txhash",
		Usage: "Input transaction hash",
		Value: "",
	}
	OverFlag = cli.BoolFlag{
		Name:  "over",
		Usage: "Cancel all delegate value",
	}
	CancelFlag = cli.BoolFlag{
		Name:  "cancel",
		Usage: "Only cancel delegate not delegate",
	}
	WithdrawFlag = cli.BoolFlag{
		Name:  "withdraw",
		Usage: "withdraw the value",
	}
	RewardVerifyFlag = cli.BoolFlag{
		Name:  "rewardverify",
		Usage: "Verify reward valid time",
	}
	PubKeyKeyFlag = cli.StringFlag{
		Name:  "pubkey",
		Usage: "Committee public key for BFT (no 0x prefix)",
		Value: "",
	}
	BFTKeyKeyFlag = cli.StringFlag{
		Name:  "bftkey",
		Usage: "Committee bft key for BFT (no 0x prefix)",
		Value: "",
	}
	SnailNumberFlag = cli.Uint64Flag{
		Name:  "snailnumber",
		Usage: "Query reward use snail number,please current snail number -14",
		Value: 0,
	}
	ImpawnFlags = []cli.Flag{
		KeyFlag,
		KeyStoreFlag,
		utils.RPCListenAddrFlag,
		utils.RPCPortFlag,
		TrueValueFlag,
	}
)

func init() {
	app = cli.NewApp()
	app.Usage = "TrueChain Impawn tool"
	app.Name = filepath.Base(os.Args[0])
	app.Version = "1.0.0"
	app.Copyright = "Copyright 2019-2020 The TrueChain Authors"
	app.Flags = []cli.Flag{
		KeyFlag,
		KeyStoreFlag,
		utils.RPCListenAddrFlag,
		utils.RPCPortFlag,
		TrueValueFlag,
		AddressFlag,
		TxHashFlag,
		DelegateFlag,
		FeeFlag,
		CountFlag,
		IntervalFlag,
		OverFlag,
		CancelFlag,
		WithdrawFlag,
		RewardVerifyFlag,
		PubKeyKeyFlag,
		BFTKeyKeyFlag,
		SnailNumberFlag,
	}
	app.Action = utils.MigrateFlags(impawn)
	app.CommandNotFound = func(ctx *cli.Context, cmd string) {
		fmt.Fprintf(os.Stderr, "No such command: %s\n", cmd)
		os.Exit(1)
	}
	// Add subcommands.
	app.Commands = []cli.Command{
		queryStakingCommand,
		sendCommand,
		queryTxCommand,
		queryLogCommand,
		queryRewardCommand,
	}
	cli.CommandHelpTemplate = utils.OriginCommandHelpTemplate
	sort.Sort(cli.CommandsByName(app.Commands))
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
