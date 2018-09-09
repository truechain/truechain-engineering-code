package types

import (
	"time"
	"sort"
	"sync"
	"sync/atomic"
	"math/big"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"errors"
)

var (
	rttMinEstimate   = 2 * time.Second          // Minimum round-trip time to target for download requests
	rttMaxEstimate   = 20 * time.Second         // Maximum round-trip time to target for download requests
	qosTuningPeers   = 5    // Number of peers to tune based on (best peers)
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

// LightPeer encapsulates the methods required to synchronise with a remote light peer.
type LightPeer interface {
	Head() (common.Hash, *big.Int)
	RequestHeadersByHash(common.Hash, int, int, bool, bool) error
	RequestHeadersByNumber(uint64, int, int, bool, bool) error
}

// Peer encapsulates the methods required to synchronise with a remote full peer.
type Peer interface {
	LightPeer

	RequestBodies([]common.Hash,bool) error
	RequestReceipts([]common.Hash,bool) error
	RequestNodeData([]common.Hash,bool) error
}




type PeerConnection interface {
	BlockCapacity(targetRTT time.Duration) int
	FetchHeaders(from uint64, count int) error
	FetchBodies(request *FetchRequest) error
	FetchNodeData(hashes []common.Hash) error
	FetchReceipts(request *FetchRequest) error
	HeaderCapacity(targetRTT time.Duration) int
	Lacks(hash common.Hash) bool
	MarkLacking(hash common.Hash)
	NodeDataCapacity(targetRTT time.Duration) int
	ReceiptCapacity(targetRTT time.Duration) int
	Reset()

	SetBlocksIdle(delivered int)
	SetBodiesIdle(delivered int)
	SetHeadersIdle(delivered int)
	SetNodeDataIdle(delivered int)
	SetReceiptsIdle(delivered int)

	GetID() string


	GetHeaderIdle() int32
	GetBlockIdle() int32
	GetReceiptIdle() int32
	GetStateIdle() int32

	GetHeaderThroughput() float64
	GetBlockThroughput() float64
	GetReceiptThroughput() float64
	GetStateThroughput() float64

	SetHeaderThroughput(t float64)
	SetBlockThroughput(t float64)
	SetReceiptThroughput(t float64)
	SetStateThroughput(t float64)

	GetRtt() time.Duration // Request round trip time to track responsiveness (QoS)
	SetRtt(d time.Duration)

	GetHeaderStarted()  time.Time
	GetBlockStarted()  time.Time
	GetReceiptStarted()  time.Time
	GetStateStarted()  time.Time


	GetLock() *sync.RWMutex
	GetVersion() int        // Eth protocol version number to switch strategies
	GetPeer() Peer
	GetLog() log.Logger
}





// dataPack is a data message returned by a peer for some query.
type DataPack interface {
	PeerId() string
	Items() int
	Stats() string
}


// newPeerSet creates a new peer set top track the active download sources.
func NewPeerSet() *PeerSet {
	return &PeerSet{
		peers: make(map[string]PeerConnection),
	}
}


// peerSet represents the collection of active peer participating in the chain
// download procedure.
type PeerSet struct {
	peers        map[string]PeerConnection
	newPeerFeed  event.Feed
	peerDropFeed event.Feed
	lock         sync.RWMutex
}



// SubscribeNewPeers subscribes to peer arrival events.
func (ps *PeerSet) SubscribeNewPeers(ch chan<- PeerConnection) event.Subscription {
	return ps.newPeerFeed.Subscribe(ch)
}

// SubscribePeerDrops subscribes to peer departure events.
func (ps *PeerSet) SubscribePeerDrops(ch chan<- PeerConnection) event.Subscription {
	return ps.peerDropFeed.Subscribe(ch)
}

// Reset iterates over the current peer set, and resets each of the known peers
// to prepare for a next batch of block retrieval.
func (ps *PeerSet) Reset() {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	//for _, peer := range ps.peers {
	//	peer.Reset()
	//}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known.
//
// The method also sets the starting throughput values of the new peer to the
// average of all existing peers, to give it a realistic chance of being used
// for data retrievals.
func (ps *PeerSet) Register(p PeerConnection) error {
	// Retrieve the current median RTT as a sane default
	p.SetRtt(ps.MedianRTT())

	// Register the new peer with some meaningful defaults
	ps.lock.Lock()
	if _, ok := ps.peers[p.GetID()]; ok {
		//ps.lock.Unlock()
		delete(ps.peers, p.GetID())
		//return errAlreadyRegistered
	}
	if len(ps.peers) > 0 {
		//p.headerThroughput, p.blockThroughput, p.receiptThroughput, p.stateThroughput = 0, 0, 0, 0
		p.SetHeaderThroughput(0)
		p.SetBlockThroughput(0)
		p.SetReceiptThroughput(0)
		p.SetStateThroughput(0)

		for _, peer := range ps.peers {
			peer.GetLock().RLock()

			//p.headerThroughput += peer.headerThroughput
			//p.blockThroughput += peer.blockThroughput
			//p.receiptThroughput += peer.receiptThroughput
			//p.stateThroughput += peer.stateThroughput

			p.SetHeaderThroughput(p.GetHeaderThroughput() + peer.GetHeaderThroughput())
			p.SetBlockThroughput(p.GetBlockThroughput() + peer.GetBlockThroughput())
			p.SetReceiptThroughput(p.GetReceiptThroughput() + peer.GetReceiptThroughput())
			p.SetStateThroughput(p.GetStateThroughput() + peer.GetStateThroughput())


			peer.GetLock().RUnlock()
		}

		//p.headerThroughput /= float64(len(ps.peers))
		//p.blockThroughput /= float64(len(ps.peers))
		//p.receiptThroughput /= float64(len(ps.peers))
		//p.stateThroughput /= float64(len(ps.peers))

		p.SetHeaderThroughput(p.GetHeaderThroughput() / float64(len(ps.peers)))
		p.SetBlockThroughput(p.GetBlockThroughput() / float64(len(ps.peers)))
		p.SetReceiptThroughput(p.GetReceiptThroughput() / float64(len(ps.peers)))
		p.SetStateThroughput(p.GetStateThroughput() / float64(len(ps.peers)))
	}

	ps.peers[p.GetID()] = p
	ps.lock.Unlock()

	ps.newPeerFeed.Send(p)
	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *PeerSet) Unregister(id string) error {
	ps.lock.Lock()
	p, ok := ps.peers[id]
	if !ok {
		defer ps.lock.Unlock()
		return errNotRegistered
	}
	delete(ps.peers, id)
	ps.lock.Unlock()

	ps.peerDropFeed.Send(p)
	return nil
}

// Peer retrieves the registered peer with the given id.
func (ps *PeerSet) Peer(id string) PeerConnection {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// Len returns if the current number of peers in the set.
func (ps *PeerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// AllPeers retrieves a flat list of all the peers within the set.
func (ps *PeerSet) AllPeers() []PeerConnection {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]PeerConnection, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	return list
}

// HeaderIdlePeers retrieves a flat list of all the currently header-idle peers
// within the active peer set, ordered by their reputation.
func (ps *PeerSet) HeaderIdlePeers() ([]PeerConnection, int) {
	idle := func(p PeerConnection) bool {
		idle_v := p.GetHeaderIdle()
		return atomic.LoadInt32(&idle_v) == 0
	}
	throughput := func(p PeerConnection) float64 {
		p.GetLock().RLock()
		defer p.GetLock().RUnlock()
		return p.GetHeaderThroughput()
	}
	return ps.idlePeers(62, 64, idle, throughput)
}

// BodyIdlePeers retrieves a flat list of all the currently body-idle peers within
// the active peer set, ordered by their reputation.
func (ps *PeerSet) BodyIdlePeers() ([]PeerConnection, int) {
	idle := func(p PeerConnection) bool {
		idle_v := p.GetBlockIdle()
		return atomic.LoadInt32(&idle_v) == 0
	}
	throughput := func(p PeerConnection) float64 {
		p.GetLock().RLock()
		defer p.GetLock().RUnlock()
		return p.GetBlockThroughput()
	}
	return ps.idlePeers(62, 64, idle, throughput)
}

// ReceiptIdlePeers retrieves a flat list of all the currently receipt-idle peers
// within the active peer set, ordered by their reputation.
func (ps *PeerSet) ReceiptIdlePeers() ([]PeerConnection, int) {
	idle := func(p PeerConnection) bool {
		idle_v := p.GetReceiptIdle()
		return atomic.LoadInt32(&idle_v) == 0
	}
	throughput := func(p PeerConnection) float64 {
		p.GetLock().RLock()
		defer p.GetLock().RUnlock()
		return p.GetReceiptThroughput()
	}
	return ps.idlePeers(63, 64, idle, throughput)
}

// NodeDataIdlePeers retrieves a flat list of all the currently node-data-idle
// peers within the active peer set, ordered by their reputation.
func (ps *PeerSet) NodeDataIdlePeers() ([]PeerConnection, int) {
	idle := func(p PeerConnection) bool {
		idle_v := p.GetStateIdle()
		return atomic.LoadInt32(&idle_v) == 0
	}
	throughput := func(p PeerConnection) float64 {
		p.GetLock().RLock()
		defer p.GetLock().RUnlock()
		return p.GetStateThroughput()
	}
	return ps.idlePeers(63, 64, idle, throughput)
}

// idlePeers retrieves a flat list of all currently idle peers satisfying the
// protocol version constraints, using the provided function to check idleness.
// The resulting set of peers are sorted by their measure throughput.
func (ps *PeerSet) idlePeers(minProtocol, maxProtocol int, idleCheck func(PeerConnection) bool, throughput func(PeerConnection) float64) ([]PeerConnection, int) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	idle, total := make([]PeerConnection, 0, len(ps.peers)), 0
	for _, p := range ps.peers {
		if p.GetVersion() >= minProtocol && p.GetVersion() <= maxProtocol {
			if idleCheck(p) {
				idle = append(idle, p)
			}
			total++
		}
	}
	for i := 0; i < len(idle); i++ {
		for j := i + 1; j < len(idle); j++ {
			if throughput(idle[i]) < throughput(idle[j]) {
				idle[i], idle[j] = idle[j], idle[i]
			}
		}
	}
	return idle, total
}

// medianRTT returns the median RTT of the peerset, considering only the tuning
// peers if there are more peers available.
func (ps *PeerSet) MedianRTT() time.Duration {
	// Gather all the currently measured round trip times
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	rtts := make([]float64, 0, len(ps.peers))
	for _, p := range ps.peers {
		p.GetLock().RLock()
		rtts = append(rtts, float64(p.GetRtt()))
		p.GetLock().RUnlock()
	}
	sort.Float64s(rtts)

	median := rttMaxEstimate
	if qosTuningPeers <= len(rtts) {
		median = time.Duration(rtts[qosTuningPeers/2]) // Median of our tuning peers
	} else if len(rtts) > 0 {
		median = time.Duration(rtts[len(rtts)/2]) // Median of our connected peers (maintain even like this some baseline qos)
	}
	// Restrict the RTT into some QoS defaults, irrelevant of true RTT
	if median < rttMinEstimate {
		median = rttMinEstimate
	}
	if median > rttMaxEstimate {
		median = rttMaxEstimate
	}
	return median
}


// fetchRequest is a currently running data retrieval operation.
type FetchRequest struct {
	Peer    PeerConnection // Peer to which the request was sent
	From    uint64          // [eth/62] Requested chain element index (used for skeleton fills only)
	Sheaders []*types.SnailHeader // [eth/62] Requested headers, sorted by request order
	Fheaders []*types.Header // [eth/62] Requested headers, sorted by request order
	Time    time.Time       // Time when the request was made

}

// fetchResult is a struct collecting partial results from data fetchers until
// all outstanding pieces complete and the result as a whole can be processed.
type FetchResult struct {
	Pending int         // Number of data fetches still pending
	Hash    common.Hash // Hash of the header to prevent recalculating

	Signs   	 []*types.PbftSign
	Sheader      *types.SnailHeader
	Fruits 		 types.SnailBlocks

	Fheader      *types.Header
	Uncles       []*types.Header
	Transactions types.Transactions
	Receipts     types.Receipts
}

// peerDropFn is a callback type for dropping a peer detected as malicious.
type PeerDropFn func(id string)

