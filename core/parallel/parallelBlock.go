package parallel

import (
	"container/list"
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"sort"
	"sync"
)

var associatedAddressMngr = NewAssociatedAddressMngr()

type ParallelBlock struct {
	block                *types.Block
	transactions         types.Transactions
	executionGroups      map[int]*ExecutionGroup
	associatedAddressMap map[common.Address]*TouchedAddressObject
	trxHashToIndexMap    map[common.Hash]int
	trxHashToMsgMap      map[common.Hash]*types.Message
	trxHashToGroupIdMap  map[common.Hash]int
	nextGroupId          int
	statedb              *state.StateDB
	config               *params.ChainConfig
	context              core.ChainContext
	vmConfig             vm.Config
}

func NewParallelBlock(block *types.Block, statedb *state.StateDB, config *params.ChainConfig, bc core.ChainContext, cfg vm.Config) *ParallelBlock {
	return &ParallelBlock{block: block, transactions: block.Transactions(), statedb: statedb, config: config, vmConfig: cfg}
}

func (pb *ParallelBlock) group() {
	//pb.newGroups = make(map[int]*ExecutionGroup)
	tmpExecutionGroupMap := pb.groupTransactions(pb.transactions, false)

	for _, execGroup := range tmpExecutionGroupMap {
		execGroup.setId(pb.nextGroupId)
		pb.executionGroups[pb.nextGroupId] = execGroup

		for _, tx := range execGroup.transactions {
			pb.trxHashToGroupIdMap[tx.Hash()] = pb.nextGroupId
		}

		pb.nextGroupId++
	}
}

func (pb *ParallelBlock) reGroupAndRevert(conflictGroups []map[int]struct{}, conflictTxs map[common.Hash]struct{}) {
	var txsToRevert []int

	for _, conflictGroupIds := range conflictGroups {
		var txs types.Transactions
		conflictGroups := make(map[int]*ExecutionGroup)

		for groupId := range conflictGroupIds {
			txs = append(txs, pb.executionGroups[groupId].Transactions()...)
			conflictGroups[groupId] = pb.executionGroups[groupId]
			delete(pb.executionGroups, groupId)
		}

		tmpExecGroupMap := pb.groupTransactions(txs, true)

		for _, group := range tmpExecGroupMap {
			conflict := false
			for index, trx := range group.transactions {
				txHash := trx.Hash()
				oldGroupId := pb.trxHashToGroupIdMap[txHash]
				if !conflict {
					if _, ok := conflictTxs[txHash]; ok {
						conflict = true
						group.setStartTrxPos(index)
					} else {
						group.trxHashToResultMap[txHash] = conflictGroups[oldGroupId].trxHashToResultMap[txHash]
					}
				} else {
					txsToRevert = append(txsToRevert, pb.trxHashToIndexMap[txHash])
				}
				pb.trxHashToGroupIdMap[txHash] = pb.nextGroupId
			}

			group.setId(pb.nextGroupId)
			pb.executionGroups[pb.nextGroupId] = group
			pb.nextGroupId++
		}
	}

	sort.Ints(txsToRevert)

	for i := len(txsToRevert) - 1; i >= 0; i-- {
		pb.statedb.RevertTrxResultByIndex(i)
	}
}

func (pb *ParallelBlock) groupTransactions(transactions types.Transactions, regroup bool) map[int]*ExecutionGroup {
	executionGroupMap := make(map[int]*ExecutionGroup)
	writtenAccounts := make(map[common.Address]struct{})
	writtenStorages := make(map[StorageAddress]struct{})
	groupWrittenAccountMap := make(map[int]map[common.Address]struct{})
	groupWrittenStorageMap := make(map[int]map[StorageAddress]struct{})
	groupId := 0
	transactions = sortTrxByIndex(transactions, pb.trxHashToIndexMap)

	for _, tx := range transactions {
		groupsToMerge := make(map[int]struct{})
		groupWrittenAccount := make(map[common.Address]struct{})
		groupWrittenStorage := make(map[StorageAddress]struct{})
		trxTouchedAddress := pb.getTrxTouchedAddress(tx.Hash(), regroup)

		for addr, op := range trxTouchedAddress.AccountOp() {
			if _, ok := writtenAccounts[addr]; ok {
				for gId, addrs := range groupWrittenAccountMap {
					if _, ok := addrs[addr]; ok {
						groupsToMerge[gId] = struct{}{}
					}
				}
			} else if op {
				writtenAccounts[addr] = struct{}{}
			}
			if op {
				groupWrittenAccount[addr] = struct{}{}
			}
		}

		for storage, op := range trxTouchedAddress.StorageOp() {
			if _, ok := writtenStorages[storage]; ok {
				for gId, storages := range groupWrittenStorageMap {
					if _, ok := storages[storage]; ok {
						groupsToMerge[gId] = struct{}{}
					}
				}
			} else if op {
				writtenStorages[storage] = struct{}{}
			}
			if op {
				groupWrittenStorage[storage] = struct{}{}
			}
		}

		tmpExecutionGroup := NewExecutionGroup()
		tmpExecutionGroup.AddTransaction(tx)
		tmpExecutionGroup.SetHeader(pb.block.Header())
		for gId := range groupsToMerge {
			tmpExecutionGroup.AddTransactions(executionGroupMap[gId].Transactions())
			delete(executionGroupMap, gId)
			for k, v := range groupWrittenAccountMap[gId] {
				groupWrittenAccount[k] = v
			}
			delete(groupWrittenAccountMap, gId)
			for k, v := range groupWrittenStorageMap[gId] {
				groupWrittenStorage[k] = v
			}
			delete(groupWrittenStorageMap, gId)
		}
		groupId++
		groupWrittenAccountMap[groupId] = groupWrittenAccount
		groupWrittenStorageMap[groupId] = groupWrittenStorage
		executionGroupMap[groupId] = tmpExecutionGroup
	}

	for _, group := range executionGroupMap {
		group.transactions = sortTrxByIndex(group.transactions, pb.trxHashToGroupIdMap)
	}

	return executionGroupMap
}

func (pb *ParallelBlock) getTrxTouchedAddress(hash common.Hash, regroup bool) *TouchedAddressObject {
	if regroup {
		if result, ok := pb.executionGroups[pb.trxHashToGroupIdMap[hash]].trxHashToResultMap[hash]; ok {
			return result.touchedAddresses
		}
	}

	touchedAddressObj := NewTouchedAddressObject()
	msg := pb.trxHashToMsgMap[hash]

	if msg.From() != params.EmptyAddress {
		touchedAddressObj.AddAccountOp(msg.Payment(), true)
	}
	touchedAddressObj.AddAccountOp(msg.From(), true)

	if to := msg.To(); to != nil {
		if associatedAddressObj, ok := pb.associatedAddressMap[*to]; ok {
			touchedAddressObj.Merge(associatedAddressObj)
		} else {
			if msg.Value().Sign() != 0 {
				touchedAddressObj.AddAccountOp(*to, true)
			} else {
				touchedAddressObj.AddAccountOp(*to, false)
			}
		}
	}

	return touchedAddressObj
}

func (pb *ParallelBlock) checkConflict() ([]map[int]struct{}, map[common.Hash]struct{}) {
	var conflictGroups []map[int]struct{}
	conflictTxs := make(map[common.Hash]struct{})
	addrGroupIdsMap := make(map[common.Address]map[int]struct{})
	storageGroupIdsMap := make(map[StorageAddress]map[int]struct{})

	for _, trx := range pb.transactions {
		var touchedAddressObj *TouchedAddressObject = nil
		trxHash := trx.Hash()
		curTrxGroup := pb.trxHashToGroupIdMap[trxHash]
		touchedAddressObj = pb.getTrxTouchedAddress(trxHash, true)

		for addr, op := range touchedAddressObj.AccountOp() {
			if groupIds, ok := addrGroupIdsMap[addr]; ok {
				if _, ok := groupIds[curTrxGroup]; !ok {
					groupIds[curTrxGroup] = struct{}{}
					conflictTxs[trxHash] = struct{}{}
				}
			} else if op {
				groupSet := make(map[int]struct{})
				groupSet[curTrxGroup] = struct{}{}
				addrGroupIdsMap[addr] = groupSet
			}
		}
		for storage, op := range touchedAddressObj.StorageOp() {
			if groupIds, ok := storageGroupIdsMap[storage]; ok {
				if _, ok := groupIds[curTrxGroup]; !ok {
					groupIds[curTrxGroup] = struct{}{}
					conflictTxs[trxHash] = struct{}{}
				}
			} else if op {
				groupSet := make(map[int]struct{})
				groupSet[curTrxGroup] = struct{}{}
				storageGroupIdsMap[storage] = groupSet
			}
		}
	}

	groupsList := list.New()
	for _, groups := range addrGroupIdsMap {
		groupsList.PushBack(groups)
	}
	for _, groups := range storageGroupIdsMap {
		groupsList.PushBack(groups)
	}
	for i := groupsList.Front(); i != nil; i = i.Next() {
		groups := i.Value.(map[int]struct{})
		if len(groups) <= 1 {
			continue
		}

		for i := len(conflictGroups) - 1; i >= 0; i-- {
			conflictGroupId := conflictGroups[i]
			if overlapped(conflictGroupId, groups) {
				for k, _ := range conflictGroupId {
					groups[k] = struct{}{}
				}
				conflictGroups = append(conflictGroups[:i], conflictGroups[i+1:]...)
			}
		}

		conflictGroups = append(conflictGroups, groups)
	}

	return conflictGroups, conflictTxs
}

func (pb *ParallelBlock) checkGas(txIndex int) (error, int) {
	// TODO: check gas after all transactions are executed
	return nil, -1
}

func overlapped(set0 map[int]struct{}, set1 map[int]struct{}) bool {
	for k, _ := range set0 {
		if _, ok := set1[k]; ok {
			return true
		}
	}
	return false
}

func (pb *ParallelBlock) executeGroup(group *ExecutionGroup, wg sync.WaitGroup) {
	defer wg.Done()

	var (
		usedGas   = new(uint64)
		feeAmount = big.NewInt(0)
		gp        = new(core.GasPool).AddGas(pb.block.GasLimit())
		statedb   = pb.statedb.Copy()
	)

	// Iterate over and process the individual transactions
	for i := group.startTrxIndex; i < group.transactions.Len(); i++ {
		tx := group.transactions[i]
		txHash := tx.Hash()
		ti := pb.trxHashToIndexMap[txHash]
		statedb.Prepare(txHash, pb.block.Hash(), ti)
		receipt, trxUsedGas, err := core.ApplyTransaction(pb.config, pb.context, gp, statedb, pb.block.Header(), tx, usedGas, feeAmount, pb.vmConfig)
		if err != nil {
			group.err = err
			group.trxHashToResultMap[txHash] = NewTrxResult(nil, nil, statedb.GetTouchedAddress(), trxUsedGas)
			return
		}
		group.trxHashToResultMap[txHash] = NewTrxResult(receipt, receipt.Logs, statedb.GetTouchedAddress(), trxUsedGas)
	}
	group.usedGas = *usedGas
}

func (pb *ParallelBlock) executeInParallel() {
	wg := sync.WaitGroup{}

	for _, group := range pb.executionGroups {
		if !group.finished {
			wg.Add(1)
			go pb.executeGroup(group, wg)
		}
	}

	wg.Wait()
}

func (pb *ParallelBlock) prepare() error {
	var toAddresses []common.Address

	for ti, trx := range pb.block.Transactions() {
		msg, err := trx.AsMessage(types.MakeSigner(pb.config, pb.block.Header().Number))
		if err != nil {
			return err
		}
		pb.trxHashToMsgMap[trx.Hash()] = &msg
		pb.trxHashToIndexMap[trx.Hash()] = ti

		if to := trx.To(); to != nil {
			toAddresses = append(toAddresses, *to)
		}
	}

	pb.associatedAddressMap = associatedAddressMngr.LoadAssociatedAddresses(toAddresses)

	return nil
}

func (pb *ParallelBlock) collectResult() (types.Receipts, []*types.Log, uint64, error, int) {
	var (
		err      error
		txIndex  = -1
		receipts = make(types.Receipts, pb.transactions.Len())
		usedGas  = uint64(0)
		allLogs  []*types.Log
	)
	for _, group := range pb.executionGroups {
		if group.err != nil && (group.errTxIndex < txIndex || txIndex == -1) {
			err = group.err
			txIndex = group.errTxIndex
		}
		usedGas += group.usedGas

		for _, tx := range group.transactions {
			txHash := tx.Hash()
			receipts[pb.trxHashToIndexMap[txHash]] = group.trxHashToResultMap[txHash].receipt
		}
	}

	for _, receipt := range receipts {
		allLogs = append(allLogs, receipt.Logs...)
	}

	return receipts, allLogs, usedGas, err, txIndex
}

func (pb *ParallelBlock) Process() (types.Receipts, []*types.Log, uint64, error) {
	if err := pb.prepare(); err != nil {
		return nil, nil, 0, err
	}

	pb.group()

	for {
		pb.executeInParallel()

		if conflictGroups, conflictTxs := pb.checkConflict(); len(conflictGroups) != 0 {
			pb.reGroupAndRevert(conflictGroups, conflictTxs)
		} else {
			break
		}
	}

	receipts, allLogs, usedGas, err, ti := pb.collectResult()
	if err != nil {
		err2, ti2 := pb.checkGas(ti)

		if err2 != nil {
			pb.statedb.RevertTrxResultsBetween(ti2, pb.transactions.Len())
			return nil, nil, 0, err2
		}

		pb.statedb.RevertTrxResultsBetween(ti+1, pb.transactions.Len())
		return nil, nil, 0, err
	} else {
		err2, ti2 := pb.checkGas(pb.transactions.Len() - 1)

		if err2 != nil {
			pb.statedb.RevertTrxResultsBetween(ti2, pb.transactions.Len())
			return nil, nil, 0, err2
		}
	}

	return receipts, allLogs, usedGas, nil
}

func sortTrxByIndex(txs types.Transactions, trxHashToIndexMap map[common.Hash]int) types.Transactions {
	return nil
}
