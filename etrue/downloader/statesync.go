// Copyright 2017 The go-ethereum Authors
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

package downloader

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	etrue "github.com/truechain/truechain-engineering-code/etrue/types"
)





// syncState starts downloading state with the given root hash.
func (d *Downloader) SyncState(root common.Hash) *etrue.StateSync {
	s := etrue.NewStateSync(d, root)
	select {
	case d.stateSyncStart <- s:
	case <-d.quitCh:
		s.Err = etrue.ErrCancelStateFetch
		close(s.Done)
	}
	return s
}

// stateFetcher manages the active state sync and accepts requests
// on its behalf.
func (d *Downloader) stateFetcher() {
	for {
		select {
		case s := <-d.stateSyncStart:
			for next := s; next != nil; {
				next = d.runStateSync(next)
			}
		case <-d.stateCh:
			// Ignore state responses while no sync is running.
		case <-d.quitCh:
			return
		}
	}
}

// runStateSync runs a state synchronisation until it completes or another root
// hash is requested to be switched over to.
func (d *Downloader) runStateSync(s *etrue.StateSync) *etrue.StateSync {
	var (
		active   = make(map[string]*etrue.StateReq) // Currently in-flight requests
		finished []*etrue.StateReq                  // Completed or failed requests
		timeout  = make(chan *etrue.StateReq)       // Timed out active requests
	)
	defer func() {
		// Cancel active request timers on exit. Also set peers to idle so they're
		// available for the next sync.
		for _, req := range active {
			req.Timer.Stop()
			req.Peer.SetNodeDataIdle(len(req.Items))
		}
	}()
	// Run the state sync.
	go s.Run()
	defer s.Cancel()

	// Listen for peer departure events to cancel assigned tasks
	peerDrop := make(chan etrue.PeerConnection, 1024)
	peerSub := s.D.Peers.SubscribePeerDrops(peerDrop)
	defer peerSub.Unsubscribe()

	for {
		// Enable sending of the first buffered element if there is one.
		var (
			deliverReq   *etrue.StateReq
			deliverReqCh chan *etrue.StateReq
		)
		if len(finished) > 0 {
			deliverReq = finished[0]
			deliverReqCh = s.Deliver
		}

		select {
		// The stateSync lifecycle:
		case next := <-d.stateSyncStart:
			return next

		case <-s.Done:
			return nil

		// Send the next finished request to the current sync:
		case deliverReqCh <- deliverReq:
			log.Debug("deliverReqCh ==== ","deliverReq",deliverReq.Items[0])
			// Shift out the first request, but also set the emptied slot to nil for GC
			copy(finished, finished[1:])
			finished[len(finished)-1] = nil
			finished = finished[:len(finished)-1]

		// Handle incoming state packs:
		case pack := <-d.stateCh:
			// Discard any data not requested (or previously timed out)
			req := active[pack.PeerId()]
			if req == nil {
				log.Debug("Unrequested node data", "peer", pack.PeerId(), "len", pack.Items())
				continue
			}
			// Finalize the request and queue up for processing
			req.Timer.Stop()
			req.Response = pack.(*statePack).states

			finished = append(finished, req)
			delete(active, pack.PeerId())

			// Handle dropped peer connections:
		case p := <-peerDrop:
			// Skip if no request is currently pending
			req := active[p.GetID()]
			if req == nil {
				continue
			}
			// Finalize the request and queue up for processing
			req.Timer.Stop()
			req.Dropped = true

			finished = append(finished, req)
			delete(active, p.GetID())

		// Handle timed-out requests:
		case req := <-timeout:
			// If the peer is already requesting something else, ignore the stale timeout.
			// This can happen when the timeout and the delivery happens simultaneously,
			// causing both pathways to trigger.
			if active[req.Peer.GetID()] != req {
				continue
			}
			// Move the timed out data back into the download queue
			finished = append(finished, req)
			delete(active, req.Peer.GetID())

		// Track outgoing state requests:
		case req := <-d.TrackStateReq:
			// If an active request already exists for this peer, we have a problem. In
			// theory the trie node schedule must never assign two requests to the same
			// peer. In practice however, a peer might receive a request, disconnect and
			// immediately reconnect before the previous times out. In this case the first
			// request is never honored, alas we must not silently overwrite it, as that
			// causes valid requests to go missing and sync to get stuck.
			if old := active[req.Peer.GetID()]; old != nil {
				log.Warn("Busy peer assigned new state fetch", "peer", old.Peer.GetID())

				// Make sure the previous one doesn't get siletly lost
				old.Timer.Stop()
				old.Dropped = true

				finished = append(finished, old)
			}
			// Start a timer to notify the sync loop if the peer stalled.
			req.Timer = time.AfterFunc(req.Timeout, func() {
				select {
				case timeout <- req:
				case <-s.Done:
					// Prevent leaking of timer goroutines in the unlikely case where a
					// timer is fired just before exiting runStateSync.
				}
			})
			active[req.Peer.GetID()] = req
		}
	}
}
