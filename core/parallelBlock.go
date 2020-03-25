package core

import (
	"container/list"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"

	"sort"
	"sync"
	"time"
)

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

type grouper struct {
	executionGroupMap      map[int]*ExecutionGroup
	groupWrittenAccountMap map[int]map[common.Address]struct{}
	groupWrittenStorageMap map[int]map[state.StorageAddress]struct{}
	addrToGroupMap         map[common.Address]int
	storageToGroupMap      map[state.StorageAddress]int
	groupId                int
	regroup                bool
}

type TxHashGroupIdPair struct {
	txHash     common.Hash
	oldGroupId int
}

type txGroupPair struct {
	tx    *types.Transaction
	group *ExecutionGroup
}

type addressRlpDataPair struct {
	address common.Address
	rlpData []byte
}

func newGrouper(regroup bool) *grouper {
	return &grouper{
		executionGroupMap:      make(map[int]*ExecutionGroup),
		groupWrittenAccountMap: make(map[int]map[common.Address]struct{}),
		groupWrittenStorageMap: make(map[int]map[state.StorageAddress]struct{}),
		addrToGroupMap:         make(map[common.Address]int),
		storageToGroupMap:      make(map[state.StorageAddress]int),
		groupId:                0,
		regroup:                regroup,
	}
}

func (gp *grouper) groupNewTransaction(tx *types.Transaction, pb *ParallelBlock) {
	d := int64(0)

	t0 := time.Now()
	groupsToMerge := make(map[int]struct{})
	groupWrittenAccount := make(map[common.Address]struct{})
	groupWrittenStorage := make(map[state.StorageAddress]struct{})
	firstGroup := true
	var tmpExecutionGroup *ExecutionGroup
	trxTouchedAddress := pb.getTrxTouchedAddress(tx.Hash(), gp.regroup)

	d0 := time.Since(t0)
	t1 := time.Now()

	for addr, op := range trxTouchedAddress.AccountOp() {
		if gId, ok := gp.addrToGroupMap[addr]; ok {
			groupsToMerge[gId] = struct{}{}
		}
		if op {
			groupWrittenAccount[addr] = struct{}{}
			gp.addrToGroupMap[addr] = gp.groupId
		}
	}

	d1 := time.Since(t1)
	t2 := time.Now()

	for storage, op := range trxTouchedAddress.StorageOp() {
		if gId, ok := gp.storageToGroupMap[storage]; ok {
			groupsToMerge[gId] = struct{}{}
		}
		if op {
			groupWrittenStorage[storage] = struct{}{}
			gp.storageToGroupMap[storage] = gp.groupId
		}
	}

	d2 := time.Since(t2)
	t3 := time.Now()

	if len(groupsToMerge) == 0 {
		tmpExecutionGroup = NewExecutionGroup()
		tmpExecutionGroup.AddTransaction(tx)
		tmpExecutionGroup.SetHeader(pb.block.Header())
		tmpExecutionGroup.SetId(gp.groupId)
	}

	d3 := time.Since(t3)
	t4 := time.Now()
	for gId := range groupsToMerge {
		if firstGroup {
			tmpExecutionGroup = gp.executionGroupMap[gId]
			tmpExecutionGroup.AddTransaction(tx)
			tmpExecutionGroup.SetId(gp.groupId)

			for k := range gp.groupWrittenAccountMap[gId] {
				gp.addrToGroupMap[k] = gp.groupId
			}

			for k := range gp.groupWrittenStorageMap[gId] {
				gp.storageToGroupMap[k] = gp.groupId
			}

			for k, v := range groupWrittenAccount {
				gp.groupWrittenAccountMap[gId][k] = v
			}
			groupWrittenAccount = gp.groupWrittenAccountMap[gId]

			for k, v := range groupWrittenStorage {
				gp.groupWrittenStorageMap[gId][k] = v
			}
			groupWrittenStorage = gp.groupWrittenStorageMap[gId]

			firstGroup = false
		} else {
			tmpExecutionGroup.AddTransactions(gp.executionGroupMap[gId].Transactions())
			delete(gp.executionGroupMap, gId)
			for k, v := range gp.groupWrittenAccountMap[gId] {
				groupWrittenAccount[k] = v
				gp.addrToGroupMap[k] = gp.groupId
			}
			for k, v := range gp.groupWrittenStorageMap[gId] {
				groupWrittenStorage[k] = v
				gp.storageToGroupMap[k] = gp.groupId
			}
		}
		delete(gp.executionGroupMap, gId)
		delete(gp.groupWrittenAccountMap, gId)
		delete(gp.groupWrittenStorageMap, gId)
	}

	d4 := time.Since(t4)
	t5 := time.Now()
	gp.groupWrittenAccountMap[gp.groupId] = groupWrittenAccount
	gp.groupWrittenStorageMap[gp.groupId] = groupWrittenStorage
	gp.executionGroupMap[gp.groupId] = tmpExecutionGroup
	gp.groupId++

	d5 := time.Since(t5)
	d += time.Since(t0).Nanoseconds()
	log.Trace("group",
		"d0", common.PrettyDuration(d0),
		"d1", common.PrettyDuration(d1),
		"d2", common.PrettyDuration(d2),
		"d3", common.PrettyDuration(d3),
		"d4", common.PrettyDuration(d4),
		"d5", common.PrettyDuration(d5),
	)
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
	t0 := time.Now()
	tmpExecutionGroupMap := pb.groupTransactions(pb.transactions, false)
	d0 := time.Since(t0)

	d1 := int64(0)

	for _, execGroup := range tmpExecutionGroupMap {
		execGroup.SetId(pb.nextGroupId)
		if len(tmpExecutionGroupMap) == 1 {
			execGroup.SetStatedb(pb.statedb)
		} else {
			t1 := time.Now()
			execGroup.SetStatedb(pb.statedb.Copy())

			d1 += time.Since(t1).Nanoseconds()
		}
		pb.executionGroups[pb.nextGroupId] = execGroup

		for _, tx := range execGroup.transactions {
			pb.trxHashToGroupIdMap[tx.Hash()] = pb.nextGroupId
		}

		pb.nextGroupId++
	}
	log.Info("group", "group tx", common.PrettyDuration(d0), "copy db", common.PrettyDuration(d1))
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

		txs = sortTrxByIndex(txs, pb.trxHashToIndexMap)
		tmpExecGroupMap := pb.groupTransactions(txs, true)

		for _, group := range tmpExecGroupMap {
			var (
				txsToReuse []TxHashGroupIdPair
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
						txsToReuse = append(txsToReuse, TxHashGroupIdPair{txHash, oldGroupId})
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
	grouper := newGrouper(regroup)

	for _, tx := range transactions {
		grouper.groupNewTransaction(tx, pb)
	}

	for _, group := range grouper.executionGroupMap {
		group.transactions = sortTrxByIndex(group.transactions, pb.trxHashToIndexMap)
	}

	return grouper.executionGroupMap
}

func (pb *ParallelBlock) groupTransactionsFromChan(ch chan *types.Transaction, regroup bool) map[int]*ExecutionGroup {
	grouper := newGrouper(regroup)

	for tx := range ch {
		grouper.groupNewTransaction(tx, pb)
	}

	for _, group := range grouper.executionGroupMap {
		group.transactions = sortTrxByIndex(group.transactions, pb.trxHashToIndexMap)
	}

	return grouper.executionGroupMap
}

func (pb *ParallelBlock) groupNewTransaction(tx *types.Transaction, regroup bool,
	executionGroupMap map[int]*ExecutionGroup,
	groupWrittenAccountMap map[int]map[common.Address]struct{},
	groupWrittenStorageMap map[int]map[state.StorageAddress]struct{},
	addrToGroupMap map[common.Address]int,
	storageToGroupMap map[state.StorageAddress]int,
	groupId int) {
	d := int64(0)

	t0 := time.Now()
	groupsToMerge := make(map[int]struct{})
	groupWrittenAccount := make(map[common.Address]struct{})
	groupWrittenStorage := make(map[state.StorageAddress]struct{})
	firstGroup := true
	var tmpExecutionGroup *ExecutionGroup
	trxTouchedAddress := pb.getTrxTouchedAddress(tx.Hash(), regroup)

	d0 := time.Since(t0)
	t1 := time.Now()

	for addr, op := range trxTouchedAddress.AccountOp() {
		if gId, ok := addrToGroupMap[addr]; ok {
			groupsToMerge[gId] = struct{}{}
		}
		if op {
			groupWrittenAccount[addr] = struct{}{}
			addrToGroupMap[addr] = groupId
		}
	}

	d1 := time.Since(t1)
	t2 := time.Now()

	for storage, op := range trxTouchedAddress.StorageOp() {
		if gId, ok := storageToGroupMap[storage]; ok {
			groupsToMerge[gId] = struct{}{}
		}
		if op {
			groupWrittenStorage[storage] = struct{}{}
			storageToGroupMap[storage] = groupId
		}
	}

	d2 := time.Since(t2)
	t3 := time.Now()

	if len(groupsToMerge) == 0 {
		tmpExecutionGroup = NewExecutionGroup()
		tmpExecutionGroup.AddTransaction(tx)
		tmpExecutionGroup.SetHeader(pb.block.Header())
		tmpExecutionGroup.SetId(groupId)
	}

	d3 := time.Since(t3)
	t4 := time.Now()
	for gId := range groupsToMerge {
		if firstGroup {
			tmpExecutionGroup = executionGroupMap[gId]
			tmpExecutionGroup.AddTransaction(tx)
			tmpExecutionGroup.SetId(groupId)

			for k := range groupWrittenAccountMap[gId] {
				addrToGroupMap[k] = groupId
			}

			for k := range groupWrittenStorageMap[gId] {
				storageToGroupMap[k] = groupId
			}

			for k, v := range groupWrittenAccount {
				groupWrittenAccountMap[gId][k] = v
			}
			groupWrittenAccount = groupWrittenAccountMap[gId]

			for k, v := range groupWrittenStorage {
				groupWrittenStorageMap[gId][k] = v
			}
			groupWrittenStorage = groupWrittenStorageMap[gId]

			firstGroup = false
		} else {
			tmpExecutionGroup.AddTransactions(executionGroupMap[gId].Transactions())
			delete(executionGroupMap, gId)
			for k, v := range groupWrittenAccountMap[gId] {
				groupWrittenAccount[k] = v
				addrToGroupMap[k] = groupId
			}
			for k, v := range groupWrittenStorageMap[gId] {
				groupWrittenStorage[k] = v
				storageToGroupMap[k] = groupId
			}
		}
		delete(executionGroupMap, gId)
		delete(groupWrittenAccountMap, gId)
		delete(groupWrittenStorageMap, gId)
	}

	d4 := time.Since(t4)
	t5 := time.Now()
	groupWrittenAccountMap[groupId] = groupWrittenAccount
	groupWrittenStorageMap[groupId] = groupWrittenStorage
	executionGroupMap[groupId] = tmpExecutionGroup
	groupId++

	d5 := time.Since(t5)
	d += time.Since(t0).Nanoseconds()
	log.Trace("group",
		"d0", common.PrettyDuration(d0),
		"d1", common.PrettyDuration(d1),
		"d2", common.PrettyDuration(d2),
		"d3", common.PrettyDuration(d3),
		"d4", common.PrettyDuration(d4),
		"d5", common.PrettyDuration(d5),
	)
}

func (pb *ParallelBlock) getTrxTouchedAddress(hash common.Hash, regroup bool) *state.TouchedAddressObject {
	if regroup {
		if result, ok := pb.executionGroups[pb.trxHashToGroupIdMap[hash]].trxHashToResultMap[hash]; ok {
			return result.touchedAddresses
		}
	}

	touchedAddressObj := state.NewTouchedAddressObject()
	msg := pb.trxHashToMsgMap[hash]

	if msg.Payment() != params.EmptyAddress {
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

	if len(pb.executionGroups) == 1 {
		return conflictGroups, conflictTxs
	}

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
		receipt, trxUsedGas, err := ApplyTransactionMsg(pb.config, pb.context, gp, statedb, pb.block.Header(),
			pb.trxHashToMsgMap[txHash], tx, &group.usedGas, feeAmount, pb.vmConfig)
		if err != nil {
			group.err = err
			group.errTxIndex = ti
			group.trxHashToResultMap[txHash] = NewTrxResult(nil, statedb.FinalizeTouchedAddress(), trxUsedGas, feeAmount)
			group.startTrxIndex = -1
			return
		}
		group.trxHashToResultMap[txHash] = NewTrxResult(receipt, statedb.FinalizeTouchedAddress(), trxUsedGas, feeAmount)
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
	contractAddrs := make([]common.Address, 0, pb.block.Transactions().Len())
	wg := sync.WaitGroup{}
	lock := sync.RWMutex{}

	ch := make(chan error, pb.block.Transactions().Len())

	for ti, trx := range pb.block.Transactions() {
		pb.trxHashToIndexMap[trx.Hash()] = ti

		wg.Add(1)
		go func(trx *types.Transaction) {
			msg, err := trx.AsMessage(types.MakeSigner(pb.config, pb.block.Header().Number))
			if err != nil {
				ch <- err
				return
			}
			ch <- nil

			lock.Lock()
			pb.trxHashToMsgMap[trx.Hash()] = &msg
			lock.Unlock()
			wg.Done()
		}(trx)

		if to := trx.To(); to != nil {
			contractAddrs = append(contractAddrs, *to)
		}
	}

	pb.associatedAddressMap = associatedAddressMngr.LoadAssociatedAddresses(contractAddrs)
	wg.Wait()

	if len(ch) != 0 {
		return <-ch
	}

	return nil
}

func (pb *ParallelBlock) prepareAndGroup() error {
	contractAddrs := make([]common.Address, 0, pb.block.Transactions().Len())
	wg := sync.WaitGroup{}
	lock := sync.RWMutex{}

	ch := make(chan error, pb.block.Transactions().Len())

	for ti, trx := range pb.block.Transactions() {
		pb.trxHashToIndexMap[trx.Hash()] = ti

		wg.Add(1)
		go func(trx *types.Transaction) {
			msg, err := trx.AsMessage(types.MakeSigner(pb.config, pb.block.Header().Number))
			if err != nil {
				ch <- err
				return
			}
			ch <- nil

			lock.Lock()
			pb.trxHashToMsgMap[trx.Hash()] = &msg
			lock.Unlock()
			wg.Done()
		}(trx)

		if to := trx.To(); to != nil {
			contractAddrs = append(contractAddrs, *to)
		}
	}

	pb.associatedAddressMap = associatedAddressMngr.LoadAssociatedAddresses(contractAddrs)
	wg.Wait()

	if len(ch) != 0 {
		return <-ch
	}

	return nil
}

func (pb *ParallelBlock) collectResult() (types.Receipts, []*types.Log, uint64, error) {
	var (
		err                 error
		errIndex            = -1
		receipts            = make(types.Receipts, pb.transactions.Len())
		usedGas             = uint64(0)
		allLogs             []*types.Log
		gp                  = new(GasPool).AddGas(pb.block.GasLimit())
		cumulative          = uint64(0)
		wg                  = sync.WaitGroup{}
		chForAssociatedAddr = make(chan *txGroupPair, pb.transactions.Len())
		chForUpdateCache    = make(chan *addressRlpDataPair, pb.transactions.Len())
		chForFinish         = make(chan bool)
	)

	go pb.processAssociatedAddressOfContract(chForAssociatedAddr)
	go pb.updateStateDB(chForUpdateCache, chForFinish)

	// Copy updated state object to pb.stateDB
	for _, group := range pb.executionGroups {
		stateObjsToReuse := make(map[common.Address]*state.StateObjectToReuse)

		for _, tx := range group.transactions {
			if result, ok := group.trxHashToResultMap[tx.Hash()]; ok {
				appendStateObjToReuse(stateObjsToReuse, result.touchedAddresses)
			}
		}
		wg.Add(1)
		go func(db *state.StateDB, stateObjsToReuse map[common.Address]*state.StateObjectToReuse) {
			defer wg.Done()
			for addr, obj := range stateObjsToReuse {
				data, changed := pb.statedb.CopyStateObjRlpDataFromOtherDB(db, obj)
				if changed {
					chForUpdateCache <- &addressRlpDataPair{
						address: addr,
						rlpData: data,
					}
				}
			}
		}(group.statedb, stateObjsToReuse)
	}

	for _, group := range pb.executionGroups {
		if group.err != nil && (group.errTxIndex < errIndex || errIndex == -1) {
			err = group.err
			errIndex = group.errTxIndex
		}
		usedGas += group.usedGas
		pb.feeAmount.Add(group.feeAmount, pb.feeAmount)

		// Update contract associated address
		for _, tx := range group.transactions {
			txHash := tx.Hash()
			if result, ok := group.trxHashToResultMap[txHash]; ok {
				receipts[pb.trxHashToIndexMap[txHash]] = result.receipt
				chForAssociatedAddr <- &txGroupPair{
					tx:    tx,
					group: group,
				}
			}
		}
	}

	close(chForAssociatedAddr)

	for index, tx := range pb.transactions {
		if gasErr := gp.SubGas(tx.Gas()); gasErr != nil {
			return nil, nil, 0, gasErr
		}

		if errIndex != -1 && index >= errIndex {
			return nil, nil, 0, err
		}

		gp.AddGas(tx.Gas() - receipts[index].GasUsed)
		cumulative += receipts[index].GasUsed
		receipts[index].CumulativeGasUsed = cumulative
		allLogs = append(allLogs, receipts[index].Logs...)
	}

	wg.Wait()
	close(chForUpdateCache)
	<-chForFinish

	return receipts, allLogs, usedGas, nil
}

func (pb *ParallelBlock) processAssociatedAddressOfContract(ch chan *txGroupPair) {
	associatedAddrs := make(map[common.Address]*state.TouchedAddressObject)

	for txAndGroup := range ch {
		tx := txAndGroup.tx
		group := txAndGroup.group
		txHash := txAndGroup.tx.Hash()
		if to := tx.To(); to != nil && group.statedb.StateObjIsContract(*to) {
			touchedAddr := group.trxHashToResultMap[txHash].touchedAddresses
			msg := pb.trxHashToMsgMap[txHash]
			touchedAddr.RemoveAccount(msg.From())
			touchedAddr.RemoveAccount(msg.Payment())
			associatedAddrs[*to] = touchedAddr
		}
	}

	associatedAddressMngr.UpdateAssociatedAddresses(associatedAddrs)
}

func (pb *ParallelBlock) updateStateDB(ch chan *addressRlpDataPair, chForFinish chan bool) {
	if len(pb.executionGroups) == 1 {
		pb.statedb.FinaliseGroup(true)
	} else {
		for addrData := range ch {
			pb.statedb.UpdateDBTrie(addrData.address, addrData.rlpData)
		}
	}
	chForFinish <- true
}

func (pb *ParallelBlock) Process() (types.Receipts, []*types.Log, uint64, error) {
	var d2, d3 time.Duration
	t0 := time.Now()
	if err := pb.prepare(); err != nil {
		return nil, nil, 0, err
	}
	d0 := time.Since(t0)

	t0 = time.Now()
	pb.group()
	d1 := time.Since(t0)

	for {
		t0 = time.Now()
		pb.executeInParallel()
		d2 = time.Since(t0)

		t0 = time.Now()
		if conflictGroups, conflictTxs := pb.checkConflict(); len(conflictGroups) != 0 {
			pb.reGroupAndRevert(conflictGroups, conflictTxs)
		} else {
			d3 = time.Since(t0)
			break
		}
	}

	t0 = time.Now()
	receipts, logs, gas, err := pb.collectResult()
	d4 := time.Since(t0)

	if len(pb.executionGroups) != 0 {
		log.Info("Process:", "block ", pb.block.Number(), "txs", pb.transactions.Len(),
			"group", len(pb.executionGroups), "prepare", common.PrettyDuration(d0), "group", common.PrettyDuration(d1),
			"executeInParallel", common.PrettyDuration(d2), "checkConflict", common.PrettyDuration(d3),
			"collectResult", common.PrettyDuration(d4))
	}

	return receipts, logs, gas, err
}

func sortTrxByIndex(txs types.Transactions, trxHashToIndexMap map[common.Hash]int) types.Transactions {
	sort.Slice(txs, func(i, j int) bool {
		return trxHashToIndexMap[txs[i].Hash()] < trxHashToIndexMap[txs[j].Hash()]
	})
	return txs
}
