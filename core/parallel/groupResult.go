package parallel

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
)

type GroupResult struct {
	accountRecords   map[common.Address][]*AccountRecord
	storageRecords   map[StorageAddress][]*StorageRecord
	trxHashToResult  map[common.Hash]*TrxResult
	trxIndexToResult map[int]*TrxResult
	usedGas          uint64
	err              error
}

type TrxResult struct {
	accountRecord    map[common.Address]*AccountRecord
	storageRecord    map[StorageAddress]*StorageRecord
	hashToCode       map[common.Hash]state.Code
	receipt          *types.Receipt
	logs             []*types.Log
	touchedAddresses *TouchedAddressObject
	usedGas          uint64
}

type AccountRecord struct {
	index    int
	balance  *big.Int
	codeHash []byte
	nonce    uint64
}

func (a AccountRecord) Index() int {
	return a.index
}

func (a AccountRecord) Balance() *big.Int {
	return a.balance
}

func (a AccountRecord) CodeHash() []byte {
	return a.codeHash
}

func (a AccountRecord) Nonce() uint64 {
	return a.nonce
}

type StorageRecord struct {
	index int
	Value common.Hash
}

func NewTrxResult(receipt *types.Receipt, logs []*types.Log, touchedAddresses *TouchedAddressObject, usedGas uint64) *TrxResult {
	return &TrxResult{receipt: receipt, logs: logs, touchedAddresses: touchedAddresses, usedGas: usedGas}
}

func NewGroupResult() *GroupResult {
	return &GroupResult{}
}

func NewAccountRecord(index int, balance *big.Int, codeHash []byte, nonce uint64) *AccountRecord {
	return &AccountRecord{index: index, balance: balance, codeHash: codeHash, nonce: nonce}
}

func NewStorageRecord(index int, value common.Hash) *StorageRecord {
	return &StorageRecord{index: index, Value: value}
}

func (gr *GroupResult) merge(result *GroupResult) {
	for addr, records := range result.accountRecords {
		gr.accountRecords[addr] = append(gr.accountRecords[addr], records...)
	}
	for storage, records := range result.storageRecords {
		gr.storageRecords[storage] = append(gr.storageRecords[storage], records...)
	}
	//for k, v := range result.trxHashToResult {
	//	gr.trxHashToResult[k] = v
	//}
	for k, v := range result.trxIndexToResult {
		gr.trxIndexToResult[k] = v
	}
	//gr.errs = append(gr.errs, result.errs...)
}

func (gr *GroupResult) removeResultAfterTrxPos(index int) {
	for _, records := range gr.accountRecords {
		for i := len(records) - 1; i > 0; i-- {
			if record := records[i]; record.index >= index {
				records = append(records[:i], records[i+1:]...)
			} else {
				break
			}
		}
	}

	for _, records := range gr.storageRecords {
		for i := len(records) - 1; i > 0; i-- {
			if record := records[i]; record.index >= index {
				records = append(records[:i], records[i+1:]...)
			} else {
				break
			}
		}
	}
}

//func (gr *GroupResult) rollbackExecResult(trxHashes map[common.Hash]struct{}) *TouchedAddressObject {
//	totalTouchedAddress := NewTouchedAddressObject()
//
//	for trxHash, _ := range trxHashes {
//		touchedAddressObj := gr.trxHashToTouchedAddress[trxHash]
//		delete(gr.trxHashToTouchedAddress, trxHash)
//		totalTouchedAddress.Merge(touchedAddressObj)
//
//		for account, op := range touchedAddressObj.AccountOp() {
//			if op {
//				delete(gr.accountRecords, account)
//			}
//		}
//
//		for storage, op := range touchedAddressObj.StorageOp() {
//			if op {
//				delete(gr.storageRecords, storage)
//			}
//		}
//
//		delete(gr.receipt, trxHash)
//	}
//
//	return totalTouchedAddress
//}

//func (gr* GroupResult) getResultByTrxHash(trxHash common.Hash) {
//
//	 groupResult := NewGroupResult()
//
//	 touchedAddressObj := gr.trxHashToTouchedAddress[trxHash]
//
//	 for addr, op := range touchedAddressObj.accountOp {
//	 	if op {
//	 		if accountRecords, ok := gr.accountRecords[addr]; ok {
//	 			groupResult.accountRecords[addr] = accountRecords
//			}
//		}
//	 }
//
//	for storage, op := range touchedAddressObj.storageOp {
//		if op {
//			if storageRecords, ok := gr.storageRecords[storage]; ok {
//				groupResult.storageRecords[storage] = storageRecords
//			}
//		}
//	}
//
//	//for (Map.Entry<String, Boolean> entry : touchedAddressObjMap.get(trxHash).getTouchedAccounts().entrySet()) {
//	//	if (entry.getValue()) {
//	//		String address = entry.getKey();
//	//		List<AccountRecordObj> accountRecordObjs = this.accountRecords.get(address);
//	//		if (accountRecordObjs != null) {
//	//			groupExecResult.getAccountRecords().put(address, accountRecordObjs);
//	//			if (accountRecordObjs.get(0).isContract() && contractAccountStates.containsKey(address)) {
//	//				groupExecResult.getContractAccountStates().put(address, this.contractAccountStates.get(address));
//	//			}
//	//		}
//	//
//	//		ContractObj contractObj = this.contractObjs.get(address);
//	//		if (contractObj != null) {
//	//			groupExecResult.getContractObjs().put(address, contractObj);
//	//		}
//	//	}
//	//}
//	//
//	//
//	//groupExecResult.getExecResults().put(trxHash, this.execResults.get(trxHash));
//	//groupExecResult.getFee4Miner().put(trxHash, this.fee4Miner.get(trxHash));
//	//groupExecResult.getTouchedAddressObjMap().put(trxHash, this.touchedAddressObjMap.get(trxHash));
//	//
//	//return groupExecResult;
//}
