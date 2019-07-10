// Copyright 2016 The go-ethereum Authors
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

// Package les implements the Light Ethereum Subprotocol.
package les

import (
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/truechain/truechain-engineering-code/light/fast"
	"github.com/truechain/truechain-engineering-code/light/public"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
)

const (
	hashLimit = 512 // Maximum number of unique blocks a peer may have announced
)

// fastLightFetcher implements retrieval of newly announced headers. It also provides a peerHasBlock function for the
// ODR system to ensure that we only request data related to a certain block from peers who have already processed
// and announced that block.
type fastLightFetcher struct {
	pm    *ProtocolManager
	chain *fast.LightChain

	lock     sync.Mutex // lock protects access to the fetcher's internal state variables except sent requests
	peers    map[*peer]*fastPeerInfo
	syncing  bool
	syncDone chan *peer

	reqMu      sync.RWMutex // reqMu protects access to sent header fetch requests
	requested  map[uint64]fetchFastRequest
	deliverChn chan fetchFastResponse
	timeoutChn chan uint64
	requestChn chan bool // true if initiated from outside
	// head cache
	queue  *prque.Prque            // Queue containing the import operations (head number sorted)
	queued map[common.Hash]*inject // Set of already queued heads (to dedupe imports)
}

// fastPeerInfo holds fetcher-specific information about each active peer
type fastPeerInfo struct {
	firstAnnounced, lastAnnounced *announce
	nodeByHash                    map[common.Hash]*announce
}

// announce is the hash notification of the availability of a new block in the
// network.
type announce struct {
	hash      common.Hash // Hash of the block being announced
	number    uint64      // Number of the block being announced (0 = unknown | old protocol)
	requested bool
	origin    string // Identifier of the peer originating the notification
}

// inject represents a schedules import operation.
type inject struct {
	origin string
	header *types.Header
	signs  []*types.PbftSign
}

// fetchFastRequest represents a header download request
type fetchFastRequest struct {
	hash    common.Hash
	amount  uint64
	peer    *peer
	sent    mclock.AbsTime
	timeout bool
}

// fetchFastResponse represents a header download response
type fetchFastResponse struct {
	reqID uint64
	peer  *peer
	hss   *headsWithSigns
}

// newLightFetcher creates a new light fetcher
func newFastLightFetcher(pm *ProtocolManager) *fastLightFetcher {
	f := &fastLightFetcher{
		pm:         pm,
		chain:      pm.fblockchain.(*fast.LightChain),
		peers:      make(map[*peer]*fastPeerInfo),
		deliverChn: make(chan fetchFastResponse, 100),
		requested:  make(map[uint64]fetchFastRequest),
		timeoutChn: make(chan uint64),
		requestChn: make(chan bool, 100),
		syncDone:   make(chan *peer),
		queue:      prque.New(nil),
		queued:     make(map[common.Hash]*inject),
	}
	pm.peers.notify(f)

	f.pm.wg.Add(1)
	go f.syncLoop()
	return f
}

// syncLoop is the main event loop of the light fetcher
func (f *fastLightFetcher) syncLoop() {
	requesting := false
	defer f.pm.wg.Done()
	for {
		select {
		case <-f.pm.quitSync:
			return
		// when a new announce is received, request loop keeps running until
		// no further requests are necessary or possible
		case newAnnounce := <-f.requestChn:
			f.lock.Lock()
			s := requesting
			requesting = false
			var (
				rq      *distReq
				reqID   uint64
				syncing bool
			)
			if !f.syncing && !(newAnnounce && s) {
				rq, reqID, syncing = f.nextRequest()
			}
			f.lock.Unlock()

			if syncing {
				f.lock.Lock()
				f.syncing = true
				f.lock.Unlock()
			}
			if rq != nil {
				requesting = true
				if _, ok := <-f.pm.reqDist.queue(rq); ok {
					if !syncing {
						go func() {
							time.Sleep(softRequestTimeout)
							f.reqMu.Lock()
							req, ok := f.requested[reqID]
							if ok {
								req.timeout = true
								f.requested[reqID] = req
							}
							f.reqMu.Unlock()
							// keep starting new requests while possible
							f.requestChn <- false
						}()
					}
				} else {
					f.requestChn <- false
				}
			}
		case reqID := <-f.timeoutChn:
			f.reqMu.Lock()
			req, ok := f.requested[reqID]
			if ok {
				delete(f.requested, reqID)
			}
			f.reqMu.Unlock()
			if ok {
				f.pm.serverPool.adjustResponseTime(req.peer.poolEntry, time.Duration(mclock.Now()-req.sent), true)
				req.peer.Log().Debug("Fetching fast data timed out hard")
				go f.pm.removePeer(req.peer.id, public.FetcherFastTimerCall)
			}
		case resp := <-f.deliverChn:
			f.reqMu.Lock()
			req, ok := f.requested[resp.reqID]
			if ok && req.peer != resp.peer {
				ok = false
			}
			if ok {
				delete(f.requested, resp.reqID)
			}
			f.reqMu.Unlock()
			if ok {
				f.pm.serverPool.adjustResponseTime(req.peer.poolEntry, time.Duration(mclock.Now()-req.sent), req.timeout)
			}
			f.lock.Lock()
			if !ok || !(f.syncing || f.processResponse(req, resp)) {
				resp.peer.Log().Debug("Failed processing response")
				go f.pm.removePeer(resp.peer.id, public.FetcherDeliverCall)
			}
			f.lock.Unlock()
		case p := <-f.syncDone:
			f.lock.Lock()
			p.Log().Debug("Done fast synchronising with peer")
			f.syncing = false
			f.lock.Unlock()
			f.checkQueueHeight()
			f.requestChn <- false
		}
	}
}

// registerPeer adds a new peer to the fetcher's peer set
func (f *fastLightFetcher) registerPeer(p *peer) {
	p.lock.Lock()
	p.hasBlock = func(hash common.Hash, number uint64, hasState bool) bool {
		return f.peerHasBlock(p, hash, number, hasState)
	}
	p.lock.Unlock()

	f.lock.Lock()
	defer f.lock.Unlock()

	f.peers[p] = &fastPeerInfo{nodeByHash: make(map[common.Hash]*announce)}
}

// unregisterPeer removes a new peer from the fetcher's peer set
func (f *fastLightFetcher) unregisterPeer(p *peer) {
	p.lock.Lock()
	p.hasBlock = nil
	p.lock.Unlock()

	f.lock.Lock()
	defer f.lock.Unlock()

	delete(f.peers, p)
}

// announce processes a new announcement message received from a peer, adding new
// nodes to the peer's block tree and removing old nodes if necessary
func (f *fastLightFetcher) announce(p *peer, head *announceData) {
	f.lock.Lock()
	defer f.lock.Unlock()

	fp := f.peers[p]
	if fp == nil {
		p.Log().Debug("Announcement from unknown peer")
		return
	}

	if fp.lastAnnounced != nil && head.FastNumber <= fp.lastAnnounced.number {
		if head.FastNumber == fp.lastAnnounced.number && head.FastHash == fp.lastAnnounced.hash {
			log.Info("Announce duplicated", "number", head.FastNumber, "hash", head.FastHash)
			return
		}
		// announced tds should be strictly monotonic
		p.Log().Debug("Received non-monotonic td", "current", head.FastNumber, "previous", fp.lastAnnounced.number)
		go f.pm.removePeer(p.id, public.FetcherAnnounceCall)
		return
	}

	current := f.chain.CurrentHeader().Number.Uint64()

	n := &announce{hash: head.FastHash, number: head.FastNumber, origin: p.id}
	if fp.firstAnnounced == nil {
		fp.firstAnnounced = n
	}
	fp.nodeByHash[n.hash] = n

	p.Log().Debug("Received fast new announcement", "number", head.FastNumber, "hash", head.FastHash, "current", current)

	f.checkKnownNode(fp, n)
	p.lock.Lock()
	p.headInfo.FastNumber, p.headInfo.FastHash = head.FastNumber, head.FastHash
	fp.lastAnnounced = n
	p.lock.Unlock()

	if head.FastNumber-current < hashLimit {
		f.requestChn <- true
	}
}

// checkKnownNode checks if a block tree node is known (downloaded and validated)
// If it was not known previously but found in the database, sets its known flag
func (f *fastLightFetcher) checkKnownNode(fp *fastPeerInfo, n *announce) {
	currentHeight := f.chain.CurrentHeader().Number.Uint64()
	if fp.firstAnnounced.number < currentHeight {
		for _, announce := range fp.nodeByHash {
			if announce.number < currentHeight {
				delete(fp.nodeByHash, announce.hash)
				if insert := f.queued[announce.hash]; insert != nil {
					delete(f.queued, announce.hash)
				}
			}
			if announce.number == currentHeight {
				fp.firstAnnounced = announce
			}
		}
	}
}

// peerHasBlock returns true if we can assume the peer knows the given block
// based on its announcements
func (f *fastLightFetcher) peerHasBlock(p *peer, hash common.Hash, number uint64, hasState bool) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	fp := f.peers[p]
	if fp == nil || fp.firstAnnounced == nil {
		return false
	}

	if hasState {
		if fp.lastAnnounced == nil || fp.lastAnnounced.number > number+serverStateAvailable {
			return false
		}
	}

	if f.syncing {
		// always return true when syncing
		// false positives are acceptable, a more sophisticated condition can be implemented later
		return true
	}

	if number >= fp.firstAnnounced.number {
		// it is recent enough that if it is known, is should be in the peer's block tree
		return fp.nodeByHash[hash] != nil
	}
	return false
}

// requestAmount calculates the amount of headers to be downloaded starting
// from a certain head backwards
func (f *fastLightFetcher) requestAmount(n *announce, height uint64) uint64 {
	amount := uint64(0)
	if n.number > height {
		return n.number - height
	}
	return amount
}

// requestedID tells if a certain reqID has been requested by the fetcher
func (f *fastLightFetcher) requestedID(reqID uint64) bool {
	f.reqMu.RLock()
	_, ok := f.requested[reqID]
	f.reqMu.RUnlock()
	return ok
}

// nextRequest selects the peer and announced head to be requested next, amount
// to be downloaded starting from the head backwards is also returned
func (f *fastLightFetcher) nextRequest() (*distReq, uint64, bool) {
	var (
		bestHash   common.Hash
		bestAmount uint64
	)
	bestSyncing := false
	currentHeight := f.chain.CurrentHeader().Number.Uint64()
	bestHeight := currentHeight

	for _, fp := range f.peers {
		for hash, n := range fp.nodeByHash {
			if !n.requested && n.number > currentHeight {
				amount := f.requestAmount(n, currentHeight)
				if n.number > bestHeight || amount < bestAmount {
					bestHash = hash
					bestHeight = n.number
					bestAmount = amount
				}
			}
		}
	}

	if bestHeight == currentHeight {
		return nil, 0, false
	}
	bestSyncing = bestHeight-currentHeight > hashLimit

	if !f.queue.Empty() {
		op := f.queue.PopItem().(*inject)
		header := op.header
		hash := header.Hash()
		number := header.Number.Uint64()
		if bestHeight > number {
			bestHash = hash
			bestHeight = number
			bestAmount = number - currentHeight
		}
		// If too high up the chain or phase, continue later
		f.queue.Push(op, -int64(number))
	}

	if bestAmount > MaxHeaderFetch {
		bestAmount = MaxHeaderFetch
	}

	var rq *distReq
	reqID := genReqID()
	if !bestSyncing {
		rq = &distReq{
			getCost: func(dp distPeer) uint64 {
				p := dp.(*peer)
				return p.GetRequestCost(GetFastBlockHeadersMsg, int(bestAmount))
			},
			canSend: func(dp distPeer) bool {
				p := dp.(*peer)
				f.lock.Lock()
				defer f.lock.Unlock()

				fp := f.peers[p]
				if fp == nil {
					return false
				}
				n := fp.nodeByHash[bestHash]
				return n != nil && !n.requested
			},
			request: func(dp distPeer) func() {
				p := dp.(*peer)
				f.lock.Lock()
				fp := f.peers[p]
				if fp != nil {
					n := fp.nodeByHash[bestHash]
					if n != nil {
						n.requested = true
					}
				}
				f.lock.Unlock()

				cost := p.GetRequestCost(GetFastBlockHeadersMsg, int(bestAmount))
				p.fcServer.QueueRequest(reqID, cost)
				f.reqMu.Lock()
				f.requested[reqID] = fetchFastRequest{hash: bestHash, amount: bestAmount, peer: p, sent: mclock.Now()}
				f.reqMu.Unlock()
				go func() {
					time.Sleep(hardRequestTimeout)
					f.timeoutChn <- reqID
				}()
				return func() { p.RequestHeadersByHash(reqID, cost, bestHash, int(bestAmount), 0, true, true, false) }
			},
		}
	}
	return rq, reqID, bestSyncing
}

// deliverHeaders delivers header download request responses for processing
func (f *fastLightFetcher) deliverHeaders(peer *peer, reqID uint64, hss *headsWithSigns) {
	f.deliverChn <- fetchFastResponse{reqID: reqID, hss: hss, peer: peer}
}

// processResponse processes header download request responses, returns true if successful
func (f *fastLightFetcher) processResponse(req fetchFastRequest, resp fetchFastResponse) bool {
	fheads := resp.hss.Heads
	if uint64(len(fheads)) != req.amount || fheads[0].Hash() != req.hash {
		req.peer.Log().Debug("Response content mismatch", "requested", len(fheads), "reqfrom", fheads[0], "delivered", req.amount, "delfrom", req.hash)
		return false
	}
	headers := make([]*types.Header, req.amount)
	for i, header := range fheads {
		headers[int(req.amount)-1-i] = header
	}
	signs := make([][]*types.PbftSign, req.amount)
	for i, sign := range resp.hss.Signs {
		signs[int(req.amount)-1-i] = sign
	}

	log.Debug("processResponse", "headers", len(headers), "number", headers[0].Number, "lastnumber", headers[len(fheads)-1].Number, "current", f.chain.CurrentHeader().Number)
	if headers[0].Number.Uint64() > f.chain.CurrentHeader().Number.Uint64()+1 {
		for i, head := range headers {
			hash := head.Hash()
			num := head.Number.Uint64()
			// Schedule the head for future importing
			if _, ok := f.queued[hash]; !ok {
				op := &inject{
					origin: req.peer.id,
					header: head,
					signs:  signs[i],
				}
				f.queued[hash] = op
				f.queue.Push(op, -int64(num))
				log.Debug("Queued propagated fast block", "peer", req.peer.id, "number", num, "hash", hash, "queued", f.queue.Size())
			}
		}
		return true
	}
	return f.insertHeaderChain(headers, signs)
}

// insertHeaderChain processes header download request responses, returns true if successful
func (f *fastLightFetcher) insertHeaderChain(headers []*types.Header, signs [][]*types.PbftSign) bool {
	for i, header := range headers {
		_, errs := f.pm.election.VerifySigns(signs[i])
		for _, err := range errs {
			if err != nil {
				log.Info("VerifySigns error", "num", headers[i].Number, "hash", headers[i].Hash(), "err", err)
				return false
			}
		}
		if _, err := f.chain.InsertHeaderChain([]*types.Header{header}, 1); err != nil {
			if err == consensus.ErrFutureBlock {
				return true
			}
			log.Debug("Failed to insert fast header chain", "err", err)
			return false
		}
	}

	for _, fp := range f.peers {
		f.checkKnownNode(fp, nil)
	}
	return true
}

func (f *fastLightFetcher) notifySyncDone(p *peer) {
	f.syncDone <- p
}

// checkQueueHeight checks if a block tree node is known (downloaded and validated)
// If it was not known previously but found in the database, sets its known flag
func (f *fastLightFetcher) checkQueueHeight() {
	// Import any queued headers that could potentially fit
	headers := []*types.Header{}
	signs := [][]*types.PbftSign{}
	for !f.queue.Empty() {
		height := f.chain.CurrentHeader().Number.Uint64()
		op := f.queue.PopItem().(*inject)
		header := op.header
		hash := header.Hash()

		// If too high up the chain or phase, continue later
		number := header.Number.Uint64()
		if number > height+1 {
			f.queue.Push(op, -int64(number))
			break
		}
		// Otherwise if fresh and still unknown, try and import
		if f.chain.GetHeaderByHash(hash) != nil {
			for _, fp := range f.peers {
				f.checkKnownNode(fp, nil)
			}
			continue
		}
		headers = append(headers, header)
		signs = append(signs, op.signs)
	}
	if len(headers) > 0 {
		f.insertHeaderChain(headers, signs)
	}
}
