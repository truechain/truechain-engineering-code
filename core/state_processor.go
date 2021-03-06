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
	"fmt"
	//"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/metrics"
	"math"
	"time"

	//"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/params"

	"math/big"
)

var (
	blockExecutionTxTimer = metrics.NewRegisteredTimer("chain/state/executiontx", nil)
	blockFinalizeTimer    = metrics.NewRegisteredTimer("chain/state/finalize", nil)
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
func (fp *StateProcessor) Process(block *types.Block, statedb *state.StateDB,
	cfg vm.Config) (types.Receipts, []*types.Log, uint64, *types.ChainReward, error) {
	var (
		receipts  types.Receipts
		usedGas   = new(uint64)
		feeAmount = big.NewInt(0)
		header    = block.Header()
		allLogs   []*types.Log
		gp        = new(GasPool).AddGas(block.GasLimit())
	)
	start := time.Now()
	blockContext := NewEVMBlockContext(header, fp.bc, nil, nil)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, fp.config, cfg)
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		msg, err := tx.AsMessage(types.MakeSigner(fp.config, header.Number))
		if err != nil {
			return nil, nil, 0, nil, err
		}
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		//receipt, err := ApplyTransaction(fp.config, fp.bc, gp, statedb, header, tx, usedGas, feeAmount, cfg)
		receipt, err := applyTransaction(msg, gp, statedb, header, tx, usedGas, vmenv, feeAmount)
		if err != nil {
			return nil, nil, 0, nil, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	t1 := time.Now()
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	_, infos, err := fp.engine.Finalize(fp.bc, header, statedb, block.Transactions(), receipts, feeAmount)
	if err != nil {
		return nil, nil, 0, nil, err
	}
	blockExecutionTxTimer.Update(t1.Sub(start))
	blockFinalizeTimer.Update(time.Since(t1))
	return receipts, allLogs, *usedGas, infos, nil
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func applyTransaction(msg types.Message, gp *GasPool, statedb *state.StateDB, header *types.Header,
	tx *types.Transaction, usedGas *uint64, evm *vm.EVM, feeAmount *big.Int) (*types.Receipt, error) {
	txContext := NewEVMTxContext(msg)
	evm.Reset(txContext, statedb)
	// Apply the transaction to the current state (included in the env)
	result, err := ApplyMessage(evm, msg, gp)

	if err != nil {
		return nil, err
	}
	// Update the state with pending changes
	var root []byte

	statedb.Finalise(true)

	*usedGas += result.UsedGas
	gasFee := new(big.Int).Mul(new(big.Int).SetUint64(result.UsedGas), msg.GasPrice())
	feeAmount.Add(gasFee, feeAmount)
	if msg.Fee() != nil {
		feeAmount.Add(msg.Fee(), feeAmount) //add fee
	}

	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := &types.Receipt{PostState: root, CumulativeGasUsed: *usedGas}
	if result.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.BlockHash = statedb.BlockHash()
	receipt.BlockNumber = header.Number
	receipt.TransactionIndex = uint(statedb.TxIndex())

	return receipt, err
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, gp *GasPool, statedb *state.StateDB,
	header *types.Header, tx *types.Transaction, usedGas *uint64, feeAmount *big.Int, cfg vm.Config) (*types.Receipt, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, err
	}
	if err := types.ForbidAddress(msg.From()); err != nil {
		return nil, err
	}
	// Create a new context to be used in the EVM environment
	blockContext := NewEVMBlockContext(header, bc, nil, nil)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, config, cfg)
	return applyTransaction(msg, gp, statedb, header, tx, usedGas, vmenv, feeAmount)
}

// ReadTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the result
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ReadTransaction(config *params.ChainConfig, bc ChainContext,
	statedb *state.StateDB, header *types.Header, tx *types.Transaction, cfg vm.Config) ([]byte, uint64, error) {

	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}

	msgCopy := types.NewMessage(msg.From(), msg.To(), msg.Payment(), 0, msg.Value(), msg.Fee(), msg.Gas(), msg.GasPrice(), msg.Data(), false)

	if err := types.ForbidAddress(msgCopy.From()); err != nil {
		return nil, 0, err
	}
	// Create a new context to be used in the EVM environment
	txCtx := NewEVMTxContext(msgCopy)
	blockCtx := NewEVMBlockContext(header, bc, nil, nil)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(blockCtx, txCtx, statedb, config, cfg)
	// Apply the transaction to the current state (included in the env)
	gp := new(GasPool).AddGas(math.MaxUint64)
	result, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, 0, err
	}

	return result.ReturnData, result.UsedGas, err
}
