// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
package vm

import (
	"math/big"
	"strings"

	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
)

// StakingAddress is defined as Address('truestaking')
// i.e. contractAddress = 0x000000000000000000747275657374616b696E67
var StakingAddress = common.BytesToAddress([]byte("truestaking"))

// Staking contract ABI
var abiStaking abi.ABI

type StakeContract struct{}

func init() {
	abiStaking, _ = abi.JSON(strings.NewReader(abiJSON))
}

// RunStaking execute truechain staking contract
func RunStaking(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	method, err := abiStaking.MethodById(input)
	if err != nil {
		log.Error("No method found")
		return nil, ErrStakingInvalidInput
	}

	data := input[4:]

	switch method.Name {
	case "getDeposit":
		ret, err = getDeposit(evm, contract, data)
	case "deposit":
		ret, err = deposit(evm, contract, data)
	case "withdraw":
		ret, err = withdraw(evm, contract, data)
	default:
		log.Warn("Staking call fallback function")
		err = ErrStakingInvalidInput
	}

	return ret, err
}

// deposit
func deposit(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	var pubkey []byte
	method, _ := abiStaking.Methods["deposit"]

	err = method.Inputs.Unpack(&pubkey, input)
	if err != nil {
		log.Error("Unpack deposit pubkey error", "err", err)
		return nil, ErrStakingInvalidInput
	}

	from := contract.caller.Address()

	log.Info("Staking deposit", "address", contract.caller.Address(), "value", contract.value)
	impawn := NewImpawnImpl()
	impawn.Load(evm.StateDB, StakingAddress)

	impawn.InsertSAccount2(evm.Context.BlockNumber.Uint64(), from, pubkey, contract.value, big.NewInt(0), true)

	return nil, nil
}

// withdraw
func withdraw(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	from := contract.caller.Address()

	pre := evm.StateDB.GetPOSState(StakingAddress, common.BytesToHash(from[:]))
	balance := new(big.Int).SetBytes(pre)

	if balance.Cmp(common.Big0) <= 0 {
		log.Warn("Staking withdraw zero value", "address", contract.caller.Address())
		return nil, ErrStakingInvalidInput
	}
	log.Info("Staking withdraw", "address", contract.caller.Address(), "value", balance)

	_, left, err := evm.Call(contract.self, from, nil, evm.callGasTemp, balance, nil)
	if err != nil {
		log.Info("Staking withdraw transfer failed", "err", err)
		return nil, nil
	}
	log.Info("Staking withdraw", "gas", left)
	evm.StateDB.SetPOSState(StakingAddress, common.BytesToHash(from[:]), nil)

	return nil, nil
}

// getDeposit
func getDeposit(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	if len(input)%32 != 0 {
		log.Error("Call get_deposit input error")
		return nil, ErrStakingInvalidInput
	}

	var depositAddr common.Address
	method, _ := abiStaking.Methods["getDeposit"]

	// depositAddr := struct {Validator common.Address}{}
	err = method.Inputs.Unpack(&depositAddr, input)
	if err != nil {
		log.Error("Unpack get_deposit input error")
		return nil, ErrStakingInvalidInput
	}

	impawn := NewImpawnImpl()
	impawn.Load(evm.StateDB, StakingAddress)

	epoch := types.GetEpochFromHeight(evm.Context.BlockNumber.Uint64())
	account, err := impawn.GetStakingAccount(epoch.EpochID, depositAddr)
	if err != nil {
		log.Error("Staking fetch account error", "error", err)
		ret, _ = method.Outputs.Pack(big.NewInt(0))
		return ret, err
	}

	balance := new(big.Int)
	for _, u := range account.unit.value {
		balance.Add(balance, u.amount)
	}
	log.Info("Get staking get_deposit", "address", depositAddr, "balance", balance)

	ret, err = method.Outputs.Pack(balance)
	return ret, err
}

// Staking Contract json abi
const abiJSON = `
[
  {
    "name": "deposit",
    "outputs": [],
    "inputs": [
      {
        "type": "bytes",
        "name": "pubkey"
      }
    ],
    "constant": false,
    "payable": true,
    "type": "function",
    "gas": 371
  },
  {
    "name": "getDeposit",
    "outputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "out"
      }
    ],
    "inputs": [
      {
        "type": "address",
        "name": "owner"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function",
    "gas": 420
  },
  {
    "name": "withdraw",
    "outputs": [],
    "inputs": [],
    "constant": false,
    "payable": false,
    "type": "function",
    "gas": 366
  }
]
`
