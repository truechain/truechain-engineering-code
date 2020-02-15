package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
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
	"github.com/truechain/truechain-engineering-code/params"
	"gopkg.in/urfave/cli.v1"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
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
	delegate      uint64
	holder        common.Address
)

var (
	first        = false
	firstNumber  = uint64(0)
	epoch        = uint64(0)
	send         = false
	delegateNum  = 0
	delegateKey  []*ecdsa.PrivateKey
	delegateAddr []common.Address
	seed         = new(big.Int).SetInt64(0)
	concurrence  = delegateNum / int(params.NewEpochLength)
	deleValue    = new(big.Int).SetInt64(0)
	sendValue    = new(big.Int).SetInt64(0)
	deleEValue   = new(big.Int).SetInt64(0)
	chainID      *big.Int
)

const (
	datadirPrivateKey      = "key"
	datadirDefaultKeyStore = "keystore"
)

func impawn(ctx *cli.Context) error {

	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	chainID, _ = conn.ChainID(context.Background())
	header := printBaseInfo(conn, url)

	PrintBalance(conn, from)

	delegateNum = int(ctx.GlobalUint64(DelegateFlag.Name))

	if !first {
		first = true
		firstNumber = header.Number.Uint64()
		epoch = types.GetEpochFromHeight(firstNumber).EpochID
		send = true
		delegateKey = make([]*ecdsa.PrivateKey, delegateNum)
		delegateAddr = make([]common.Address, delegateNum)
		for i := 0; i < delegateNum; i++ {
			delegateKey[i], _ = crypto.GenerateKey()
			delegateAddr[i] = crypto.PubkeyToAddress(delegateKey[i].PublicKey)
		}
		deleValue = getBalance(conn, from)
		if delegateNum > 0 {
			sendValue = new(big.Int).Div(deleValue, new(big.Int).SetInt64(int64(delegateNum*100)))
			deleEValue = new(big.Int).Sub(sendValue, new(big.Int).Div(sendValue, new(big.Int).SetInt64(int64(10))))
		}
		seed, _ = rand.Int(rand.Reader, big.NewInt(9))
		fmt.Println("seed ", seed, "cancel height ", types.GetEpochFromID(epoch+1).BeginHeight, " withdraw height ", types.MinCalcRedeemHeight(epoch+1))
	}

	loop(conn, header, ctx)
	currentNum := firstNumber

	for {
		time.Sleep(time.Millisecond * 500)
		header, err := conn.HeaderByNumber(context.Background(), nil)
		if header.Number.Uint64() > currentNum {
			loop(conn, header, ctx)
			currentNum = header.Number.Uint64()
		}
		if err != nil {
			log.Fatal(err)
		}
	}

	return nil
}

func loop(conn *etrueclient.Client, header *types.Header, ctx *cli.Context) {
	number := header.Number.Uint64()
	diff := number - firstNumber - seed.Uint64()
	cEpoch := types.GetEpochFromHeight(number).EpochID
	if cEpoch != epoch && cEpoch-epoch == 4 {
		send = true
		firstNumber = types.GetEpochFromID(cEpoch).BeginHeight
		seed, _ = rand.Int(rand.Reader, big.NewInt(8))
		epoch = cEpoch
		fmt.Println("firstNumber", firstNumber, "cancel height ", types.GetEpochFromID(epoch+1).BeginHeight, " withdraw height ", types.MinCalcRedeemHeight(epoch+1))
	}
	if send {
		value := trueToWei(ctx, false)
		if diff == 5 {
			deposit(ctx, conn, value)
		}
		if diff == types.GetEpochFromID(epoch+1).BeginHeight-firstNumber {
			cancel(conn, value)
		}
		if diff == types.MinCalcRedeemHeight(epoch+1)-firstNumber {
			withdrawImpawn(conn, value)
		}
		for i := 0; i < int(params.NewEpochLength); i++ {
			for j := 0; j < concurrence+1; j++ {
				if j*int(params.NewEpochLength)+i > len(delegateKey)-1 {
					continue
				}
				if delegateKey[j*int(params.NewEpochLength)+i] == nil {
					continue
				}
				addr := delegateAddr[j*int(params.NewEpochLength)+i]
				key := delegateKey[j*int(params.NewEpochLength)+i]

				if diff == uint64(i)+1 {
					sendContractTransaction(conn, from, addr, sendValue, priKey, nil)
				}
				if diff == uint64(i)+14 {
					delegateImpawn(conn, deleEValue, addr, key)
				}
				if diff == types.GetEpochFromID(epoch+1).BeginHeight-firstNumber+uint64(i) {
					cancelDImpawn(conn, deleEValue, addr, key)
				}
				if diff == types.MinCalcRedeemHeight(epoch+1)-firstNumber+uint64(i) {
					withdrawDImpawn(conn, deleEValue, addr, key)
				}
			}
		}
		if diff-types.MinCalcRedeemHeight(epoch+1)+firstNumber-uint64(params.NewEpochLength) == 0 {
			fmt.Println("diff", diff, " epoch ", types.MinCalcRedeemHeight(epoch+1))
			send = false
		}
	}
}

func deposit(ctx *cli.Context, conn *etrueclient.Client, value *big.Int) {
	fee := ctx.GlobalUint64(FeeFlag.Name)
	pubkey, pk, _ := getPubKey(ctx, conn)

	fmt.Println("Fee", fee, " Pubkey ", pubkey, " value ", value)
	input := packInput("deposit", pk, new(big.Int).SetUint64(delegate))
	txHash := sendContractTransaction(conn, from, types.StakingAddress, value, priKey, input)
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

func sendContractTransaction(client *etrueclient.Client, from, toAddress common.Address, value *big.Int, privateKey *ecdsa.PrivateKey, input []byte) common.Hash {
	// Ensure a valid value field and resolve the account nonce
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		log.Fatal(err)
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	gasLimit := uint64(866328) // in units

	// Create the transaction, sign it and schedule it for execution
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, input)

	fmt.Println("TX data nonce ", nonce, " transfer value ", value, " gasPrice ", gasPrice, " from ", from.Hex())

	signedTx, err := types.SignTx(tx, types.NewTIP1Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatal(err)
	}

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

func queryTx(conn *etrueclient.Client, txHash common.Hash, contract bool, pending bool, delegate bool) {

	if pending {
		_, isPending, err := conn.TransactionByHash(context.Background(), txHash)
		if err != nil {
			log.Fatal(err)
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
	} else if receipt.Status == types.ReceiptStatusFailed {
		fmt.Println("Transaction Failed ", " Block Number", receipt.BlockNumber.Uint64())
	}
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

func printTest(a ...interface{}) {
	tlog.Info("test", "SendTX", a)
}
