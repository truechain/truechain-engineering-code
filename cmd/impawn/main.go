package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	truechain "github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrueclient"
)

var (
	abiStaking, _ = abi.JSON(strings.NewReader(vm.StakeABIJSON))
	priKey, _     = crypto.HexToECDSA("0260c952edc49037129d8cabbe4603d15185d83aa718291279937fb6db0fa7a2")
	account       = common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69")
	skey1, _      = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	saddr1        = crypto.PubkeyToAddress(skey1.PublicKey)
)

const (
	datadirPrivateKey      = "key"
	datadirDefaultKeyStore = "keystore"
	PrivateKeyFileName     = "bftkey"
)

func main() {
	var action string
	var method string
	if len(os.Args) > 1 {
		action = os.Args[1]
	}
	if len(os.Args) > 2 {
		method = os.Args[2]
	}
	fmt.Println("action ", action, " method ", method, "os.Args", len(os.Args), " ", os.Args[0])
	pub := crypto.FromECDSAPub(&priKey.PublicKey)

	// Create an IPC based RPC connection to a remote node
	conn, err := etrueclient.Dial("http://39.100.120.25:8545")
	//conn, err := etrueclient.Dial("http://127.0.0.1:8545")
	//conn, err := etrueclient.Dial("/root/data/node3/getrue.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := conn.NetworkID(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	header, err := conn.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("chainID ", chainID.Uint64(), " Number ", header.Number.String())

	balance, err := conn.BalanceAt(context.Background(), account, nil)
	if err != nil {
		log.Fatal(err)
	}
	fbalance := new(big.Float)
	fbalance.SetString(balance.String())
	impawnValue := new(big.Float).Quo(fbalance, big.NewFloat(math.Pow10(18)))

	sbalance, err := conn.BalanceAt(context.Background(), types.StakingAddress, nil)
	generalABalance, err := conn.BalanceAt(context.Background(), saddr1, nil)
	fmt.Println(" Value ", impawnValue, " stake ", types.ToTrue(sbalance), " general ", types.ToTrue(generalABalance))

	if strings.Contains(action, "contractT") {

		input, err := abiStaking.Pack("deposit", pub)
		if err != nil {
			fmt.Println("err ", err)
		}
		txHash := sendContractTransaction(conn, saddr1, types.StakingAddress, skey1, input, header.Number)

		time.Sleep(5 * time.Millisecond)

		tx, isPending, err := conn.TransactionByHash(context.Background(), txHash)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(tx.Hash().String(), " isPending ", isPending)

		receipt, err := conn.TransactionReceipt(context.Background(), txHash)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("number", receipt.BlockNumber.Uint64(), "Status ", receipt.Status, " Logs ", len(receipt.Logs)) // 1

		block, err := conn.BlockByHash(context.Background(), receipt.BlockHash)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("number ", block.Number().Uint64(), " count ", block.Transactions())
	} else if strings.Contains(action, "create") {
		createKs()
	} else if strings.Contains(action, "import") {
		importKs()
	}
}

func sendContractTransaction(client *etrueclient.Client, from, toAddress common.Address, privateKey *ecdsa.PrivateKey, input []byte, height *big.Int) common.Hash {
	// Ensure a valid value field and resolve the account nonce
	//nonce, err := client.PendingNonceAt(context.Background(), from)
	nonce, err := client.GetNonceAtBlockNumber(context.Background(), from, height)
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
	fmt.Println("nonce ", nonce, " value ", types.ToTrue(value), " gasLimit ", gasLimit, " gasPrice ", gasPrice)

	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, input)
	//tx := types.NewTransaction(nonce, toAddress, value, 47200, big.NewInt(1000000), input)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Fatal(err)
	}

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

func importKs() {
	abs, _ := filepath.Abs("bftkey")
	key, err := crypto.LoadECDSA("bftkey")
	saddr1 = crypto.PubkeyToAddress(key.PublicKey)
	file, err := GetAllFile("./createKs")
	if err != nil {
		log.Fatal(err)
	}
	cks, _ := filepath.Abs("createKs")
	fmt.Println("abs ", abs, " ", filepath.IsAbs(abs), " ", filepath.Join(abs, "path"), " ./createKs ", filepath.Join("createKs", file), " saddr1 ", saddr1.String())
	fmt.Println(" ./createKs ", filepath.Join(cks, file))

	ks := keystore.NewKeyStore("createKs", keystore.StandardScryptN, keystore.StandardScryptP)
	jsonBytes, err := ioutil.ReadFile(filepath.Join(cks, file))
	if err != nil {
		log.Fatal(err)
	}

	password := "secret"
	account, err := ks.Import(jsonBytes, password, password)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(account.Address.Hex()) // 0x20F8D42FB0F667F2E53930fed426f225752453b3
}

func LoadPrivateKey(fileName string) common.Address {
	cks, _ := filepath.Abs(datadirPrivateKey)
	key, err := crypto.LoadECDSA(filepath.Join(cks, fileName))
	if err != nil {
		printError("file name error", err)
	}
	return crypto.PubkeyToAddress(key.PublicKey)
}

func GetAllFile(pathname string) (string, error) {
	rd, err := ioutil.ReadDir(pathname)
	for _, fi := range rd {
		if fi.IsDir() {
			fmt.Printf("[%s]\n", pathname+"\\"+fi.Name())
			GetAllFile(pathname + fi.Name() + "\\")
			return "", errors.New("path error")
		} else {
			fmt.Println(fi.Name())
			return fi.Name(), nil
		}
	}
	return "", err
}

func printError(error ...interface{}) {
	log.Fatal(error)
}
