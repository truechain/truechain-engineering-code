// Copyright 2015 The go-ethereum Authors
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

// Contains the active peer-set of the downloader, maintaining both failures
// as well as reputation metrics to prioritize the block retrievals.

package fastdownloader

import (
	"errors"
	"fmt"
	"math"
	"math/big"
		"sync"
	"sync/atomic"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
		"github.com/truechain/truechain-engineering-code/log"
	etrue "github.com/truechain/truechain-engineering-code/etrue/types"
	)

const (
	maxLackingHashes  = 4096 // Maximum number of entries allowed on the list or lacking items
	measurementImpact = 0.1  // The impact a single measurement has on a peer's final throughput value.
)

var (
	errAlreadyFetching   = errors.New("already fetching blocks from peer")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

// peerConnection represents an active peer from which hashes and blocks are retrieved.
type peerConnection struct {
	id string // Unique identifier of the peer

	headerIdle  int32 // Current header activity state of the peer (idle = 0, active = 1)
	blockIdle   int32 // Current block activity state of the peer (idle = 0, active = 1)
	receiptIdle int32 // Current receipt activity state of the peer (idle = 0, active = 1)
	stateIdle   int32 // Current node data activity state of the peer (idle = 0, active = 1)

	headerThroughput  float64 // Number of headers measured to be retrievable per second
	blockThroughput   float64 // Number of blocks (bodies) measured to be retrievable per second
	receiptThroughput float64 // Number of receipts measured to be retrievable per second
	stateThroughput   float64 // Number of node data pieces measured to be retrievable per second

	rtt time.Duration // Request round trip time to track responsiveness (QoS)

	headerStarted  time.Time // Time instance when the last header fetch was started
	blockStarted   time.Time // Time instance when the last block (body) fetch was started
	receiptStarted time.Time // Time instance when the last receipt fetch was started
	stateStarted   time.Time // Time instance when the last node data fetch was started

	lacking map[common.Hash]struct{} // Set of hashes not to request (didn't have previously)

	peer etrue.Peer

	version int        // Eth protocol version number to switch strategies
	log     log.Logger // Contextual logger to add extra infos to peer logs
	lock    sync.RWMutex
}


// lightPeerWrapper wraps a LightPeer struct, stubbing out the Peer-only methods.
type lightPeerWrapper struct {
	peer etrue.LightPeer
}

func (w *lightPeerWrapper) Head() (common.Hash, *big.Int) { return w.peer.Head() }
func (w *lightPeerWrapper) RequestHeadersByHash(h common.Hash, amount int, skip int, reverse bool,isFastchain bool) error {
	return w.peer.RequestHeadersByHash(h, amount, skip, reverse,isFastchain)
}
func (w *lightPeerWrapper) RequestHeadersByNumber(i uint64, amount int, skip int, reverse bool,isFastchain bool) error {
	return w.peer.RequestHeadersByNumber(i, amount, skip, reverse,isFastchain)
}
func (w *lightPeerWrapper) RequestBodies([]common.Hash,bool) error {
	panic("RequestBodies not supported in light client mode sync")
}
func (w *lightPeerWrapper) RequestReceipts([]common.Hash,bool) error {
	panic("RequestReceipts not supported in light client mode sync")
}
func (w *lightPeerWrapper) RequestNodeData([]common.Hash,bool) error {
	panic("RequestNodeData not supported in light client mode sync")
}

// newPeerConnection creates a new downloader peer.
func newPeerConnection(id string, version int, peer etrue.Peer, logger log.Logger) *peerConnection {
	return &peerConnection{
		id:      id,
		lacking: make(map[common.Hash]struct{}),
		peer: peer,
		version: version,
		log:     logger,
	}
}

func (p *peerConnection) GetHeaderIdle() int32 {return p.headerIdle}
func (p *peerConnection) GetBlockIdle() int32 {return p.blockIdle}
func (p *peerConnection) GetReceiptIdle() int32 {return p.receiptIdle}
func (p *peerConnection) GetStateIdle() int32 {return p.stateIdle}

func (p *peerConnection) GetHeaderThroughput() float64 {return p.headerThroughput}
func (p *peerConnection) GetBlockThroughput() float64 {return p.blockThroughput}
func (p *peerConnection) GetReceiptThroughput() float64 {return p.receiptThroughput}
func (p *peerConnection) GetStateThroughput() float64 {return p.stateThroughput}

func (p *peerConnection) SetHeaderThroughput(t float64){ p.headerThroughput=t }
func (p *peerConnection) SetBlockThroughput(t float64){ p.blockThroughput=t }
func (p *peerConnection) SetReceiptThroughput(t float64){ p.receiptThroughput=t }
func (p *peerConnection) SetStateThroughput(t float64){ p.stateThroughput=t }

func (p *peerConnection) GetRtt() time.Duration  {return p.rtt}// Request round trip time to track responsiveness (QoS)
func (p *peerConnection) SetRtt(d time.Duration )  {p.rtt=d}// Request round trip time to track responsiveness (QoS)

func (p *peerConnection) GetHeaderStarted()  time.Time {return p.headerStarted}
func (p *peerConnection) GetBlockStarted()  time.Time {return p.blockStarted}
func (p *peerConnection) GetReceiptStarted()  time.Time {return p.receiptStarted}
func (p *peerConnection) GetStateStarted()  time.Time {return p.stateStarted}

func (p *peerConnection) GetID() string {return p.id}
func (p *peerConnection) GetVersion() int {return p.version}

func (p *peerConnection) GetPeer() etrue.Peer {return p.peer}
func (p *peerConnection) GetLog() log.Logger {return p.log}
func (p *peerConnection) GetLock() *sync.RWMutex {return &p.lock}


// Reset clears the internal state of a peer entity.
func (p *peerConnection) Reset() {
	p.lock.Lock()
	defer p.lock.Unlock()

	atomic.StoreInt32(&p.headerIdle, 0)
	atomic.StoreInt32(&p.blockIdle, 0)
	atomic.StoreInt32(&p.receiptIdle, 0)
	atomic.StoreInt32(&p.stateIdle, 0)

	p.headerThroughput = 0
	p.blockThroughput = 0
	p.receiptThroughput = 0
	p.stateThroughput = 0

	p.lacking = make(map[common.Hash]struct{})
}

// FetchHeaders sends a header retrieval request to the remote peer.
func (p *peerConnection) FetchHeaders(from uint64, count int) error {
	// Sanity check the protocol version
	if p.version < 62 {
		panic(fmt.Sprintf("header fetch [eth/62+] requested on eth/%d", p.version))
	}
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.headerIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.headerStarted = time.Now()

	// Issue the header retrieval request (absolut upwards without gaps)
	go p.peer.RequestHeadersByNumber(from, count, 0, false,true)

	return nil
}

// FetchBodies sends a block body retrieval request to the remote peer.
func (p *peerConnection) FetchBodies(request *etrue.FetchRequest) error {
	// Sanity check the protocol version
	if p.version < 62 {
		panic(fmt.Sprintf("body fetch [eth/62+] requested on eth/%d", p.version))
	}
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.blockIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.blockStarted = time.Now()

	// Convert the header set to a retrievable slice
	hashes := make([]common.Hash, 0, len(request.Fheaders))
	for _, header := range request.Fheaders {
		hashes = append(hashes, header.Hash())
	}
	go p.peer.RequestBodies(hashes,true)

	return nil
}

// FetchReceipts sends a receipt retrieval request to the remote peer.
func (p *peerConnection) FetchReceipts(request *etrue.FetchRequest) error {
	// Sanity check the protocol version
	if p.version < 63 {
		panic(fmt.Sprintf("body fetch [eth/63+] requested on eth/%d", p.version))
	}
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.receiptIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.receiptStarted = time.Now()

	// Convert the header set to a retrievable slice
	hashes := make([]common.Hash, 0, len(request.Fheaders))
	for _, header := range request.Fheaders {
		hashes = append(hashes, header.Hash())
	}
	go p.peer.RequestReceipts(hashes,true)

	return nil
}

// FetchNodeData sends a node state data retrieval request to the remote peer.
func (p *peerConnection) FetchNodeData(hashes []common.Hash) error {
	// Sanity check the protocol version
	if p.version < 63 {
		panic(fmt.Sprintf("node data fetch [eth/63+] requested on eth/%d", p.version))
	}
	// Short circuit if the peer is already fetching
	if !atomic.CompareAndSwapInt32(&p.stateIdle, 0, 1) {
		return errAlreadyFetching
	}
	p.stateStarted = time.Now()

	go p.peer.RequestNodeData(hashes,true)

	return nil
}

// SetHeadersIdle sets the peer to idle, allowing it to execute new header retrieval
// requests. Its estimated header retrieval throughput is updated with that measured
// just now.
func (p *peerConnection) SetHeadersIdle(delivered int) {
	p.setIdle(p.headerStarted, delivered, &p.headerThroughput, &p.headerIdle)
}

// SetBlocksIdle sets the peer to idle, allowing it to execute new block retrieval
// requests. Its estimated block retrieval throughput is updated with that measured
// just now.
func (p *peerConnection) SetBlocksIdle(delivered int) {
	p.setIdle(p.blockStarted, delivered, &p.blockThroughput, &p.blockIdle)
}

// SetBodiesIdle sets the peer to idle, allowing it to execute block body retrieval
// requests. Its estimated body retrieval throughput is updated with that measured
// just now.
func (p *peerConnection) SetBodiesIdle(delivered int) {
	p.setIdle(p.blockStarted, delivered, &p.blockThroughput, &p.blockIdle)
}

// SetReceiptsIdle sets the peer to idle, allowing it to execute new receipt
// retrieval requests. Its estimated receipt retrieval throughput is updated
// with that measured just now.
func (p *peerConnection) SetReceiptsIdle(delivered int) {
	p.setIdle(p.receiptStarted, delivered, &p.receiptThroughput, &p.receiptIdle)
}

// SetNodeDataIdle sets the peer to idle, allowing it to execute new state trie
// data retrieval requests. Its estimated state retrieval throughput is updated
// with that measured just now.
func (p *peerConnection) SetNodeDataIdle(delivered int) {
	p.setIdle(p.stateStarted, delivered, &p.stateThroughput, &p.stateIdle)
}

// setIdle sets the peer to idle, allowing it to execute new retrieval requests.
// Its estimated retrieval throughput is updated with that measured just now.
func (p *peerConnection) setIdle(started time.Time, delivered int, throughput *float64, idle *int32) {
	// Irrelevant of the scaling, make sure the peer ends up idle
	defer atomic.StoreInt32(idle, 0)

	p.lock.Lock()
	defer p.lock.Unlock()

	// If nothing was delivered (hard timeout / unavailable data), reduce throughput to minimum
	if delivered == 0 {
		*throughput = 0
		return
	}
	// Otherwise update the throughput with a new measurement
	elapsed := time.Since(started) + 1 // +1 (ns) to ensure non-zero divisor
	measured := float64(delivered) / (float64(elapsed) / float64(time.Second))

	*throughput = (1-measurementImpact)*(*throughput) + measurementImpact*measured
	p.rtt = time.Duration((1-measurementImpact)*float64(p.rtt) + measurementImpact*float64(elapsed))

	p.log.Trace("Peer throughput measurements updated",
		"hps", p.headerThroughput, "bps", p.blockThroughput,
		"rps", p.receiptThroughput, "sps", p.stateThroughput,
		"miss", len(p.lacking), "rtt", p.rtt)
}

// HeaderCapacity retrieves the peers header download allowance based on its
// previously discovered throughput.
func (p *peerConnection) HeaderCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return int(math.Min(1+math.Max(1, p.headerThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxHeaderFetch)))
}

// BlockCapacity retrieves the peers block download allowance based on its
// previously discovered throughput.
func (p *peerConnection) BlockCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return int(math.Min(1+math.Max(1, p.blockThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxBlockFetch)))
}

// ReceiptCapacity retrieves the peers receipt download allowance based on its
// previously discovered throughput.
func (p *peerConnection) ReceiptCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return int(math.Min(1+math.Max(1, p.receiptThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxReceiptFetch)))
}

// NodeDataCapacity retrieves the peers state download allowance based on its
// previously discovered throughput.
func (p *peerConnection) NodeDataCapacity(targetRTT time.Duration) int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return int(math.Min(1+math.Max(1, p.stateThroughput*float64(targetRTT)/float64(time.Second)), float64(MaxStateFetch)))
}

// MarkLacking appends a new entity to the set of items (blocks, receipts, states)
// that a peer is known not to have (i.e. have been requested before). If the
// set reaches its maximum allowed capacity, items are randomly dropped off.
func (p *peerConnection) MarkLacking(hash common.Hash) {
	p.lock.Lock()
	defer p.lock.Unlock()

	for len(p.lacking) >= maxLackingHashes {
		for drop := range p.lacking {
			delete(p.lacking, drop)
			break
		}
	}
	p.lacking[hash] = struct{}{}
}


// Lacks retrieves whether the hash of a blockchain item is on the peers lacking
// list (i.e. whether we know that the peer does not have it).
func (p *peerConnection) Lacks(hash common.Hash) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	_, ok := p.lacking[hash]
	return ok
}