package core

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
)

type ExecutionGroup struct {
	id            int
	header        *types.Header
	transactions  types.Transactions
	startTrxIndex int
	statedb       *state.StateDB

	// transaction execution result
	trxHashToResultMap map[common.Hash]*TrxResult
	err                error
	errTxIndex         int
	usedGas            uint64
	feeAmount          *big.Int
}

type TrxResult struct {
	receipt          *types.Receipt
	touchedAddresses *state.TouchedAddressObject
	usedGas          uint64
	feeAmount        *big.Int
}

func NewTrxResult(receipt *types.Receipt, touchedAddresses *state.TouchedAddressObject, usedGas uint64, feeAmount *big.Int) *TrxResult {
	return &TrxResult{receipt: receipt, touchedAddresses: touchedAddresses, usedGas: usedGas, feeAmount: feeAmount}
}

func NewExecutionGroup() *ExecutionGroup {
	return &ExecutionGroup{
		trxHashToResultMap: make(map[common.Hash]*TrxResult),
		feeAmount:          big.NewInt(0),
		errTxIndex:         -1,
	}
}

func (e *ExecutionGroup) Transactions() types.Transactions {
	return e.transactions
}

func (e *ExecutionGroup) SetTransactions(transactions types.Transactions) {
	e.transactions = transactions
}

func (e *ExecutionGroup) Header() *types.Header {
	return e.header
}

func (e *ExecutionGroup) SetHeader(header *types.Header) {
	e.header = header
}

func (e *ExecutionGroup) AddTransaction(trx *types.Transaction) {
	e.transactions = append(e.transactions, trx)
}

func (e *ExecutionGroup) AddTransactions(transactions types.Transactions) {
	e.transactions = append(e.transactions, transactions...)
}

func (e *ExecutionGroup) SetId(groupId int) {
	e.id = groupId
}

func (e *ExecutionGroup) SetStartTrxPos(index int) {
	e.startTrxIndex = index
}

func (e *ExecutionGroup) SetStatedb(statedb *state.StateDB) {
	e.statedb = statedb
}

func (e *ExecutionGroup) AddUsedGas(usedGas uint64) {
	e.usedGas += usedGas
}

func (e *ExecutionGroup) AddFeeAmount(feeAmount *big.Int) {
	e.feeAmount.Add(e.feeAmount, feeAmount)
}

func (e *ExecutionGroup) reuseTxResults(txsToReuse []TxHashGroupIdPair, conflictGroups map[int]*ExecutionGroup) {
	stateObjsFromOtherGroup := make(map[int]map[common.Address]*state.StateObjectToReuse)

	for gId, _ := range conflictGroups {
		stateObjsFromOtherGroup[gId] = make(map[common.Address]*state.StateObjectToReuse)
	}

	for i := len(txsToReuse) - 1; i >= 0; i-- {
		txHash := txsToReuse[i].txHash
		oldGroupId := txsToReuse[i].oldGroupId

		if result, ok := conflictGroups[oldGroupId].trxHashToResultMap[txHash]; ok {
			appendStateObjToReuse(stateObjsFromOtherGroup[oldGroupId], result.touchedAddresses)

			e.statedb.CopyTxJournalFromOtherDB(conflictGroups[oldGroupId].statedb, txHash)

			e.trxHashToResultMap[txHash] = result
			e.AddUsedGas(result.usedGas)
			e.AddFeeAmount(result.feeAmount)
		}
	}

	for gId, stateObjsMap := range stateObjsFromOtherGroup {
		e.statedb.CopyStateObjFromOtherDB(conflictGroups[gId].statedb, stateObjsMap)
	}
}

func appendStateObjToReuse(stateObjsToReuse map[common.Address]*state.StateObjectToReuse, touchedAddr *state.TouchedAddressObject) {
	for addr, op := range touchedAddr.AccountOp() {
		if op {
			if stateObj, ok := stateObjsToReuse[addr]; !ok {
				stateObj = state.NewStateObjectToReuse(addr, nil, true)
				stateObjsToReuse[addr] = stateObj
			} else {
				stateObj.ReuseData = true
			}
		}
	}
	for storage, op := range touchedAddr.StorageOp() {
		if op {
			addr := storage.AccountAddress
			stateObj, ok := stateObjsToReuse[addr]
			if !ok {
				stateObj = state.NewStateObjectToReuse(addr, nil, false)
				stateObjsToReuse[addr] = stateObj
			}
			stateObj.Keys = append(stateObj.Keys, storage.Key)
		}
	}
}
