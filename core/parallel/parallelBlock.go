package parallel

import (
	"container/list"
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
)

type ParallelBlock struct {
	header                     *types.Header
	transactions               types.Transactions
	executionGroups            map[int]*ExecutionGroup
	associatedAddressMap       map[common.Address]*TouchedAddressObject
	conflictIdsGroups          []map[int]struct{}
	conflictTrxPos             map[int]bool
	newGroups                  map[int]*ExecutionGroup
	trxHashToTouchedAddressMap map[common.Hash]*TouchedAddressObject
	trxHashToIndexMap          map[common.Hash]int
	trxHashToMsgMap            map[common.Hash]*types.Message
	trxHashToGroupIdMap        map[common.Hash]int
	nextGroupId                int
	statedb                    *state.StateDB
}

func NewParallelBlock(block *types.Block, statedb *state.StateDB) *ParallelBlock {
	return &ParallelBlock{header: block.Header(), transactions: block.Transactions(), statedb: statedb}
}

func (pb *ParallelBlock) Group() map[int]*ExecutionGroup {
	pb.newGroups = make(map[int]*ExecutionGroup)
	tmpExecutionGroupMap := pb.groupTransactions(pb.transactions, false)

	for _, execGroup := range tmpExecutionGroupMap {
		execGroup.sortTrxByIndex(pb.trxHashToIndexMap)
		pb.nextGroupId++
		execGroup.setId(pb.nextGroupId)
		pb.executionGroups[pb.nextGroupId] = execGroup
		pb.newGroups[pb.nextGroupId] = execGroup
	}

	return pb.newGroups
}

func (pb *ParallelBlock) ReGroup() {
	pb.newGroups = make(map[int]*ExecutionGroup)

	for _, conflictGroups := range pb.conflictIdsGroups {
		executionGroup := NewExecutionGroup()
		originTrxHashToGroupMap := make(map[common.Hash]int)
		for groupId, _ := range conflictGroups {
			executionGroup.AddTransactions(pb.executionGroups[groupId].Transactions())

			for _, trx := range pb.executionGroups[groupId].Transactions() {
				originTrxHashToGroupMap[trx.Hash()] = groupId
			}
		}
		executionGroup.sortTrxByIndex(pb.trxHashToIndexMap)

		tmpExecGroupMap := pb.groupTransactions(executionGroup.Transactions(), true)

		for _, group := range tmpExecGroupMap {
			conflict := false
			group.sortTrxByIndex(pb.trxHashToIndexMap)
			for _, trx := range group.transactions {
				trxHash := trx.Hash()
				oldGroupId := originTrxHashToGroupMap[trxHash]
				if !conflict {
					trxIndex := pb.trxHashToIndexMap[trxHash]
					if _, ok := pb.conflictTrxPos[trxIndex]; ok {
						conflict = true
						group.setStartTrxPos(trxHash, trxIndex)
					} else {
						group.addTrxHashToGetPartResult(oldGroupId, trxHash)
					}
				}
				group.addTrxToRollbackInOtherGroup(oldGroupId, trxHash)
			}

			if conflict {
				pb.nextGroupId++
				group.setId(pb.nextGroupId)
				pb.executionGroups[pb.nextGroupId] = group
				pb.newGroups[pb.nextGroupId] = group
			}
		}
	}
}

func (pb *ParallelBlock) groupTransactions(transactions types.Transactions, regroup bool) map[int]*ExecutionGroup {
	executionGroupMap := make(map[int]*ExecutionGroup)
	accountWrited := make(map[common.Address]bool)
	storageWrited := make(map[StorageAddress]bool)
	groupTouchedAccountMap := make(map[int]map[common.Address]bool)
	groupTouchedStorageMap := make(map[int]map[StorageAddress]bool)
	groupId := 0

	for _, trx := range transactions {
		groupsToMerge := make(map[int]bool)
		groupTouchedAccount := make(map[common.Address]bool)
		groupTouchedStorage := make(map[StorageAddress]bool)
		trxTouchedAddres := pb.getTrxTouchedAddress(trx.Hash(), regroup)

		for addr, op := range trxTouchedAddres.AccountOp() {
			if _, ok := accountWrited[addr]; ok {
				for gId, addrs := range groupTouchedAccountMap {
					if _, ok := groupsToMerge[gId]; ok {
						continue
					}

					if _, ok := addrs[addr]; ok {
						groupsToMerge[gId] = true
					}
				}
			} else if op {
				accountWrited[addr] = true
			}

			if op {
				groupTouchedAccount[addr] = true
			}
		}

		for storage, op := range trxTouchedAddres.StorageOp() {
			if _, ok := storageWrited[storage]; ok {
				for gId, storages := range groupTouchedStorageMap {
					if _, ok := groupsToMerge[gId]; ok {
						continue
					}

					if _, ok := storages[storage]; ok {
						groupsToMerge[gId] = true
					}
				}
			} else if op {
				storageWrited[storage] = true
			}

			if op {
				groupTouchedStorage[storage] = true
			}
		}

		tmpExecutionGroup := NewExecutionGroup()
		tmpExecutionGroup.AddTransaction(trx)
		tmpExecutionGroup.SetHeader(pb.header)
		for gId := range groupsToMerge {
			tmpExecutionGroup.AddTransactions(executionGroupMap[gId].Transactions())
			delete(executionGroupMap, gId)
			for k, v := range groupTouchedAccountMap[gId] {
				groupTouchedAccount[k] = v
			}
			delete(groupTouchedAccountMap, gId)
			for k, v := range groupTouchedStorageMap[gId] {
				groupTouchedStorage[k] = v
			}
			delete(groupTouchedStorageMap, gId)
		}

		groupId++
		groupTouchedAccountMap[groupId] = groupTouchedAccount
		groupTouchedStorageMap[groupId] = groupTouchedStorage
		executionGroupMap[groupId] = tmpExecutionGroup
	}

	return executionGroupMap
}

func (pb *ParallelBlock) getTrxTouchedAddress(hash common.Hash, regroup bool) *TouchedAddressObject {
	var touchedAddressObj *TouchedAddressObject = nil
	msg := pb.trxHashToMsgMap[hash]

	if regroup {
		touchedAddressObj = pb.trxHashToTouchedAddressMap[hash]
	} else {
		touchedAddressObj = NewTouchedAddressObject()
		touchedAddressObj.AddAccountOp(msg.From(), true)
		touchedAddressObj.AddAccountOp(msg.Payment(), true)

		if msg.To() != nil {
			if associatedAddressObj, ok := pb.associatedAddressMap[*msg.To()]; ok {
				touchedAddressObj.Merge(associatedAddressObj)
			} else {
				touchedAddressObj.AddAccountOp(*msg.To(), true)
			}
		}
	}

	return touchedAddressObj
}

func (pb *ParallelBlock) CheckConflict() bool {
	pb.conflictIdsGroups = make([]map[int]struct{}, 10)
	addrGroupIdsMap := make(map[common.Address]map[int]struct{})
	storageGroupIdsMap := make(map[StorageAddress]map[int]struct{})

	for _, trx := range pb.transactions {
		trxHash := trx.Hash()
		touchedAddressObj := pb.trxHashToTouchedAddressMap[trxHash]

		for addr, op := range touchedAddressObj.AccountOp() {
			curTrxGroup := pb.trxHashToGroupIdMap[trxHash]

			if groupIds, ok := addrGroupIdsMap[addr]; ok {
				if _, ok := groupIds[curTrxGroup]; !ok {
					groupIds[curTrxGroup] = struct{}{}
					pb.conflictTrxPos[pb.trxHashToIndexMap[trxHash]] = true
				}
			} else if op {
				groupSet := make(map[int]struct{})
				groupSet[curTrxGroup] = struct{}{}
				addrGroupIdsMap[addr] = groupSet
			}
		}
		for storage, op := range touchedAddressObj.StorageOp() {
			curTrxGroup := pb.trxHashToGroupIdMap[trxHash]

			if groupIds, ok := storageGroupIdsMap[storage]; ok {
				if _, ok := groupIds[curTrxGroup]; !ok {
					groupIds[curTrxGroup] = struct{}{}
					pb.conflictTrxPos[pb.trxHashToIndexMap[trxHash]] = true
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

		for i := len(pb.conflictIdsGroups) - 1; i >= 0; i-- {
			conflictGroupId := pb.conflictIdsGroups[i]
			if setsOverlapped(conflictGroupId, groups) {
				for k, _ := range conflictGroupId {
					groups[k] = struct{}{}
				}
				pb.conflictIdsGroups = append(pb.conflictIdsGroups[:i], pb.conflictIdsGroups[i+1:]...)
			}
		}

		pb.conflictIdsGroups = append(pb.conflictIdsGroups, groups)
	}

	return len(pb.conflictIdsGroups) != 0
}

func setsOverlapped(set0 map[int]struct{}, set1 map[int]struct{}) bool {
	for k, _ := range set0 {
		if _, ok := set1[k]; ok {
			return true
		}
	}
	return false
}

func (pb *ParallelBlock) Process() {

}

func (pb *ParallelBlock) Rollback() {

}
