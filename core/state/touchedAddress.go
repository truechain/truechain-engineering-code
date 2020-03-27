package state

import "github.com/ethereum/go-ethereum/common"

type TouchedAddressObject struct {
	accountOp    map[common.Address]bool
	accountInArg []common.Address
}

func NewTouchedAddressObject() *TouchedAddressObject {
	return &TouchedAddressObject{
		accountOp: make(map[common.Address]bool),
	}
}

func (tao *TouchedAddressObject) AccountOp() map[common.Address]bool {
	return tao.accountOp
}

func (tao *TouchedAddressObject) AddAccountOp(addr common.Address, op bool) {
	if op {
		tao.accountOp[addr] = op
	} else {
		if _, exist := tao.accountOp[addr]; !exist {
			tao.accountOp[addr] = op
		}
	}
}

func (tao *TouchedAddressObject) SetAccountOp(addr common.Address, op bool) {
	tao.accountOp[addr] = op
}

// Merge 2 TouchedAddressObject, return true if first object changes
func (tao *TouchedAddressObject) Merge(other *TouchedAddressObject) bool {
	changed := false
	for address, op := range other.accountOp {
		if origOp, exist := tao.accountOp[address]; !exist || (op == true && origOp == false) {
			changed = true
			tao.accountOp[address] = op
		}
	}

	return changed
}

func (tao *TouchedAddressObject) RemoveAccount(address common.Address) {
	delete(tao.accountOp, address)
}

func (tao *TouchedAddressObject) AddAccountInArg(address common.Address) {
	tao.accountInArg = append(tao.accountInArg, address)
}

func (tao *TouchedAddressObject) RemoveAccountsInArgs() {
	for _, address := range tao.accountInArg {
		delete(tao.accountOp, address)
	}
}
