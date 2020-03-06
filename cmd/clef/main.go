// Copyright 2018 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	colorable "github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/console"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/node"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
	"github.com/truechain/truechain-engineering-code/signer/core"
	"github.com/truechain/truechain-engineering-code/signer/fourbyte"
	"github.com/truechain/truechain-engineering-code/signer/storage"
	"gopkg.in/urfave/cli.v1"
)

const legalWarning = `
WARNING!

TrueKeyService is an account management tool. It may, like any software, contain bugs.

Please take care to
- backup your keystore files,
- verify that the keystore(s) can be opened with your password.

TrueKeyService is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR
PURPOSE. See the GNU General Public License for more details.
`

var (
	logLevelFlag = cli.IntFlag{
		Name:  "loglevel",
		Value: 4,
		Usage: "log level to emit to the screen",
	}
	advancedMode = cli.BoolFlag{
		Name:  "advanced",
		Usage: "If enabled, issues warnings instead of rejections for suspicious requests. Default off",
	}
	keystoreFlag = cli.StringFlag{
		Name:  "keystore",
		Value: filepath.Join(node.DefaultDataDir(), "keystore"),
		Usage: "Directory for the keystore",
	}
	configdirFlag = cli.StringFlag{
		Name:  "configdir",
		Value: DefaultConfigDir(),
		Usage: "Directory for TrueKeyService configuration",
	}
	chainIdFlag = cli.Int64Flag{
		Name:  "chainid",
		Value: params.MainnetChainConfig.ChainID.Int64(),
		Usage: "Chain id to use for signing (1=mainnet, 3=Ropsten, 4=Rinkeby, 5=Goerli)",
	}
	rpcPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
		Value: node.DefaultHTTPPort + 5,
	}
	signerSecretFlag = cli.StringFlag{
		Name:  "signersecret",
		Usage: "A file containing the (encrypted) master seed to encrypt TrueKeyService data, e.g. keystore credentials and ruleset hash",
	}
	customDBFlag = cli.StringFlag{
		Name:  "4bytedb-custom",
		Usage: "File used for writing new 4byte-identifiers submitted via API",
		Value: "./4byte-custom.json",
	}
	auditLogFlag = cli.StringFlag{
		Name:  "auditlog",
		Usage: "File used to emit audit logs. Set to \"\" to disable",
		Value: "audit.log",
	}
	ruleFlag = cli.StringFlag{
		Name:  "rules",
		Usage: "Path to the rule file to auto-authorize requests with",
	}
	stdiouiFlag = cli.BoolFlag{
		Name: "stdio-ui",
		Usage: "Use STDIN/STDOUT as a channel for an external UI. " +
			"This means that an STDIN/STDOUT is used for RPC-communication with a e.g. a graphical user " +
			"interface, and can be used when TrueKeyService is started by an external process.",
	}
	app         = cli.NewApp()
	initCommand = cli.Command{
		Action:    utils.MigrateFlags(initializeSecrets),
		Name:      "init",
		Usage:     "Initialize the signer, generate secret storage",
		ArgsUsage: "",
		Flags: []cli.Flag{
			logLevelFlag,
			configdirFlag,
		},
		Description: `
The init command generates a master seed which TrueKeyService can use to store credentials and data needed for
the rule-engine to work.`,
	}
)

func init() {
	app.Name = "TrueKeyService"
	app.Usage = "Manage TrueChain account operations"
	app.Flags = []cli.Flag{
		logLevelFlag,
		keystoreFlag,
		configdirFlag,
		chainIdFlag,
		utils.LightKDFFlag,
		utils.NoUSBFlag,
		utils.RPCListenAddrFlag,
		utils.RPCVirtualHostsFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
		utils.RPCEnabledFlag,
		rpcPortFlag,
		signerSecretFlag,
		customDBFlag,
		auditLogFlag,
		ruleFlag,
		stdiouiFlag,
		advancedMode,
	}
	app.Action = signer
	app.Commands = []cli.Command{initCommand}
	cli.CommandHelpTemplate = utils.OriginCommandHelpTemplate
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initializeSecrets(c *cli.Context) error {
	// Get past the legal message
	if err := initialize(c); err != nil {
		return err
	}
	// Ensure the master key does not yet exist, we're not willing to overwrite
	configDir := c.GlobalString(configdirFlag.Name)
	if err := os.Mkdir(configDir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	location := filepath.Join(configDir, "masterseed.json")
	if _, err := os.Stat(location); err == nil {
		return fmt.Errorf("master key %v already exists, will not overwrite", location)
	}
	// Key file does not exist yet, generate a new one and encrypt it
	masterSeed := make([]byte, 256)
	num, err := io.ReadFull(rand.Reader, masterSeed)
	if err != nil {
		return err
	}
	if num != len(masterSeed) {
		return fmt.Errorf("failed to read enough random")
	}
	n, p := keystore.StandardScryptN, keystore.StandardScryptP
	if c.GlobalBool(utils.LightKDFFlag.Name) {
		n, p = keystore.LightScryptN, keystore.LightScryptP
	}
	text := "The master seed of clef will be locked with a password.\nPlease specify a password. Do not forget this password!"
	var password string
	for {
		password = getPassPhrase(text, true)
		if err := core.ValidatePasswordFormat(password); err != nil {
			fmt.Printf("invalid password: %v\n", err)
		} else {
			fmt.Println()
			break
		}
	}
	cipherSeed, err := encryptSeed(masterSeed, []byte(password), n, p)
	if err != nil {
		return fmt.Errorf("failed to encrypt master seed: %v", err)
	}
	// Double check the master key path to ensure nothing wrote there in between
	if err = os.Mkdir(configDir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	if _, err := os.Stat(location); err == nil {
		return fmt.Errorf("master key %v already exists, will not overwrite", location)
	}
	// Write the file and print the usual warning message
	if err = ioutil.WriteFile(location, cipherSeed, 0400); err != nil {
		return err
	}
	fmt.Printf("A master seed has been generated into %s\n", location)
	fmt.Printf(`
This is required to be able to store credentials, such as:
* Passwords for keystores (used by rule engine)
* Storage for JavaScript auto-signing rules
* Hash of JavaScript rule-file

You should treat 'masterseed.json' with utmost secrecy and make a backup of it!
* The password is necessary but not enough, you need to back up the master seed too!
* The master seed does not contain your accounts, those need to be backed up separately!

`)
	return nil
}

func initialize(c *cli.Context) error {
	// Set up the logger to print everything
	logOutput := os.Stdout
	if c.GlobalBool(stdiouiFlag.Name) {
		logOutput = os.Stderr
		// If using the stdioui, we can't do the 'confirm'-flow
		fmt.Fprint(logOutput, legalWarning)
	} else {
		if !confirm(legalWarning) {
			return fmt.Errorf("aborted by user")
		}
		fmt.Println()
	}
	usecolor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	output := io.Writer(logOutput)
	if usecolor {
		output = colorable.NewColorable(logOutput)
	}
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(c.Int(logLevelFlag.Name)), log.StreamHandler(output, log.TerminalFormat(usecolor))))

	return nil
}

// ipcEndpoint resolves an IPC endpoint based on a configured value, taking into
// account the set data folders as well as the designated platform we're currently
// running on.
func ipcEndpoint(ipcPath, datadir string) string {
	// On windows we can only use plain top-level pipes
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(ipcPath, `\\.\pipe\`) {
			return ipcPath
		}
		return `\\.\pipe\` + ipcPath
	}
	// Resolve names into the data directory full paths otherwise
	if filepath.Base(ipcPath) == ipcPath {
		if datadir == "" {
			return filepath.Join(os.TempDir(), ipcPath)
		}
		return filepath.Join(datadir, ipcPath)
	}
	return ipcPath
}

func signer(c *cli.Context) error {
	// If we have some unrecognized command, bail out
	if args := c.Args(); len(args) > 0 {
		return fmt.Errorf("invalid command: %q", args[0])
	}
	if err := initialize(c); err != nil {
		return err
	}
	var (
		ui core.UIClientAPI
	)
	if c.GlobalBool(stdiouiFlag.Name) {
		log.Info("Using stdin/stdout as UI-channel")
		ui = core.NewStdIOUI()
	} else {
		log.Info("Using CLI as UI-channel")
		ui = core.NewCommandlineUI()
	}
	// 4bytedb data
	fourByteLocal := c.GlobalString(customDBFlag.Name)
	db, err := fourbyte.NewWithFile(fourByteLocal)
	if err != nil {
		utils.Fatalf(err.Error())
	}
	embeds, locals := db.Size()
	log.Info("Loaded 4byte database", "embeds", embeds, "locals", locals, "local", fourByteLocal)

	var (
		api       core.ExternalAPI
		pwStorage storage.Storage = &storage.NoStorage{}
	)

	configDir := c.GlobalString(configdirFlag.Name)
	if stretchedKey, err := readMasterKey(c, ui); err != nil {
		log.Warn("Failed to open master, rules disabled", "err", err)
	} else {
		vaultLocation := filepath.Join(configDir, common.Bytes2Hex(crypto.Keccak256([]byte("vault"), stretchedKey)[:10]))

		// Generate domain specific keys
		pwkey := crypto.Keccak256([]byte("credentials"), stretchedKey)

		// Initialize the encrypted storages
		pwStorage = storage.NewAESEncryptedStorage(filepath.Join(vaultLocation, "credentials.json"), pwkey)
	}
	var (
		chainId  = c.GlobalInt64(chainIdFlag.Name)
		ksLoc    = c.GlobalString(keystoreFlag.Name)
		lightKdf = c.GlobalBool(utils.LightKDFFlag.Name)
		advanced = c.GlobalBool(advancedMode.Name)
		nousb    = true
	)
	log.Info("Starting signer", "chainid", chainId, "keystore", ksLoc,
		"light-kdf", lightKdf, "advanced", advanced)
	am := core.StartClefAccountManager(ksLoc, nousb, lightKdf)
	apiImpl := core.NewSignerAPI(am, chainId, nousb, ui, db, advanced, pwStorage)

	// Establish the bidirectional communication, by creating a new UI backend and registering
	// it with the UI.
	clef := core.NewUIServerAPI(apiImpl)
	api = apiImpl
	// Audit logging
	if logfile := c.GlobalString(auditLogFlag.Name); logfile != "" {
		api, err = core.NewAuditLogger(logfile, api)
		if err != nil {
			utils.Fatalf(err.Error())
		}
		log.Info("Audit logs configured", "file", logfile)
	}
	// register signer API with server
	var (
		extapiURL = "n/a"
		ipcapiURL = "n/a"
	)
	rpcAPI := []rpc.API{
		{
			Namespace: "account",
			Public:    true,
			Service:   api,
			Version:   "1.0"},
		{
			Namespace: "clef",
			Public:    true,
			Service:   clef,
			Version:   "1.0"},
	}
	if c.GlobalBool(utils.RPCEnabledFlag.Name) {
		vhosts := splitAndTrim(c.GlobalString(utils.RPCVirtualHostsFlag.Name))
		cors := splitAndTrim(c.GlobalString(utils.RPCCORSDomainFlag.Name))

		// start http server
		httpEndpoint := fmt.Sprintf("%s:%d", c.GlobalString(utils.RPCListenAddrFlag.Name), c.Int(rpcPortFlag.Name))
		listener, _, err := rpc.StartHTTPEndpoint(httpEndpoint, rpcAPI, []string{"account"}, cors, vhosts)
		if err != nil {
			utils.Fatalf("Could not start RPC api: %v", err)
		}
		extapiURL = fmt.Sprintf("http://%s", httpEndpoint)
		log.Info("HTTP endpoint opened", "url", extapiURL)

		defer func() {
			listener.Close()
			log.Info("HTTP endpoint closed", "url", httpEndpoint)
		}()
	}
	if !c.GlobalBool(utils.IPCDisabledFlag.Name) {
		givenPath := c.GlobalString(utils.IPCPathFlag.Name)
		ipcapiURL = ipcEndpoint(filepath.Join(givenPath, "clef.ipc"), configDir)
		listener, _, err := rpc.StartIPCEndpoint(ipcapiURL, rpcAPI)
		if err != nil {
			utils.Fatalf("Could not start IPC api: %v", err)
		}
		log.Info("IPC endpoint opened", "url", ipcapiURL)
		defer func() {
			listener.Close()
			log.Info("IPC endpoint closed", "url", ipcapiURL)
		}()
	}

	ui.OnSignerStartup(core.StartupInfo{
		Info: map[string]interface{}{
			"intapi_version": core.InternalAPIVersion,
			"extapi_version": core.ExternalAPIVersion,
			"extapi_http":    extapiURL,
			"extapi_ipc":     ipcapiURL,
		},
	})

	abortChan := make(chan os.Signal, 1)
	signal.Notify(abortChan, os.Interrupt)

	sig := <-abortChan
	log.Info("Exiting...", "signal", sig)

	return nil
}

// splitAndTrim splits input separated by a comma
// and trims excessive white space from the substrings.
func splitAndTrim(input string) []string {
	result := strings.Split(input, ",")
	for i, r := range result {
		result[i] = strings.TrimSpace(r)
	}
	return result
}

// DefaultConfigDir is the default config directory to use for the vaults and other
// persistence requirements.
func DefaultConfigDir() string {
	// Try to place the data folder in the user's home dir
	home := homeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Signer")
		} else if runtime.GOOS == "windows" {
			appdata := os.Getenv("APPDATA")
			if appdata != "" {
				return filepath.Join(appdata, "Signer")
			} else {
				return filepath.Join(home, "AppData", "Roaming", "Signer")
			}
		} else {
			return filepath.Join(home, ".clef")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
func readMasterKey(ctx *cli.Context, ui core.UIClientAPI) ([]byte, error) {
	var (
		file      string
		configDir = ctx.GlobalString(configdirFlag.Name)
	)
	if ctx.GlobalIsSet(signerSecretFlag.Name) {
		file = ctx.GlobalString(signerSecretFlag.Name)
	} else {
		file = filepath.Join(configDir, "masterseed.json")
	}
	if err := checkFile(file); err != nil {
		return nil, err
	}
	cipherKey, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var password string
	// If ui is not nil, get the password from ui.
	if ui != nil {
		resp, err := ui.OnInputRequired(core.UserInputRequest{
			Title:      "Master Password",
			Prompt:     "Please enter the password to decrypt the master seed",
			IsPassword: true})
		if err != nil {
			return nil, err
		}
		password = resp.Text
	} else {
		password = getPassPhrase("Decrypt master seed of clef", false)
	}
	masterSeed, err := decryptSeed(cipherKey, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the master seed of clef")
	}
	if len(masterSeed) < 256 {
		return nil, fmt.Errorf("master seed of insufficient length, expected >255 bytes, got %d", len(masterSeed))
	}
	// Create vault location
	vaultLocation := filepath.Join(configDir, common.Bytes2Hex(crypto.Keccak256([]byte("vault"), masterSeed)[:10]))
	err = os.Mkdir(vaultLocation, 0700)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}
	return masterSeed, nil
}

// checkFile is a convenience function to check if a file
// * exists
// * is mode 0400
func checkFile(filename string) error {
	//info, err := os.Stat(filename)
	//if err != nil {
	//	return fmt.Errorf("failed stat on %s: %v", filename, err)
	//}
	//// Check the unix permission bits
	//if info.Mode().Perm()&0377 != 0 {
	//	return fmt.Errorf("file (%v) has insecure file permissions (%v)", filename, info.Mode().String())
	//}
	return nil
}

// confirm displays a text and asks for user confirmation
func confirm(text string) bool {
	fmt.Print(text)
	return true
}

// getPassPhrase retrieves the password associated with clef, either fetched
// from a list of preloaded passphrases, or requested interactively from the user.
// TODO: there are many `getPassPhrase` functions, it will be better to abstract them into one.
func getPassPhrase(prompt string, confirmation bool) string {
	fmt.Println(prompt)
	password, err := console.Stdin.PromptPassword("Password: ")
	if err != nil {
		utils.Fatalf("Failed to read password: %v", err)
	}
	if confirmation {
		confirm, err := console.Stdin.PromptPassword("Repeat password: ")
		if err != nil {
			utils.Fatalf("Failed to read password confirmation: %v", err)
		}
		if password != confirm {
			utils.Fatalf("Passwords do not match")
		}
	}
	return password
}

type encryptedSeedStorage struct {
	Description string              `json:"description"`
	Version     int                 `json:"version"`
	Params      keystore.CryptoJSON `json:"params"`
}

// encryptSeed uses a similar scheme as the keystore uses, but with a different wrapping,
// to encrypt the master seed
func encryptSeed(seed []byte, auth []byte, scryptN, scryptP int) ([]byte, error) {
	cryptoStruct, err := keystore.EncryptDataV3(seed, auth, scryptN, scryptP)
	if err != nil {
		return nil, err
	}
	return json.Marshal(&encryptedSeedStorage{"TrueKeyService seed", 1, cryptoStruct})
}

// decryptSeed decrypts the master seed
func decryptSeed(keyjson []byte, auth string) ([]byte, error) {
	var encSeed encryptedSeedStorage
	if err := json.Unmarshal(keyjson, &encSeed); err != nil {
		return nil, err
	}
	if encSeed.Version != 1 {
		log.Warn(fmt.Sprintf("unsupported encryption format of seed: %d, operation will likely fail", encSeed.Version))
	}
	seed, err := keystore.DecryptDataV3(encSeed.Params, auth)
	if err != nil {
		return nil, err
	}
	return seed, err
}
