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

// StakingGas defines all method gas
var StakingGas = map[string]uint64{
	"getDeposit":       120000,
	"getDelegate":      150000,
	"deposit":          800000,
	"append":           800000,
	"setFee":           800000,
	"withdraw":         840000,
	"cancel":           800000,
	"delegate":         500000,
	"undelegate":       500000,
	"withdrawDelegate": 540000,
}

// Staking contract ABI
var abiStaking abi.ABI

type StakeContract struct{}

func init() {
	abiStaking, _ = abi.JSON(strings.NewReader(StakeABIJSON))
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
	case "getDelegate":
		ret, err = getDelegate(evm, contract, data)
	case "deposit":
		ret, err = deposit(evm, contract, data)
	case "append":
		ret, err = depositAppend(evm, contract, data)
	case "withdraw":
		ret, err = withdraw(evm, contract, data)
	case "cancel":
		ret, err = cancel(evm, contract, data)
	case "setFee":
		ret, err = setFeeRate(evm, contract, data)
	case "delegate":
		ret, err = delegate(evm, contract, data)
	case "undelegate":
		ret, err = undelegate(evm, contract, data)
	case "withdrawDelegate":
		ret, err = withdrawDelegate(evm, contract, data)
	default:
		log.Warn("Staking call fallback function")
		err = ErrStakingInvalidInput
	}

	return ret, err
}

// logN add event log to receipt with topics up to 4
func logN(evm *EVM, contract *Contract, topics []common.Hash, data []byte) ([]byte, error) {
	evm.StateDB.AddLog(&types.Log{
		Address: contract.Address(),
		Topics:  topics,
		Data:    data,
		// This is a non-consensus field, but assigned here because
		// core/state doesn't know the current block number.
		BlockNumber: evm.BlockNumber.Uint64(),
	})
	return nil, nil
}


// deposit
func deposit(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	args := struct {
		Pubkey []byte
		Fee    *big.Int
	}{}
	method, _ := abiStaking.Methods["deposit"]

	err = method.Inputs.Unpack(&args, input)
	if err != nil {
		log.Error("Unpack deposit pubkey error", "err", err)
		return nil, ErrStakingInvalidInput
	}

	from := contract.caller.Address()

	log.Info("Staking deposit", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "value", contract.value)
	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	err = impawn.InsertSAccount2(evm.Context.BlockNumber.Uint64(), from, args.Pubkey, contract.value, args.Fee, true)
	if err != nil {
		log.Error("Staking deposit", "address", contract.caller.Address(), "value", contract.value, "error", err)
		return nil, err
	}

	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["Deposit"]
	logData, err := event.Inputs.PackNonIndexed(args.Pubkey, contract.value, args.Fee)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
	}
	logN(evm, contract, topics, logData)

	return nil, nil
}

func depositAppend(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	from := contract.caller.Address()

	log.Info("Staking deposit extra", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "value", contract.value)
	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	err = impawn.AppendSAAmount(evm.Context.BlockNumber.Uint64(), from, contract.value)
	if err != nil {
		log.Error("Staking deposit extra", "address", contract.caller.Address(), "value", contract.value, "error", err)
		return nil, err
	}
	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["Append"]
	logData, err := event.Inputs.PackNonIndexed(contract.value)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
	}
	logN(evm, contract, topics, logData)
	return nil, nil
}

func setFeeRate(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	fee := big.NewInt(0)
	method, _ := abiStaking.Methods["setFee"]

	err = method.Inputs.Unpack(&fee, input)
	if err != nil {
		log.Error("Unpack deposit pubkey error", "err", err)
		return nil, ErrStakingInvalidInput
	}

	from := contract.caller.Address()

	log.Info("Staking set fee", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "fee", fee)
	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	err = impawn.UpdateSAFee(evm.Context.BlockNumber.Uint64(), from, fee)
	if err != nil {
		log.Error("Staking fee", "address", contract.caller.Address(), "error", err)
		return nil, err
	}

	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["SetFee"]
	logData, err := event.Inputs.PackNonIndexed(fee)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
	}
	logN(evm, contract, topics, logData)
	return nil, nil
}

// delegate
func delegate(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	var holder common.Address

	method, _ := abiStaking.Methods["delegate"]
	err = method.Inputs.Unpack(&holder, input)
	if err != nil {
		log.Error("Unpack deposit pubkey error", "err", err)
		return nil, ErrStakingInvalidInput
	}
	from := contract.caller.Address()

	log.Info("Staking delegate", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "holder", holder, "value", contract.value)
	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}
	err = impawn.InsertDAccount2(evm.Context.BlockNumber.Uint64(), holder, from, contract.value)
	if err != nil {
		log.Error("Staking delegate", "address", contract.caller.Address(), "value", contract.value, "error", err)
		return nil, err
	}
	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["Delegate"]
	logData, err := event.Inputs.PackNonIndexed(contract.value)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
		common.BytesToHash(holder[:]),
	}
	logN(evm, contract, topics, logData)

	return nil, nil
}

// undelegate
func undelegate(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	args := struct {
		Holder common.Address
		Value  *big.Int
	}{}

	method, _ := abiStaking.Methods["undelegate"]
	err = method.Inputs.Unpack(&args, input)
	if err != nil {
		log.Error("Unpack undelegate error", "err", err)
		return nil, ErrStakingInvalidInput
	}
	from := contract.caller.Address()

	log.Info("Staking undelegate", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "holder", args.Holder, "value", args.Value)
	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}
	err = impawn.CancelDAccount(evm.Context.BlockNumber.Uint64(), args.Holder, from, args.Value)
	if err != nil {
		log.Error("Staking undelegate", "address", contract.caller.Address(), "value", args.Value, "error", err)
		return nil, err
	}

	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["Undelegate"]
	logData, err := event.Inputs.PackNonIndexed(args.Value)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
		common.BytesToHash(args.Holder[:]),
	}
	logN(evm, contract, topics, logData)
	return nil, nil
}

// cancel
func cancel(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	from := contract.caller.Address()
	amount := new(big.Int)

	method, _ := abiStaking.Methods["cancel"]
	err = method.Inputs.Unpack(&amount, input)
	if err != nil {
		log.Error("Unpack cancel input error")
		return nil, ErrStakingInvalidInput
	}

	log.Info("Staking cancel", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "value", amount)
	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}
	err = impawn.CancelSAccount(evm.Context.BlockNumber.Uint64(), from, amount)
	if err != nil {
		log.Error("Staking cancel error", "address", from, "value", amount)
		return nil, err
	}

	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["Cancel"]
	logData, err := event.Inputs.PackNonIndexed(contract.value)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
	}
	logN(evm, contract, topics, logData)

	return nil, nil
}

// withdraw
func withdraw(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	from := contract.caller.Address()
	amount := new(big.Int)

	method, _ := abiStaking.Methods["withdraw"]
	err = method.Inputs.Unpack(&amount, input)
	if err != nil {
		log.Error("Unpack withdraw input error")
		return nil, ErrStakingInvalidInput
	}

	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	log.Info("Staking withdraw", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "value", amount)
	err = impawn.RedeemSAccount(evm.Context.BlockNumber.Uint64(), from, amount)
	if err != nil {
		log.Error("Staking withdraw error", "address", from, "value", amount, "err", err)
		return nil, err
	}

	_, left, err := evm.Call(contract.self, from, nil, evm.callGasTemp, amount, nil)
	if err != nil {
		log.Info("Staking withdraw transfer failed", "err", err)
		return nil, nil
	}

	log.Info("Staking withdraw", "gas", left)
	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["Withdraw"]
	logData, err := event.Inputs.PackNonIndexed(amount)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
	}
	logN(evm, contract, topics, logData)
	return nil, nil
}

func withdrawDelegate(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	args := struct {
		Holder common.Address
		Value  *big.Int
	}{}
	from := contract.caller.Address()

	method, _ := abiStaking.Methods["withdrawDelegate"]
	err = method.Inputs.Unpack(&args, input)
	if err != nil {
		log.Error("Unpack withdraw delegate input error")
		return nil, ErrStakingInvalidInput
	}

	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	log.Info("Staking withdraw", "number", evm.Context.BlockNumber.Uint64(), "address", contract.caller.Address(), "value", args.Value)

	err = impawn.RedeemDAccount(evm.Context.BlockNumber.Uint64(), args.Holder, from, args.Value)
	if err != nil {
		log.Error("Staking withdraw delegate error", "address", from, "holer", args.Holder, "value", args.Value, "err", err)
		return nil, err
	}

	_, left, err := evm.Call(contract.self, from, nil, evm.callGasTemp, args.Value, nil)
	if err != nil {
		log.Info("Staking withdraw delegate transfer failed", "err", err)
		return nil, nil
	}

	log.Info("Staking withdraw delegate", "gas", left)
	err = impawn.Save(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking save state error", "error", err)
		return nil, err
	}

	event := abiStaking.Events["WithdrawDelegate"]
	logData, err := event.Inputs.PackNonIndexed(args.Value)
	if err != nil {
		log.Error("Pack staking log error", "error", err)
		return nil, err
	}
	topics := []common.Hash{
		event.ID(),
		common.BytesToHash(from[:]),
		common.BytesToHash(args.Holder[:]),
	}
	logN(evm, contract, topics, logData)
	return nil, nil
}

// getDeposit
func getDeposit(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	var depositAddr common.Address
	method, _ := abiStaking.Methods["getDeposit"]
	var (
		staked   = big.NewInt(0)
		locked   = big.NewInt(0)
		unlocked = big.NewInt(0)
	)

	err = method.Inputs.Unpack(&depositAddr, input)
	if err != nil {
		log.Error("Unpack get_deposit input error")
		return nil, ErrStakingInvalidInput
	}

	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	asset := impawn.GetAllCancelableAsset(depositAddr)
	if stake, ok := asset[depositAddr]; ok {
		staked.Add(staked, stake)
	}

	lockedAsset := impawn.GetLockedAsset2(depositAddr, evm.Context.BlockNumber.Uint64())
	if stake, ok := lockedAsset[depositAddr]; ok {
		for _, item := range stake.Value {
			if item.Locked {
				locked.Add(locked, item.Amount)
			} else {
				unlocked.Add(unlocked, item.Amount)
			}
		}
	}

	log.Info("Get staking get_deposit", "address", depositAddr, "staked", staked, "locked", locked, "unlocked", unlocked)

	ret, err = method.Outputs.Pack(staked, locked, unlocked)
	return ret, err
}

func getDelegate(evm *EVM, contract *Contract, input []byte) (ret []byte, err error) {
	args := struct {
		Owner  common.Address
		Holder common.Address
	}{}
	method, _ := abiStaking.Methods["getDelegate"]
	var (
		staked   = big.NewInt(0)
		locked   = big.NewInt(0)
		unlocked = big.NewInt(0)
	)

	err = method.Inputs.Unpack(&args, input)
	if err != nil {
		log.Error("Unpack get_deposit input error")
		return nil, ErrStakingInvalidInput
	}

	impawn := NewImpawnImpl()
	err = impawn.Load(evm.StateDB, types.StakingAddress)
	if err != nil {
		log.Error("Staking load error", "error", err)
		return nil, err
	}

	asset := impawn.GetAllCancelableAsset(args.Owner)
	if stake, ok := asset[args.Holder]; ok {
		staked.Add(staked, stake)
	}

	lockedAsset := impawn.GetLockedAsset2(args.Owner, evm.Context.BlockNumber.Uint64())
	if stake, ok := lockedAsset[args.Holder]; ok {
		for _, item := range stake.Value {
			if item.Locked {
				locked.Add(locked, item.Amount)
			} else {
				unlocked.Add(unlocked, item.Amount)
			}
		}
	}

	log.Info("Get staking get_delegate", "address", args.Owner, "holder", args.Holder,
		"staked", staked, "locked", locked, "unlocked", unlocked)

	ret, err = method.Outputs.Pack(staked, locked, unlocked)
	return ret, err
}

// Staking Contract json abi
const StakeABIJSON = `
[
  {
    "name": "Deposit",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "bytes",
        "name": "pubkey",
        "indexed": false
      },
      {
        "type": "uint256",
        "name": "value",
        "indexed": false
      },
      {
        "type": "uint256",
        "name": "fee",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "Delegate",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "address",
        "name": "holder",
        "indexed": true
      },
      {
        "type": "uint256",
        "name": "value",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "Undelegate",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "address",
        "name": "holder",
        "indexed": true
      },
      {
        "type": "uint256",
        "name": "value",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "WithdrawDelegate",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "address",
        "name": "holder",
        "indexed": true
      },
      {
        "type": "uint256",
        "name": "value",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "Cancel",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "uint256",
        "name": "value",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "Withdraw",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "uint256",
        "name": "value",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "Append",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "uint256",
        "name": "value",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "SetFee",
    "inputs": [
      {
        "type": "address",
        "name": "from",
        "indexed": true
      },
      {
        "type": "uint256",
        "name": "fee",
        "indexed": false
      }
    ],
    "anonymous": false,
    "type": "event"
  },
  {
    "name": "deposit",
    "outputs": [],
    "inputs": [
      {
        "type": "bytes",
        "name": "pubkey"
      },
      {
        "type": "uint256",
        "name": "fee"
      }
    ],
    "constant": false,
    "payable": true,
    "type": "function"
  },
  {
    "name": "setFee",
    "outputs": [],
    "inputs": [
      {
        "type": "uint256",
        "name": "fee"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  },
  {
    "name": "append",
    "outputs": [],
    "inputs": [],
    "constant": false,
    "payable": true,
    "type": "function"
  },
  {
    "name": "delegate",
    "outputs": [],
    "inputs": [
      {
        "type": "address",
        "name": "holder"
      }
    ],
    "constant": false,
    "payable": true,
    "type": "function"
  },
  {
    "name": "undelegate",
    "outputs": [],
    "inputs": [
      {
        "type": "address",
        "name": "holder"
      },
      {
        "type": "uint256",
        "unit": "wei",
        "name": "value"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  },
  {
    "name": "getDeposit",
    "outputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "staked"
      },
      {
        "type": "uint256",
        "unit": "wei",
        "name": "locked"
      },
      {
        "type": "uint256",
        "unit": "wei",
        "name": "unlocked"
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
    "type": "function"
  },
  {
    "name": "getDelegate",
    "outputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "delegated"
      },
      {
        "type": "uint256",
        "unit": "wei",
        "name": "locked"
      },
      {
        "type": "uint256",
        "unit": "wei",
        "name": "unlocked"
      }
    ],
    "inputs": [
      {
        "type": "address",
        "name": "owner"
      },
      {
        "type": "address",
        "name": "holder"
      }
    ],
    "constant": true,
    "payable": false,
    "type": "function"
  },
  {
    "name": "cancel",
    "outputs": [],
    "inputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "value"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  },
  {
    "name": "withdraw",
    "outputs": [],
    "inputs": [
      {
        "type": "uint256",
        "unit": "wei",
        "name": "value"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  },
  {
    "name": "withdrawDelegate",
    "outputs": [],
    "inputs": [
      {
        "type": "address",
        "name": "holder"
      },
      {
        "type": "uint256",
        "unit": "wei",
        "name": "value"
      }
    ],
    "constant": false,
    "payable": false,
    "type": "function"
  }
]
`
