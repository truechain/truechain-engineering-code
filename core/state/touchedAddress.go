package state

import "github.com/ethereum/go-ethereum/common"

type StorageAddress struct {
	AccountAddress common.Address
	Key            common.Hash
}

type TouchedAddressObject struct {
	accountOp    map[common.Address]bool
	storageOp    map[StorageAddress]bool
	accountInArg []common.Address
}

type StateObjectToReuse struct {
	Address   common.Address
	Keys      []common.Hash
	ReuseData bool
}

func NewTouchedAddressObject() *TouchedAddressObject {
	return &TouchedAddressObject{
		accountOp: make(map[common.Address]bool),
		storageOp: make(map[StorageAddress]bool),
	}
}

func NewStateObjectToReuse(address common.Address, keys []common.Hash, reuseData bool) *StateObjectToReuse {
	return &StateObjectToReuse{Address: address, Keys: keys, ReuseData: reuseData}
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

// Merge 2 TouchedAddressObject, return true if first object changes
func (self *TouchedAddressObject) Merge(another *TouchedAddressObject) bool {
	changed := false
	for address, op := range another.accountOp {
		if origOp, exist := self.accountOp[address]; !exist || (op == true && origOp == false) {
			changed = true
			self.accountOp[address] = op
		}
	}

	for address, op := range another.storageOp {
		if origOp, exist := self.storageOp[address]; !exist || (op == true && origOp == false) {
			changed = true
			self.storageOp[address] = op
		}
	}

	return changed
}

func (self *TouchedAddressObject) RemoveAccount(address common.Address) {
	delete(self.accountOp, address)
}

func (self *TouchedAddressObject) RemoveStorage(storage StorageAddress) {
	delete(self.storageOp, storage)
}

func (self *TouchedAddressObject) AddAccountInArg(address common.Address) {
	self.accountInArg = append(self.accountInArg, address)
}

func (self *TouchedAddressObject) RemoveAccountsInArgs() {
	for _, address := range self.accountInArg {
		delete(self.accountOp, address)
	}
}
