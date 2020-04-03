package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/accounts/abi/bind"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
	"log"
	"math/big"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const CoinABI = "[{\"inputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"Sent\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"balances\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"addresspayable\",\"name\":\"receiver0\",\"type\":\"address\"},{\"internalType\":\"addresspayable\",\"name\":\"receiver1\",\"type\":\"address\"}],\"name\":\"rechargeToAccount\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"addresspayable\",\"name\":\"receiver0\",\"type\":\"address\"},{\"internalType\":\"addresspayable\",\"name\":\"receiver1\",\"type\":\"address\"}],\"name\":\"rechargeToAccount2\",\"outputs\":[],\"stateMutability\":\"payable\",\"type\":\"function\"}]"

var CoinBin = "0x608060405234801561001057600080fd5b50610383806100206000396000f3fe6080604052600436106100345760003560e01c806327e235e3146100395780633ca93d8d1461007e578063a7253b13146100ae575b600080fd5b34801561004557600080fd5b5061006c6004803603602081101561005c57600080fd5b50356001600160a01b03166100dc565b60408051918252519081900360200190f35b6100ac6004803603604081101561009457600080fd5b506001600160a01b03813581169160200135166100ee565b005b6100ac600480360360408110156100c457600080fd5b506001600160a01b038135811691602001351661023e565b60006020819052908152604090205481565b6032826001600160a01b0316311015610196576001600160a01b038216600081815260208190526040808220805434908101909155905181156108fc0292818181858888f19350505050158015610149573d6000803e3d6000fd5b50604080513381526001600160a01b0384166020820152348183015290517f3990db2d31862302a685e8086b5755072a6e2b5b780af1ee81ece35ee3cd33459181900360600190a161023a565b6032816001600160a01b031631101561023a576001600160a01b038116600081815260208190526040808220805434908101909155905181156108fc0292818181858888f193505050501580156101f1573d6000803e3d6000fd5b50604080513381526001600160a01b0383166020820152348183015290517f3990db2d31862302a685e8086b5755072a6e2b5b780af1ee81ece35ee3cd33459181900360600190a15b5050565b6032826001600160a01b0316311015610299576001600160a01b038216600081815260208190526040808220805434908101909155905181156108fc0292818181858888f19350505050158015610149573d6000803e3d6000fd5b6032816001600160a01b0316311015610341576001600160a01b038116600081815260208190526040808220805434908101909155905181156108fc0292818181858888f193505050501580156102f4573d6000803e3d6000fd5b50604080513381526001600160a01b0383166020820152348183015290517f3990db2d31862302a685e8086b5755072a6e2b5b780af1ee81ece35ee3cd33459181900360600190a161023a565b816001600160a01b0316fffea2646970667358221220f033756591c3eebd3154abeff46863ded90c108162ff58224373ec514dcd174f64736f6c63430006010033"

func main() {
	var accounts []string
	//var privKeys []*ecdsa.PrivateKey
	contractCount := 4000
	step := 1
	contract := true
	raw := true
	randomReceiver := false

	rpcClient, err := rpc.Dial("http://127.0.0.1:8888")
	if err != nil {
		log.Fatalf("Failed to connect to the client: %v", err)
	}
	client := etrueclient.NewClient(rpcClient)

	err = rpcClient.Call(&accounts, "etrue_accounts")
	if err != nil {
		log.Fatalf("etrue_accounts Error: %v", err)
	}

	_, err = unlockAccount(rpcClient, accounts[0], "admin", 9000000, "main")
	if err != nil {
		log.Fatalf("Failed to unlockAccount: %v", err)
	}

	for {
		var receivers []common.Address
		var privKeys []*ecdsa.PrivateKey

		// Generate and recharge to accounts
		for i := 0; i < contractCount; i++ {
			priKey, _ := crypto.GenerateKey()
			privKeys = append(privKeys, priKey)
			address := crypto.PubkeyToAddress(priKey.PublicKey)

			_, err = sendRawTransaction(rpcClient, accounts[0], address.Hex(), "0x10000000000000")
			if err != nil {
				log.Fatalf("Failed to sendRawTransaction: %v", err)
			}
		}

		// generate receiver accounts to receive the balance recharged by contracts
		for i := 0; i < contractCount; i++ {
			priKey, _ := crypto.GenerateKey()
			receivers = append(receivers, crypto.PubkeyToAddress(priKey.PublicKey))
		}

		time.Sleep(time.Duration(4) * time.Second)

		nonce := uint64(1)
		if contract {
			// deploy one contract for each private key
			contracts := deployContractsInBatch(rpcClient, client, privKeys)

			// recharge the receiver accounts
			for i := 0; i < 15; i++ {
				time.Sleep(time.Duration(4) * time.Second)
				rechargeToAccountsByContract(rpcClient, client, privKeys, contracts, receivers,
					nonce, step, randomReceiver)
				nonce += uint64(step)
			}
		}

		if raw {
			// send raw transactions
			for i := 0; i < 15; i++ {
				time.Sleep(time.Duration(4) * time.Second)
				rechargeToAccountsByTx(rpcClient, privKeys, receivers, nonce, 1)
				nonce++
			}
		}
	}
}

func deployContractsInBatch(rpcClient *rpc.Client, client *etrueclient.Client, privKeys []*ecdsa.PrivateKey) []common.Address {
	var contracts []common.Address
	var batch []rpc.BatchElem
	var batchList [][]rpc.BatchElem

	for i, key := range privKeys {
		if i%50 == 0 {
			batch = make([]rpc.BatchElem, 50)
			batchList = append(batchList, batch)
		}

		tx, coinAddr := newContractTransaction(key, 0, common.FromHex(CoinBin))
		data, err := rlp.EncodeToBytes(tx)
		contracts = append(contracts, coinAddr)
		if err != nil {
			log.Fatalf("Failed to encode create contract tx to bytes: %v", err)
		}
		batch[i%50] = rpc.BatchElem{
			Method: "etrue_sendTrueRawTransaction",
			Args:   []interface{}{hexutil.Encode(data)},
			Result: new(string),
		}
	}

	wg := sync.WaitGroup{}
	t0 := time.Now()
	for _, batch := range batchList {
		wg.Add(1)
		go func(batch []rpc.BatchElem) {
			defer wg.Done()

			err := rpcClient.BatchCallContext(context.TODO(), batch)
			if err != nil {
				log.Fatalf("Failed to call BatchCallContext to create contract: %v", err)
			}
		}(batch)

	}
	wg.Wait()
	log.Println("deploy", len(privKeys), "contracts elapse:", common.PrettyDuration(time.Since(t0)))

	return contracts
}

func rechargeToAccountsByContract(rpcClient *rpc.Client, client *etrueclient.Client, privKeys []*ecdsa.PrivateKey,
	contracts []common.Address, receivers []common.Address, nonce uint64, count int, randomReceiver bool) map[common.Address]*big.Int {
	var batch []rpc.BatchElem
	var batchList [][]rpc.BatchElem
	result := make(map[common.Address]*big.Int)
	countInBatch := 200
	parsed, err := abi.JSON(strings.NewReader(CoinABI))
	if err != nil {
		log.Fatalf("Failed to parse abi %v", err)
	}

	log.Println("new block start")

	val := int64(2)

	for j := 0; j < count; j++ {
		for i, key := range privKeys {
			if i%countInBatch == 0 {
				batch = make([]rpc.BatchElem, countInBatch)
				batchList = append(batchList, batch)
			}
			receiverIndex0 := i
			receiverIndex1 := i
			if randomReceiver {
				receiverIndex0 = rand.Intn(len(receivers))
				receiverIndex1 = rand.Intn(len(receivers))
			}
			input, _ := parsed.Pack("rechargeToAccount", receivers[receiverIndex0], receivers[receiverIndex1])
			value := big.NewInt(val)
			//val+=10
			tx := newCallTransaction(key, contracts[i], nonce+uint64(j), input, value)
			data, err := rlp.EncodeToBytes(tx)
			if err != nil {
				log.Fatalf("Failed to encode create contract tx to bytes: %v", err)
			}
			batch[i%countInBatch] = rpc.BatchElem{
				Method: "etrue_sendTrueRawTransaction",
				Args:   []interface{}{hexutil.Encode(data)},
				Result: new(string),
			}
			//log.Println("tx", tx.Hash().String(),
			//	"from", crypto.PubkeyToAddress(key.PublicKey).String(),
			//	"contract", contracts[i].String(),
			//	"value", value.String(),
			//	"receiver0", receivers[receiverIndex0].String(),
			//	"receiver1", receivers[receiverIndex1].String())
		}
	}

	wg := sync.WaitGroup{}
	t0 := time.Now()
	for _, batch := range batchList {
		wg.Add(1)
		go func(batch []rpc.BatchElem) {
			defer wg.Done()

			err := rpcClient.BatchCallContext(context.TODO(), batch)
			if err != nil {
				log.Fatalf("Failed to call BatchCallContext to create contract: %v", err)
			}
		}(batch)

	}
	wg.Wait()
	log.Println("call", count*len(privKeys), "contracts elapse: ", common.PrettyDuration(time.Since(t0)))

	return result
}

func rechargeToAccountsByTx(rpcClient *rpc.Client, privKeys []*ecdsa.PrivateKey,
	receivers []common.Address, nonce uint64, count int) {
	var batch []rpc.BatchElem
	var batchList [][]rpc.BatchElem
	countInBatch := 400

	log.Println("new block start")

	val := int64(2)

	for j := 0; j < count; j++ {
		for i, key := range privKeys {
			if i%countInBatch == 0 {
				batch = make([]rpc.BatchElem, countInBatch)
				batchList = append(batchList, batch)
			}
			value := big.NewInt(val)
			tx := newCallTransaction(key, receivers[i], nonce+uint64(j), nil, value)
			data, err := rlp.EncodeToBytes(tx)
			if err != nil {
				log.Fatalf("Failed to encode tx to bytes: %v", err)
			}
			batch[i%countInBatch] = rpc.BatchElem{
				Method: "etrue_sendTrueRawTransaction",
				Args:   []interface{}{hexutil.Encode(data)},
				Result: new(string),
			}
		}
	}

	wg := sync.WaitGroup{}
	t0 := time.Now()
	for _, batch := range batchList {
		wg.Add(1)
		go func(batch []rpc.BatchElem) {
			defer wg.Done()

			err := rpcClient.BatchCallContext(context.TODO(), batch)
			if err != nil {
				log.Fatalf("Failed to call BatchCallContext to transfer: %v", err)
			}
		}(batch)

	}
	wg.Wait()
	log.Println("call", count*len(privKeys), "tx elapse: ", common.PrettyDuration(time.Since(t0)))
}

func sendRawTransaction(client *rpc.Client, from string, to string, value string) (string, error) {
	mapData := make(map[string]interface{})

	mapData["from"] = from
	mapData["to"] = to
	mapData["value"] = value

	var result string
	err := client.Call(&result, "etrue_sendTransaction", mapData)
	return result, err
}

func newContractTransaction(key *ecdsa.PrivateKey, nonce uint64, data []byte) (*types.Transaction, common.Address) {
	opts := bind.NewKeyedTransactor(key)
	value := new(big.Int)
	gasLimit := uint64(500000)
	gasPrice := big.NewInt(10000)

	rawTx := types.NewContractCreation(nonce, value, gasLimit, gasPrice, data)
	signer := types.NewTIP1Signer(params.SingleNodeChainConfig.ChainID)
	signedTx, _ := opts.Signer(signer, opts.From, rawTx)
	address := crypto.CreateAddress(opts.From, signedTx.Nonce())

	return signedTx, address
}

func newCallTransaction(key *ecdsa.PrivateKey, to common.Address, nonce uint64, input []byte, value *big.Int) *types.Transaction {
	opts := bind.NewKeyedTransactor(key)
	gasLimit := uint64(500000)
	gasPrice := big.NewInt(10000)

	rawTx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, input)
	signer := types.NewTIP1Signer(params.SingleNodeChainConfig.ChainID)
	signedTx, _ := opts.Signer(signer, opts.From, rawTx)

	return signedTx
}

func unlockAccount(client *rpc.Client, account string, password string, time int, name string) (bool, error) {
	var reBool bool
	err := client.Call(&reBool, "personal_unlockAccount", account, password, time)
	fmt.Println(name, " personal_unlockAccount Ok", reBool)
	return reBool, err
}

func getAccountBalanceValue(client *rpc.Client, account string, print bool) (*big.Int, string) {
	var hex string
	err := client.Call(&hex, "etrue_getBalance", account, "latest")
	if err != nil {
		log.Fatalf("Failed to get balance: %v", err)
	}

	value, _ := new(big.Int).SetString(strings.TrimPrefix(hex, "0x"), 16)
	if print {
		balance := new(big.Int).Set(value)
		fmt.Println("account", account, balance.Div(balance, big.NewInt(1000000000000000000)), " value ", value, " hex ", hex)
	}

	return value, hex
}
