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
	"errors"
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

// StakingAddress is defined as Address('truestaking')
// 0x000000000000000000747275657374616b696E67
var StakingAddress = common.BytesToAddress([]byte("truestaking"))

type StakeContract struct {}

func RunStaking(evm *EVM, input []byte) (ret []byte, err error) {
	if len(input) < 4 {
		return nil, errors.New("input data invalid")
	}

	var selector []byte = input[:4]

	if bytes.Equal(selector, common.Hex2Bytes("d0e30db0")) {
		log.Info("Call staking deposit")
	} else if bytes.Equal(selector, common.Hex2Bytes("3ccfd60b")) {
		log.Info("Call staking withdraw")
	} else if bytes.Equal(selector, common.Hex2Bytes("f4607feb")) {
		log.Info("Call staking get_deposit")
		balance := evm.StateDB.GetBalance(common.BytesToAddress(input[4:36])).Bytes()
		ret = make([]byte, 32)
		copy(ret[32-len(balance):], balance)
	} else {
		log.Error("Call staking default")
	}
	return ret, nil
}
// deposit MethodId  0xd0e30db0
func deposit(evm *EVM, input []byte) (ret []byte, err error) {
	return nil, nil
}

// withdraw MethodId "0x3ccfd60b"
func withdraw(evm *EVM, input []byte) (ret []byte, err error) {
	return nil, nil
}

// getDeposit MethodId 0xf4607feb
func getDeposit(evm *EVM, input []byte) (ret []byte, err error) {
	log.Info("Call staking get_deposit")
	balance := evm.StateDB.GetBalance(common.BytesToAddress(input[4:36])).Bytes()
	ret = make([]byte, 32)
	copy(ret[32-len(balance):], balance)
	return ret, nil
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