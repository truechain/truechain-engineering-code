package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	"gopkg.in/urfave/cli.v1"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"path/filepath"
	"strings"
	"time"
)

var (
	password string
	ip       string
	port     int
)

var (
	abiStaking, _ = abi.JSON(strings.NewReader(vm.StakeABIJSON))
	priKey        *ecdsa.PrivateKey
	from          common.Address
	value         uint64
	fee           uint64
)

const (
	datadirPrivateKey      = "key"
	datadirDefaultKeyStore = "keystore"
	DefaultHTTPHost        = "localhost" // Default host interface for the HTTP RPC server
	DefaultHTTPPort        = 8545        // Default TCP port for the HTTP RPC server
)

var keyCommand = cli.Command{
	Name:   "key",
	Usage:  "Use private key in directory key",
	Action: utils.MigrateFlags(KeyImpawn),
	Flags:  ImpawnFlags,
}

var keystoreCommand = cli.Command{
	Name:   "keystore",
	Usage:  "Use keystore file in directory keystore",
	Action: utils.MigrateFlags(keystoreImpawn),
	Flags:  append(ImpawnFlags, utils.PasswordFileFlag),
}

func KeyImpawn(ctx *cli.Context) error {
	LoadPrivateKey()
	impawn(ctx)
	return nil
}

func keystoreImpawn(ctx *cli.Context) error {
	password = ctx.GlobalString(utils.PasswordFileFlag.Name)
	if password == "" {
		printError("password is nil ")
	}
	importKs(password)
	impawn(ctx)
	return nil
}

func impawn(ctx *cli.Context) error {
	ip = ctx.GlobalString(utils.RPCListenAddrFlag.Name)
	port = ctx.GlobalInt(utils.RPCPortFlag.Name)
	value = ctx.GlobalUint64(ValueFlag.Name)
	fee = ctx.GlobalUint64(FeeFlag.Name)

	url := fmt.Sprintf("http://%s", fmt.Sprintf("%s:%d", ip, port))
	// Create an IPC based RPC connection to a remote node
	// "http://39.100.97.129:8545"
	conn, err := etrueclient.Dial(url)
	if err != nil {
		log.Fatalf("Failed to connect to the Truechain client: %v", err)
	}

	printBaseInfo(conn)

	if priKey == nil {
		printError("load privateKey failed")
	}

	PrintBalance(conn, from)

	pubkey, err := conn.Pubkey(context.Background())

	if err != nil {
		printError("get pubkey error", err)
	}

	input, err := abiStaking.Pack("deposit", common.Hex2Bytes(pubkey), new(big.Int).SetUint64(fee))
	if err != nil {
		fmt.Println("abi input err ", err)
	}
	txHash := sendContractTransaction(conn, from, types.StakingAddress, priKey, input)

	for {
		time.Sleep(3 * time.Millisecond)
		tx, isPending, err := conn.TransactionByHash(context.Background(), txHash)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(tx.Hash().String(), " isPending ", isPending, "tx", tx.Info())
		if !isPending {
			break
		}
	}

	receipt, err := conn.TransactionReceipt(context.Background(), txHash)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("number", receipt.BlockNumber.Uint64(), "Status ", receipt.Status, " Logs ", len(receipt.Logs))

	block, err := conn.BlockByHash(context.Background(), receipt.BlockHash)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("number ", block.Number().Uint64(), " count ", len(block.Transactions()))
	return nil
}

func sendContractTransaction(client *etrueclient.Client, from, toAddress common.Address, privateKey *ecdsa.PrivateKey, input []byte) common.Hash {
	// Ensure a valid value field and resolve the account nonce
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		log.Fatal(err)
	}

	value := big.NewInt(1000000000000000000) // in wei (1 eth)
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	gasLimit := uint64(2100000) // in units
	// If the contract surely has code (or code is not needed), estimate the transaction
	msg := truechain.CallMsg{From: from, To: &toAddress, GasPrice: gasPrice, Value: value, Data: input}
	gasLimit, err = client.EstimateGas(context.Background(), msg)
	if err != nil {
		fmt.Println("err ", err)
	}

	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonce, toAddress, value, 1100000, gasPrice, input)

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("nonce ", nonce, " value ", types.ToTrue(value), " gasLimit ", gasLimit, " gasPrice ", gasPrice, " chainID ", chainID)

	signedTx, err := types.SignTx(tx, types.NewTIP1Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("tx sent: %s", signedTx.Hash().Hex())
	return signedTx.Hash()
}

func createKs() {
	ks := keystore.NewKeyStore("./createKs", keystore.StandardScryptN, keystore.StandardScryptP)
	password := "secret"
	account, err := ks.NewAccount(password)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(account.Address.Hex()) // 0x20F8D42FB0F667F2E53930fed426f225752453b3
}

func importKs(password string) common.Address {
	file, err := getAllFile(datadirDefaultKeyStore)
	if err != nil {
		log.Fatal(err)
	}
	cks, _ := filepath.Abs(datadirDefaultKeyStore)

	jsonBytes, err := ioutil.ReadFile(filepath.Join(cks, file))
	if err != nil {
		log.Fatal(err)
	}

	//password := "secret"
	key, err := keystore.DecryptKey(jsonBytes, password)
	if err != nil {
		log.Fatal(err)
	}
	priKey = key.PrivateKey
	from = crypto.PubkeyToAddress(priKey.PublicKey)

	fmt.Println("address ", from.Hex())
	return from
}

func LoadPrivateKey() common.Address {
	file, err := getAllFile(datadirPrivateKey)
	if err != nil {
		printError(" getAllFile file name error", err)
	}
	kab, _ := filepath.Abs(datadirPrivateKey)
	key, err := crypto.LoadECDSA(filepath.Join(kab, file))
	if err != nil {
		printError("LoadECDSA error", err)
	}
	priKey = key
	from = crypto.PubkeyToAddress(key.PublicKey)
	fmt.Println("address ", from.Hex())
	return from
}

func getAllFile(path string) (string, error) {
	rd, err := ioutil.ReadDir(path)
	if err != nil {
		printError("path ", err)
	}
	for _, fi := range rd {
		if fi.IsDir() {
			fmt.Printf("[%s]\n", path+"\\"+fi.Name())
			getAllFile(path + fi.Name() + "\\")
			return "", errors.New("path error")
		} else {
			fmt.Println(path, "dir has ", fi.Name(), "file")
			return fi.Name(), nil
		}
	}
	return "", err
}

func printError(error ...interface{}) {
	log.Fatal(error)
}

func PrintBalance(conn *etrueclient.Client, from common.Address) {
	balance, err := conn.BalanceAt(context.Background(), from, nil)
	if err != nil {
		log.Fatal(err)
	}
	fbalance := new(big.Float)
	fbalance.SetString(balance.String())
	trueValue := new(big.Float).Quo(fbalance, big.NewFloat(math.Pow10(18)))

	sbalance, err := conn.BalanceAt(context.Background(), types.StakingAddress, nil)
	fmt.Println("balance value ", trueValue, " stake ", types.ToTrue(sbalance))
}

func printBaseInfo(conn *etrueclient.Client) *types.Header {
	chainID, err := conn.NetworkID(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	header, err := conn.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("chainID ", chainID.Uint64(), " Number ", header.Number.String())
	return header
}
