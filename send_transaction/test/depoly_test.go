package test

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/log"
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
"inputs":[],"stateMutability":"nonpayable","type":"constructor"},
{"anonymous":false,"inputs":[{"indexed":false,"internalType":"address","name":"from","type":"address"},
							{"indexed":false,"internalType":"address","name":"to","type":"address"},
							{"indexed":false,"internalType":"uint256","name":"amount","type":"uint256"}],
							"name":"Sent","type":"event"},
{"inputs":[{"internalType":"address","name":"","type":"address"}],
"name":"balances",
"outputs":[{"internalType":"uint256","name":"","type":"uint256"}],
"stateMutability":"view","type":"function"},
		{"inputs":[{"internalType":"address","name":"receiver","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],
		"name":"mint","outputs":[],"stateMutability":"nonpayable","type":"function"},
		{"inputs":[],"name":"minter",
		"outputs":[{"internalType":"address","name":"","type":"address"}],
		"stateMutability":"view","type":"function"},
		{"inputs":[{"internalType":"address","name":"receiver","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],
		"name":"send",
		"outputs":[],"stateMutability":"nonpayable","type":"function"}]`

// ContractBin is the compiled bytecode used for deploying new contracts.
const ContractBin = `60806040523480156100115760006000fd5b505b33600060006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055505b61005a565b6104f9806100696000396000f3fe60806040523480156100115760006000fd5b50600436106100515760003560e01c8063075461721461005757806327e235e3146100a157806340c10f19146100fa578063d0679d341461014957610051565b60006000fd5b61005f610198565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6100e4600480360360208110156100b85760006000fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506101be565b6040518082815260200191505060405180910390f35b610147600480360360408110156101115760006000fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506101d9565b005b610196600480360360408110156101605760006000fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506102b7565b005b600060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b60016000506020528060005260406000206000915090505481565b600060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff161415156102365760006000fd5b789f4f2726179a224501d762422c946590d910000000000000008110151561025e5760006000fd5b80600160005060008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282825054019250508190909055505b5050565b600160005060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020600050548111151515610377576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260158152602001807f496e73756666696369656e742062616c616e63652e000000000000000000000081526020015060200191505060405180910390fd5b80600160005060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020600082828250540392505081909090555080600160005060008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008282825054019250508190909055507f3990db2d31862302a685e8086b5755072a6e2b5b780af1ee81ece35ee3cd3345338383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a15b505056fea2646970667358221220a16cfcbaa783003065fcbcb028c638918e3837d80e5579358a91f23a6a4d405964736f6c63430006040033`

var (
	contractABI, _  = abi.JSON(strings.NewReader(ContractABI))
	bytecode        = common.FromHex(ContractBin)
	contractAddress common.Address
)

func TestDeployContract(t *testing.T) {
	var (
		db  = etruedb.NewMemDatabase()
		pow = minerva.NewFaker()
	)

	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)
	gspec.Config.TIP7 = &params.BlockConfig{FastNumber: big.NewInt(0)}
	gspec.Config.TIP8 = &params.BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(-1)}
	gspec.Config.TIP9 = &params.BlockConfig{SnailNumber: big.NewInt(20)}
	genesis := gspec.MustFastCommit(db)

	// Import the chain. This runs all block validation rules.
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, pow, vm.Config{})
	defer blockchain.Stop()

	// This call generates a chain of 5 blocks. The function runs for
	// each block and adds different features to gen based on the
	// block index.
	chain, _ := core.GenerateChain(gspec.Config, genesis, pow, db, 10, func(i int, gen *core.BlockGen) {
		switch i {
		case 1:
			tx, _ := types.SignTx(types.NewContractCreation(gen.TxNonce(mAccount), big.NewInt(30000000), 1000000, new(big.Int).SetUint64(10000000), bytecode), signer, priKey)
			gen.AddTx(tx)
			contractAddress = crypto.CreateAddress(mAccount, tx.Nonce())
			fmt.Println("contractAddress", contractAddress.String())
		case 2:
			// In block 2, addr1 sends some more ether to addr2.
			// addr2 passes it on to addr3.
			input := packInput(contractABI, "minter", "sendGetDepositTransaction")
			args := struct {
				common.Address
			}{}
			tx1, _ := types.SignTx(types.NewTransaction(gen.TxNonce(mAccount), contractAddress, big.NewInt(1000), 500000, nil, input), signer, priKey)
			gen.AddTx(tx1)
			output, gas := gen.ReadTxWithChain(blockchain, tx1)
			UnpackOutput(contractABI, "minter", output, &args)
			fmt.Println("gas", gas, " address ", args.String())
		case 3:
			// Block 3 is empty but was mined by addr3.
			gen.SetExtra([]byte("yeehaw"))
		}
	})

	if i, err := blockchain.InsertChain(chain); err != nil {
		fmt.Printf("insert error (block %d): %v\n", chain[i].NumberU64(), err)
		return
	}

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
