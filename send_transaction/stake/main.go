package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/accounts/abi/bind"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	"github.com/truechain/truechain-engineering-code/send_transaction/stake/contract"
	"log"
	"math"
	"math/big"
	"os"
	"strings"
)

var (
	priKey, _          = crypto.HexToECDSA("0260c952edc49037129d8cabbe4603d15185d83aa718291279937fb6db0fa7a2")
	depositFnSignature = []byte("transfer(address,uint256)")
	abiStaking, _      = abi.JSON(strings.NewReader(abiJSON))
)

func main() {
	var action string
	if len(os.Args[1]) > 2 {
		action = os.Args[1]
	}
	pub := crypto.FromECDSAPub(&priKey.PublicKey)

	transactOpts := bind.NewKeyedTransactor(priKey)
	transactOpts.Value = new(big.Int).SetUint64(100)

	go printCurrentBlock()

	// Create an IPC based RPC connection to a remote node
	conn, err := etrueclient.Dial("http://39.100.97.129:8545")
	//conn, err := etrueclient.Dial("/root/data/node3/getrue.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	chainID, err := conn.NetworkID(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(chainID.Uint64()) // 5671744

	header, err := conn.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(header.Number.String()) // 5671744

	account := common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69")
	balance, err := conn.BalanceAt(context.Background(), account, nil)
	if err != nil {
		log.Fatal(err)
	}
	fbalance := new(big.Float)
	fbalance.SetString(balance.String())
	ethValue := new(big.Float).Quo(fbalance, big.NewFloat(math.Pow10(18)))

	fmt.Println(ethValue) // 25.729324269165216041

	if strings.Contains(action, "contractS") {

		callContract(conn, transactOpts, pub)

	} else if strings.Contains(action, "tx") {

		sendTransaction(conn, account, vm.StakingAddress, priKey)

	} else if strings.Contains(action, "contractT") {

		input, err := abiStaking.Pack("deposit", pub)
		fmt.Println(" err ", err)
		sendContractTransaction(conn, account, vm.StakingAddress, priKey, input)

	}
}

func sendContractTransaction(client *etrueclient.Client, from, toAddress common.Address, privateKey *ecdsa.PrivateKey, input []byte) {
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		log.Fatal(err)
	}

	value := big.NewInt(1000000000000000000) // in wei (1 eth)
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	gasLimit, err := client.EstimateGas(context.Background(), truechain.CallMsg{
		To:   &toAddress,
		Data: input,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(gasLimit)

	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, input)

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
}

func sendTransaction(client *etrueclient.Client, from, toAddress common.Address, privateKey *ecdsa.PrivateKey) {
	nonce, err := client.PendingNonceAt(context.Background(), from)
	if err != nil {
		log.Fatal(err)
	}

	value := big.NewInt(1000000000000000000) // in wei (1 eth)
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
	stake, err := contract.NewToken(vm.StakingAddress, conn)
	if err != nil {
		log.Fatalf("Failed to instantiate a Token contract: %v", err)
	}

	tx, err := stake.Deposit(transactOpts, pub)

	if err != nil {
		log.Fatalf("Failed to retrieve token name: %v", err)
	}
	fmt.Println("Tx info: ", tx.Info())
}

func printCurrentBlock() {
	client, err := etrueclient.Dial("wss://127.0.0.1/ws")
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

const abiJSON = `
[
  {
    "name": "deposit",
    "outputs": [],
    "inputs": [
      {
        "type": "bytes",
        "name": "pubkey"
      }
    ],
    "constant": false,
    "payable": true,
    "type": "function"
  },
  {
    "name": "delegate",
    "outputs": [],
    "inputs": [
      {
        "type": "address",
        "name": "holder"
      }
    ],
    "constant": false,
    "payable": true,
    "type": "function"
  },
  {
    "name": "undelegate",
    "outputs": [],
    "inputs": [
      {
        "type": "address",
        "name": "holder"
      },
      {
        "type": "uint256",
        "unit": "wei",
        "name": "value"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  },
  {
    "name": "getDeposit",
    "outputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "out"
      }
    ],
    "inputs": [
      {
        "type": "address",
        "name": "owner"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function"
  },
  {
    "name": "cancel",
    "outputs": [],
    "inputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "value"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  },
  {
    "name": "withdraw",
    "outputs": [],
    "inputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "value"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  }
]
`
