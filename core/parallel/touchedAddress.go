package parallel

import "github.com/ethereum/go-ethereum/common"

type StorageAddress struct {
	accountAddress common.Address
	key            common.Hash
}

type TouchedAddressObject struct {
	accountOp map[common.Address]bool
	storageOp map[StorageAddress]bool
}

func NewTouchedAddressObject() *TouchedAddressObject {
	return &TouchedAddressObject{}
}

func (self *TouchedAddressObject) AccountOp() map[common.Address]bool {
	return self.accountOp
}

func (self *TouchedAddressObject) AddAccountOp(add common.Address, op bool) {
	if op {
		self.accountOp[add] = op
	} else {
		if _, exist := self.accountOp[add]; !exist {
			self.accountOp[add] = op
		}
	}
}

func (self *TouchedAddressObject) StorageOp() map[StorageAddress]bool {
	return self.storageOp
}

func (self *TouchedAddressObject) AddStorageOp(add StorageAddress, op bool) {
	if op {
		self.storageOp[add] = op
	} else {
		if _, exist := self.storageOp[add]; !exist {
			self.storageOp[add] = op
		}
	}
}

func (self *TouchedAddressObject) Merge(another *TouchedAddressObject) {
	for address, op := range another.accountOp {
		if op || self.accountOp[address] == true {
			self.accountOp[address] = true
		} else {
			self.accountOp[address] = false
		}
	}

	for address, op := range another.storageOp {
		if op || self.storageOp[address] == true {
			self.storageOp[address] = true
		} else {
			self.storageOp[address] = false
		}
	}
}
