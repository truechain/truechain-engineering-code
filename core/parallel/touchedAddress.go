package parallel

import "github.com/ethereum/go-ethereum/common"

type StorageAddress struct {
	AccountAddress common.Address
	Key            common.Hash
}

type TouchedAddressObject struct {
	accountOp map[common.Address]bool
	storageOp map[StorageAddress]bool
}

func NewTouchedAddressObject() *TouchedAddressObject {
	return &TouchedAddressObject{
		accountOp: make(map[common.Address]bool),
		storageOp: make(map[StorageAddress]bool),
	}
}

func (self *TouchedAddressObject) AccountOp() map[common.Address]bool {
	return self.accountOp
}

func (self *TouchedAddressObject) AddAccountOp(addr common.Address, op bool) {
	if op {
		self.accountOp[addr] = op
	} else {
		if _, exist := self.accountOp[addr]; !exist {
			self.accountOp[addr] = op
		}
	}
}

func (self *TouchedAddressObject) SetAccountOp(addr common.Address, op bool) {
	self.accountOp[addr] = op
}

func (self *TouchedAddressObject) StorageOp() map[StorageAddress]bool {
	return self.storageOp
}

func (self *TouchedAddressObject) AddStorageOp(storage StorageAddress, op bool) {
	if op {
		self.storageOp[storage] = op
	} else {
		if _, exist := self.storageOp[storage]; !exist {
			self.storageOp[storage] = op
		}
	}
}

func (self *TouchedAddressObject) SetStorageOp(storage StorageAddress, op bool) {
	self.storageOp[storage] = op
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
