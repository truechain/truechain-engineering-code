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
	txInfos              []*txInfo
	executionGroups      map[int]*ExecutionGroup
	associatedAddressMap map[common.Address]*state.TouchedAddressObject
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
	addrToGroupMap         map[common.Address]int
	groupId                int
	regroup                bool
}

type txInfo struct {
	index   int
	hash    common.Hash
	msg     *types.Message
	tx      *types.Transaction
	groupId int
	result  *TrxResult
}

func newTxInfo(index int, hash common.Hash, msg *types.Message, tx *types.Transaction) *txInfo {
	return &txInfo{index: index, hash: hash, msg: msg, tx: tx}
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
	address  common.Address
	stateObj interface{}
	rlpData  []byte
}

func newGrouper(regroup bool) *grouper {
	return &grouper{
		executionGroupMap:      make(map[int]*ExecutionGroup),
		groupWrittenAccountMap: make(map[int]map[common.Address]struct{}),
		addrToGroupMap:         make(map[common.Address]int),
		groupId:                0,
		regroup:                regroup,
	}
}

func (gp *grouper) groupNewTxInfo(txInfo *txInfo, pb *ParallelBlock) {
	d := int64(0)

	t0 := time.Now()
	groupsToMerge := make(map[int]struct{})
	groupWrittenAccount := make(map[common.Address]struct{})
	firstGroup := true
	var tmpExecutionGroup *ExecutionGroup
	trxTouchedAddress := pb.getTrxTouchedAddress(txInfo, gp.regroup)

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

	if len(groupsToMerge) == 0 {
		tmpExecutionGroup = NewExecutionGroup()
		tmpExecutionGroup.addTxInfo(txInfo)
		tmpExecutionGroup.SetHeader(pb.block.Header())
		tmpExecutionGroup.SetId(gp.groupId)
	}

	d3 := time.Since(t2)
	t4 := time.Now()
	for gId := range groupsToMerge {
		if firstGroup {
			tmpExecutionGroup = gp.executionGroupMap[gId]
			tmpExecutionGroup.addTxInfo(txInfo)
			tmpExecutionGroup.SetId(gp.groupId)

			for k := range gp.groupWrittenAccountMap[gId] {
				gp.addrToGroupMap[k] = gp.groupId
			}

			for k, v := range groupWrittenAccount {
				gp.groupWrittenAccountMap[gId][k] = v
			}
			groupWrittenAccount = gp.groupWrittenAccountMap[gId]

			firstGroup = false
		} else {
			tmpExecutionGroup.addTxInfos(gp.executionGroupMap[gId].getTxInfos())
			delete(gp.executionGroupMap, gId)
			for k, v := range gp.groupWrittenAccountMap[gId] {
				groupWrittenAccount[k] = v
				gp.addrToGroupMap[k] = gp.groupId
			}
		}
		delete(gp.executionGroupMap, gId)
		delete(gp.groupWrittenAccountMap, gId)
	}

	d4 := time.Since(t4)
	t5 := time.Now()
	gp.groupWrittenAccountMap[gp.groupId] = groupWrittenAccount
	gp.executionGroupMap[gp.groupId] = tmpExecutionGroup
	gp.groupId++

	d5 := time.Since(t5)
	d += time.Since(t0).Nanoseconds()
	log.Trace("group",
		"d0", common.PrettyDuration(d0),
		"d1", common.PrettyDuration(d1),
		"d3", common.PrettyDuration(d3),
		"d4", common.PrettyDuration(d4),
		"d5", common.PrettyDuration(d5),
	)
}

func NewParallelBlock(block *types.Block, statedb *state.StateDB, config *params.ChainConfig, bc ChainContext, cfg vm.Config, feeAmount *big.Int) *ParallelBlock {
	return &ParallelBlock{
		block:           block,
		txInfos:         make([]*txInfo, block.Transactions().Len()),
		executionGroups: make(map[int]*ExecutionGroup),
		statedb:         statedb,
		config:          config,
		context:         bc,
		vmConfig:        cfg,
		feeAmount:       feeAmount,
	}
}

func (pb *ParallelBlock) groupTxs() {
	tmpExecutionGroupMap := pb.groupTransactions(pb.txInfos, false)

	for _, execGroup := range tmpExecutionGroupMap {
		execGroup.SetId(pb.nextGroupId)
		if len(tmpExecutionGroupMap) == 1 {
			execGroup.SetStatedb(pb.statedb)
		} else {
			execGroup.SetStatedb(pb.statedb.Copy())
		}
		pb.executionGroups[pb.nextGroupId] = execGroup

		for _, txInfo := range execGroup.txInfos {
			txInfo.groupId = pb.nextGroupId
		}

		pb.nextGroupId++
	}
}

func (pb *ParallelBlock) group(chForTxInfo chan *txInfo, chForGroup chan bool) {
	tmpExecutionGroupMap := pb.groupTransactionsFromChan(chForTxInfo, false)

	for _, execGroup := range tmpExecutionGroupMap {
		execGroup.SetId(pb.nextGroupId)
		if len(tmpExecutionGroupMap) == 1 {
			execGroup.SetStatedb(pb.statedb)
		} else {
			execGroup.SetStatedb(pb.statedb.Copy())
		}
		pb.executionGroups[pb.nextGroupId] = execGroup

		for _, txInfo := range execGroup.txInfos {
			txInfo.groupId = pb.nextGroupId
		}

		pb.nextGroupId++
	}
	chForGroup <- true
}

func (pb *ParallelBlock) reGroupAndRevert(conflictGroups []map[int]struct{}, conflictTxs map[common.Hash]struct{}) {
	for _, conflictGroupIds := range conflictGroups {
		var txInfos []*txInfo
		conflictGroups := make(map[int]*ExecutionGroup)

		for groupId := range conflictGroupIds {
			txInfos = append(txInfos, pb.executionGroups[groupId].getTxInfos()...)
			conflictGroups[groupId] = pb.executionGroups[groupId]
			delete(pb.executionGroups, groupId)
		}

		txInfos = sortTxInfosByIndex(txInfos)
		tmpExecGroupMap := pb.groupTransactions(txInfos, true)

		for _, group := range tmpExecGroupMap {
			var (
				txsToReuse []*txInfo
				conflict   = false
			)
			for index, txInfo := range group.txInfos {
				txHash := txInfo.hash
				if !conflict {
					if _, ok := conflictTxs[txHash]; ok {
						conflict = true
						group.SetStartTrxPos(index)
					} else {
						txsToReuse = append(txsToReuse, txInfo)
					}
				}

				if conflict {
					// revert txInfos which will be re-executed in reversed order
					conflictGroups[txInfo.groupId].statedb.RevertTrxResultByHash(txHash)
				}
			}

			// copy transaction results and state changes from old group which can be reused
			group.SetId(pb.nextGroupId)
			group.reuseTxResults(txsToReuse, conflictGroups)
			pb.executionGroups[pb.nextGroupId] = group
			pb.nextGroupId++
		}
	}
}

func (pb *ParallelBlock) groupTransactions(txInfos []*txInfo, regroup bool) map[int]*ExecutionGroup {
	grouper := newGrouper(regroup)

	for _, txInfo := range txInfos {
		grouper.groupNewTxInfo(txInfo, pb)
	}

	for _, group := range grouper.executionGroupMap {
		group.txInfos = sortTxInfosByIndex(group.txInfos)
	}

	return grouper.executionGroupMap
}

func (pb *ParallelBlock) groupTransactionsFromChan(ch chan *txInfo, regroup bool) map[int]*ExecutionGroup {
	grouper := newGrouper(regroup)

	for tx := range ch {
		grouper.groupNewTxInfo(tx, pb)
	}

	for _, group := range grouper.executionGroupMap {
		group.txInfos = sortTxInfosByIndex(group.txInfos)
	}

	return grouper.executionGroupMap
}

func (pb *ParallelBlock) getTrxTouchedAddress(txInfo *txInfo, regroup bool) *state.TouchedAddressObject {
	if regroup {
		if result := txInfo.result; result != nil {
			return result.touchedAddresses
		}
	}

	touchedAddressObj := state.NewTouchedAddressObject()
	msg := txInfo.msg

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

	if len(pb.executionGroups) == 1 {
		return conflictGroups, conflictTxs
	}

	for _, txInfo := range pb.txInfos {
		var touchedAddressObj *state.TouchedAddressObject = nil
		trxHash := txInfo.hash
		curTrxGroup := txInfo.groupId
		touchedAddressObj = pb.getTrxTouchedAddress(txInfo, true)

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
	}

	groupsList := list.New()
	for _, groups := range addrGroupIdsMap {
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

	// Iterate over and process the individual txInfos
	for i := group.startTrxIndex; i < len(group.txInfos); i++ {
		txInfo := group.txInfos[i]
		txHash := txInfo.hash
		ti := txInfo.index
		statedb.Prepare(txHash, pb.block.Hash(), ti)
		receipt, trxUsedGas, err := ApplyTransactionMsg(pb.config, pb.context, gp, statedb, pb.block.Header(),
			txInfo.msg, txInfo.tx, &group.usedGas, feeAmount, pb.vmConfig)
		if err != nil {
			group.err = err
			group.errTxIndex = ti
			txInfo.result = NewTrxResult(nil, statedb.FinalizeTouchedAddress(), trxUsedGas, feeAmount)
			group.startTrxIndex = -1
			return
		}
		txInfo.result = NewTrxResult(receipt, statedb.FinalizeTouchedAddress(), trxUsedGas, feeAmount)
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
	txCount := pb.block.Transactions().Len()
	contractAddrs := make([]common.Address, 0, txCount)
	chForError := make(chan error, txCount)
	wg := sync.WaitGroup{}

	for ti, tx := range pb.block.Transactions() {
		wg.Add(1)
		go func(ti int, tx *types.Transaction) {
			msg, err := tx.AsMessage(types.MakeSigner(pb.config, pb.block.Header().Number))
			if err != nil {
				chForError <- err
				return
			}
			txInfo := newTxInfo(ti, tx.Hash(), &msg, tx)
			pb.txInfos[ti] = txInfo
			wg.Done()
		}(ti, tx)

		if to := tx.To(); to != nil {
			contractAddrs = append(contractAddrs, *to)
		}
	}

	pb.associatedAddressMap = associatedAddressMngr.LoadAssociatedAddresses(contractAddrs)
	wg.Wait()

	if len(chForError) != 0 {
		return <-chForError
	}

	return nil
}

func (pb *ParallelBlock) prepareAndGroup() error {
	txCount := pb.block.Transactions().Len()
	contractAddrs := make([]common.Address, 0, txCount)
	chForTxInfo := make(chan *txInfo, txCount)
	chForGroup := make(chan bool)

	go pb.group(chForTxInfo, chForGroup)

	for ti, tx := range pb.block.Transactions() {
		msg, err := tx.AsMessage(types.MakeSigner(pb.config, pb.block.Header().Number))
		if err != nil {
			close(chForTxInfo)
			return err
		}
		txInfo := newTxInfo(ti, tx.Hash(), &msg, tx)
		chForTxInfo <- txInfo
		pb.txInfos[ti] = txInfo
		if to := tx.To(); to != nil {
			contractAddrs = append(contractAddrs, *to)
		}
	}

	close(chForTxInfo)
	pb.associatedAddressMap = associatedAddressMngr.LoadAssociatedAddresses(contractAddrs)
	<-chForGroup

	return nil
}

func (pb *ParallelBlock) collectResult() (types.Receipts, []*types.Log, uint64, error) {
	var (
		err                 error
		errIndex            = -1
		receipts            = make(types.Receipts, len(pb.txInfos))
		usedGas             = uint64(0)
		allLogs             []*types.Log
		gp                  = new(GasPool).AddGas(pb.block.GasLimit())
		cumulative          = uint64(0)
		wg                  = sync.WaitGroup{}
		chForAssociatedAddr = make(chan *txInfo, len(pb.txInfos))
		chForUpdateCache    = make(chan *addressRlpDataPair, len(pb.txInfos))
		chForFinish         = make(chan bool)
	)

	go pb.processAssociatedAddressOfContract(chForAssociatedAddr)
	go pb.updateStateDB(chForUpdateCache, chForFinish)

	// Copy updated state object to pb.stateDB
	for _, group := range pb.executionGroups {
		stateObjsToReuse := make(map[common.Address]struct{})

		for _, txInfo := range group.txInfos {
			if result := txInfo.result; result != nil {
				appendStateObjToReuse(stateObjsToReuse, result.touchedAddresses)
			}
		}
		wg.Add(1)
		go func(db *state.StateDB, stateObjsToReuse map[common.Address]struct{}) {
			defer wg.Done()
			for addr := range stateObjsToReuse {
				stateObj, data, changed := pb.statedb.CopyStateObjRlpDataFromOtherDB(db, addr)
				if changed {
					chForUpdateCache <- &addressRlpDataPair{
						address:  addr,
						stateObj: stateObj,
						rlpData:  data,
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
		for _, txInfo := range group.txInfos {
			if result := txInfo.result; result != nil {
				receipts[txInfo.index] = result.receipt
				chForAssociatedAddr <- txInfo
			}
		}
	}

	close(chForAssociatedAddr)

	for index, tx := range pb.block.Transactions() {
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

func (pb *ParallelBlock) processAssociatedAddressOfContract(ch chan *txInfo) {
	associatedAddrs := make(map[common.Address]*state.TouchedAddressObject)

	for txInfo := range ch {
		if to := txInfo.tx.To(); to != nil {
			touchedAddr := txInfo.result.touchedAddresses
			msg := txInfo.msg
			touchedAddr.RemoveAccount(msg.From())
			touchedAddr.RemoveAccount(msg.Payment())
			if len(touchedAddr.AccountOp()) > 1 {
				associatedAddrs[*to] = touchedAddr
			}
		}
	}

	associatedAddressMngr.UpdateAssociatedAddresses(associatedAddrs)
}

func (pb *ParallelBlock) updateStateDB(ch chan *addressRlpDataPair, chForFinish chan bool) {
	if len(pb.executionGroups) == 1 {
		pb.statedb.FinaliseGroup(true)
	} else {
		for addrData := range ch {
			pb.statedb.UpdateDBTrie(addrData.address, addrData.stateObj, addrData.rlpData)
		}
	}
	chForFinish <- true
}

func (pb *ParallelBlock) Process() (types.Receipts, []*types.Log, uint64, error) {
	var d2, d3 time.Duration
	t0 := time.Now()
	if err := pb.prepareAndGroup(); err != nil {
		return nil, nil, 0, err
	}
	d0 := time.Since(t0)

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
		log.Info("Process:", "block ", pb.block.Number(), "txs", len(pb.txInfos),
			"group", len(pb.executionGroups),
			"prepareAndGroup", common.PrettyDuration(d0),
			"exec", common.PrettyDuration(d2),
			"check", common.PrettyDuration(d3),
			"collectResult", common.PrettyDuration(d4))
	}

	return receipts, logs, gas, err
}

func sortTxInfosByIndex(txInfos []*txInfo) []*txInfo {
	sort.Slice(txInfos, func(i, j int) bool {
		return txInfos[i].index < txInfos[j].index
	})
	return txInfos
}
