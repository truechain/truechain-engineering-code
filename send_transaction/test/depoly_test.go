package test

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"os"
	"strings"
	"testing"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

// ContractABI is the input ABI used to generate the binding from.
const ContractABI = `[
	{
		"inputs": [],
		"stateMutability": "nonpayable",
		"type": "constructor"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": true,
				"internalType": "address",
				"name": "_owner",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "address",
				"name": "_spender",
				"type": "address"
			},
			{
				"indexed": false,
				"internalType": "uint256",
				"name": "_value",
				"type": "uint256"
			}
		],
		"name": "Approval",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": true,
				"internalType": "address",
				"name": "_from",
				"type": "address"
			},
			{
				"indexed": true,
				"internalType": "address",
				"name": "_to",
				"type": "address"
			},
			{
				"indexed": false,
				"internalType": "uint256",
				"name": "_value",
				"type": "uint256"
			}
		],
		"name": "Transfer",
		"type": "event"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "_owner",
				"type": "address"
			},
			{
				"internalType": "address",
				"name": "_spender",
				"type": "address"
			}
		],
		"name": "allowance",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "remaining",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "_spender",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "_value",
				"type": "uint256"
			}
		],
		"name": "approve",
		"outputs": [
			{
				"internalType": "bool",
				"name": "success",
				"type": "bool"
			}
		],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "_owner",
				"type": "address"
			}
		],
		"name": "balanceOf",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "balance",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "decimals",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "name",
		"outputs": [
			{
				"internalType": "string",
				"name": "",
				"type": "string"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "symbol",
		"outputs": [
			{
				"internalType": "string",
				"name": "",
				"type": "string"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "totalSupply",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "",
				"type": "uint256"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "_to",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "_value",
				"type": "uint256"
			}
		],
		"name": "transfer",
		"outputs": [
			{
				"internalType": "bool",
				"name": "success",
				"type": "bool"
			}
		],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{
				"internalType": "address",
				"name": "_from",
				"type": "address"
			},
			{
				"internalType": "address",
				"name": "_to",
				"type": "address"
			},
			{
				"internalType": "uint256",
				"name": "_value",
				"type": "uint256"
			}
		],
		"name": "transferFrom",
		"outputs": [
			{
				"internalType": "bool",
				"name": "success",
				"type": "bool"
			}
		],
		"stateMutability": "nonpayable",
		"type": "function"
	}
]`

// ContractBin is the compiled bytecode used for deploying new contracts.
const ContractBin = `60806040526000805534801561001457600080fd5b506a52b7d2dcc80cd2e4000000600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610d3d806100736000396000f3fe608060405234801561001057600080fd5b50600436106100935760003560e01c8063313ce56711610066578063313ce5671461022557806370a082311461024357806395d89b411461029b578063a9059cbb1461031e578063dd62ed3e1461038457610093565b806306fdde0314610098578063095ea7b31461011b57806318160ddd1461018157806323b872dd1461019f575b600080fd5b6100a06103fc565b6040518080602001828103825283818151815260200191508051906020019080838360005b838110156100e05780820151818401526020810190506100c5565b50505050905090810190601f16801561010d5780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b6101676004803603604081101561013157600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610435565b604051808215151515815260200191505060405180910390f35b610189610527565b6040518082815260200191505060405180910390f35b61020b600480360360608110156101b557600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610536565b604051808215151515815260200191505060405180910390f35b61022d61099b565b6040518082815260200191505060405180910390f35b6102856004803603602081101561025957600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506109a0565b6040518082815260200191505060405180910390f35b6102a36109e9565b6040518080602001828103825283818151815260200191508051906020019080838360005b838110156102e35780820151818401526020810190506102c8565b50505050905090810190601f1680156103105780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b61036a6004803603604081101561033457600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610a22565b604051808215151515815260200191505060405180910390f35b6103e66004803603604081101561039a57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610c4d565b6040518082815260200191505060405180910390f35b6040518060400160405280600881526020017f545553445445535400000000000000000000000000000000000000000000000081525081565b600081600260003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508273ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff167f8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925846040518082815260200191505060405180910390a36001905092915050565b6a52b7d2dcc80cd2e400000081565b60008073ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff1614156105b5576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401808060200182810382526000815260200160200191505060405180910390fd5b81600160008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410158015610680575081600260008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205410155b6106f2576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600b8152602001807f6e6f7420656e672061616100000000000000000000000000000000000000000081525060200191505060405180910390fd5b61078182600260008773ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610cd490919063ffffffff16565b600260008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000208190555061085382600160008773ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610cd490919063ffffffff16565b600160008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055506108e882600160008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610ceb90919063ffffffff16565b600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508273ffffffffffffffffffffffffffffffffffffffff168473ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef846040518082815260200191505060405180910390a3600190509392505050565b601281565b6000600160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b6040518060400160405280600481526020017f555344540000000000000000000000000000000000000000000000000000000081525081565b600081600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020541015610ab4576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401808060200182810382526000815260200160200191505060405180910390fd5b610b0682600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610cd490919063ffffffff16565b600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610b9b82600160008673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610ceb90919063ffffffff16565b600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508273ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef846040518082815260200191505060405180910390a36001905092915050565b6000600260008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054905092915050565b600082821115610ce057fe5b818303905092915050565b600080828401905083811015610cfd57fe5b809150509291505056fea26469706673582212202e2dd37d810a895ba9cb30c7173ce6e2e390f400c1cf49fe404db7f100b581fa64736f6c63430006080033`

var (
	contractABI, _  = abi.JSON(strings.NewReader(ContractABI))
	bytecode        = common.FromHex(ContractBin)
	contractAddress common.Address
	priKey, _       = crypto.HexToECDSA("0260c952edc49037129d8cabbe4603d15185d83aa718291279937fb6db0fa7a2")
	mAccount        = common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69")
	skey1, _        = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	saddr1          = crypto.PubkeyToAddress(skey1.PublicKey)
	gspec           = DefaulGenesisBlock()
)

func DefaulGenesisBlock() *core.Genesis {
	i, _ := new(big.Int).SetString("10000000000000000000000", 10)
	key1 := hexutil.MustDecode("0x04d341c94a16b02cee86a627d0f6bc6e814741af4cab5065637aa013c9a7d9f26051bb6546030cd67e440d6df741cb65debaaba6c0835579f88a282193795ed369")
	key2 := hexutil.MustDecode("0x0496e0f18d4bf38e0b0de161edd2aa168adaf6842706e5ebf31e1d46cb79fe7b720c750a9e7a3e1a528482b0da723b5dfae739379e555a2893e8693747559f83cd")
	key3 := hexutil.MustDecode("0x0418196ee090081bdec01e8840941b9f6a141a713dd3461b78825edf0d8a7f8cdf3f612832dc9d94249c10c72629ea59fbe0bdd09bea872ddab2799748964c93a8")
	key4 := hexutil.MustDecode("0x04c4935993a3ce206318ab884871fbe2d4dce32a022795c674784f58e7faf3239631b6952b82471fe1e93ef999108a18d028e5d456cd88bb367d610c5e57c7e443")

	return &core.Genesis{
		Config:     params.DevnetChainConfig,
		Nonce:      928,
		ExtraData:  nil,
		GasLimit:   88080384,
		Difficulty: big.NewInt(20000),
		Alloc: map[common.Address]types.GenesisAccount{
			common.HexToAddress("0xC02f50f4F41f46b6a2f08036ae65039b2F9aCd69"): {Balance: i},
			common.HexToAddress("0x6d348e0188Cc2596aaa4046a1D50bB3BA50E8524"): {Balance: i},
			common.HexToAddress("0xE803895897C3cCd35315b2E41c95F817543811A5"): {Balance: i},
			common.HexToAddress("0x3F739ffD8A59965E07e1B8d7CCa938125BCe8CFb"): {Balance: i},
		},
		Committee: []*types.CommitteeMember{
			{Coinbase: common.HexToAddress("0x3f9061bf173d8f096c94db95c40f3658b4c7eaad"), Publickey: key1},
			{Coinbase: common.HexToAddress("0x2cdac3658f85b5da3b70223cc3ad3b2dfe7c1930"), Publickey: key2},
			{Coinbase: common.HexToAddress("0x41acde8dd7611338c2a30e90149e682566716e9d"), Publickey: key3},
			{Coinbase: common.HexToAddress("0x0ffd116a3bf97a7112ff8779cc770b13ea3c66a5"), Publickey: key4},
		},
	}
}

func TestDeployContract(t *testing.T) {
	var (
		db     = etruedb.NewMemDatabase()
		pow    = minerva.NewFaker()
		signer = types.NewTIP1Signer(gspec.Config.ChainID)
	)

	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)
	genesis := gspec.MustFastCommit(db)

	// Import the chain. This runs all block validation rules.
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, pow, vm.Config{})
	defer blockchain.Stop()

	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.
	blocks, _ := core.GenerateChain(gspec.Config, genesis, pow, db, 5, func(number int, gen *core.BlockGen) {
		switch number {
		case 1:
			tx, _ := types.SignTx(types.NewContractCreation(gen.TxNonce(mAccount), big.NewInt(5), 1000000, new(big.Int).SetUint64(1), bytecode), signer, priKey)
			contractAddress = crypto.CreateAddress(mAccount, tx.Nonce())
			fmt.Println("contractAddress", contractAddress.String(), " nonde ", tx.Nonce(), " ", blockchain.CurrentBlock().NumberU64())
			gen.AddTxWithChain(blockchain, tx)
		case 2:
			input := packInput(contractABI, "transfer", "sendGetDepositTransaction", saddr1, big.NewInt(1000))
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(mAccount), contractAddress, big.NewInt(1), 5000000, new(big.Int).SetUint64(1), input), signer, priKey)
			gen.AddTxWithChain(blockchain, tx)
		}
	})

	if _, err := blockchain.InsertChain(blocks); err != nil {
		panic(err)
	}

	fmt.Println(" 11 ", blockchain.CurrentBlock().NumberU64())

	blocks, _ = core.GenerateChain(gspec.Config, blockchain.CurrentBlock(), pow, db, 1, func(number int, gen *core.BlockGen) {
		input := packInput(contractABI, "balanceOf", "sendGetDepositTransaction", saddr1)
		tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(mAccount), contractAddress, nil, 5000000, new(big.Int).SetUint64(1), input), signer, priKey)
		output, gas := gen.ReadTxWithChain(blockchain, tx)
		fmt.Println("output ", output)
		args := struct {
			Balance *big.Int
		}{}
		UnpackOutput(contractABI, "balanceOf", output, &args)
		fmt.Println("gas", gas, " address ", args.Balance)
	})

	state, _ := blockchain.State()
	fmt.Printf("last block: #%d\n", blockchain.CurrentBlock().Number())
	fmt.Println("balance of addr1:", state.GetBalance(mAccount))
}

func UnpackOutput(abiStaking abi.ABI, abiMethod string, output []byte, result interface{}) {
	err := abiStaking.Unpack(result, abiMethod, output)
	if err != nil {
		printTest(abiMethod, " error ", err)
	}
}

func packInput(abiStaking abi.ABI, abiMethod, method string, params ...interface{}) []byte {
	input, err := abiStaking.Pack(abiMethod, params...)
	if err != nil {
		printTest(method, " error ", err)
	}
	return input
}

func printTest(a ...interface{}) {
	log.Info("test", "SendTX", a)
}
