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
	"sync"
)

type ParallelBlock struct {
	block                      *types.Block
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
	config                     *params.ChainConfig
	context                    core.ChainContext
	vmConfig                   vm.Config
}

func NewParallelBlock(block *types.Block, statedb *state.StateDB, config *params.ChainConfig, bc core.ChainContext, cfg vm.Config) *ParallelBlock {
	return &ParallelBlock{block: block, transactions: block.Transactions(), statedb: statedb, config: config, vmConfig: cfg}
}

func (pb *ParallelBlock) group() map[int]*ExecutionGroup {
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

func (pb *ParallelBlock) reGroup() {
	pb.newGroups = make(map[int]*ExecutionGroup)

	for _, conflictGroupIds := range pb.conflictIdsGroups {
		executionGroup := NewExecutionGroup()
		originTrxHashToGroupMap := make(map[common.Hash]int)
		conflictGroups := make(map[int]*ExecutionGroup)

		for groupId, _ := range conflictGroupIds {
			executionGroup.AddTransactions(pb.executionGroups[groupId].Transactions())

			for _, trx := range pb.executionGroups[groupId].Transactions() {
				originTrxHashToGroupMap[trx.Hash()] = groupId
			}

			conflictGroups[groupId] = pb.executionGroups[groupId]
			delete(pb.executionGroups, groupId)
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
						group.mergeTrxResultFromOtherGroup(conflictGroups[oldGroupId].result, trxHash)
					}
				}
			}

			pb.nextGroupId++
			group.setId(pb.nextGroupId)
			pb.executionGroups[pb.nextGroupId] = group

			if conflict {
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
		trxTouchedAddress := pb.getTrxTouchedAddress(trx.Hash(), regroup)

		for addr, op := range trxTouchedAddress.AccountOp() {
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

		for storage, op := range trxTouchedAddress.StorageOp() {
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
		tmpExecutionGroup.SetHeader(pb.block.Header())
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
	pb.conflictIdsGroups = pb.conflictIdsGroups[0:0]
	addrGroupIdsMap := make(map[common.Address]map[int]struct{})
	storageGroupIdsMap := make(map[StorageAddress]map[int]struct{})

	for _, trx := range pb.transactions {
		trxHash := trx.Hash()
		touchedAddressObj, ok := pb.trxHashToTouchedAddressMap[trxHash]
		if !ok {
			// The transaction is not executed because of error in previous transactions
			touchedAddressObj = pb.getTrxTouchedAddress(trxHash, false)
		}

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

func (pb *ParallelBlock) CheckGas() bool {
	// TODO: check gas after all transactions are executed
	return true
}

func setsOverlapped(set0 map[int]struct{}, set1 map[int]struct{}) bool {
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
	group.result = NewGroupResult()

	// Iterate over and process the individual transactions
	for _, tx := range group.Transactions() {
		ti := pb.trxHashToIndexMap[tx.Hash()]
		statedb.Prepare(tx.Hash(), pb.block.Hash(), ti)
		receipt, trxUsedGas, err := core.ApplyTransaction(pb.config, pb.context, gp, statedb, pb.block.Header(), tx, usedGas, feeAmount, pb.vmConfig)
		group.result.usedGas = *usedGas
		if err != nil {
			group.result.err = err
			group.result.trxIndexToResult[ti] = NewTrxResult(nil, nil, statedb.GetTouchedAddress(), trxUsedGas)
			return
		}
		group.result.trxIndexToResult[ti] = NewTrxResult(receipt, receipt.Logs, statedb.GetTouchedAddress(), trxUsedGas)
	}
}

func (pb *ParallelBlock) executeInParallel() (types.Receipts, []*types.Log, uint64, error) {
	receipts := make([]*types.Receipt, pb.transactions.Len())
	var allLogs []*types.Log
	usedGas := uint64(0)
	wg := sync.WaitGroup{}

	for _, group := range pb.newGroups {
		wg.Add(1)
		go pb.executeGroup(group, wg)
	}

	wg.Wait()

	for _, group := range pb.newGroups {
		if err := group.result.err; err != nil {
			return nil, nil, 0, err
		}

		for i, trxResult := range group.result.trxIndexToResult {
			receipts[i] = trxResult.receipt
		}
		usedGas += group.result.usedGas
	}

	return receipts, allLogs, 0, nil
}

func (pb *ParallelBlock) convertTrxToMsg() error {
	for ti, trx := range pb.block.Transactions() {
		msg, err := trx.AsMessage(types.MakeSigner(pb.config, pb.block.Header().Number))
		if err != nil {
			return err
		}
		pb.trxHashToMsgMap[trx.Hash()] = &msg
		pb.trxHashToIndexMap[trx.Hash()] = ti
	}

	return nil
}

func (pb *ParallelBlock) Process() (types.Receipts, []*types.Log, uint64, error) {
	if err := pb.convertTrxToMsg(); err != nil {
		return nil, nil, 0, nil
	}

	pb.group()

	for {
		pb.executeInParallel()

		if pb.CheckConflict() {
			pb.reGroup()
		} else {
			break
		}
	}

	// TODO
	return nil, nil, 0, nil
}
