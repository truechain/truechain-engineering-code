package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/accounts/keystore"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/console"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
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
	"strings"
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
	abiStaking, _ = abi.JSON(strings.NewReader(vm.StakeABIJSON))
	priKey        *ecdsa.PrivateKey
	from          common.Address
	trueValue     uint64
	holder        common.Address
)

var (
	first         = false
	firstNumber   = uint64(0)
	epoch         = uint64(0)
	delegateNum   = 10
	delegateToal  = 0
	delegateKey   []*ecdsa.PrivateKey
	delegateAddr  []common.Address
	delegateTx    map[common.Address]common.Hash
	delegateSu    map[common.Address]bool
	delegateFail  map[common.Address]bool
	deleValue     = new(big.Int).SetInt64(0)
	sendValue     = new(big.Int).SetInt64(0)
	deleEValue    = new(big.Int).SetInt64(0)
	chainID       *big.Int
	nonce         uint64
	nonceEpoch    = uint64(0)
	loopCount     = uint64(0)
	count         = uint64(0)
	blockMutex    = new(sync.Mutex)
	loopMutex     = new(sync.Mutex)
	interval      = uint64(1)
	keyAccounts   KeyAccount
	startDelegate bool
	startCancel   bool
	load          bool
)

const (
	datadirPrivateKey      = "key"
	datadirDefaultKeyStore = "keystore"
	defaultKeyAccount      = "accounts"
)

func impawn(ctx *cli.Context) error {

	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	chainID, _ = conn.ChainID(context.Background())
	header := printBaseInfo(conn, url)

	PrintBalance(conn, from)

	delegateNum = int(ctx.GlobalUint64(DelegateFlag.Name))
	delegateToal = int(ctx.GlobalUint64(CountFlag.Name))
	interval = ctx.GlobalUint64(IntervalFlag.Name)

	if !first {
		firstNumber = header.Number.Uint64()
		epoch = types.GetEpochFromHeight(firstNumber).EpochID
		delegateTx = make(map[common.Address]common.Hash)
		delegateSu = make(map[common.Address]bool)
		delegateFail = make(map[common.Address]bool)
		redistributionDelegate(conn)
		fmt.Println("cancel height ", types.GetEpochFromID(epoch+1).BeginHeight, " withdraw height ", types.MinCalcRedeemHeight(epoch+1), "first", firstNumber)
		first = true
		startDelegate = true
		startCancel = false
		load = false
	}
	var err error
	for {
		loop(conn, header, ctx)
		header, err = conn.HeaderByNumber(context.Background(), nil)
		querySendTx(conn)
		time.Sleep(time.Second * time.Duration(interval))
		if err != nil {
			log.Fatal(err)
		}
	}

	return nil
}

func loop(conn *etrueclient.Client, header *types.Header, ctx *cli.Context) {
	loopMutex.Lock()
	defer loopMutex.Unlock()
	number := header.Number.Uint64()
	diff := loopCount
	loopCount = loopCount + 1
	if nonceEpoch < 1 {
		nonce, _ = conn.PendingNonceAt(context.Background(), from)
	}
	nonceEpoch = 0
	fmt.Println("number", number, "diff", diff, "send", "nonce", nonce, "tx", len(delegateTx), "su", len(delegateSu), "fail", len(delegateFail))
	value := trueToWei(ctx, false)
	if diff == 1 {
		deposit(ctx, conn, value)
	}
	if number == types.GetEpochFromID(epoch+1).BeginHeight {
		cancel(conn, value)
	}
	if number == types.MinCalcRedeemHeight(epoch+1) {
		withdrawImpawn(conn, value)
	}

	if startDelegate {
		startDelegateTx(conn, diff, number)
	}

	if startCancel {
		if load {
			loadKeyAccounts()
		}

		for i := 0; i < len(delegateAddr); i++ {
			addr := delegateAddr[i]
			key := delegateKey[i]
			if diff == uint64(i) {
				value := queryDelegateInfo(conn, addr)
				if value > 0 {
					cancelDImpawn(conn, new(big.Int).SetUint64(value), addr, key)
				}
			}
			//if number == types.MinCalcRedeemHeight(epoch+1) {
			//	withdrawDImpawn(conn, deleEValue, addr, key)
			//}
			if loopCount == uint64(len(delegateAddr)) {
				fmt.Println("Tx send Over.")
				os.Exit(0)
			}
		}
	}

	if len(delegateSu) == delegateNum {
		if delegateToal > 0 || count > 1000 {
			if count >= uint64(delegateToal) {
				loopCount = 0
				for k, _ := range delegateSu {
					delete(delegateSu, k)
				}
				load = true
				startDelegate = false
				startCancel = true
				return
			}
		}
		fmt.Println("count", count, " create account", " delegateToal ", delegateToal)
		for k, _ := range delegateSu {
			delete(delegateSu, k)
		}
		redistributionDelegate(conn)
	}
}

func loadKeyAccounts() {
	keyAccounts = loadNodesJSON(defaultKeyAccount)
	fmt.Println("LoadKeyAccounts from local ", len(keyAccounts))
	delegateKey = append(delegateKey[:0], delegateKey[len(delegateKey):]...)
	delegateAddr = append(delegateAddr[:0], delegateAddr[len(delegateAddr):]...)
	for addr, key := range keyAccounts {
		delegatePV, _ := crypto.HexToECDSA(key)
		delegateKey = append(delegateKey, delegatePV)
		delegateAddr = append(delegateAddr, addr)
	}
	load = false
}

func startDelegateTx(conn *etrueclient.Client, diff, number uint64) {
	for i := 0; i < delegateNum; i++ {
		addr := delegateAddr[i]
		key := delegateKey[i]

		if v, ok := delegateFail[addr]; ok {
			if v {
				txhash, err := sendContractTransaction(conn, from, addr, sendValue, priKey, nil, "sendtx")
				if err != nil {
					delegateFail[addr] = true
				} else {
					delegateTx[addr] = txhash
					delete(delegateFail, addr)
				}
			}
		}

		if diff == uint64(i) {
			txhash, err := sendContractTransaction(conn, from, addr, sendValue, priKey, nil, "sendtx")
			if err != nil {
				delegateFail[addr] = true
			} else {
				delegateTx[addr] = txhash
				delete(delegateFail, addr)
			}
		}

		if v, ok := delegateSu[addr]; ok {
			if v {
				delegateImpawn(conn, deleEValue, addr, key)
				delegateSu[addr] = false
			}
		}
	}
}

func redistributionDelegate(conn *etrueclient.Client) {
	loopCount = 0
	count = count + uint64(delegateNum)
	if !first {
		count = count + uint64(len(loadNodesJSON(defaultKeyAccount)))
	}

	kas := make(KeyAccount, delegateNum)
	delegateKey = make([]*ecdsa.PrivateKey, delegateNum)
	delegateAddr = make([]common.Address, delegateNum)
	for i := 0; i < delegateNum; i++ {
		delegateKey[i], _ = crypto.GenerateKey()
		delegateAddr[i] = crypto.PubkeyToAddress(delegateKey[i].PublicKey)
		kas[delegateAddr[i]] = hex.EncodeToString(crypto.FromECDSA(delegateKey[i]))
	}
	writeNodesJSON(defaultKeyAccount, kas)

	deleValue = getBalance(conn, from)
	if delegateNum > 0 {
		sendValue = new(big.Int).Div(deleValue, new(big.Int).SetInt64(int64(delegateNum*10000)))
		deleEValue = new(big.Int).Sub(sendValue, new(big.Int).Div(sendValue, new(big.Int).SetInt64(int64(10))))
	}
}

func deposit(ctx *cli.Context, conn *etrueclient.Client, value *big.Int) {
	fee := ctx.GlobalUint64(FeeFlag.Name)
	pubkey, pk, _ := getPubKey(ctx, conn)

	fmt.Println("Fee", fee, " Pubkey ", pubkey, " value ", value)
	input := packInput("deposit", pk, new(big.Int).SetUint64(fee))
	txHash, _ := sendContractTransaction(conn, from, types.StakingAddress, value, priKey, input, "deposit")
	getResult(conn, txHash, true, false)
}

func getBalance(conn *etrueclient.Client, address common.Address) *big.Int {
	balance, err := conn.BalanceAt(context.Background(), address, nil)
	if err != nil {
		log.Fatal(err)
	}
	return balance
}

func getPubKey(ctx *cli.Context, conn *etrueclient.Client) (string, []byte, error) {
	var (
		pubkey string
		err    error
	)

	pubkey, err = conn.Pubkey(context.Background())
	if err != nil {
		printError("get pubkey error", err)
	}

	pk := common.Hex2Bytes(pubkey)
	if err = types.ValidPk(pk); err != nil {
		printError("ValidPk error", err)
	}
	return pubkey, pk, err
}

func sendOtherContractTransaction(client *etrueclient.Client, f, toAddress common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, input []byte, method string) common.Hash {
	blockMutex.Lock()
	defer blockMutex.Unlock()
	nonceOther, _ := client.PendingNonceAt(context.Background(), f)
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	gasLimit := uint64(2100000) // in units
	// If the contract surely has code (or code is not needed), estimate the transaction
	msg := truechain.CallMsg{From: from, To: &toAddress, GasPrice: gasPrice, Value: value, Data: input}
	gasLimit, err = client.EstimateGas(context.Background(), msg)
	if err != nil {
		fmt.Println("Contract exec failed", err)
	}
	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonceOther, toAddress, value, gasLimit, gasPrice, input)

	signedTx, err := types.SignTx(tx, types.NewTIP1Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	fmt.Println("TX other data nonce ", nonceOther, " method ", method, " transfer value ", value, " gasPrice ", gasPrice, " gasLimit ", gasLimit, "err", err, " from ", f.Hex())
	return signedTx.Hash()
}

func sendContractTransaction(client *etrueclient.Client, from, toAddress common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, input []byte, method string) (common.Hash, error) {
	blockMutex.Lock()
	defer blockMutex.Unlock()
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	gasLimit := uint64(2100000) // in units
	// If the contract surely has code (or code is not needed), estimate the transaction
	msg := truechain.CallMsg{From: from, To: &toAddress, GasPrice: gasPrice, Value: value, Data: input}
	gasLimit, err = client.EstimateGas(context.Background(), msg)
	if err != nil {
		fmt.Println("Contract exec failed", err)
	}
	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, input)

	signedTx, err := types.SignTx(tx, types.NewTIP1Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	fmt.Println("TX data nonce ", nonce, " method ", method, " transfer value ", value, " gasPrice ", gasPrice, " gasLimit ", gasLimit, "err", err, " to ", toAddress.Hex(), "from", from.Hex())
	nonce = nonce + 1
	nonceEpoch = nonceEpoch + 1
	return signedTx.Hash(), err
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

func trueToWei(ctx *cli.Context, zero bool) *big.Int {
	trueValue = ctx.GlobalUint64(TrueValueFlag.Name)
	if !zero && trueValue <= 0 {
		printError("Value must bigger than 0")
	}
	baseUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	value := new(big.Int).Mul(big.NewInt(int64(trueValue)), baseUnit)
	return value
}

func weiToTrue(value *big.Int) uint64 {
	baseUnit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	valueT := new(big.Int).Div(value, baseUnit).Uint64()
	return valueT
}

func getResult(conn *etrueclient.Client, txHash common.Hash, contract bool, delegate bool) {
	fmt.Println("Please waiting ", " txHash ", txHash.String())
	//count := 0
	//for {
	//	time.Sleep(time.Millisecond * 200)
	//	_, isPending, err := conn.TransactionByHash(context.Background(), txHash)
	//	if err != nil {
	//		log.Fatal(err)
	//	}
	//	count++
	//	if !isPending {
	//		break
	//	}
	//	if count >= 40 {
	//		fmt.Println("Please use querytx sub command query later.")
	//		os.Exit(0)
	//	}
	//}
	//
	//queryTx(conn, txHash, contract, false, delegate)
}

func querySendTx(conn *etrueclient.Client) {
	for addr, v := range delegateTx {
		_, isPending, err := conn.TransactionByHash(context.Background(), v)
		if err != nil {
			delete(delegateTx, addr)
			continue
		}
		if !isPending {
			if queryTx(conn, v, false, false, false) {
				delegateSu[addr] = true
				delete(delegateTx, addr)
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
		if contract && common.IsHexAddress(from.Hex()) {
			queryStakingInfo(conn, false, delegate)
		}
		return true
	} else if receipt.Status == types.ReceiptStatusFailed {
		fmt.Println("Transaction Failed ", " Block Number", receipt.BlockNumber.Uint64())
	}
	return false
}

func packInput(abiMethod string, params ...interface{}) []byte {
	input, err := abiStaking.Pack(abiMethod, params...)
	if err != nil {
		printError(abiMethod, " error ", err)
	}
	return input
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
	//fmt.Println("address ", from.Hex(), "key", hex.EncodeToString(crypto.FromECDSA(priKey)))
	return from
}

func queryStakingInfo(conn *etrueclient.Client, query bool, delegate bool) {
	header, err := conn.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	var input []byte
	if delegate {
		input = packInput("getDelegate", from, holder)
	} else {
		input = packInput("getDeposit", from)
	}
	msg := truechain.CallMsg{From: from, To: &types.StakingAddress, Data: input}
	output, err := conn.CallContract(context.Background(), msg, header.Number)
	if err != nil {
		printError("method CallContract error", err)
	}
	if len(output) != 0 {
		args := struct {
			Staked   *big.Int
			Locked   *big.Int
			Unlocked *big.Int
		}{}
		err = abiStaking.Unpack(&args, "getDeposit", output)
		if err != nil {
			printError("abi error", err)
		}
		fmt.Println("Staked ", args.Staked.String(), "wei =", weiToTrue(args.Staked), "true Locked ",
			args.Locked.String(), " wei =", weiToTrue(args.Locked), "true",
			"Unlocked ", args.Unlocked.String(), " wei =", weiToTrue(args.Unlocked), "true")
		if query && args.Locked.Sign() > 0 {
			lockAssets, err := conn.GetLockedAsset(context.Background(), from, header.Number)
			if err != nil {
				printError("GetLockedAsset error", err)
			}
			for k, v := range lockAssets {
				for m, n := range v.LockValue {
					if !n.Locked {
						fmt.Println("Your can instant withdraw", " count value ", weiToTrue(n.Amount), " true")
					} else {
						if n.EpochID > 0 || n.Amount.Sign() > 0 {
							fmt.Println("Your can withdraw after height", n.Height.Uint64(), " count value ", weiToTrue(n.Amount), " true  index", k+m, " lock ", n.Locked)
						}
					}
				}
			}
		}
	} else {
		fmt.Println("Contract query failed result len == 0")
	}
}

func queryDelegateInfo(conn *etrueclient.Client, from common.Address) uint64 {
	header, err := conn.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	input := packInput("getDelegate", from, holder)
	msg := truechain.CallMsg{From: from, To: &types.StakingAddress, Data: input}
	output, err := conn.CallContract(context.Background(), msg, header.Number)
	if err != nil {
		printError("method CallContract error", err)
	}
	if len(output) != 0 {
		args := struct {
			Delegated *big.Int
			Locked    *big.Int
			Unlocked  *big.Int
		}{}
		err = abiStaking.Unpack(&args, "getDelegate", output)
		if err != nil {
			printError("abi error", err)
		}
		fmt.Println("Staked ", args.Delegated.String(), "wei =", weiToTrue(args.Delegated), "true Locked ",
			args.Locked.String(), " wei =", weiToTrue(args.Locked), "true",
			"Unlocked ", args.Unlocked.String(), " wei =", weiToTrue(args.Unlocked), "true")
		return args.Delegated.Uint64()
	} else {
		fmt.Println("Contract query failed result len == 0")
	}
	return 0
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
