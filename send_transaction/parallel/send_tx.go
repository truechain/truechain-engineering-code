package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/console"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	tlog "github.com/truechain/truechain-engineering-code/log"
	"gopkg.in/urfave/cli.v1"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	key   string
	store string
	ip    string
	port  int
)

var (
	priKey *ecdsa.PrivateKey
	from   common.Address
)

var (
	firstNumber  = uint64(0)
	delegateNum  = 10
	delegateToal = 0
	delegateKey  []*ecdsa.PrivateKey
	delegateAddr []common.Address
	toAddr       []common.Address
	mapAddress   map[common.Address]*ecdsa.PrivateKey
	delegateTx   map[common.Address]common.Hash
	delegateSu   map[common.Address]bool
	delegateFail map[common.Address]bool
	deleValue    = new(big.Int).SetInt64(0)
	sendValue    = new(big.Int).SetInt64(0)
	chainID      *big.Int
	nonce        uint64
	nonceEpoch   = uint64(0)
	count        = uint64(0)
	blockMutex   = new(sync.Mutex)
	loopMutex    = new(sync.Mutex)
	interval     = uint64(1)
	keyAccounts  KeyAccount
	gas          = new(big.Int).Exp(big.NewInt(10), big.NewInt(9), nil)
	minValue     = new(big.Int).Mul(big.NewInt(400), gas)
	gasPrice     *big.Int
)

const (
	datadirPrivateKey = "key"
	defaultKeyAccount = "accounts"
)

func impawn(ctx *cli.Context) error {

	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	chainID, _ = conn.ChainID(context.Background())
	header := printBaseInfo(conn, url)

	PrintBalance(conn, from)

	delegateNum = int(ctx.GlobalUint64(SonFlag.Name))
	delegateToal = int(ctx.GlobalUint64(CountFlag.Name))
	interval = ctx.GlobalUint64(IntervalFlag.Name)

	firstNumber = header.Number.Uint64()
	delegateTx = make(map[common.Address]common.Hash)
	delegateSu = make(map[common.Address]bool)
	delegateFail = make(map[common.Address]bool)
	var err error
	gasPrice, err = conn.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	initAccount(conn)

	count := 0
	blockNumber := firstNumber
	for {
		header, err = conn.HeaderByNumber(context.Background(), nil)
		if header.Number.Uint64() == blockNumber {
			count++
		} else {
			blockNumber = header.Number.Uint64()
			count = 0
		}
		if count < 5 {
			loop(conn, ctx)
			querySendTx(conn)
		}

		time.Sleep(time.Second * time.Duration(interval))
		if err != nil {
			log.Fatal(err)
		}
	}

	return nil
}

func loop(conn *etrueclient.Client, ctx *cli.Context) {
	loopMutex.Lock()
	defer loopMutex.Unlock()
	if nonceEpoch > 0 {
		nonce, _ = conn.PendingNonceAt(context.Background(), from)
	}
	nonceEpoch = 0
	fmt.Println("count", count, "nonce", nonce, "tx", len(delegateTx), "su", len(delegateSu), "fail", len(delegateFail))

	startDelegateTx(conn)
	if count >= uint64(delegateToal) {
		fmt.Println(" send tx over ", "count", count, " delegateToal ", delegateToal, " keyAccounts ", len(keyAccounts))
		return
	}
}

func startDelegateTx(conn *etrueclient.Client) {

	for addr, v := range delegateFail {
		if v {
			txhash, err := sendSonTransaction(conn, from, addr, sendValue, priKey, nil, "sendtx")
			if err != nil {
				delegateFail[addr] = true
			} else {
				delegateTx[addr] = txhash
				delete(delegateFail, addr)
			}
		}
	}

	for index, addr := range delegateAddr {
		if getBalance(conn, addr).Cmp(minValue) < 1 {
			addr := delegateAddr[index]
			txhash, err := sendSonTransaction(conn, from, addr, sendValue, priKey, nil, "sendtx")
			if err != nil {
				delegateFail[addr] = true
			} else {
				delegateTx[addr] = txhash
				delete(delegateFail, addr)
			}
		}
	}

	sendTransactions(conn, minValue)
}

//send count transaction
func sendTransactions(client *etrueclient.Client, value *big.Int) {
	waitGroup := &sync.WaitGroup{}
	Time := time.Now()

	i := 0
	for k, v := range delegateSu {
		if v {
			waitGroup.Add(1)
			nonceOther, _ := client.PendingNonceAt(context.Background(), k)
			go sendOtherContractTransaction(client, toAddr[i], value, mapAddress[k], nonceOther, waitGroup)
			i++
		}
	}
	waitGroup.Wait()
	fmt.Println(" Complete ", count, " time ", Time, " count ", delegateNum)
}

func initAccount(conn *etrueclient.Client) {
	deleValue = getBalance(conn, from)
	if delegateNum > 0 {
		sendValue = new(big.Int).Div(deleValue, new(big.Int).SetInt64(int64(delegateNum*10000)))
	}

	kas := make(KeyAccount, delegateNum)
	delegateKey = make([]*ecdsa.PrivateKey, delegateNum)
	delegateAddr = make([]common.Address, delegateNum)
	mapAddress = make(map[common.Address]*ecdsa.PrivateKey, delegateNum)
	toAddr = make([]common.Address, delegateNum)
	for i := 0; i < delegateNum; i++ {
		delegateKey[i], _ = crypto.GenerateKey()
		delegateAddr[i] = crypto.PubkeyToAddress(delegateKey[i].PublicKey)
		kas[delegateAddr[i]] = hex.EncodeToString(crypto.FromECDSA(delegateKey[i]))
		mapAddress[delegateAddr[i]] = delegateKey[i]
		toKey, _ := crypto.GenerateKey()
		toAddr[i] = crypto.PubkeyToAddress(toKey.PublicKey)
	}
	writeNodesJSON(defaultKeyAccount, kas)
}

func getBalance(conn *etrueclient.Client, address common.Address) *big.Int {
	balance, err := conn.BalanceAt(context.Background(), address, nil)
	if err != nil {
		log.Fatal(err)
	}
	return balance
}

func sendOtherContractTransaction(client *etrueclient.Client, toAddress common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, nonceOther uint64, wait *sync.WaitGroup) (common.Hash, error) {
	defer wait.Done()

	gasLimit := uint64(210000) // in units

	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonceOther, toAddress, value, gasLimit, gasPrice, nil)

	signedTx, err := types.SignTx(tx, types.NewTIP1Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)

	if err != nil {
		fmt.Println("sendRawTransaction error ", err)
	} else {
		count++
	}

	return signedTx.Hash(), err
}

func sendSonTransaction(client *etrueclient.Client, from, toAddress common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, input []byte, method string) (common.Hash, error) {
	blockMutex.Lock()
	defer blockMutex.Unlock()

	gasLimit := uint64(210000) // in units
	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, input)

	signedTx, err := types.SignTx(tx, types.NewTIP1Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	nonce = nonce + 1
	nonceEpoch = nonceEpoch + 1
	return signedTx.Hash(), err
}

func loadPrivateKey(path string) common.Address {
	var err error
	if path == "" {
		file, err := getAllFile(datadirPrivateKey)
		if err != nil {
			printError(" getAllFile file name error", err)
		}
		kab, _ := filepath.Abs(datadirPrivateKey)
		path = filepath.Join(kab, file)
	}
	priKey, err = crypto.LoadECDSA(path)
	if err != nil {
		printError("LoadECDSA error", err)
	}
	from = crypto.PubkeyToAddress(priKey.PublicKey)
	return from
}

func hexToECDSA(key string) common.Address {
	var err error
	priKey, err = crypto.HexToECDSA(key)
	if err != nil {
		printError("LoadECDSA error", err)
	}
	from = crypto.PubkeyToAddress(priKey.PublicKey)
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

func querySendTx(conn *etrueclient.Client) {
	if len(delegateSu) != delegateNum {
		for addr, v := range delegateTx {
			if _, ok := delegateSu[addr]; ok {
				continue
			}
			_, isPending, err := conn.TransactionByHash(context.Background(), v)
			if err != nil {
				delete(delegateTx, addr)
				continue
			}
			if !isPending {
				if queryTx(conn, v, false, false, false) {
					delegateSu[addr] = true
				}
			}
		}
	}
}

func queryTx(conn *etrueclient.Client, txHash common.Hash, contract bool, pending bool, delegate bool) bool {
	if pending {
		_, isPending, err := conn.TransactionByHash(context.Background(), txHash)
		if err != nil {
			fmt.Println("queryTx", "txHash", txHash.Hex(), "err", err)
		}
		if isPending {
			println("In tx_pool no validator  process this, please query later")
			os.Exit(0)
		}
	}

	receipt, err := conn.TransactionReceipt(context.Background(), txHash)
	if err != nil {
		log.Fatal(err)
	}

	if receipt.Status == types.ReceiptStatusSuccessful {
		block, err := conn.BlockByHash(context.Background(), receipt.BlockHash)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("Transaction Success", " block Number", receipt.BlockNumber.Uint64(), " block txs", len(block.Transactions()), "blockhash", block.Hash().Hex())
		return true
	} else if receipt.Status == types.ReceiptStatusFailed {
		fmt.Println("Transaction Failed ", " Block Number", receipt.BlockNumber.Uint64())
	}
	return false
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
	fmt.Println("Your wallet balance is ", trueValue, "'true ", " current Total Stake ", types.ToTrue(sbalance))
}

func loadPrivate(ctx *cli.Context) {
	key = ctx.GlobalString(KeyFlag.Name)
	store = ctx.GlobalString(KeyStoreFlag.Name)
	if key != "" {
		loadPrivateKey(key)
	} else if store != "" {
		loadSigningKey(store)
	} else if ctx.GlobalIsSet(PrivKeyFlag.Name) {
		hexToECDSA(ctx.GlobalString(PrivKeyFlag.Name))
	} else {
		printError("Must specify --key or --keystore")
	}

	if priKey == nil {
		printError("load privateKey failed")
	}
}

func dialConn(ctx *cli.Context) (*etrueclient.Client, string) {
	ip = ctx.GlobalString(utils.RPCListenAddrFlag.Name)
	port = ctx.GlobalInt(utils.RPCPortFlag.Name)

	url := fmt.Sprintf("http://%s", fmt.Sprintf("%s:%d", ip, port))
	// Create an IPC based RPC connection to a remote node
	// "http://39.100.97.129:8545"
	conn, err := etrueclient.Dial(url)
	if err != nil {
		log.Fatalf("Failed to connect to the Truechain client: %v", err)
	}
	return conn, url
}

func printBaseInfo(conn *etrueclient.Client, url string) *types.Header {
	header, err := conn.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	if common.IsHexAddress(from.Hex()) {
		fmt.Println("Connect url ", url, " current number ", header.Number.String(), " address ", from.Hex())
	} else {
		fmt.Println("Connect url ", url, " current number ", header.Number.String())
	}

	return header
}

// loadSigningKey loads a private key in Ethereum keystore format.
func loadSigningKey(keyfile string) common.Address {
	keyjson, err := ioutil.ReadFile(keyfile)
	if err != nil {
		printError(fmt.Errorf("failed to read the keyfile at '%s': %v", keyfile, err))
	}
	password, _ := console.Stdin.PromptPassword("Please enter the password for '" + keyfile + "': ")
	//password := "secret"
	key, err := keystore.DecryptKey(keyjson, password)
	if err != nil {
		printError(fmt.Errorf("error decrypting key: %v", err))
	}
	priKey = key.PrivateKey
	from = crypto.PubkeyToAddress(priKey.PublicKey)
	return from
}

func printTest(a ...interface{}) {
	tlog.Info("test", "SendTX", a)
}

const jsonIndent = "    "

// nodeSet is the nodes.json file format. It holds a set of node records
// as a JSON object.
type KeyAccount map[common.Address]string

func loadNodesJSON(file string) KeyAccount {
	var nodes KeyAccount
	if isExist(file) {
		if err := common.LoadJSON(file, &nodes); err != nil {
			printError("loadNodesJSON error", err)
		}
	}
	return nodes
}

func writeNodesJSON(file string, nodes KeyAccount) {
	for k, v := range loadNodesJSON(file) {
		nodes[k] = v
	}

	nodesJSON, err := json.MarshalIndent(nodes, "", jsonIndent)
	if err != nil {
		printError("MarshalIndent error", err)
	}
	if file == "-" {
		os.Stdout.Write(nodesJSON)
		return
	}
	if err := ioutil.WriteFile(file, nodesJSON, 0644); err != nil {
		printError("writeFile error", err)
	}
}

func isExist(f string) bool {
	_, err := os.Stat(f)
	return err == nil || os.IsExist(err)
}
