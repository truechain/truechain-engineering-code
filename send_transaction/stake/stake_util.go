package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	ethereum "github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/cmd/utils"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrueclient"
	"github.com/truechain/truechain-engineering-code/rpc"
	"gopkg.in/urfave/cli.v1"
	"log"
	"math/big"
	"reflect"
)

func cancel(conn *etrueclient.Client, value *big.Int) error {
	input := packInput("cancel", value)
	txHash, _ := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input, "cancel")
	getResult(conn, txHash, true, false)
	return nil
}

func withdrawImpawn(conn *etrueclient.Client, value *big.Int) (common.Hash, error) {
	input := packInput("withdraw", value)
	txHash, err := sendContractTransaction(conn, from, types.StakingAddress, new(big.Int).SetInt64(0), priKey, input, "withdrawImpawn")
	getResult(conn, txHash, true, false)
	return txHash, err
}

var queryStakingCommand = cli.Command{
	Name:   "querystaking",
	Usage:  "Query staking info, can cancel info and can withdraw info",
	Action: utils.MigrateFlags(queryStakingImpawn),
	Flags:  ImpawnFlags,
}

func queryStakingImpawn(ctx *cli.Context) error {
	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	queryStakingInfo(conn, true, false)
	return nil
}

var sendCommand = cli.Command{
	Name:   "send",
	Usage:  "Send general transaction",
	Action: utils.MigrateFlags(sendTX),
	Flags:  append(ImpawnFlags, AddressFlag),
}

func sendTX(ctx *cli.Context) error {
	loadPrivate(ctx)
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)
	PrintBalance(conn, from)

	address := ctx.GlobalString(AddressFlag.Name)
	if !common.IsHexAddress(address) {
		printError("Must input correct address")
	}

	value := trueToWei(ctx, false)
	txHash, _ := sendContractTransaction(conn, from, common.HexToAddress(address), value, priKey, nil, "sendTX")
	getResult(conn, txHash, false, false)
	return nil
}

func delegateImpawn(conn *etrueclient.Client, value *big.Int, address common.Address, key *ecdsa.PrivateKey) (common.Hash, error) {
	input := packInput("delegate", from, value)
	txHash, err := sendOtherContractTransaction(conn, address, types.StakingAddress, nil, key, input, "delegateImpawn")
	getResult(conn, txHash, true, true)
	return txHash, err
}

func cancelDImpawn(conn *etrueclient.Client, value *big.Int, address common.Address, key *ecdsa.PrivateKey) error {
	input := packInput("undelegate", from, value)
	txHash, err := sendOtherContractTransaction(conn, address, types.StakingAddress, new(big.Int).SetInt64(0), key, input, "cancelDImpawn")
	getResult(conn, txHash, true, true)
	return err
}

func withdrawDImpawn(conn *etrueclient.Client, value *big.Int, address common.Address, key *ecdsa.PrivateKey) (common.Hash, error) {
	input := packInput("withdrawDelegate", from, value)
	txHash, err := sendOtherContractTransaction(conn, address, types.StakingAddress, new(big.Int).SetInt64(0), key, input, "withdrawDImpawn")
	getResult(conn, txHash, true, true)
	return txHash, err
}

var queryTxCommand = cli.Command{
	Name:   "querytx",
	Usage:  "Query tx hash, get transaction result",
	Action: utils.MigrateFlags(queryTxImpawn),
	Flags:  append(ImpawnFlags, TxHashFlag),
}

func queryTxImpawn(ctx *cli.Context) error {
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)

	txhash := ctx.GlobalString(TxHashFlag.Name)
	if txhash == "" {
		printError("Must input tx hash")
	}
	queryTx(conn, common.HexToHash(txhash), false, true, false)
	return nil
}

var queryLogCommand = cli.Command{
	Name:   "querylog",
	Usage:  "Query log hash, get transaction result",
	Action: utils.MigrateFlags(queryLogImpawn),
	Flags:  append(ImpawnFlags, TxHashFlag),
}

func queryLogImpawn(ctx *cli.Context) error {
	conn, url := dialConn(ctx)
	printBaseInfo(conn, url)
	filterLogs(conn)
	return nil
}

func filterLogs(client *etrueclient.Client) {
	method := "Deposit"
	contractAddress := types.StakingAddress
	//address := common.HexToAddress("0x25e7ba30a8ca432996553987da8d9f855016059b")
	event := abiStaking.Events[method]

	var nodeRule []interface{}
	//nodeRule = append(nodeRule, address.Bytes())

	// Append the event selector to the query parameters and construct the topic set
	queryT := append([][]interface{}{{event.ID}}, nodeRule)

	topics, err := makeTopics(queryT...)
	if err != nil {
		fmt.Println(" makeTopics err ", err)
	}

	header, err := client.HeaderByNumber(context.Background(), nil)

	if err != nil {
		log.Fatal(err)
	}

	for i := uint64(4706111); i < header.Number.Uint64(); {
		query := ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(i),
			ToBlock:   new(big.Int).SetUint64(i + 100),
			Addresses: []common.Address{
				contractAddress,
			},
			Topics: topics,
		}

		logs, err := client.FilterLogs(context.Background(), query)
		if err != nil {
			log.Fatal(err)
		}

		for _, vLog := range logs {
			fmt.Println("BlockNumber ", vLog.BlockNumber, " TxHash ", vLog.TxHash.Hex(), " BlockHash ", vLog.BlockHash.Hex()) // 2394201
			if method == "SetFee" {
				FeeS := struct {
					Fee *big.Int
				}{}
				err := abiStaking.Unpack(&FeeS, "SetFee", vLog.Data)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println("fee ", FeeS.Fee) // foo
			} else if method == "Deposit" {
				deposit := struct {
					Pubkey []byte
					Value  *big.Int
					Fee    *big.Int
				}{}
				err := abiStaking.Unpack(&deposit, "Deposit", vLog.Data)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println("value", types.ToTrue(deposit.Value), "fee ", deposit.Fee, "pk", hex.EncodeToString(deposit.Pubkey)) // foo
			}

			for i := range vLog.Topics {
				fmt.Println("i ", i, " topic ", vLog.Topics[i].String()) // 0xe79e73da417710ae99aa2088575580a60415d359acfad9cdd3382d59c80281d4
			}
		}
		i = i + 100
		if i%1000 == 0 {
			fmt.Println("index ", i)
		}
	}
}

// makeTopics converts a filter query argument list into a filter topic set.
func makeTopics(query ...[]interface{}) ([][]common.Hash, error) {
	topics := make([][]common.Hash, len(query))
	for i, filter := range query {
		for _, rule := range filter {
			var topic common.Hash

			// Try to generate the topic based on simple types
			switch rule := rule.(type) {
			case common.Hash:
				copy(topic[:], rule[:])
			case common.Address:
				copy(topic[common.HashLength-common.AddressLength:], rule[:])
			case *big.Int:
				blob := rule.Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case bool:
				if rule {
					topic[common.HashLength-1] = 1
				}
			case int8:
				blob := big.NewInt(int64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case int16:
				blob := big.NewInt(int64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case int32:
				blob := big.NewInt(int64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case int64:
				blob := big.NewInt(rule).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint8:
				blob := new(big.Int).SetUint64(uint64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint16:
				blob := new(big.Int).SetUint64(uint64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint32:
				blob := new(big.Int).SetUint64(uint64(rule)).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case uint64:
				blob := new(big.Int).SetUint64(rule).Bytes()
				copy(topic[common.HashLength-len(blob):], blob)
			case string:
				hash := crypto.Keccak256Hash([]byte(rule))
				copy(topic[:], hash[:])
			case []byte:
				hash := crypto.Keccak256Hash(rule)
				copy(topic[:], hash[:])

			default:
				// parameters that are not value types i.e. arrays and structs are not
				// stored directly but instead a keccak256-hash of an encoding is stored.
				//
				// We only convert stringS and bytes to hash, still need to deal with
				// array(both fixed-size and dynamic-size) and struct.

				// Attempt to generate the topic from funky types
				val := reflect.ValueOf(rule)
				switch {
				// static byte array
				case val.Kind() == reflect.Array && reflect.TypeOf(rule).Elem().Kind() == reflect.Uint8:
					reflect.Copy(reflect.ValueOf(topic[:val.Len()]), val)
				default:
					return nil, fmt.Errorf("unsupported indexed type: %T", rule)
				}
			}
			topics[i] = append(topics[i], topic)
		}
	}
	return topics, nil
}

var queryRewardCommand = cli.Command{
	Name:   "queryreward",
	Usage:  "Query committee reward info, contain deposit and delegate reward",
	Action: utils.MigrateFlags(queryRewardImpawn),
	Flags:  append(ImpawnFlags, SnailNumberFlag),
}

func queryRewardImpawn(ctx *cli.Context) error {
	ip = ctx.GlobalString(utils.RPCListenAddrFlag.Name)
	port = ctx.GlobalInt(utils.RPCPortFlag.Name)
	url := fmt.Sprintf("http://%s", fmt.Sprintf("%s:%d", ip, port))

	client, err := rpc.Dial(url)
	if err != nil {
		fmt.Println("Dail:", ip, err.Error())
		return err
	}
	head := struct {
		Number *hexutil.Big `json:"number"`
	}{}

	err = client.Call(&head, "etrue_getSnailBlockByNumber", "latest", false)
	if err != nil {
		fmt.Println("etrue_getSnailBlockByNumber:", err.Error())
		return err
	}

	var (
		snailNumber uint64
		start       uint64
	)

	if ctx.GlobalIsSet(SnailNumberFlag.Name) {
		snailNumber = ctx.GlobalUint64(SnailNumberFlag.Name)
	}

	if snailNumber < 1 {
		start = head.Number.ToInt().Uint64() - 100
	} else {
		start = snailNumber
	}

	for i := start; i < head.Number.ToInt().Uint64()-14; i++ {

		var reward types.ChainReward
		address := common.HexToAddress("0x0000000000000000000000000000000000000000")
		err = client.Call(&reward, "etrue_getChainRewardContent", hexutil.EncodeBig(new(big.Int).SetUint64(i)), address)
		if err != nil {
			fmt.Println("etrue_getChainRewardContent:", err.Error())
			return err
		}

		total := new(big.Int).SetUint64(0)
		for _, v := range reward.CommitteeBase {
			for _, vv := range v.Items {
				total.Add(total, vv.Amount)
			}
		}
		committeeReward, _ := new(big.Int).SetString("19268837169230000000", 10)
		if total.Cmp(committeeReward) > 0 {
			fmt.Println("----------------------------------------------------------------")
		}
		fmt.Println("Block Number", i, " committeeReward total ", total)
	}

	return nil
}
