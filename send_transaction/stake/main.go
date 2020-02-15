package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math"
	"math/big"
	"os"
	"strings"
	"time"

	truechain "github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/accounts/abi/bind"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	"github.com/truechain/truechain-engineering-code/send_transaction/stake/contract"
)

var (
	abiStaking, _ = abi.JSON(strings.NewReader(vm.StakeABIJSON))
	priKey, _     = crypto.HexToECDSA("0260c952edc49037129d8cabbe4603d15185d83aa718291279937fb6db0fa7a2")
	account       = common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69")
	skey1, _      = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	saddr1        = crypto.PubkeyToAddress(skey1.PublicKey)
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
	fmt.Println("action ", action, " method ", method)
	pub := crypto.FromECDSAPub(&priKey.PublicKey)

	// Create an IPC based RPC connection to a remote node
	conn, err := etrueclient.Dial("http://39.100.120.25:8545")
	//conn, err := etrueclient.Dial("http://127.0.0.1:8545")
	//conn, err := etrueclient.Dial("/root/data/node3/getrue.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	printCurrentBlock(conn)

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

	if strings.Contains(action, "contractS") {
		transactOpts := bind.NewKeyedTransactor(priKey)
		transactOpts.Value = new(big.Int).SetUint64(100)
		callContract(conn, transactOpts, pub)

	} else if strings.Contains(action, "tx") {

		sendTransaction(conn, account, saddr1, priKey)

	} else if strings.Contains(action, "contractT") {

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

func sendTransaction(client *etrueclient.Client, from, toAddress common.Address, privateKey *ecdsa.PrivateKey) {
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		log.Fatal(err)
	}

	value := big.NewInt(2000000000000000000) // in wei (1 eth)
	gasLimit := uint64(21000)                // in units
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	var data []byte
	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, data)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("nonce ", nonce, " value ", types.ToTrue(value), " gasLimit ", gasLimit, " gasPrice ", gasPrice)

	signedTx, err := types.SignTx(tx, types.NewTIP1Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("tx sent: %s", signedTx.Hash().Hex())
}

func callContract(conn *etrueclient.Client, transactOpts *bind.TransactOpts, pub []byte) {
	// Instantiate the contract and display its name
	stake, err := contract.NewToken(types.StakingAddress, conn)
	if err != nil {
		log.Fatalf("Failed to instantiate a Token contract: %v", err)
	}

	tx, err := stake.Deposit(transactOpts, pub)

	if err != nil {
		log.Fatalf("Failed to retrieve token name: %v", err)
	}
	fmt.Println("Tx info: ", tx.Info())
}

func printCurrentBlock(client *etrueclient.Client) {
	//ws://[::]:8546 wss://39.100.120.25:8545/ws
	client, err := etrueclient.Dial("ws://127.0.0.1:8546")
	if err != nil {
		log.Fatal(err)
	}
	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(context.Background(), headers)
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case err := <-sub.Err():
			log.Fatal(err)
		case header := <-headers:
			fmt.Println(header.Hash().Hex()) // 0xbc10defa8dda384c96a17640d84de5578804945d347072e091b4e5f390ddea7f

			block, err := client.BlockByHash(context.Background(), header.Hash())
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println(block.Hash().Hex())        // 0xbc10defa8dda384c96a17640d84de5578804945d347072e091b4e5f390ddea7f
			fmt.Println(block.Number().Uint64())   // 3477413
			fmt.Println(block.Time().Uint64())     // 1529525947
			fmt.Println(len(block.Transactions())) // 7
		}
	}
}
