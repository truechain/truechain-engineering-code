package core

import (
	"container/list"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"sort"
	"sync"
)

var emptyCodeHash = crypto.Keccak256Hash(nil)
var associatedAddressMngr = NewAssociatedAddressMngr()

type ParallelBlock struct {
	block                *types.Block
	transactions         types.Transactions
	executionGroups      map[int]*ExecutionGroup
	associatedAddressMap map[common.Address]*state.TouchedAddressObject
	trxHashToIndexMap    map[common.Hash]int
	trxHashToMsgMap      map[common.Hash]*types.Message
	trxHashToGroupIdMap  map[common.Hash]int
	nextGroupId          int
	statedb              *state.StateDB
	config               *params.ChainConfig
	context              ChainContext
	vmConfig             vm.Config
	feeAmount            *big.Int
}

type TxWithOldGroup struct {
	txHash     common.Hash
	oldGroupId int
}

func NewParallelBlock(block *types.Block, statedb *state.StateDB, config *params.ChainConfig, bc ChainContext, cfg vm.Config, feeAmount *big.Int) *ParallelBlock {
	return &ParallelBlock{
		block:               block,
		transactions:        block.Transactions(),
		executionGroups:     make(map[int]*ExecutionGroup),
		trxHashToIndexMap:   make(map[common.Hash]int),
		trxHashToMsgMap:     make(map[common.Hash]*types.Message),
		trxHashToGroupIdMap: make(map[common.Hash]int),
		statedb:             statedb,
		config:              config,
		context:             bc,
		vmConfig:            cfg,
		feeAmount:           feeAmount,
	}
}

func (pb *ParallelBlock) group() {
	tmpExecutionGroupMap := pb.groupTransactions(pb.transactions, false)

	for _, execGroup := range tmpExecutionGroupMap {
		execGroup.SetId(pb.nextGroupId)
		execGroup.SetStatedb(pb.statedb.Copy())
		pb.executionGroups[pb.nextGroupId] = execGroup

		for _, tx := range execGroup.transactions {
			pb.trxHashToGroupIdMap[tx.Hash()] = pb.nextGroupId
		}

		pb.nextGroupId++
	}
}

func (pb *ParallelBlock) reGroupAndRevert(conflictGroups []map[int]struct{}, conflictTxs map[common.Hash]struct{}) {
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
			var (
				txsToReuse []TxWithOldGroup
				conflict   = false
			)
			for index, trx := range group.transactions {
				txHash := trx.Hash()
				oldGroupId := pb.trxHashToGroupIdMap[txHash]
				if !conflict {
					if _, ok := conflictTxs[txHash]; ok {
						conflict = true
						group.SetStartTrxPos(index)
					} else {
						txsToReuse = append(txsToReuse, TxWithOldGroup{txHash, oldGroupId})
					}
				}

				if conflict {
					// revert transactions which will be re-executed in reversed order
					conflictGroups[oldGroupId].statedb.RevertTrxResultByHash(txHash)
				}
				pb.trxHashToGroupIdMap[txHash] = pb.nextGroupId
			}

			// copy transaction results and state changes from old group which can be reused
			group.reuseTxResults(txsToReuse, conflictGroups)
			group.SetId(pb.nextGroupId)
			pb.executionGroups[pb.nextGroupId] = group
			pb.nextGroupId++
		}
	}
}

func (pb *ParallelBlock) groupTransactions(transactions types.Transactions, regroup bool) map[int]*ExecutionGroup {
	executionGroupMap := make(map[int]*ExecutionGroup)
	writtenAccounts := make(map[common.Address]struct{})
	writtenStorages := make(map[state.StorageAddress]struct{})
	groupWrittenAccountMap := make(map[int]map[common.Address]struct{})
	groupWrittenStorageMap := make(map[int]map[state.StorageAddress]struct{})
	groupId := 0
	transactions = sortTrxByIndex(transactions, pb.trxHashToIndexMap)

	for _, tx := range transactions {
		groupsToMerge := make(map[int]struct{})
		groupWrittenAccount := make(map[common.Address]struct{})
		groupWrittenStorage := make(map[state.StorageAddress]struct{})
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

func (pb *ParallelBlock) getTrxTouchedAddress(hash common.Hash, regroup bool) *state.TouchedAddressObject {
	if regroup {
		if result, ok := pb.executionGroups[pb.trxHashToGroupIdMap[hash]].trxHashToResultMap[hash]; ok {
			return result.touchedAddresses
		}
	}

	touchedAddressObj := state.NewTouchedAddressObject()
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
	storageGroupIdsMap := make(map[state.StorageAddress]map[int]struct{})

	for _, trx := range pb.transactions {
		var touchedAddressObj *state.TouchedAddressObject = nil
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

func (pb *ParallelBlock) checkGas(txIndex int, receipts types.Receipts) error {
	gp := new(GasPool).AddGas(pb.block.GasLimit())
	txs := pb.block.Transactions()

	for i := 0; i < txIndex; i++ {
		if err := gp.SubGas(txs[i].Gas()); err != nil {
			return err
		}

		gp.AddGas(txs[i].Gas() - receipts[i].GasUsed)
	}

	return nil
}

func overlapped(set0 map[int]struct{}, set1 map[int]struct{}) bool {
	for k, _ := range set0 {
		if _, ok := set1[k]; ok {
			return true
		}
	}
	return false
}

func (pb *ParallelBlock) executeGroup(group *ExecutionGroup, wg *sync.WaitGroup) {
	defer wg.Done()

	var (
		feeAmount = big.NewInt(0)
		gp        = new(GasPool).AddGas(pb.block.GasLimit())
		statedb   = group.statedb
	)

	// Iterate over and process the individual transactions
	for i := group.startTrxIndex; i < group.transactions.Len(); i++ {
		tx := group.transactions[i]
		txHash := tx.Hash()
		ti := pb.trxHashToIndexMap[txHash]
		statedb.Prepare(txHash, pb.block.Hash(), ti)
		receipt, trxUsedGas, err := ApplyTransaction(pb.config, pb.context, gp, statedb, pb.block.Header(), tx, &group.usedGas, feeAmount, pb.vmConfig)
		if err != nil {
			group.err = err
			group.trxHashToResultMap[txHash] = NewTrxResult(nil, nil, statedb.GetTouchedAddress(), trxUsedGas, feeAmount)
			group.startTrxIndex = -1
			return
		}
		group.trxHashToResultMap[txHash] = NewTrxResult(receipt, receipt.Logs, statedb.GetTouchedAddress(), trxUsedGas, feeAmount)
	}

	group.feeAmount.Add(feeAmount, group.feeAmount)
	group.startTrxIndex = -1
}

func (pb *ParallelBlock) executeInParallel() {
	wg := sync.WaitGroup{}

	for _, group := range pb.executionGroups {
		if group.startTrxIndex != -1 {
			wg.Add(1)
			go pb.executeGroup(group, &wg)
		}
	}

	wg.Wait()
}

func (pb *ParallelBlock) prepare() error {
	var contractAddrs []common.Address

	for ti, trx := range pb.block.Transactions() {
		msg, err := trx.AsMessage(types.MakeSigner(pb.config, pb.block.Header().Number))
		if err != nil {
			return err
		}
		pb.trxHashToMsgMap[trx.Hash()] = &msg
		pb.trxHashToIndexMap[trx.Hash()] = ti

		if to := trx.To(); to != nil {
			codeHash := pb.statedb.GetCodeHash(*to)
			if codeHash != (common.Hash{}) && codeHash != emptyCodeHash {
				contractAddrs = append(contractAddrs, *to)
			}
		}
	}

	pb.associatedAddressMap = associatedAddressMngr.LoadAssociatedAddresses(contractAddrs)

	return nil
}

func (pb *ParallelBlock) collectResult() (types.Receipts, []*types.Log, uint64, error, int) {
	var (
		err             error
		txIndex         = -1
		receipts        = make(types.Receipts, pb.transactions.Len())
		usedGas         = uint64(0)
		allLogs         []*types.Log
		associatedAddrs = make(map[common.Address]*state.TouchedAddressObject)
	)

	for _, group := range pb.executionGroups {
		if group.err != nil && (group.errTxIndex < txIndex || txIndex == -1) {
			err = group.err
			txIndex = group.errTxIndex
		}
		usedGas += group.usedGas
		pb.feeAmount.Add(group.feeAmount, pb.feeAmount)

		stateObjsToReuse := make(map[common.Address]*state.StateObjectToReuse)

		for _, tx := range group.transactions {
			txHash := tx.Hash()

			if result, ok := group.trxHashToResultMap[txHash]; ok {
				appendStateObjToReuse(stateObjsToReuse, result.touchedAddresses)
				receipts[pb.trxHashToIndexMap[txHash]] = group.trxHashToResultMap[txHash].receipt

				// collect associated address of contract
				if to := tx.To(); to != nil {
					touchedAddr := group.trxHashToResultMap[txHash].touchedAddresses
					touchedAddr.RemoveAccount(pb.trxHashToMsgMap[txHash].From())

					if len(touchedAddr.AccountOp()) > 1 && len(touchedAddr.StorageOp()) > 0 {
						associatedAddrs[*to] = touchedAddr
					}
				}
			}
		}

		// merge statedb changes
		pb.statedb.CopyStateObjFromOtherDB(group.statedb, stateObjsToReuse)
	}
	pb.statedb.Finalise(true)

	associatedAddressMngr.UpdateAssociatedAddresses(associatedAddrs)

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
		err2 := pb.checkGas(ti, receipts)
		if err2 != nil {
			return nil, nil, 0, err2
		}
		return nil, nil, 0, err
	}

	err = pb.checkGas(pb.transactions.Len()-1, receipts)
	if err != nil {
		return nil, nil, 0, err
	}

	return receipts, allLogs, usedGas, nil
}

func sortTrxByIndex(txs types.Transactions, trxHashToIndexMap map[common.Hash]int) types.Transactions {
	sort.Slice(txs, func(i, j int) bool {
		return trxHashToIndexMap[txs[i].Hash()] < trxHashToIndexMap[txs[j].Hash()]
	})
	return txs
}
