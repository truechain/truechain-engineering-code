// Copyright 2018 The TrueChain Authors
// This file is part of the truechain-engineering-code library.
//
// The truechain-engineering-code library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The truechain-engineering-code library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the truechain-engineering-code library. If not, see <http://www.gnu.org/licenses/>.

package dashboard

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"time"
)

// fruitInfo contains the info of mine fruit.
type fruitInfo struct {
	FastNumber    ChartEntries   `json:"fastNumber,omitempty"`
	FruitUsedTime ChartEntries   `json:"fruitUsedTime,omitempty"`
	PointerNumber ChartEntries   `json:"pointerNumber,omitempty"`
	PointerHash   common.Hash    `json:"pointerHash,omitempty"`
	FruitMiner    common.Address `json:"fruitMiner,omitempty"`
}

// snailBlockInfo contains the info of mine fruit.
type snailBlockInfo struct {
	BlockNumber     ChartEntries   `json:"blockNumber,omitempty"`
	BlockUsedTime   ChartEntries   `json:"blockUsedTime,omitempty"`
	FruitsCount     ChartEntries   `json:"fruitsCount,omitempty"`
	MaxFruitNumber  ChartEntries   `json:"maxFruitNumber,omitempty"`
	BlockParentHash common.Hash    `json:"blockParentHash,omitempty"`
	BlockMiner      common.Address `json:"blockMiner,omitempty"`
}

// collectMinerData gathers data about the miner and sends it to the clients.
func (db *Dashboard) collectMinerData() {
	defer db.wg.Done()
	block := db.etrue.Miner().GetCurrentBlock()

	for {
		select {
		case errc := <-db.quit:
			errc <- nil
			return
		case <-time.After(db.config.Refresh):
			fruits := block.Fruits()
			count := len(fruits)
			time := new(big.Int).Sub(big.NewInt(time.Now().Unix()), block.Time())
			miner := block.Coinbase()
			fastNumber := block.FastNumber()
			pointerNumber := block.PointNumber()
			pointerHash := block.PointerHash()
			number := block.Number()
			parentHash := block.ParentHash()

			fruitCount := &ChartEntry{
				Value: float64(count),
			}
			useTime := &ChartEntry{
				Value: float64(time.Uint64()),
			}
			ftNumber := &ChartEntry{
				Value: float64(fastNumber.Uint64()),
			}
			pNumber := &ChartEntry{
				Value: float64(pointerNumber.Uint64()),
			}
			snailNumber := &ChartEntry{
				Value: float64(number.Uint64()),
			}
			maxFruitNumber := &ChartEntry{
				Value: float64(0),
			}
			if count >= 60 {
				maxFruitNumber = &ChartEntry{
					Value: float64(fruits[len(fruits)-1].FastNumber().Uint64()),
				}
			}

			snailBlockInfo := &snailBlockInfo{
				BlockNumber:     append([]*ChartEntry{}, snailNumber),
				BlockUsedTime:   append([]*ChartEntry{}, useTime),
				FruitsCount:     append([]*ChartEntry{}, fruitCount),
				MaxFruitNumber:  append([]*ChartEntry{}, maxFruitNumber),
				BlockParentHash: parentHash,
				BlockMiner:      miner,
			}
			fruitInfo := &fruitInfo{
				FastNumber:    append([]*ChartEntry{}, ftNumber),
				FruitUsedTime: append([]*ChartEntry{}, useTime),
				PointerNumber: append([]*ChartEntry{}, pNumber),
				PointerHash:   pointerHash,
				FruitMiner:    miner,
			}

			if len(fruits) >= 60 {
				if block.FastNumber().Cmp(fruits[len(fruits)-1].FastNumber()) > 0 {
					db.sendToAll(&Message{
						Miner: &MinerMessage{
							FruitInfo:      fruitInfo,
							SnailBlockInfo: snailBlockInfo,
						},
					})
				} else {
					db.sendToAll(&Message{
						Miner: &MinerMessage{
							SnailBlockInfo: snailBlockInfo,
						},
					})
				}
			} else {
				db.sendToAll(&Message{
					Miner: &MinerMessage{
						FruitInfo: fruitInfo,
					},
				})
			}
		}
	}
}
