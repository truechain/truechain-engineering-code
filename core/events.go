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

package core

import (
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
)

// NewTxsEvent is posted when a batch of transactions enter the transaction pool.
type NewTxsEvent struct{ Txs []*types.Transaction }

// AddTxEvent is posted when a transactions remove from the transaction pool.
type AddTxEvent struct{ Tx *types.Transaction }

// RemoveTxEvent is posted when a transactions remove from the transaction pool.
type RemoveTxEvent struct{ Hash common.Hash }

//for fruit and record
type NewFruitsEvent struct{ Fruits []*types.SnailBlock }

// PendingLogsEvent is posted pre mining and notifies of pending logs.
type PendingLogsEvent struct {
	Logs []*types.Log
}

// PendingStateEvent is posted pre mining and notifies of pending state changes.
type PendingStateEvent struct{}

type NewMinedSnailBlockEvent struct{ Block *types.SnailBlock }

// NewMinedFruitEvent is posted when a block has been imported.
type NewMinedFruitEvent struct{ Block *types.SnailBlock }

// RemovedLogsEvent is posted when a reorg happens
type RemovedLogsEvent struct{ Logs []*types.Log }

type ChainEvent struct {
	Block *types.Block
	Hash  common.Hash
	Logs  []*types.Log
}

type ChainSideEvent struct {
	Block *types.Block
}

type ChainHeadEvent struct{ Block *types.Block }

type FastBlockEvent struct {
	Block    *types.Block
	Receipts map[common.Hash]*types.Receipt
}

// for fruit event
type FruitEvent struct {
	Block *types.Block
	Hash  common.Hash
	Logs  []*types.Log
}

type ElectionEvent struct {
	Option           uint
	CommitteeID      *big.Int
	CommitteeMembers []*types.CommitteeMember
	BeginFastNumber  *big.Int
	EndFastNumber    *big.Int
	//CommitteeInfo *types.CommitteeInfo
}

type PbftSignEvent struct {
	Block    *types.Block
	PbftSign *types.PbftSign
}

// NewBlockEvent is posted when a block has been generate .
type NewBlockEvent struct{ Block *types.Block }

// NodeInfoEvent is posted when nodeInfo send
type NodeInfoEvent struct{ NodeInfo *types.EncryptNodeMessage }

// StateChangeEvent hold the result of the balance change
type StateChangeEvent struct {
	Height   uint64
	Balances []*common.AddressWithBalance
}

// RewardsEvent hold the amount of rewards with Height and Hash of snail block.
type RewardsEvent struct {
	Height  uint64
	Hash    common.Hash
	Rewards []*common.AddressWithBalance
}
