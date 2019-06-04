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
	"time"
)

// collectTxpoolData gathers data about the tx_pool and sends it to the clients.
func (db *Dashboard) collectFruitpoolData() {
	defer db.wg.Done()
	fruitpool := db.etrue.SnailPool()
	pending, queued := fruitpool.Stats()
	var (
		// Metrics for the pending pool
		fruitPendingDiscardCounter = standardCounterCollector("fruitpool/pending/discard")
		fruitpendingReplaceCounter = standardCounterCollector("fruitpool/pending/replace")

		// Metrics for the allfruit pool
		allDiscardCounter = standardCounterCollector("fruitpool/all/discard")
		allReplaceCounter = standardCounterCollector("fruitpool/all/replace")

		// Metrics for the received fruits
		allReceivedCounter = standardCounterCollector("fruitpool/received/count")
		allTimesCounter    = standardCounterCollector("fruitpool/received/times")
		allFilterCounter   = standardCounterCollector("fruitpool/received/filter")
		allMinedCounter    = standardCounterCollector("fruitpool/received/mined")

		// Metrics for the received fruits
		allSendCounter      = standardCounterCollector("fruitpool/send/count")
		allSendTimesCounter = standardCounterCollector("fruitpool/send/times")
	)

	for {
		select {
		case errc := <-db.quit:
			errc <- nil
			return
		case <-time.After(db.config.Refresh):

			ftStatusQueued := &ChartEntry{
				Value: float64(queued),
			}
			ftStatusPending := &ChartEntry{
				Value: float64(pending),
			}
			fruitPendingDiscardCounter := &ChartEntry{
				Value: float64(fruitPendingDiscardCounter()),
			}
			fruitpendingReplaceCounter := &ChartEntry{
				Value: float64(fruitpendingReplaceCounter()),
			}
			allDiscardCounter := &ChartEntry{
				Value: float64(allDiscardCounter()),
			}
			allReplaceCounter := &ChartEntry{
				Value: float64(allReplaceCounter()),
			}
			allReceivedCounter := &ChartEntry{
				Value: float64(allReceivedCounter()),
			}
			allTimesCounter := &ChartEntry{
				Value: float64(allTimesCounter()),
			}
			allFilterCounter := &ChartEntry{
				Value: float64(allFilterCounter()),
			}
			allMinedCounter := &ChartEntry{
				Value: float64(allMinedCounter()),
			}
			allSendCounter := &ChartEntry{
				Value: float64(allSendCounter()),
			}
			allSendTimesCounter := &ChartEntry{
				Value: float64(allSendTimesCounter()),
			}

			db.fruitPoolLock.Lock()
			ftPool := db.history.FtPool
			ftPool.FtStatusQueued = append(ftPool.FtStatusQueued[1:], ftStatusQueued)
			ftPool.FtStatusPending = append(ftPool.FtStatusPending[1:], ftStatusPending)
			ftPool.FruitPendingDiscardCounter = append(ftPool.FruitPendingDiscardCounter[1:], fruitPendingDiscardCounter)
			ftPool.FruitpendingReplaceCounter = append(ftPool.FruitpendingReplaceCounter[1:], fruitpendingReplaceCounter)
			ftPool.AllDiscardCounter = append(ftPool.AllDiscardCounter[1:], allDiscardCounter)
			ftPool.AllReplaceCounter = append(ftPool.AllReplaceCounter[1:], allReplaceCounter)
			ftPool.AllReceivedCounter = append(ftPool.AllReceivedCounter[1:], allReceivedCounter)
			ftPool.AllTimesCounter = append(ftPool.AllTimesCounter[1:], allTimesCounter)
			ftPool.AllFilterCounter = append(ftPool.AllFilterCounter[1:], allFilterCounter)
			ftPool.AllMinedCounter = append(ftPool.AllMinedCounter[1:], allMinedCounter)
			ftPool.AllSendCounter = append(ftPool.AllSendCounter[1:], allSendCounter)
			ftPool.AllSendTimesCounter = append(ftPool.AllSendTimesCounter[1:], allSendTimesCounter)
			db.fruitPoolLock.Unlock()

			db.sendToAll(&Message{
				FtPool: &FtPoolMessage{
					FtStatusQueued:             ChartEntries{ftStatusQueued},
					FtStatusPending:            ChartEntries{ftStatusPending},
					FruitPendingDiscardCounter: ChartEntries{fruitPendingDiscardCounter},
					FruitpendingReplaceCounter: ChartEntries{fruitpendingReplaceCounter},
					AllDiscardCounter:          ChartEntries{allDiscardCounter},
					AllReplaceCounter:          ChartEntries{allReplaceCounter},
					AllReceivedCounter:         ChartEntries{allReceivedCounter},
					AllTimesCounter:            ChartEntries{allTimesCounter},
					AllFilterCounter:           ChartEntries{allFilterCounter},
					AllMinedCounter:            ChartEntries{allMinedCounter},
					AllSendCounter:             ChartEntries{allSendCounter},
					AllSendTimesCounter:        ChartEntries{allSendTimesCounter},
				},
			})
		}
	}
}
