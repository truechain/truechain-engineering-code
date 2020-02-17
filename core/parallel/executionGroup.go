package parallel

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/types"
)

type ExecutionGroup struct {
	id                             int
	header                         *types.Header
	transactions                   types.Transactions
	result                         *GroupResult
	startTrxHash                   common.Hash
	startTrxIndex                  int
	trxesToGetResultFromOtherGroup map[int]map[common.Hash]struct{}
	trxesToRollBackInOtherGroup    map[int]map[common.Hash]struct{}
}

func (e *ExecutionGroup) Transactions() types.Transactions {
	return e.transactions
}

func (e *ExecutionGroup) SetTransactions(transactions types.Transactions) {
	e.transactions = transactions
}

func (e *ExecutionGroup) Result() *GroupResult {
	return e.result
}

func (e *ExecutionGroup) SetResult(result *GroupResult) {
	e.result = result
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

func NewExecutionGroup() *ExecutionGroup {
	return &ExecutionGroup{}
}

func (e *ExecutionGroup) sortTrxByIndex(trxHashToIndexMap map[common.Hash]int) {

}

func (e *ExecutionGroup) setId(groupId int) {
	e.id = groupId
}

func (e *ExecutionGroup) setStartTrxPos(hash common.Hash, index int) {
	e.startTrxHash = hash
	e.startTrxIndex = index
}

func (e *ExecutionGroup) addTrxHashToGetPartResult(oldGroup int, trxHash common.Hash) {
	e.updateMap(oldGroup, trxHash, e.trxesToGetResultFromOtherGroup)
}

func (e *ExecutionGroup) addTrxToRollbackInOtherGroup(groupId int, trxHash common.Hash) {
	e.updateMap(groupId, trxHash, e.trxesToRollBackInOtherGroup)
}

func (e *ExecutionGroup) updateMap(oldGroup int, trxHash common.Hash, integerSetMap map[int]map[common.Hash]struct{}) {
	trxHashes, ok := integerSetMap[oldGroup]
	if !ok {
		trxHashes = make(map[common.Hash]struct{})
	}
	trxHashes[trxHash] = struct{}{}
	integerSetMap[oldGroup] = trxHashes
}
