package parallel

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
)

type GroupResult struct {
	accountRecords          map[common.Address][]*AccountRecord
	storageRecords          map[StorageAddress][]*StorageRecord
	hashToCode              map[common.Hash]state.Code
	receipt                 map[common.Hash]*types.Receipt
	trxHashToTouchedAddress map[common.Hash]*TouchedAddressObject
}

type AccountRecord struct {
	index    int
	balance  *big.Int
	codeHash []byte
}

type StorageRecord struct {
	index int
	Value common.Hash
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

func (gr *GroupResult) rollbackExecResult(trxHashes map[common.Hash]struct{}) *TouchedAddressObject {
	totalTouchedAddress := NewTouchedAddressObject()

	for trxHash, _ := range trxHashes {
		touchedAddressObj := gr.trxHashToTouchedAddress[trxHash]
		delete(gr.trxHashToTouchedAddress, trxHash)
		totalTouchedAddress.Merge(touchedAddressObj)

		for account, op := range touchedAddressObj.AccountOp() {
			if op {
				delete(gr.accountRecords, account)
			}
		}

		for storage, op := range touchedAddressObj.StorageOp() {
			if op {
				delete(gr.storageRecords, storage)
			}
		}

		delete(gr.receipt, trxHash)
	}

	return totalTouchedAddress
}
