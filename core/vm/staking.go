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
	"strings"
	"math/big"

	"github.com/truechain/truechain-engineering-code/accounts/abi"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/log"

)

// StakingAddress is defined as Address('truestaking')
// 0x000000000000000000747275657374616b696E67
var StakingAddress = common.BytesToAddress([]byte("truestaking"))

type StakeContract struct{}

// RunStaking execute truechain staking contract
func RunStaking(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {

	abiStaking, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		log.Error("Gen staking abi exception")
		return nil, nil
	}

	method, err := abiStaking.MethodById(input)

	if err != nil {
		log.Error("No method found")
		return nil, nil
	}

	if method.Name == "get_deposit" {
		log.Info("Call staking get_deposit")
		return getDeposit(evm, contract, input[4:])
	} else if method.Name == "deposit" {
		return deposit(evm, contract, input[4:])
	} else if method.Name == "withdraw" {
		return withdraw(evm, contract, input[4:])
	} else {
		log.Warn("Staking call fallback function")
	}
	return ret, nil
}

// deposit MethodId  0xd0e30db0
func deposit(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	from := contract.caller.Address()

	pre := evm.StateDB.GetPOSState(StakingAddress, common.BytesToHash(from[:]))
	balance := new(big.Int).SetBytes(pre)
	balance.Add(balance, contract.value)

	evm.StateDB.SetPOSState(StakingAddress, common.BytesToHash(from[:]), balance.Bytes())
	log.Info("Staking deposit", "address", contract.caller.Address(), "value", contract.value)

	return nil, nil
}

// withdraw MethodId "0x3ccfd60b"
func withdraw(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	from := contract.caller.Address()

	pre := evm.StateDB.GetPOSState(StakingAddress, common.BytesToHash(from[:]))
	balance := new(big.Int).SetBytes(pre)

	log.Info("Staking withdraw", "address", contract.caller.Address(), "value", balance)
	return nil, nil
}

// getDeposit MethodId 0xf4607feb
func getDeposit(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	abiStaking, _ := abi.JSON(strings.NewReader(abiJSON))

	method, _ := abiStaking.Methods["get_deposit"];
	if len(input)%32 != 0 {
		log.Error("Call get_deposit input error")
		return nil, nil
	}

	// depositAddr := struct {Validator common.Address}{}
	var depositAddr common.Address
	err = method.Inputs.Unpack(&depositAddr, input)
	if err != nil {
		log.Error("Unpack get_deposit input error")
		return nil, err
	}

	pre := evm.StateDB.GetPOSState(StakingAddress, common.BytesToHash(depositAddr[:]))
	balance := new(big.Int).SetBytes(pre)
	log.Info("Get staking get_deposit", "address", depositAddr, "balance", balance)

	ret, err = method.Outputs.Pack(balance)
	return ret, err
}

// Staking Contract json abi
const abiJSON = `[
  {
    "outputs": [],
    "inputs": [],
    "constant": false,
    "payable": false,
    "type": "constructor"
  },
  {
    "name": "get_deposit",
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
        "name": "validator"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function",
    "gas": 855
  },
  {
    "name": "withdraw",
    "outputs": [],
    "inputs": [],
    "constant": false,
    "payable": false,
    "type": "function",
    "gas": 75939
  },
  {
    "name": "deposit",
    "outputs": [],
    "inputs": [],
    "constant": false,
    "payable": true,
    "type": "function",
    "gas": 106877
  },
  {
    "name": "MIN_DEPOSIT_AMOUNT",
    "outputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "out"
      }
    ],
    "inputs": [],
    "constant": true,
    "payable": false,
    "type": "function",
    "gas": 821
  },
  {
    "name": "deposits",
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
        "name": "arg0"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function",
    "gas": 1005
  },
  {
    "name": "staking",
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
        "name": "arg0"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function",
    "gas": 1035
  },
  {
    "name": "start_epoch",
    "outputs": [
      {
        "type": "uint256",
        "name": "out"
      }
    ],
    "inputs": [
      {
        "type": "address",
        "name": "arg0"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function",
    "gas": 1065
  },
  {
    "name": "validators",
    "outputs": [
      {
        "type": "address",
        "name": "out"
      }
    ],
    "inputs": [
      {
        "type": "int128",
        "name": "arg0"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function",
    "gas": 1140
  }
]`
