package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/etrue"
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
	RPCListenAddrFlag = cli.StringFlag{
		Name:  "rpcaddr",
		Usage: "HTTP-RPC server listening interface",
		Value: DefaultHTTPHost,
	}
	RPCPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
		Value: DefaultHTTPPort,
	}
	PasswordFileFlag = cli.StringFlag{
		Name:  "password",
		Usage: "Password file to use for non-interactive password input",
		Value: "",
	}
	ValueFlag = cli.Uint64Flag{
		Name:  "value",
		Usage: "Staking value units one true",
		Value: etrue.DefaultConfig.NetworkId,
	}
	FeeFlag = cli.Uint64Flag{
		Name:  "fee",
		Usage: "Staking fee",
		Value: etrue.DefaultConfig.NetworkId,
	}
	ImpawnFlags = []cli.Flag{
		RPCListenAddrFlag,
		RPCPortFlag,
		ValueFlag,
		FeeFlag,
	}
)

func init() {
	app = cli.NewApp()
	app.Usage = "TrueChain Impawn tool"
	app.Name = filepath.Base(os.Args[0])
	app.Version = "1.0.0"
	app.Copyright = "Copyright 2019-2020 The TrueChain Authors"
	app.Flags = []cli.Flag{
		RPCListenAddrFlag,
		RPCPortFlag,
		PasswordFileFlag,
		ValueFlag,
		FeeFlag,
	}

	app.CommandNotFound = func(ctx *cli.Context, cmd string) {
		fmt.Fprintf(os.Stderr, "No such command: %s\n", cmd)
		os.Exit(1)
	}
	// Add subcommands.
	app.Commands = []cli.Command{
		keyCommand,
		keystoreCommand,
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
