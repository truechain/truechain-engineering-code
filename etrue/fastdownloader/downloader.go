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

// Package downloader contains the manual full chain synchronisation.
package fastdownloader

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/ethdb"
	etrue "github.com/truechain/truechain-engineering-code/etrue/types"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/metrics"
	"github.com/truechain/truechain-engineering-code/params"
)

var (
	MaxHashFetch    = 512 // Amount of hashes to be fetched per retrieval request
	MaxBlockFetch   = 128 // Amount of blocks to be fetched per retrieval request
	MaxHeaderFetch  = 192 // Amount of block headers to be fetched per retrieval request
	MaxBodyFetch    = 128 // Amount of block bodies to be fetched per retrieval request
	MaxReceiptFetch = 256 // Amount of transaction receipts to allow fetching per request
	MaxStateFetch   = 384 // Amount of node state values to allow fetching per request

	MaxForkAncestry  = 3 * params.EpochDuration // Maximum chain reorganisation
	rttMaxEstimate   = 20 * time.Second         // Maximum round-trip time to target for download requests
	rttMinConfidence = 0.1                      // Worse confidence factor in our estimated RTT value
	ttlScaling       = 3                        // Constant scaling factor for RTT -> TTL conversion
	ttlLimit         = time.Minute              // Maximum TTL allowance to prevent reaching crazy timeouts

	qosConfidenceCap = 10   // Number of peers above which not to modify RTT confidence
	qosTuningImpact  = 0.25 // Impact that a new tuning target has on the previous value

	maxQueuedHeaders  = 32 * 1024 // [eth/62] Maximum number of headers to queue for import (DOS protection)
	maxHeadersProcess = 2048      // Number of header download results to import at once into the chain
	maxResultsProcess = 2048      // Number of content download results to import at once into the chain

	reorgProtThreshold   = 48 // Threshold number of recent blocks to disable mini reorg protection
	reorgProtHeaderDelay = 2  // Number of headers to delay delivering to cover mini reorgs

	fsHeaderCheckFrequency = 100  // Verification frequency of the downloaded headers during fast sync
	fsHeaderSafetyNet      = 2048 // Number of headers to discard in case a chain violation is detected

)

var (
	errBusy                    = errors.New("fast busy")
	errUnknownPeer             = errors.New("fast peer is unknown or unhealthy")
	errBadPeer                 = errors.New("fast action from bad peer ignored")
	errStallingPeer            = errors.New("fast peer is stalling")
	errNoPeers                 = errors.New("fast no peers to keep download active")
	errTimeout                 = errors.New("fast timeout")
	errEmptyHeaderSet          = errors.New("fast empty header set by peer")
	errPeersUnavailable        = errors.New("fast no peers available or all tried for download")
	errInvalidAncestor         = errors.New("fast retrieved ancestor is invalid")
	errInvalidChain            = errors.New("fast retrieved hash chain is invalid")
	errInvalidBlock            = errors.New("fast retrieved block is invalid")
	errInvalidBody             = errors.New("fast retrieved block body is invalid")
	errInvalidReceipt          = errors.New("fast retrieved receipt is invalid")
	errCancelBlockFetch        = errors.New("fast block download canceled (requested)")
	errCancelHeaderFetch       = errors.New("fast block header download canceled (requested)")
	errCancelBodyFetch         = errors.New("fast block body download canceled (requested)")
	errCancelReceiptFetch      = errors.New("fast receipt download canceled (requested)")
	errCancelHeaderProcessing  = errors.New("fast header processing canceled (requested)")
	errCancelContentProcessing = errors.New("fast content processing canceled (requested)")
	errNoSyncActive            = errors.New("fast no sync active")
	errTooOld                  = errors.New("fast peer doesn't speak recent enough protocol version (need version >= 62)")
	errPeerNil                 = errors.New("peer is nil")
)

type SDownloader interface {
	SyncStateFd(root common.Hash) etrue.StateSyncInter
}

type Downloader struct {
	mode SyncMode       // Synchronisation mode defining the strategy used (per sync cycle)
	mux  *event.TypeMux // Event multiplexer to announce sync operation events

	genesis uint64         // Genesis block number to limit sync to (e.g. light client CHT)
	queue   *queue         // Scheduler for selecting the hashes to download
	peers   *etrue.PeerSet // Set of active peers from which download can proceed
	stateDB ethdb.Database

	rttEstimate   uint64 // Round trip time to target for download requests
	rttConfidence uint64 // Confidence in the estimated RTT (unit: millionths to allow atomic ops)

	// Statistics
	syncStatsChainOrigin uint64       // Origin block number where syncing started at
	syncStatsChainHeight uint64       // Highest block number known when syncing started
	syncStatsLock        sync.RWMutex // Lock protecting the sync stats fields

	lightchain LightChain
	blockchain BlockChain

	// Callbacks
	dropPeer etrue.PeerDropFn // Drops a peer for misbehaving

	// Status
	synchroniseMock func(id string, hash common.Hash) error // Replacement for synchronise during testing
	synchronising   int32
	notified        int32
	committed       int32

	// Channels
	headerCh      chan etrue.DataPack  // [eth/62] Channel receiving inbound block headers
	bodyCh        chan etrue.DataPack  // [eth/62] Channel receiving inbound block bodies
	receiptCh     chan etrue.DataPack  // [eth/63] Channel receiving inbound receipts
	bodyWakeCh    chan bool            // [eth/62] Channel to signal the block body fetcher of new tasks
	receiptWakeCh chan bool            // [eth/63] Channel to signal the receipt fetcher of new tasks
	headerProcCh  chan []*types.Header // [eth/62] Channel to feed the header processor new tasks

	// Cancellation and termination
	cancelPeer string         // Identifier of the peer currently being used as the master (cancel on drop)
	cancelCh   chan struct{}  // Channel to cancel mid-flight syncs
	cancelLock sync.RWMutex   // Lock to protect the cancel channel and peer in delivers
	cancelWg   sync.WaitGroup // Make sure all fetcher goroutines have exited.

	quitCh   chan struct{} // Quit channel to signal termination
	quitLock sync.RWMutex  // Lock to prevent double closes

	// Testing hooks
	syncInitHook     func(uint64, uint64)       // Method to call upon initiating a new sync run
	bodyFetchHook    func([]*types.Header)      // Method to call upon starting a block body fetch
	receiptFetchHook func([]*types.Header)      // Method to call upon starting a receipt fetch
	chainInsertHook  func([]*etrue.FetchResult) // Method to call upon inserting a chain of blocks (possibly in multiple invocations)

	remoteHeader *types.Header
	StateSync    etrue.StateSyncInter
	sDownloader  SDownloader
}

// LightChain encapsulates functions required to synchronise a light chain.
type LightChain interface {
	// HasHeader verifies a header's presence in the local chain.
	HasHeader(common.Hash, uint64) bool

	// GetHeaderByHash retrieves a header from the local chain.
	GetHeaderByHash(common.Hash) *types.Header

	// CurrentHeader retrieves the head header from the local chain.
	CurrentHeader() *types.Header

	// GetTd returns the total difficulty of a local block.
	GetTd(common.Hash, uint64) *big.Int

	// InsertHeaderChain inserts a batch of headers into the local chain.
	InsertHeaderChain([]*types.Header, int) (int, error)

	// Rollback removes a few recently added elements from the local chain.
	Rollback([]common.Hash)
}

// BlockChain encapsulates functions required to sync a (full or fast) blockchain.
type BlockChain interface {
	LightChain

	// HasBlock verifies a block's presence in the local chain.
	HasBlock(common.Hash, uint64) bool

	// HasFastBlock verifies a fast block's presence in the local chain.
	HasFastBlock(common.Hash, uint64) bool
	// GetBlockByHash retrieves a block from the local chain.
	GetBlockByHash(common.Hash) *types.Block

	// CurrentBlock retrieves the head block from the local chain.
	CurrentBlock() *types.Block

	// CurrentFastBlock retrieves the head fast block from the local chain.
	CurrentFastBlock() *types.Block

	// FastSyncCommitHead directly commits the head block to a certain entity.
	FastSyncCommitHead(common.Hash) error

	// InsertChain inserts a batch of blocks into the local chain.
	InsertChain(types.Blocks) (int, error)

	// InsertReceiptChain inserts a batch of receipts into the local chain.
	InsertReceiptChain(types.Blocks, []types.Receipts) (int, error)

	GetBlockNumber() uint64
}

// New creates a new downloader to fetch hashes and blocks from remote peers.
func New(mode SyncMode, stateDb ethdb.Database, mux *event.TypeMux, chain BlockChain, lightchain LightChain, dropPeer etrue.PeerDropFn) *Downloader {
	if lightchain == nil {
		lightchain = chain
	}

	dl := &Downloader{
		mode:          mode,
		stateDB:       stateDb,
		mux:           mux,
		queue:         newQueue(),
		peers:         etrue.NewPeerSet(),
		rttEstimate:   uint64(rttMaxEstimate),
		rttConfidence: uint64(1000000),
		blockchain:    chain,
		lightchain:    lightchain,
		dropPeer:      dropPeer,
		cancelCh:      make(chan struct{}),
		headerCh:      make(chan etrue.DataPack, 1),
		bodyCh:        make(chan etrue.DataPack, 1),
		receiptCh:     make(chan etrue.DataPack, 1),
		bodyWakeCh:    make(chan bool, 1),
		receiptWakeCh: make(chan bool, 1),
		headerProcCh:  make(chan []*types.Header, 1),

		quitCh: make(chan struct{}),
	}

	go dl.qosTuner()
	return dl
}

func (d *Downloader) GetBlockChain() BlockChain {
	return d.blockchain
}

func (d *Downloader) SetHeader(remote *types.Header) {
	d.remoteHeader = remote
}

func (d *Downloader) SetSync(StateSync etrue.StateSyncInter) {
	d.StateSync = StateSync
}

func (d *Downloader) SetSD(Sdownloader SDownloader) {
	d.sDownloader = Sdownloader
}

// Progress retrieves the synchronisation boundaries, specifically the origin
// block where synchronisation started at (may have failed/suspended); the block
// or header sync is currently at; and the latest known block which the sync targets.
//
// In addition, during the state download phase of fast synchronisation the number
// of processed and the total number of known states are also returned. Otherwise
// these are zero.
func (d *Downloader) Progress() ethereum.SyncProgress {
	// Lock the current stats and return the progress
	d.syncStatsLock.RLock()
	defer d.syncStatsLock.RUnlock()

	current := uint64(0)
	switch d.mode {
	case FullSync:
		current = d.blockchain.CurrentBlock().NumberU64()
	case FastSync:
		current = d.blockchain.CurrentFastBlock().NumberU64()
	case LightSync:
		current = d.lightchain.CurrentHeader().Number.Uint64()
	case SnapShotSync:
		current = d.lightchain.CurrentHeader().Number.Uint64()
	}
	return ethereum.SyncProgress{
		StartingBlock: d.syncStatsChainOrigin,
		CurrentBlock:  current,
		HighestBlock:  d.syncStatsChainHeight,
	}
}

// Synchronising returns whether the downloader is currently retrieving blocks.
func (d *Downloader) Synchronising() bool {
	return atomic.LoadInt32(&d.synchronising) > 0
}

// RegisterPeer injects a new download peer into the set of block source to be
// used for fetching hashes and blocks from.
func (d *Downloader) RegisterPeer(id string, version int, peer etrue.Peer) error {
	logger := log.New("peer", id)
	logger.Trace("Registering sync peer")

	if err := d.peers.Register(newPeerConnection(id, version, peer, logger)); err != nil {
		logger.Error("Failed to register sync peer", "err", err)
		return err
	}
	d.qosReduceConfidence()

	return nil
}

// RegisterLightPeer injects a light client peer, wrapping it so it appears as a regular peer.
func (d *Downloader) RegisterLightPeer(id string, version int, peer etrue.LightPeer) error {
	return d.RegisterPeer(id, version, &lightPeerWrapper{peer})
}

// UnregisterPeer remove a peer from the known list, preventing any action from
// the specified peer. An effort is also made to return any pending fetches into
// the queue.
func (d *Downloader) UnregisterPeer(id string) error {
	// Unregister the peer from the active peer set and revoke any fetch tasks
	logger := log.New("peer", id)
	logger.Trace("Unregistering sync peer")
	if err := d.peers.Unregister(id); err != nil {
		logger.Error("Failed to unregister sync peer", "err", err)
		return err
	}
	d.queue.Revoke(id)

	// If this peer was the master peer, abort sync immediately
	d.cancelLock.RLock()
	master := id == d.cancelPeer
	d.cancelLock.RUnlock()

	if master {
		d.cancel()
	}
	return nil
}

// Synchronise tries to sync up our local block chain with a remote peer, both
// adding various sanity checks as well as wrapping it with various log entries.
func (d *Downloader) Synchronise(id string, head common.Hash, td *big.Int, mode SyncMode, origin uint64, height uint64) error {
	//defer d.Terminate()

	err := d.synchronise(id, head, td, mode, origin, height)

	switch err {
	case nil:
	case errBusy:
	case types.ErrSnailHeightNotYet:
	case errTimeout, errBadPeer, errStallingPeer,
		errEmptyHeaderSet, errPeersUnavailable, errTooOld,
		errInvalidAncestor, errInvalidChain:
		log.Warn("Synchronisation failed, dropping peer", "peer", id, "err", err)
		if d.dropPeer == nil {
			// The dropPeer method is nil when `--copydb` is used for a local copy.
			// Timeouts can occur if e.g. compaction hits at the wrong time, and can be ignored
			log.Warn("Downloader wants to drop peer, but peerdrop-function is not set", "peer", id)
		} else {

			d.dropPeer(id)
		}
	default:
		log.Warn("Synchronisation failed, retrying", "err", err)
	}
	return err
}

// synchronise will select the peer and use it for synchronising. If an empty string is given
// it will use the best peer possible and synchronize if its TD is higher than our own. If any of the
// checks fail an error will be returned. This method is synchronous
func (d *Downloader) synchronise(id string, hash common.Hash, td *big.Int, mode SyncMode, origin uint64, height uint64) error {
	// Mock out the synchronisation if testing
	if d.synchroniseMock != nil {
		return d.synchroniseMock(id, hash)
	}
	// Make sure only one goroutine is ever allowed past this point at once
	if !atomic.CompareAndSwapInt32(&d.synchronising, 0, 1) {
		return errBusy
	}
	defer atomic.StoreInt32(&d.synchronising, 0)

	// Post a user notification of the sync (only once per session)
	if atomic.CompareAndSwapInt32(&d.notified, 0, 1) {
		log.Info("Fast Block synchronisation started", "origin", origin, "height", height)
	}

	// Reset the queue, peer set and wake channels to clean any internal leftover state
	d.queue.Reset()
	d.peers.Reset()

	for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
		select {
		case <-ch:
		default:
		}
	}
	for _, ch := range []chan etrue.DataPack{d.headerCh, d.bodyCh, d.receiptCh} {
		for empty := false; !empty; {
			select {
			case <-ch:
			default:
				empty = true
			}
		}
	}
	for empty := false; !empty; {
		select {
		case <-d.headerProcCh:
		default:
			empty = true
		}
	}
	// Create cancel channel for aborting mid-flight and mark the master peer
	d.cancelLock.Lock()
	d.cancelCh = make(chan struct{})
	d.cancelPeer = id
	d.cancelLock.Unlock()

	defer d.Cancel() // No matter what, we can't leave the cancel channel open

	d.mode = mode
	// Retrieve the origin peer and initiate the downloading process
	p := d.peers.Peer(id)
	if p == nil {
		log.Debug("fast synchronise err", "id", id)
		return errUnknownPeer
	}
	return d.syncWithPeer(p, hash, td, origin, height)
}

// syncWithPeer starts a block synchronization based on the hash chain from the
// specified peer and head hash.
func (d *Downloader) syncWithPeer(p etrue.PeerConnection, hash common.Hash, td *big.Int, origin uint64, height uint64) (err error) {

	if p.GetVersion() < 62 {
		return errTooOld
	}

	log.Debug("Fast Synchronising with the network", "peer", p.GetID(), "eth", p.GetVersion(), "head", hash, "td", td, "mode", d.mode, "origin", origin, "height", height)
	defer func(start time.Time) {
		log.Debug("Fast Synchronisation terminated", "elapsed", time.Since(start))
	}(time.Now())

	// Look up the sync boundaries: the common ancestor and the target block
	if err != nil {
		return err
	}
	d.syncStatsLock.Lock()
	if d.syncStatsChainHeight <= origin || d.syncStatsChainOrigin > origin {
		d.syncStatsChainOrigin = origin
	}
	d.syncStatsChainHeight = height
	d.syncStatsLock.Unlock()

	// Ensure our origin point is below any fast sync pivot point
	pivot := origin + height

	d.committed = 1
	if (d.mode == FastSync || d.mode == SnapShotSync) && pivot != 0 {
		d.committed = 0
	}
	// Initiate the sync using a concurrent header and content retrieval algorithm
	d.queue.Prepare(origin+1, d.mode)
	if d.syncInitHook != nil {
		d.syncInitHook(origin, height)
	}

	height_i := int(height)

	fetchers := []func() error{func() error { return d.fetchHeaders(p, origin+1, height_i, pivot) }}
	fetchers = append(fetchers, func() error { return d.fetchBodies(origin + 1) })
	fetchers = append(fetchers, func() error { return d.fetchReceipts(origin + 1) })
	fetchers = append(fetchers, func() error { return d.processHeaders(origin+1, pivot, td) })

	if d.mode == FastSync {
		fetchers = append(fetchers, d.processFastSyncContent)
	} else if d.mode == FullSync {
		fetchers = append(fetchers, d.processFullSyncContent)
	}

	return d.spawnSync(fetchers)
}

// spawnSync runs d.process and all given fetcher functions to completion in
// separate goroutines, returning the first error that appears.
func (d *Downloader) spawnSync(fetchers []func() error) error {
	errc := make(chan error, len(fetchers))
	d.cancelWg.Add(len(fetchers))
	for _, fn := range fetchers {
		fn := fn
		go func() {
			defer d.cancelWg.Done()
			errc <- fn()
		}()
	}
	// Wait for the first error, then terminate the others.
	var err error
	for i := 0; i < len(fetchers); i++ {
		if i == len(fetchers)-1 {
			// Close the queue when all fetchers have exited.
			// This will cause the block processor to end when
			// it has processed the queue.
			d.queue.Close()
		}

		if err = <-errc; err != nil {
			break
		}
	}

	d.queue.Close()
	d.Cancel()
	return err
}

// fetchHeight retrieves the head header of the remote peer to aid in estimating
// the total time a pending synchronisation would take.
func (d *Downloader) FetchHeight(id string, number uint64) (*types.Header, error) {

	if !atomic.CompareAndSwapInt32(&d.synchronising, 0, 1) {
		return nil, errBusy
	}
	defer atomic.StoreInt32(&d.synchronising, 0)

	return d.fetchHeight(id, number)
}

// fetchHeight retrieves the head header of the remote peer to aid in estimating
// the total time a pending synchronisation would take.
func (d *Downloader) fetchHeight(id string, number uint64) (*types.Header, error) {

	p := d.peers.Peer(id)
	if p == nil {
		return nil, errPeerNil
	}
	p.GetLog().Debug("Retrieving remote chain height")
	// Request the advertised remote head block and wait for the response
	if number != 0 {
		go p.GetPeer().RequestHeadersByNumber(number, 1, 0, false, true)
	} else {
		go p.GetPeer().RequestHeadersByHash(common.Hash{}, 0, 1, false, true)
	}

	timeout := time.After(time.Duration(10 * time.Second))
	for {
		select {

		case packet := <-d.headerCh:
			// Discard anything not from the origin peer
			if packet.PeerId() != p.GetID() {
				log.Debug("Received headers from incorrect peer", "peer", packet.PeerId())
				break
			}
			// Make sure the peer actually gave something valid
			headers := packet.(*headerPack).headers
			if len(headers) != 1 {
				p.GetLog().Debug("Multiple headers for single request", "headers", len(headers))
				return nil, errBadPeer
			}
			head := headers[0]
			p.GetLog().Debug("Remote head header identified", "number", head.Number, "hash", head.Hash())
			return head, nil

		case <-timeout:
			p.GetLog().Debug("Waiting for head header timed out")
			return nil, errTimeout

		case <-d.bodyCh:
		case <-d.receiptCh:
			// Out of bounds delivery, ignore
		}
	}
}

// cancel aborts all of the operations and resets the queue. However, cancel does
// not wait for the running download goroutines to finish. This method should be
// used when cancelling the downloads from inside the downloader.
func (d *Downloader) cancel() {
	// Close the current cancel channel
	d.cancelLock.Lock()
	if d.cancelCh != nil {
		select {
		case <-d.cancelCh:
			// Channel was already closed
		default:
			close(d.cancelCh)
		}
	}
	d.cancelLock.Unlock()
}

// Cancel aborts all of the operations and waits for all download goroutines to
// finish before returning.
func (d *Downloader) Cancel() {
	d.cancel()
	d.cancelWg.Wait()
}

// Terminate interrupts the downloader, canceling all pending operations.
// The downloader cannot be reused after calling Terminate.
func (d *Downloader) Terminate() {
	// Close the termination channel (make sure double close is allowed)
	d.quitLock.Lock()
	select {
	case <-d.quitCh:
	default:
		close(d.quitCh)
	}
	d.quitLock.Unlock()

	// Cancel any pending download requests
	d.Cancel()
}

// fetchHeaders keeps retrieving headers concurrently from the number
// requested, until no more are returned, potentially throttling on the way. To
// facilitate concurrency but still protect against malicious nodes sending bad
// headers, we construct a header chain skeleton using the "origin" peer we are
// syncing with, and fill in the missing headers using anyone else. Headers from
// other peers are only accepted if they map cleanly to the skeleton. If no one
// can fill in the skeleton - not even the origin peer - it's assumed invalid and
// the origin is dropped.
func (d *Downloader) fetchHeaders(p etrue.PeerConnection, from uint64, height int, pivot uint64) error {
	p.GetLog().Debug("Fast Directing header downloads", "origin", from)
	defer p.GetLog().Debug("Fast Header download terminated")

	// Create a timeout timer, and the associated header fetcher
	//skeleton := false           // Skeleton assembly phase or finishing up
	request := time.Now()       // time of the last skeleton fetch request
	timeout := time.NewTimer(0) // timer to dump a non-responsive active peer
	<-timeout.C                 // timeout channel should be initially empty
	defer timeout.Stop()

	var ttl time.Duration
	getHeaders := func(from uint64, height int) {
		request = time.Now()

		ttl = d.requestTTL()
		timeout.Reset(ttl)
		p.GetLog().Debug("Fast Fetching full headers", "count", height, "from", from)
		go p.GetPeer().RequestHeadersByNumber(from, height, 0, false, true)

	}

	// Start pulling the header chain skeleton until all is done
	getHeaders(from, height)

	for {
		select {
		case <-d.cancelCh:
			return errCancelHeaderFetch

		case packet := <-d.headerCh:
			// Make sure the active peer is giving us the skeleton headers
			if packet.PeerId() != p.GetID() {
				log.Debug("Fast Received skeleton from incorrect peer", "peer", packet.PeerId())
				break
			}
			headerReqTimer.UpdateSince(request)
			timeout.Stop()

			// If the skeleton's finished, pull any remaining head headers directly from the origin
			// If no more headers are inbound, notify the content fetchers and return
			if packet.Items() == 0 {
				// Don't abort header fetches while the pivot is downloading
				//if atomic.LoadInt32(&d.committed) == 0 && pivot <= from {
				// Pivot done (or not in fast sync) and no more headers, terminate the process
				p.GetLog().Debug("Fast No more fast headers available")
				select {
				case d.headerProcCh <- nil:
					return nil
				case <-d.cancelCh:
					return errCancelHeaderFetch
				}
			}
			headers := packet.(*headerPack).headers

			if n := len(headers); n > 0 {
				// Retrieve the current head we're at
				head := uint64(0)
				if d.mode == LightSync || d.mode == SnapShotSync {
					head = d.lightchain.CurrentHeader().Number.Uint64()
				} else {
					head = d.blockchain.CurrentFastBlock().NumberU64()
					if full := d.blockchain.CurrentBlock().NumberU64(); head < full {
						head = full
					}
				}
				// If the head is way older than this batch, delay the last few headers
				if head+uint64(reorgProtThreshold) < headers[n-1].Number.Uint64() {
					delay := reorgProtHeaderDelay
					if delay > n {
						delay = n
					}
					headers = headers[:n-delay]
				}
			}

			// Insert all the new headers and fetch the next batch
			if len(headers) > 0 {
				p.GetLog().Debug("d.headerProcCh <- headers ，Fast Scheduling new headers", "count", len(headers), "from", from)
				select {
				case d.headerProcCh <- headers:
				case <-d.cancelCh:
					return errCancelHeaderFetch
				}
			}

			d.headerProcCh <- nil
			return nil

		case <-timeout.C:
			if d.dropPeer == nil {
				// The dropPeer method is nil when `--copydb` is used for a local copy.
				// Timeouts can occur if e.g. compaction hits at the wrong time, and can be ignored
				p.GetLog().Warn("Fast Downloader wants to drop peer, but peerdrop-function is not set", "peer", p.GetID())
				break
			}
			// Header retrieval timed out, consider the peer bad and drop
			p.GetLog().Debug("Fast Header request timed out", "elapsed", ttl)
			headerTimeoutMeter.Mark(1)
			p.GetLog().Info("drop peer fast fetchHeaders timout ", "id", p.GetID())
			d.dropPeer(p.GetID())

			// Finish the sync gracefully instead of dumping the gathered data though
			for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
				select {
				case ch <- false:
				case <-d.cancelCh:
				}
			}
			select {
			case d.headerProcCh <- nil:
			case <-d.cancelCh:
			}
			return errBadPeer
		}
	}
}

// fetchBodies iteratively downloads the scheduled block bodies, taking any
// available peers, reserving a chunk of blocks for each, waiting for delivery
// and also periodically checking for timeouts.
func (d *Downloader) fetchBodies(from uint64) error {

	var (
		deliver = func(packet etrue.DataPack) (int, error) {
			pack := packet.(*bodyPack)
			return d.queue.DeliverBodies(pack.peerID, pack.transactions, pack.signs,pack.infos)
		}
		expire   = func() map[string]int { return d.queue.ExpireBodies(d.requestTTL()) }
		fetch    = func(p etrue.PeerConnection, req *etrue.FetchRequest) error { return p.FetchBodies(req) }
		capacity = func(p etrue.PeerConnection) int { return p.BlockCapacity(d.requestRTT()) }
		setIdle  = func(p etrue.PeerConnection, accepted int) { p.SetBodiesIdle(accepted) }
	)
	err := d.fetchParts(errCancelBodyFetch, d.bodyCh, deliver, d.bodyWakeCh, expire,
		d.queue.PendingBlocks, d.queue.InFlightBlocks, d.queue.ShouldThrottleBlocks, d.queue.ReserveBodies,
		d.bodyFetchHook, fetch, d.queue.CancelBodies, capacity, d.peers.BodyIdlePeers, setIdle, "bodies")

	log.Debug("Fast Block body download terminated", "err", err)
	return err
}

// fetchReceipts iteratively downloads the scheduled block receipts, taking any
// available peers, reserving a chunk of receipts for each, waiting for delivery
// and also periodically checking for timeouts.
func (d *Downloader) fetchReceipts(from uint64) error {
	log.Debug("Fast Downloading transaction receipts", "origin", from)

	var (
		deliver = func(packet etrue.DataPack) (int, error) {
			pack := packet.(*receiptPack)
			return d.queue.DeliverReceipts(pack.peerID, pack.receipts)
		}
		expire   = func() map[string]int { return d.queue.ExpireReceipts(d.requestTTL()) }
		fetch    = func(p etrue.PeerConnection, req *etrue.FetchRequest) error { return p.FetchReceipts(req) }
		capacity = func(p etrue.PeerConnection) int { return p.ReceiptCapacity(d.requestRTT()) }
		setIdle  = func(p etrue.PeerConnection, accepted int) { p.SetReceiptsIdle(accepted) }
	)
	err := d.fetchParts(errCancelReceiptFetch, d.receiptCh, deliver, d.receiptWakeCh, expire,
		d.queue.PendingReceipts, d.queue.InFlightReceipts, d.queue.ShouldThrottleReceipts, d.queue.ReserveReceipts,
		d.receiptFetchHook, fetch, d.queue.CancelReceipts, capacity, d.peers.ReceiptIdlePeers, setIdle, "receipts")
	log.Debug("Fast Transaction receipt download terminated", "err", err)
	return err
}

// fetchParts iteratively downloads scheduled block parts, taking any available
// peers, reserving a chunk of fetch requests for each, waiting for delivery and
// also periodically checking for timeouts.
//
// As the scheduling/timeout logic mostly is the same for all downloaded data
// types, this method is used by each for data gathering and is instrumented with
// various callbacks to handle the slight differences between processing them.
//
// The instrumentation parameters:
//  - errCancel:   error type to return if the fetch operation is cancelled (mostly makes logging nicer)
//  - deliveryCh:  channel from which to retrieve downloaded data packets (merged from all concurrent peers)
//  - deliver:     processing callback to deliver data packets into type specific download queues (usually within `queue`)
//  - wakeCh:      notification channel for waking the fetcher when new tasks are available (or sync completed)
//  - expire:      task callback method to abort requests that took too long and return the faulty peers (traffic shaping)
//  - pending:     task callback for the number of requests still needing download (detect completion/non-completability)
//  - inFlight:    task callback for the number of in-progress requests (wait for all active downloads to finish)
//  - throttle:    task callback to check if the processing queue is full and activate throttling (bound memory use)
//  - reserve:     task callback to reserve new download tasks to a particular peer (also signals partial completions)
//  - fetchHook:   tester callback to notify of new tasks being initiated (allows testing the scheduling logic)
//  - fetch:       network callback to actually send a particular download request to a physical remote peer
//  - cancel:      task callback to abort an in-flight download request and allow rescheduling it (in case of lost peer)
//  - capacity:    network callback to retrieve the estimated type-specific bandwidth capacity of a peer (traffic shaping)
//  - idle:        network callback to retrieve the currently (type specific) idle peers that can be assigned tasks
//  - setIdle:     network callback to set a peer back to idle and update its estimated capacity (traffic shaping)
//  - kind:        textual label of the type being downloaded to display in log mesages
func (d *Downloader) fetchParts(errCancel error, deliveryCh chan etrue.DataPack, deliver func(etrue.DataPack) (int, error), wakeCh chan bool,
	expire func() map[string]int, pending func() int, inFlight func() bool, throttle func() bool, reserve func(etrue.PeerConnection, int) (*etrue.FetchRequest, bool, error),
	fetchHook func([]*types.Header), fetch func(etrue.PeerConnection, *etrue.FetchRequest) error, cancel func(*etrue.FetchRequest), capacity func(etrue.PeerConnection) int,
	idle func() ([]etrue.PeerConnection, int), setIdle func(etrue.PeerConnection, int), kind string) error {

	// Create a ticker to detect expired retrieval tasks
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	update := make(chan struct{}, 1)

	// Prepare the queue and fetch block parts until the block header fetcher's done
	finished := false
	for {
		select {
		case <-d.cancelCh:
			return errCancel

		case packet := <-deliveryCh:

			// If the peer was previously banned and failed to deliver its pack
			// in a reasonable time frame, ignore its message.
			if peer := d.peers.Peer(packet.PeerId()); peer != nil {
				// Deliver the received chunk of data and check chain validity
				accepted, err := deliver(packet)
				if err == errInvalidChain {
					return err
				}
				// Unless a peer delivered something completely else than requested (usually
				// caused by a timed out request which came through in the end), set it to
				// idle. If the delivery's stale, the peer should have already been idled.
				if err != errStaleDelivery {
					setIdle(peer, accepted)
				}
				// Issue a log to the user to see what's going on
				switch {
				case err == nil && packet.Items() == 0:
					peer.GetLog().Trace("Fast Requested data not delivered", "type", kind)
				case err == nil:
					peer.GetLog().Trace("Fast Delivered new batch of data", "type", kind, "count", packet.Stats())
				default:
					peer.GetLog().Trace("Fast Failed to deliver retrieved data", "type", kind, "err", err)
				}
			}
			// Blocks assembled, try to update the progress
			select {
			case update <- struct{}{}:
			default:
			}

		case cont := <-wakeCh:
			// The header fetcher sent a continuation flag, check if it's done
			if !cont {
				finished = true
			}
			// Headers arrive, try to update the progress
			select {
			case update <- struct{}{}:
			default:
			}

		case <-ticker.C:
			// Sanity check update the progress
			select {
			case update <- struct{}{}:
			default:
			}

		case <-update:
			// Short circuit if we lost all our peers
			if d.peers.Len() == 0 {
				return errNoPeers
			}
			// Check for fetch request timeouts and demote the responsible peers
			for pid, fails := range expire() {
				if peer := d.peers.Peer(pid); peer != nil {
					// If a lot of retrieval elements expired, we might have overestimated the remote peer or perhaps
					// ourselves. Only reset to minimal throughput but don't drop just yet. If even the minimal times
					// out that sync wise we need to get rid of the peer.
					//
					// The reason the minimum threshold is 2 is because the downloader tries to estimate the bandwidth
					// and latency of a peer separately, which requires pushing the measures capacity a bit and seeing
					// how response times reacts, to it always requests one more than the minimum (i.e. min 2).
					if fails > 2 {
						peer.GetLog().Trace("Fast Data delivery timed out", "type", kind)
						setIdle(peer, 0)
					} else {
						peer.GetLog().Debug("Fast Stalling delivery, dropping", "type", kind)
						if d.dropPeer == nil {
							// The dropPeer method is nil when `--copydb` is used for a local copy.
							// Timeouts can occur if e.g. compaction hits at the wrong time, and can be ignored
							peer.GetLog().Warn("Fast Downloader wants to drop peer, but peerdrop-function is not set", "peer", pid)
						} else {
							peer.GetLog().Info("drop peer fast fetchParts", "id", peer.GetID(), "type", kind, "fails", fails)
							d.dropPeer(pid)
						}
					}
				}
			}
			// If there's nothing more to fetch, wait or terminate
			if pending() == 0 {
				if !inFlight() && finished {
					log.Debug("Fast Data fetching completed", "type", kind)
					return nil
				}
				break
			}
			// Send a download request to all idle peers, until throttled
			progressed, throttled, running := false, false, inFlight()
			idles, total := idle()

			for _, peer := range idles {
				// Short circuit if throttling activated
				if throttle() {
					throttled = true
					break
				}
				// Short circuit if there is no more available task.
				if pending() == 0 {
					break
				}
				// Reserve a chunk of fetches for a peer. A nil can mean either that
				// no more headers are available, or that the peer is known not to
				// have them.
				request, progress, err := reserve(peer, capacity(peer))
				if err != nil {
					return err
				}
				if progress {
					progressed = true
				}
				if request == nil {
					continue
				}
				if request.From > 0 {
					peer.GetLog().Trace("Fast Requesting new batch of data", "type", kind, "from", request.From)
				} else {
					peer.GetLog().Trace("Fast Requesting new batch of data", "type", kind, "count", len(request.Fheaders), "from", request.Fheaders[0].Number)
				}
				// Fetch the chunk and make sure any errors return the hashes to the queue
				if fetchHook != nil {
					fetchHook(request.Fheaders)
				}
				if err := fetch(peer, request); err != nil {
					// Although we could try and make an attempt to fix this, this error really
					// means that we've double allocated a fetch task to a peer. If that is the
					// case, the internal state of the downloader and the queue is very wrong so
					// better hard crash and note the error instead of silently accumulating into
					// a much bigger issue.
					panic(fmt.Sprintf("Fast %v: %s fetch assignment failed", peer, kind))
				}
				running = true
			}
			// Make sure that we have peers available for fetching. If all peers have been tried
			// and all failed throw an error
			if !progressed && !throttled && !running && len(idles) == total && pending() > 0 {
				return errPeersUnavailable
			}
		}
	}
}

// processHeaders takes batches of retrieved headers from an input channel and
// keeps processing and scheduling them into the header chain and downloader's
// queue until the stream ends or a failure occurs.
func (d *Downloader) processHeaders(origin uint64, pivot uint64, td *big.Int) error {
	// Keep a count of uncertain headers to roll back
	defer log.Debug("Fast processHeaders download terminated")

	rollback := []*types.Header{}
	defer func() {
		if len(rollback) > 0 {
			// Flatten the headers and roll them back
			hashes := make([]common.Hash, len(rollback))
			for i, header := range rollback {
				hashes[i] = header.Hash()
			}
			lastHeader, lastFastBlock, lastBlock := d.lightchain.CurrentHeader().Number, common.Big0, common.Big0
			if d.mode != LightSync && d.mode != SnapShotSync {
				lastFastBlock = d.blockchain.CurrentFastBlock().Number()
				lastBlock = d.blockchain.CurrentBlock().Number()
			}
			d.lightchain.Rollback(hashes)
			curFastBlock, curBlock := common.Big0, common.Big0
			if d.mode != LightSync && d.mode != SnapShotSync {
				curFastBlock = d.blockchain.CurrentFastBlock().Number()
				curBlock = d.blockchain.CurrentBlock().Number()
			}

			log.Warn("Rolled back headers", "count", len(hashes),
				"header", fmt.Sprintf("%d->%d", lastHeader, d.lightchain.CurrentHeader().Number),
				"fast", fmt.Sprintf("%d->%d", lastFastBlock, curFastBlock),
				"block", fmt.Sprintf("%d->%d", lastBlock, curBlock))
		}
	}()

	// Wait for batches of headers to process
	gotHeaders := false

	for {
		select {
		case <-d.cancelCh:
			return errCancelHeaderProcessing

		case headers := <-d.headerProcCh:
			//log.Debug("headers := <-d.headerProcCh:", "headers", headers)
			// Terminate header processing if we synced up
			if len(headers) == 0 {
				// Notify everyone that headers are fully processed
				for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
					select {
					case ch <- false:
					case <-d.cancelCh:
					}
				}
				// If no headers were retrieved at all, the peer violated its TD promise that it had a
				// better chain compared to ours. The only exception is if its promised blocks were
				// already imported by other means (e.g. fecher):
				//
				// R <remote peer>, L <local node>: Both at block 10
				// R: Mine block 11, and propagate it to L
				// L: Queue block 11 for import
				// L: Notice that R's head and TD increased compared to ours, start sync
				// L: Import of block 11 finishes
				// L: Sync begins, and finds common ancestor at 11
				// L: Request new headers up from 11 (R's TD was higher, it must have something)
				// R: Nothing to give
				if d.mode != LightSync && d.mode != SnapShotSync {
					//head := d.blockchain.CurrentBlock()
					//&& td.Cmp(d.blockchain.GetTd(head.Hash(), head.NumberU64())) > 0
					if !gotHeaders {
						return errStallingPeer
					}
				}

				// Disable any rollback and return
				rollback = nil
				return nil
			}
			// Otherwise split the chunk of headers into batches and process them
			gotHeaders = true

			for len(headers) > 0 {
				// Terminate if something failed in between processing chunks
				select {
				case <-d.cancelCh:
					return errCancelHeaderProcessing
				default:
				}
				// Select the next chunk of headers to import
				limit := maxHeadersProcess
				if limit > len(headers) {
					limit = len(headers)
				}
				chunk := headers[:limit]

				// In case of header only syncing, validate the chunk immediately
				if d.mode != FullSync {
					// Collect the yet unknown headers to mark them as uncertain
					unknown := make([]*types.Header, 0, len(headers))
					for _, header := range chunk {
						if !d.lightchain.HasHeader(header.Hash(), header.Number.Uint64()) {
							unknown = append(unknown, header)
						}
					}
					// If we're importing pure headers, verify based on their recentness
					frequency := fsHeaderCheckFrequency
					if n, err := d.lightchain.InsertHeaderChain(chunk, frequency); err != nil {
						// If some headers were inserted, add them too to the rollback list
						if n > 0 {
							rollback = append(rollback, chunk[:n]...)
						}
						log.Debug("Fast Invalid header encountered", "number", chunk[n].Number, "hash", chunk[n].Hash(), "err", err)
						return errInvalidChain
					}
					// All verifications passed, store newly found uncertain headers
					rollback = append(rollback, unknown...)
					if len(rollback) > fsHeaderSafetyNet {
						rollback = append(rollback[:0], rollback[len(rollback)-fsHeaderSafetyNet:]...)
					}
				}

				// Unless we're doing light chains, schedule the headers for associated content retrieval
				if d.mode == FullSync || d.mode == FastSync || d.mode == SnapShotSync {
					// If we've reached the allowed number of pending headers, stall a bit
					for d.queue.PendingBlocks() >= maxQueuedHeaders || d.queue.PendingReceipts() >= maxQueuedHeaders {
						select {
						case <-d.cancelCh:
							return errCancelHeaderProcessing
						case <-time.After(time.Second):
						}
					}
					// Otherwise insert the headers for content retrieval
					inserts := d.queue.Schedule(chunk, origin)
					if len(inserts) != len(chunk) {
						return errBadPeer
					}
				}
				headers = headers[limit:]
				origin += uint64(limit)
			}
			// Update the highest block number we know if a higher one is found.
			d.syncStatsLock.Lock()
			if d.syncStatsChainHeight < origin {
				d.syncStatsChainHeight = origin - 1
			}
			d.syncStatsLock.Unlock()

			// Signal the content downloaders of the availablility of new tasks
			for _, ch := range []chan bool{d.bodyWakeCh, d.receiptWakeCh} {
				select {
				case ch <- true:
				default:
				}
			}
		}
	}
}

// processFullSyncContent takes fetch results from the queue and imports them into the chain.
func (d *Downloader) processFullSyncContent() error {
	for {
		results := d.queue.Results(true)
		if len(results) == 0 {
			return nil
		}
		if d.chainInsertHook != nil {
			d.chainInsertHook(results)
		}
		if err := d.importBlockResults(results); err != nil {
			return err
		}
	}
}

func (d *Downloader) importBlockResults(results []*etrue.FetchResult) error {
	// Check for any early termination requests
	if len(results) == 0 {
		return nil
	}
	select {
	case <-d.quitCh:
		return errCancelContentProcessing
	default:
	}
	// Retrieve the a batch of results to import
	first, last := results[0].Fheader, results[len(results)-1].Fheader
	log.Debug("Inserting downloaded fast chain", "items", len(results),
		"firstnum", first.Number, "firsthash", first.Hash(),
		"lastnum", last.Number, "lasthash", last.Hash(),
	)
	blocks := make([]*types.Block, len(results))
	for i, result := range results {
		log.Trace(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>  fast block >>>>>>>>", "Number", result.Fheader.Number, "Phash", result.Fheader.ParentHash, "hash", result.Fheader.Hash())
		blocks[i] = types.NewBlockWithHeader(result.Fheader).WithBody(result.Transactions, result.Signs, nil)
		log.Trace("Fast downloader signs:", "block signs:", blocks[i].Signs())
	}
	log.Debug("Fast Downloaded>>>>", "CurrentBlock:", d.blockchain.CurrentBlock().NumberU64())
	if index, err := d.blockchain.InsertChain(blocks); err != nil {
		log.Error("Fast Downloaded item processing failed", "number", results[index].Fheader.Number, "hash", results[index].Fheader.Hash(), "err", err)
		if err == types.ErrSnailHeightNotYet {
			return err
		}
		return errInvalidChain
	}
	return nil
}

// processFastSyncContent takes fetch results from the queue and writes them to the
// database. It also controls the synchronisation of state nodes of the pivot block.
func (d *Downloader) processFastSyncContent() error {

	pivot := d.remoteHeader.Number.Uint64()

	// To cater for moving pivot points, track the pivot block and subsequently
	// accumulated download results separately.
	for {
		// Wait for the next batch of downloaded data to be available, and if the pivot
		// block became stale, move the goalpost

		results := d.queue.Results(true) // Block if we're not monitoring pivot staleness

		if len(results) == 0 {
			return nil
		}
		if d.chainInsertHook != nil {
			d.chainInsertHook(results)
		}
		// Split around the pivot block and process the two sides via fast/full sync
		P, beforeP, afterP := splitAroundPivot(pivot, results)

		if err := d.commitFastSyncData(beforeP); err != nil {
			return err
		}

		if P != nil {
			// If new pivot block found, cancel old state retrieval and restart

			d.StateSync.Cancel()

			stateSync := d.sDownloader.SyncStateFd(P.Fheader.Root)
			defer stateSync.Cancel()
			go func() {
				if err := stateSync.Wait(); err != nil && err != etrue.ErrCancelStateFetch {
					d.queue.Close() // wake up Results
				}
			}()

			// Wait for completion, occasionally checking for pivot staleness
			select {

			case <-stateSync.Done():
				if stateSync.Err() != nil {
					return stateSync.Err()
				}
				if err := d.commitPivotBlock(P); err != nil {
					log.Error("FastSyncCommitHead state ", "err", err)
					return err
				}
				atomic.StoreInt32(&d.committed, 1)

			case <-time.After(time.Second):
				continue
			}
		}

		// Fast sync done, pivot commit done, full import
		if err := d.importBlockResults(afterP); err != nil {
			return err
		}

	}
}

func splitAroundPivot(pivot uint64, results []*etrue.FetchResult) (p *etrue.FetchResult, before, after []*etrue.FetchResult) {
	for _, result := range results {
		num := result.Fheader.Number.Uint64()
		switch {

		case num < pivot || pivot == 0:
			before = append(before, result)
		case num == pivot:
			p = result
		default:
			after = append(after, result)
		}
	}
	return p, before, after
}

func (d *Downloader) commitPivotBlock(result *etrue.FetchResult) error {
	block := types.NewBlockWithHeader(result.Fheader).WithBody(result.Transactions, result.Signs, nil)
	log.Debug("Committing fast sync pivot as new head", "number", block.Number(), "hash", block.Hash())
	if _, err := d.blockchain.InsertReceiptChain([]*types.Block{block}, []types.Receipts{result.Receipts}); err != nil {
		return err
	}
	if err := d.blockchain.FastSyncCommitHead(block.Hash()); err != nil {
		return err
	}
	atomic.StoreInt32(&d.committed, 1)
	return nil
}

func (d *Downloader) commitFastSyncData(results []*etrue.FetchResult) error {
	// Check for any early termination requests
	if len(results) == 0 {
		return nil
	}
	select {
	case <-d.quitCh:
		return errCancelContentProcessing
	default:
	}
	// Retrieve the a batch of results to import
	first, last := results[0].Fheader, results[len(results)-1].Fheader
	log.Debug("Inserting fast-sync blocks", "items", len(results),
		"firstnum", first.Number, "firsthash", first.Hash(),
		"lastnumn", last.Number, "lasthash", last.Hash(),
	)
	blocks := make([]*types.Block, len(results))
	receipts := make([]types.Receipts, len(results))
	for i, result := range results {
		blocks[i] = types.NewBlockWithHeader(result.Fheader).WithBody(result.Transactions, result.Signs, nil)
		receipts[i] = result.Receipts
	}
	if index, err := d.blockchain.InsertReceiptChain(blocks, receipts); err != nil {
		log.Debug("Downloaded item processing failed", "number", results[index].Fheader.Number, "hash", results[index].Fheader.Hash(), "err", err)
		return errInvalidChain
	}
	return nil
}

// DeliverHeaders injects a new batch of block headers received from a remote
// node into the download schedule.
func (d *Downloader) DeliverHeaders(id string, headers []*types.Header) (err error) {
	return d.deliver(id, d.headerCh, &headerPack{id, headers}, headerInMeter, headerDropMeter)
}

// DeliverBodies injects a new batch of block bodies received from a remote node.
func (d *Downloader) DeliverBodies(id string, transactions [][]*types.Transaction, signs [][]*types.PbftSign,infos []*types.SwitchInfos) (err error) {
	return d.deliver(id, d.bodyCh, &bodyPack{id, transactions, signs,infos}, bodyInMeter, bodyDropMeter)
}

// DeliverReceipts injects a new batch of receipts received from a remote node.
func (d *Downloader) DeliverReceipts(id string, receipts [][]*types.Receipt) (err error) {
	return d.deliver(id, d.receiptCh, &receiptPack{id, receipts}, receiptInMeter, receiptDropMeter)
}


// deliver injects a new batch of data received from a remote node.
func (d *Downloader) deliver(id string, destCh chan etrue.DataPack, packet etrue.DataPack, inMeter, dropMeter metrics.Meter) (err error) {
	// Update the delivery metrics for both good and failed deliveries
	inMeter.Mark(int64(packet.Items()))
	defer func() {
		if err != nil {
			dropMeter.Mark(int64(packet.Items()))
		}
	}()

	// Deliver or abort if the sync is canceled while queuing
	d.cancelLock.RLock()
	cancel := d.cancelCh
	d.cancelLock.RUnlock()
	if cancel == nil {
		return errNoSyncActive
	}
	select {
	case destCh <- packet:
		return nil
	case <-cancel:
		return errNoSyncActive
	}
}

func (d *Downloader) deliverOne(id string, destCh chan etrue.DataPack, packet etrue.DataPack, inMeter, dropMeter metrics.Meter) (err error) {
	// Update the delivery metrics for both good and failed deliveries

	inMeter.Mark(int64(packet.Items()))
	defer func() {
		if err != nil {
			dropMeter.Mark(int64(packet.Items()))
		}
	}()
	// Deliver or abort if the sync is canceled while queuing
	//log.Debug("deliver <- packet ","packet",packet)
	select {
	case destCh <- packet:
		return nil
	}
}

// qosTuner is the quality of service tuning loop that occasionally gathers the
// peer latency statistics and updates the estimated request round trip time.
func (d *Downloader) qosTuner() {
	for {
		// Retrieve the current median RTT and integrate into the previoust target RTT
		rtt := time.Duration((1-qosTuningImpact)*float64(atomic.LoadUint64(&d.rttEstimate)) + qosTuningImpact*float64(d.peers.MedianRTT()))
		atomic.StoreUint64(&d.rttEstimate, uint64(rtt))

		// A new RTT cycle passed, increase our confidence in the estimated RTT
		conf := atomic.LoadUint64(&d.rttConfidence)
		conf = conf + (1000000-conf)/2
		atomic.StoreUint64(&d.rttConfidence, conf)

		// Log the new QoS values and sleep until the next RTT
		log.Debug("Recalculated fast downloader QoS values", "rtt", rtt, "confidence", float64(conf)/1000000.0, "ttl", d.requestTTL())
		select {
		case <-d.quitCh:
			return
		case <-time.After(rtt):
		}
	}
}

// qosReduceConfidence is meant to be called when a new peer joins the downloader's
// peer set, needing to reduce the confidence we have in out QoS estimates.
func (d *Downloader) qosReduceConfidence() {
	// If we have a single peer, confidence is always 1
	peers := uint64(d.peers.Len())
	if peers == 0 {
		// Ensure peer connectivity races don't catch us off guard
		return
	}
	if peers == 1 {
		atomic.StoreUint64(&d.rttConfidence, 1000000)
		return
	}
	// If we have a ton of peers, don't drop confidence)
	if peers >= uint64(qosConfidenceCap) {
		return
	}
	// Otherwise drop the confidence factor
	conf := atomic.LoadUint64(&d.rttConfidence) * (peers - 1) / peers
	if float64(conf)/1000000 < rttMinConfidence {
		conf = uint64(rttMinConfidence * 1000000)
	}
	atomic.StoreUint64(&d.rttConfidence, conf)

	rtt := time.Duration(atomic.LoadUint64(&d.rttEstimate))
	log.Debug("Relaxed fast downloader QoS values", "rtt", rtt, "confidence", float64(conf)/1000000.0, "ttl", d.requestTTL())
}

// requestRTT returns the current target round trip time for a download request
// to complete in.
//
// Note, the returned RTT is .9 of the actually estimated RTT. The reason is that
// the downloader tries to adapt queries to the RTT, so multiple RTT values can
// be adapted to, but smaller ones are preferred (stabler download stream).
func (d *Downloader) requestRTT() time.Duration {
	return time.Duration(atomic.LoadUint64(&d.rttEstimate)) * 9 / 10
}

// requestTTL returns the current timeout allowance for a single download request
// to finish under.
func (d *Downloader) requestTTL() time.Duration {
	var (
		rtt  = time.Duration(atomic.LoadUint64(&d.rttEstimate))
		conf = float64(atomic.LoadUint64(&d.rttConfidence)) / 1000000.0
	)
	ttl := time.Duration(ttlScaling) * time.Duration(float64(rtt)/conf)
	if ttl > ttlLimit {
		ttl = ttlLimit
	}
	return ttl
}
