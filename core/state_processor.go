// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/params"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.

func (fp *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts  types.Receipts
		usedGas   = new(uint64)
		feeAmount = big.NewInt(0)
		header    = block.Header()
		allLogs   []*types.Log
		gp        = new(GasPool).AddGas(block.GasLimit())
	)
	log.Warn("", "tx.length", block.Transactions().Len())
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {

		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, _, err := ApplyTransaction(fp.config, fp.bc, gp, statedb, header, tx, usedGas, feeAmount, cfg)
		if err != nil {
			return nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	_, err := fp.engine.Finalize(fp.bc, header, statedb, block.Transactions(), receipts, feeAmount)
	if err != nil {
		return nil, nil, 0, err
	}

	return receipts, allLogs, *usedGas, nil
}

type TxResult struct {
	statedb *state.StateDB
	fee     *big.Int
	Receipt *types.Receipt
}

func NewTxResult() *TxResult {
	txResult := &TxResult{}
	return txResult
}

func validateTxResult(txResults []*TxResult, gp *GasPool) error {
	var usedGas uint64
	for _, txResult := range txResults {
		if txResult.statedb == nil && txResult.Receipt == nil {
			return ErrTransactionExecution
		}
	}
	for _, txResult := range txResults {
		if txResult.Receipt != nil {
			usedGas += txResult.Receipt.GasUsed
		}
	}
	log.Info("validateTxResult print", "usedGas", usedGas, "gp", uint64(*gp))
	if uint64(*gp) < usedGas {
		log.Error("all transactions gasUsed reached block gasLimit")
		return ErrGasLimitReached
	}

	return nil
}

func (txResult *TxResult) processTxResult(usedGas *uint64, receipts types.Receipts, feeAmount *big.Int) {
	receipt := txResult.Receipt
	*usedGas += receipt.GasUsed
	receipt.CumulativeGasUsed = *usedGas

	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipts = append(receipts, txResult.Receipt)

	feeAmount.Add(feeAmount, txResult.fee)
}

func (txResult *TxResult) judgeCommonAccount(processedAccounts map[common.Address]int) bool {
	dirties := txResult.statedb.GetDirtyAccounts()
	for address, _ := range dirties {
		if _, ok := processedAccounts[address]; ok {
			return true
		}
	}
	return false
}

func CopyCfg(cfg *vm.Config) *vm.Config {
	cpy := *cfg
	return &cpy
}

func (fp *StateProcessor) Process2(bc *BlockChain, block *types.Block,
	statedb *state.StateDB, cfg vm.Config, txParallel bool) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts  types.Receipts
		usedGas   = new(uint64)
		feeAmount = big.NewInt(0)
		header    = block.Header()
		allLogs   []*types.Log
		//allErrors []error
		gp = new(GasPool).AddGas(block.GasLimit())
		//txResults []*TxResult
		//processedAccounts = make(map[common.Address]int)
	)
	log.Warn("", "tx.length", block.Transactions().Len())
	// Iterate over and process the individual transactions
	err := fp.txParallel(block, header, bc, cfg, statedb, usedGas, feeAmount, allLogs, gp, receipts)
	if err != nil {
		return nil, nil, 0, err
	}
	return receipts, allLogs, *usedGas, nil
}

func (fp *StateProcessor) txParallel(block *types.Block, header *types.Header, bc *BlockChain, cfg vm.Config,
	statedb *state.StateDB, usedGas *uint64, feeAmount *big.Int, allLogs []*types.Log, gp *GasPool, receipts types.Receipts) error {
	var (
		txResults         []*TxResult
		allErrors         []error
		processedAccounts = make(map[common.Address]int)
		nonceLock         *AddrLocker
	)

	for i, tx := range block.Transactions() {
		config := CopyCfg(&cfg)
		txResult := NewTxResult()
		//state := statedb.Copy()
		state, _ := bc.State()
		state.Prepare(tx.Hash(), block.Hash(), i)
		go ApplyTransaction2(fp.config, fp.bc, state, header, tx, config, txResult, allLogs, allErrors, nonceLock)
		txResults = append(txResults, txResult)
	}
	if len(allErrors) != 0 {
		log.Error("allErrors error ", "err", allErrors[0], "len(allErrors)", len(allErrors))
		return allErrors[0]
	}
	err := validateTxResult(txResults, gp)
	if err != nil {
		return err
	}
	//initialStateDB := statedb.Copy()
	for i, txResult := range txResults {
		flag := txResult.judgeCommonAccount(processedAccounts)
		if flag {
			log.Warn("go into combine state", "times", i)
			tx := block.Transactions()[txResult.statedb.GetTxIndex()]
			statedb.Prepare(tx.Hash(), block.Hash(), i)
			receipt, _, err := ApplyTransaction(fp.config, fp.bc, gp, statedb, header, tx, usedGas, feeAmount, cfg)
			if err != nil {
				return err
			}
			receipts = append(receipts, receipt)
			allLogs = append(allLogs, receipt.Logs...)
		} else { //Can be combined
			log.Warn("go into combine state", "times", i)
			statedb.CopyDirtyAccounts(txResult.statedb)
			statedb.Finalise(true)
			txResult.processTxResult(usedGas, receipts, feeAmount)
			allLogs = append(allLogs, txResult.Receipt.Logs...)
		}
		for account, times := range txResult.statedb.GetDirtyAccounts() {
			processedAccounts[account] = times
		}
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	_, err = fp.engine.Finalize(fp.bc, header, statedb, block.Transactions(), receipts, feeAmount)
	if err != nil {
		return err
	}
	return nil
}

func ApplyTransaction2(config *params.ChainConfig, bc ChainContext,
	statedb *state.StateDB, header *types.Header, tx *types.Transaction, cfg *vm.Config,
	txResult *TxResult, allLogs []*types.Log, allErrors []error, nonceLock *AddrLocker) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		log.Error("ApplyTransaction2 AsMessage tx error", "index", statedb.GetTxIndex(), "error", err)
		allErrors = append(allErrors, err)
		return
	}
	// Create a new context to be used in the EVM environment
	context := NewEVMContext(msg, header, bc)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(context, statedb, config, *cfg)
	// Apply the transaction to the current state (included in the env)
	//_, gas, failed, err := ApplyMessage(vmenv, msg, gp)
	nonceLock.LockAddr(msg.From())
	defer nonceLock.UnlockAddr(msg.From())
	_, gas, failed, err := ApplyMessage2(vmenv, msg)
	if err != nil {
		log.Error("ApplyTransaction2 ApplyMessage2 tx error", "index", statedb.GetTxIndex(), "error", err)
		allErrors = append(allErrors, err)
		return
	}
	// Update the state with pending changes
	var root []byte

	statedb.Finalise(true)
	//txResult.DirtyAccounts = statedb.GetDirtyAccount()
	//*usedGas += gas
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), msg.GasPrice())
	//feeAmount.Add(fee, feeAmount)
	txResult.fee = fee
	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing wether the root touch-delete accounts.
	receipt := types.NewReceipt(root, failed, 0)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	//receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	//receipts = append(receipts, receipt)
	allLogs = append(allLogs, receipt.Logs...)
	txResult.Receipt = receipt
	txResult.statedb = statedb
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, gp *GasPool,
	statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, feeAmount *big.Int, cfg vm.Config) (*types.Receipt, uint64, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}
	// Create a new context to be used in the EVM environment
	context := NewEVMContext(msg, header, bc)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(context, statedb, config, cfg)
	// Apply the transaction to the current state (included in the env)
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, 0, err
	}
	// Update the state with pending changes
	var root []byte

	statedb.Finalise(true)

	*usedGas += gas
	fee := new(big.Int).Mul(new(big.Int).SetUint64(gas), msg.GasPrice())
	feeAmount.Add(fee, feeAmount)

	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing wether the root touch-delete accounts.
	receipt := types.NewReceipt(root, failed, *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	return receipt, gas, err
}
