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
	txInfos       []*txInfo
	startTrxIndex int
	statedb       *state.StateDB

	err        error
	errTxIndex int
	usedGas    uint64
	feeAmount  *big.Int
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
		feeAmount:  big.NewInt(0),
		errTxIndex: -1,
	}
}

func (e *ExecutionGroup) getTxInfos() []*txInfo {
	return e.txInfos
}

func (e *ExecutionGroup) setTxInfos(txInfos []*txInfo) {
	e.txInfos = txInfos
}

func (e *ExecutionGroup) Header() *types.Header {
	return e.header
}

func (e *ExecutionGroup) SetHeader(header *types.Header) {
	e.header = header
}

func (e *ExecutionGroup) addTxInfo(txInfo *txInfo) {
	e.txInfos = append(e.txInfos, txInfo)
}

func (e *ExecutionGroup) addTxInfos(txInfos []*txInfo) {
	e.txInfos = append(e.txInfos, txInfos...)
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

func (e *ExecutionGroup) reuseTxResults(txsToReuse []*txInfo, conflictGroups map[int]*ExecutionGroup) {
	stateObjsFromOtherGroup := make(map[int]map[common.Address]struct{})

	for gId, _ := range conflictGroups {
		stateObjsFromOtherGroup[gId] = make(map[common.Address]struct{})
	}

	for i := len(txsToReuse) - 1; i >= 0; i-- {
		txInfo := txsToReuse[i]
		txHash := txInfo.hash
		oldGroupId := txInfo.groupId

		if result := txsToReuse[i].result; result != nil {
			appendStateObjToReuse(stateObjsFromOtherGroup[oldGroupId], result.touchedAddresses)
			e.statedb.CopyTxJournalFromOtherDB(conflictGroups[oldGroupId].statedb, txHash)
			e.AddUsedGas(result.usedGas)
			e.AddFeeAmount(result.feeAmount)
		}
		txInfo.groupId = e.id
	}

	for gId, stateObjsMap := range stateObjsFromOtherGroup {
		e.statedb.CopyStateObjFromOtherDB(conflictGroups[gId].statedb, stateObjsMap)
	}
}

func appendStateObjToReuse(stateObjsToReuse map[common.Address]struct{}, touchedAddr *state.TouchedAddressObject) {
	for addr, op := range touchedAddr.AccountOp() {
		if op {
			if _, ok := stateObjsToReuse[addr]; !ok {
				stateObjsToReuse[addr] = struct{}{}
			}
		}
	}
}
