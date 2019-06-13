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
	"time"
)

// fastChainInfo contains the info of fast chain.
type fastChainInfo struct {
	LastFastTime ChartEntries `json:"lastFastTime,omitempty"`
	LastTxsCount ChartEntries `json:"lastTxsCount,omitempty"`
	GasSpending  ChartEntries `json:"gasSpending,omitempty"`
	GasLimit     ChartEntries `json:"gasLimit,omitempty"`
}

// snailChainInfo contains the info of snail chain.
type snailChainInfo struct {
	LastSnailTime       ChartEntries   `json:"lastSnailTime,omitempty"`       // last block's mined time of the snail chain.
	LastSnailDifficulty ChartEntries   `json:"lastSnailDifficulty,omitempty"` // last block's difficulty of the snail chain.
	LastMaxFruitNumber  ChartEntries   `json:"lastMaxFruitNumber,omitempty"`  // the max fruit number in the last snail block.
	LastFruitsCount     ChartEntries   `json:"lastFruitsCount,omitempty"`     // the fruits count in the last snail block.
	LastMiner           common.Address `json:"lastMiner,omitempty"`           // last block's miner address of the snail chain.
}

// collectTxpoolData gathers data about the tx_pool and sends it to the clients.
func (db *Dashboard) collectChainData() {
	defer db.wg.Done()
	fastchain := db.etrue.BlockChain()
	snailchain := db.etrue.SnailBlockChain()

	for {
		select {
		case errc := <-db.quit:
			errc <- nil
			return
		case <-time.After(db.config.Refresh):
			lastSnailTime := snailchain.CurrentHeader().Time
			lastSnailDifficulty := snailchain.CurrentHeader().Difficulty
			lastMaxFruitNumber := snailchain.CurrentBlock().Body().Fruits[len(snailchain.CurrentBlock().Body().Fruits)-1].Number()
			lastFruitsCount := len(snailchain.CurrentBlock().Body().Fruits)
			lastMiner := snailchain.CurrentHeader().Coinbase
			snailTime := &ChartEntry{
				Value: float64(lastSnailTime.Uint64()),
			}
			snailDifficulty := &ChartEntry{
				Value: float64(lastSnailDifficulty.Uint64()),
			}
			maxFruitNumber := &ChartEntry{
				Value: float64(lastMaxFruitNumber.Uint64()),
			}
			fruitsCount := &ChartEntry{
				Value: float64(lastFruitsCount),
			}
			snailChainInfo := &snailChainInfo{
				LastSnailTime:       append([]*ChartEntry{}, snailTime),
				LastSnailDifficulty: append([]*ChartEntry{}, snailDifficulty),
				LastMaxFruitNumber:  append([]*ChartEntry{}, maxFruitNumber),
				LastFruitsCount:     append([]*ChartEntry{}, fruitsCount),
				LastMiner:           lastMiner,
			}

			lastFastTime := fastchain.CurrentHeader().Time
			lastTxsCount := len(fastchain.CurrentBlock().Body().Transactions)
			gasSpending := fastchain.CurrentBlock().GasUsed()
			gasLimit := fastchain.CurrentBlock().GasLimit()
			fastTime := &ChartEntry{
				Value: float64(lastFastTime.Uint64()),
			}
			txsCount := &ChartEntry{
				Value: float64(lastTxsCount),
			}
			spending := &ChartEntry{
				Value: float64(gasSpending),
			}
			limit := &ChartEntry{
				Value: float64(gasLimit),
			}
			fastChainInfo := &fastChainInfo{
				LastFastTime: append([]*ChartEntry{}, fastTime),
				LastTxsCount: append([]*ChartEntry{}, txsCount),
				GasSpending:  append([]*ChartEntry{}, spending),
				GasLimit:     append([]*ChartEntry{}, limit),
			}

			db.chainLock.Lock()
			db.history.Chain = &ChainMessage{
				FastChain:  fastChainInfo,
				SnailChain: snailChainInfo,
			}
			db.chainLock.Unlock()

			db.sendToAll(&Message{
				Chain: &ChainMessage{
					FastChain:  fastChainInfo,
					SnailChain: snailChainInfo,
				},
			})
		}
	}
}
