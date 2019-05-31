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
	"github.com/truechain/truechain-engineering-code/metrics"
	"time"
)

// standardCounterCollector returns a function, which retrieves the count of a specific collector.
func standardCounterCollector(name string) func() int64 {
	if collector := metrics.Get(name); collector != nil {
		m := collector.(metrics.Counter)
		return func() int64 {
			return m.Count()
		}
	}
	return func() int64 {
		return 0
	}
}

// collectTxpoolData gathers data about the tx_pool and sends it to the clients.
func (db *Dashboard) collectTxpoolData() {
	defer db.wg.Done()
	txpool := db.etrue.TxPool()
	pending, queued := txpool.Stats()
	var (
		// Metrics for the pending pool
		pendingDiscardCounter   = standardCounterCollector("txpool/pending/discard")
		pendingReplaceCounter   = standardCounterCollector("txpool/pending/replace")
		pendingRateLimitCounter = standardCounterCollector("txpool/pending/ratelimit") // Dropped due to rate limiting
		pendingNofundsCounter   = standardCounterCollector("txpool/pending/nofunds")   // Dropped due to out-of-funds

		// Metrics for the queued pool
		queuedDiscardCounter   = standardCounterCollector("txpool/queued/discard")
		queuedReplaceCounter   = standardCounterCollector("txpool/queued/replace")
		queuedRateLimitCounter = standardCounterCollector("txpool/queued/ratelimit") // Dropped due to rate limiting
		queuedNofundsCounter   = standardCounterCollector("txpool/queued/nofunds")   // Dropped due to out-of-funds

		// General tx metrics
		invalidTxCounter     = standardCounterCollector("txpool/invalid")
		underpricedTxCounter = standardCounterCollector("txpool/underpriced")

		// Metrics for the send to handler
		promotedSend = standardCounterCollector("txpool/send/promoted")
		replacedSend = standardCounterCollector("txpool/send/replaced")
	)

	for {
		select {
		case errc := <-db.quit:
			errc <- nil
			return
		case <-time.After(db.config.Refresh):

			txStatusQueued := &ChartEntry{
				Value: float64(queued),
			}
			txStatusPending := &ChartEntry{
				Value: float64(pending),
			}
			pendingDiscardCounter := &ChartEntry{
				Value: float64(pendingDiscardCounter()),
			}
			pendingReplaceCounter := &ChartEntry{
				Value: float64(pendingReplaceCounter()),
			}
			pendingRateLimitCounter := &ChartEntry{
				Value: float64(pendingRateLimitCounter()),
			}
			pendingNofundsCounter := &ChartEntry{
				Value: float64(pendingNofundsCounter()),
			}
			queuedDiscardCounter := &ChartEntry{
				Value: float64(queuedDiscardCounter()),
			}
			queuedReplaceCounter := &ChartEntry{
				Value: float64(queuedReplaceCounter()),
			}
			queuedRateLimitCounter := &ChartEntry{
				Value: float64(queuedRateLimitCounter()),
			}
			queuedNofundsCounter := &ChartEntry{
				Value: float64(queuedNofundsCounter()),
			}
			invalidTxCounter := &ChartEntry{
				Value: float64(invalidTxCounter()),
			}
			underpricedTxCounter := &ChartEntry{
				Value: float64(underpricedTxCounter()),
			}
			promotedSend := &ChartEntry{
				Value: float64(promotedSend()),
			}
			replacedSend := &ChartEntry{
				Value: float64(replacedSend()),
			}
			db.txPoolLock.Lock()
			txPool := db.history.TxPool
			txPool.TxStatusQueued = append(txPool.TxStatusQueued[1:], txStatusQueued)
			txPool.TxStatusPending = append(txPool.TxStatusPending[1:], txStatusPending)
			txPool.PendingDiscardCounter = append(txPool.PendingDiscardCounter[1:], pendingDiscardCounter)
			txPool.PendingReplaceCounter = append(txPool.PendingReplaceCounter[1:], pendingReplaceCounter)
			txPool.PendingRateLimitCounter = append(txPool.PendingRateLimitCounter[1:], pendingRateLimitCounter)
			txPool.PendingNofundsCounter = append(txPool.PendingNofundsCounter[1:], pendingNofundsCounter)
			txPool.QueuedDiscardCounter = append(txPool.QueuedDiscardCounter[1:], queuedDiscardCounter)
			txPool.QueuedReplaceCounter = append(txPool.QueuedReplaceCounter[1:], queuedReplaceCounter)
			txPool.QueuedRateLimitCounter = append(txPool.QueuedRateLimitCounter[1:], queuedRateLimitCounter)
			txPool.QueuedNofundsCounter = append(txPool.QueuedNofundsCounter[1:], queuedNofundsCounter)
			txPool.InvalidTxCounter = append(txPool.InvalidTxCounter[1:], invalidTxCounter)
			txPool.UnderpricedTxCounter = append(txPool.UnderpricedTxCounter[1:], underpricedTxCounter)
			txPool.PromotedSend = append(txPool.PromotedSend[1:], promotedSend)
			txPool.ReplacedSend = append(txPool.ReplacedSend[1:], replacedSend)
			db.txPoolLock.Unlock()

			db.sendToAll(&Message{
				TxPool: &TxPoolMessage{
					PendingDiscardCounter:   ChartEntries{pendingDiscardCounter},
					PendingReplaceCounter:   ChartEntries{pendingReplaceCounter},
					PendingRateLimitCounter: ChartEntries{pendingRateLimitCounter},
					PendingNofundsCounter:   ChartEntries{pendingNofundsCounter},
					QueuedDiscardCounter:    ChartEntries{queuedDiscardCounter},
					QueuedReplaceCounter:    ChartEntries{queuedReplaceCounter},
					QueuedRateLimitCounter:  ChartEntries{queuedRateLimitCounter},
					QueuedNofundsCounter:    ChartEntries{queuedNofundsCounter},
					InvalidTxCounter:        ChartEntries{invalidTxCounter},
					UnderpricedTxCounter:    ChartEntries{underpricedTxCounter},
					PromotedSend:            ChartEntries{promotedSend},
					ReplacedSend:            ChartEntries{replacedSend},
				},
			})
		}
	}
}
