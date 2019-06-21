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
	"github.com/truechain/truechain-engineering-code/light/fast"
	"github.com/truechain/truechain-engineering-code/light/public"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
)

// fastLightFetcher implements retrieval of newly announced headers. It also provides a peerHasBlock function for the
// ODR system to ensure that we only request data related to a certain block from peers who have already processed
// and announced that block.
type fastLightFetcher struct {
	pm    *ProtocolManager
	odr   *LesOdr
	chain *fast.LightChain

	lock               sync.Mutex // lock protects access to the fetcher's internal state variables except sent requests
	maxConfirmedHeight *big.Int
	peers              map[*peer]*fetcherFastPeerInfo
	syncing            bool
	syncDone           chan *peer

	reqMu      sync.RWMutex // reqMu protects access to sent header fetch requests
	requested  map[uint64]fetchFastRequest
	deliverChn chan fetchFastResponse
	timeoutChn chan uint64
	requestChn chan bool // true if initiated from outside
}

// fetcherFastPeerInfo holds fetcher-specific information about each active peer
type fetcherFastPeerInfo struct {
	root, lastAnnounced *fetcherFastTreeNode
	nodeCnt             int
	confirmedHeight     *big.Int
	bestConfirmed       *fetcherFastTreeNode
	nodeByHash          map[common.Hash]*fetcherFastTreeNode
}

// fetcherFastTreeNode is a node of a tree that holds information about blocks recently
// announced and confirmed by a certain peer. Each new announce message from a peer
// adds nodes to the tree, based on the previous announced head and the reorg depth.
// There are three possible states for a tree node:
// - announced: not downloaded (known) yet, but we know its head, number and td
// - intermediate: not known, hash and td are empty, they are filled out when it becomes known
// - known: both announced by this peer and downloaded (from any peer).
// This structure makes it possible to always know which peer has a certain block,
// which is necessary for selecting a suitable peer for ODR requests and also for
// canonizing new heads. It also helps to always download the minimum necessary
// amount of headers with a single request.
type fetcherFastTreeNode struct {
	hash             common.Hash
	number           uint64
	height           *big.Int
	known, requested bool
	parent           *fetcherFastTreeNode
	children         []*fetcherFastTreeNode
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
	reqID   uint64
	headers []*types.Header
	peer    *peer
}

// newLightFetcher creates a new light fetcher
func newFastLightFetcher(pm *ProtocolManager) *fastLightFetcher {
	f := &fastLightFetcher{
		pm:                 pm,
		chain:              pm.fblockchain.(*fast.LightChain),
		odr:                pm.odr,
		peers:              make(map[*peer]*fetcherFastPeerInfo),
		deliverChn:         make(chan fetchFastResponse, 100),
		requested:          make(map[uint64]fetchFastRequest),
		timeoutChn:         make(chan uint64),
		requestChn:         make(chan bool, 100),
		syncDone:           make(chan *peer),
		maxConfirmedHeight: big.NewInt(0),
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

			if rq != nil {
				requesting = true
				if _, ok := <-f.pm.reqDist.queue(rq); ok {
					if syncing {
						f.lock.Lock()
						f.syncing = true
						f.lock.Unlock()
					} else {
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
				req.peer.Log().Debug("Fetching data timed out hard")
				go f.pm.removePeer(req.peer.id, public.FetcherTimerCall)
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
			p.Log().Debug("Done synchronising with peer")
			f.syncing = false
			f.lock.Unlock()
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

	f.peers[p] = &fetcherFastPeerInfo{nodeByHash: make(map[common.Hash]*fetcherFastTreeNode)}
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
	p.Log().Debug("Received new announcement", "number", head.FastNumber, "hash", head.FastHash)

	fp := f.peers[p]
	if fp == nil {
		p.Log().Debug("Announcement from unknown peer")
		return
	}

	if fp.lastAnnounced != nil && head.FastNumber < fp.lastAnnounced.height.Uint64() {
		// announced tds should be strictly monotonic
		p.Log().Debug("Received non-monotonic td", "current", head.FastNumber, "previous", fp.lastAnnounced.height)
		go f.pm.removePeer(p.id, public.FetcherAnnounceCall)
		return
	}

	n := fp.lastAnnounced
	// n is now the reorg common ancestor, add a new branch of nodes
	if n != nil && (head.FastNumber >= n.number+maxNodeCount || head.FastNumber <= n.number) {
		// if announced head block height is lower or same as n or too far from it to add
		// intermediate nodes then discard previous announcement info and trigger a resync
		n = nil
		fp.nodeCnt = 0
		fp.nodeByHash = make(map[common.Hash]*fetcherFastTreeNode)
	}
	if n != nil {
		// check if the node count is too high to add new nodes, discard oldest ones if necessary
		locked := false
		for uint64(fp.nodeCnt)+head.FastNumber-n.number > maxNodeCount && fp.root != nil {
			if !locked {
				f.chain.LockChain()
				defer f.chain.UnlockChain()
				locked = true
			}
			// if one of root's children is canonical, keep it, delete other branches and root itself
			var newRoot *fetcherFastTreeNode
			for i, nn := range fp.root.children {
				if rawdb.ReadCanonicalHash(f.pm.chainDb, nn.number) == nn.hash {
					fp.root.children = append(fp.root.children[:i], fp.root.children[i+1:]...)
					nn.parent = nil
					newRoot = nn
					break
				}
			}
			if n == fp.root {
				n = newRoot
			}
			fp.root = newRoot
			if newRoot == nil {
				fp.bestConfirmed = nil
				fp.confirmedHeight = nil
			}

			if n == nil {
				break
			}
		}
		if n != nil {
			for n.number < head.Number {
				nn := &fetcherFastTreeNode{number: n.number + 1, parent: n}
				n.children = append(n.children, nn)
				n = nn
				fp.nodeCnt++
			}
			n.hash = head.Hash

			n.height = new(big.Int).SetUint64(head.FastNumber)
			fp.nodeByHash[n.hash] = n
		}
	}
	if n == nil {
		// could not find reorg common ancestor or had to delete entire tree, a new root and a resync is needed
		if fp.root != nil {
			fp.deleteNode(fp.root)
		}
		n = &fetcherFastTreeNode{hash: head.Hash, number: head.Number, height: new(big.Int).SetUint64(head.FastNumber)}
		fp.root = n
		fp.nodeCnt++
		fp.nodeByHash[n.hash] = n
		fp.bestConfirmed = nil
		fp.confirmedHeight = nil
	}

	p.lock.Lock()
	p.headInfo = head
	fp.lastAnnounced = n
	p.lock.Unlock()
	f.requestChn <- true
}

// peerHasBlock returns true if we can assume the peer knows the given block
// based on its announcements
func (f *fastLightFetcher) peerHasBlock(p *peer, hash common.Hash, number uint64, hasState bool) bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	fp := f.peers[p]
	if fp == nil || fp.root == nil {
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

	if number >= fp.root.number {
		// it is recent enough that if it is known, is should be in the peer's block tree
		return fp.nodeByHash[hash] != nil
	}
	f.chain.LockChain()
	defer f.chain.UnlockChain()
	// if it's older than the peer's block tree root but it's in the same canonical chain
	// as the root, we can still be sure the peer knows it
	//
	// when syncing, just check if it is part of the known chain, there is nothing better we
	// can do since we do not know the most recent block hash yet
	return rawdb.ReadCanonicalHash(f.pm.chainDb, fp.root.number) == fp.root.hash && rawdb.ReadCanonicalHash(f.pm.chainDb, number) == hash
}

// requestAmount calculates the amount of headers to be downloaded starting
// from a certain head backwards
func (f *fastLightFetcher) requestAmount(p *peer, n *fetcherFastTreeNode) uint64 {
	amount := uint64(0)
	nn := n
	for nn != nil {
		nn = nn.parent
		amount++
	}
	if nn == nil {
		amount = n.number
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
	bestTd := f.maxConfirmedHeight
	bestSyncing := false

	for p, fp := range f.peers {
		for hash, n := range fp.nodeByHash {
			if !n.requested && (bestTd == nil || n.height.Cmp(bestTd) >= 0) {
				amount := f.requestAmount(p, n)
				if bestTd == nil || n.height.Cmp(bestTd) > 0 || amount < bestAmount {
					bestHash = hash
					bestAmount = amount
					bestTd = n.height
					bestSyncing = fp.bestConfirmed == nil || fp.root == nil
				}
			}
		}
	}
	if bestTd == f.maxConfirmedHeight {
		return nil, 0, false
	}

	var rq *distReq
	reqID := genReqID()
	if bestSyncing {
		rq = &distReq{
			getCost: func(dp distPeer) uint64 {
				return 0
			},
			canSend: func(dp distPeer) bool {
				p := dp.(*peer)
				f.lock.Lock()
				defer f.lock.Unlock()

				fp := f.peers[p]
				return fp != nil && fp.nodeByHash[bestHash] != nil
			},
			request: func(dp distPeer) func() {
				go func() {
					p := dp.(*peer)
					p.Log().Debug("Synchronisation started")
					f.pm.synchronise(p)
					f.syncDone <- p
				}()
				return nil
			},
		}
	} else {
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
				return func() { p.RequestHeadersByHash(reqID, cost, bestHash, int(bestAmount), 0, true, true) }
			},
		}
	}
	return rq, reqID, bestSyncing
}

// deliverHeaders delivers header download request responses for processing
func (f *fastLightFetcher) deliverHeaders(peer *peer, reqID uint64, headers []*types.Header) {
	f.deliverChn <- fetchFastResponse{reqID: reqID, headers: headers, peer: peer}
}

// processResponse processes header download request responses, returns true if successful
func (f *fastLightFetcher) processResponse(req fetchFastRequest, resp fetchFastResponse) bool {
	if uint64(len(resp.headers)) != req.amount || resp.headers[0].Hash() != req.hash {
		req.peer.Log().Debug("Response content mismatch", "requested", len(resp.headers), "reqfrom", resp.headers[0], "delivered", req.amount, "delfrom", req.hash)
		return false
	}
	headers := make([]*types.Header, req.amount)
	for i, header := range resp.headers {
		headers[int(req.amount)-1-i] = header
	}
	if _, err := f.chain.InsertHeaderChain(headers, 1); err != nil {
		if err == consensus.ErrFutureBlock {
			return true
		}
		log.Debug("Failed to insert header chain", "err", err)
		return false
	}
	for i, header := range headers {
		if f.chain.HasHeader(header.Hash(), header.Number.Uint64()) {
			log.Debug("Total difficulty not found for header", "index", i+1, "number", header.Number, "hash", header.Hash())
			return false
		}
	}
	return true
}

// deleteNode deletes a node and its child subtrees from a peer's block tree
func (fp *fetcherFastPeerInfo) deleteNode(n *fetcherFastTreeNode) {
	if n.parent != nil {
		for i, nn := range n.parent.children {
			if nn == n {
				n.parent.children = append(n.parent.children[:i], n.parent.children[i+1:]...)
				break
			}
		}
	}
	for {
		if n.height != nil {
			delete(fp.nodeByHash, n.hash)
		}
		fp.nodeCnt--
		if len(n.children) == 0 {
			return
		}
		for i, nn := range n.children {
			if i == 0 {
				n = nn
			} else {
				fp.deleteNode(nn)
			}
		}
	}
}
