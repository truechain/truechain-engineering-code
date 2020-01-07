package main

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/accounts/abi/bind"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	"github.com/truechain/truechain-engineering-code/send_transaction/stake/contract"
	"log"
	"math/big"
)

var (
	key, _ = crypto.HexToECDSA("0260c952edc49037129d8cabbe4603d15185d83aa718291279937fb6db0fa7a2")
)

func main() {
	priKey, _ := crypto.GenerateKey()
	pub := crypto.FromECDSAPub(&priKey.PublicKey)

	transactOpts := bind.NewKeyedTransactor(key)
	transactOpts.Value = new(big.Int).SetUint64(100)

	// Create an IPC based RPC connection to a remote node
	conn, err := etrueclient.Dial("/root/data/node3/getrue.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}
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

	//go func() {
	//	for i := 0; ; i++ {
	//		if i > 0 {
	//			time.Sleep(2 * time.Second)
	//		}
	//		subscribeBlocks(client, subch)
	//	}
	//}()
}
