package parallel

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/types"
)

type ExecutionGroup struct {
	id            int
	header        *types.Header
	transactions  types.Transactions
	startTrxIndex int
	finished      bool

	// transaction execution result
	trxHashToResultMap map[common.Hash]*TrxResult
	err                error
	errTxIndex         int
	usedGas            uint64
}

type TrxResult struct {
	receipt          *types.Receipt
	logs             []*types.Log
	touchedAddresses *TouchedAddressObject
	usedGas          uint64
}

func NewTrxResult(receipt *types.Receipt, logs []*types.Log, touchedAddresses *TouchedAddressObject, usedGas uint64) *TrxResult {
	return &TrxResult{receipt: receipt, logs: logs, touchedAddresses: touchedAddresses, usedGas: usedGas}
}

func NewExecutionGroup() *ExecutionGroup {
	return &ExecutionGroup{}
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

func (e *ExecutionGroup) setId(groupId int) {
	e.id = groupId
}

func (e *ExecutionGroup) setStartTrxPos(index int) {
	e.startTrxIndex = index
}
