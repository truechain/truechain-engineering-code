package types

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/trie"
	"hash"
	"sync"
	"time"
)

// stateReq represents a batch of state fetch requests grouped together into
// a single data retrieval network packet.
type StateReq struct {
	Items    []common.Hash              // Hashes of the state items to download
	tasks    map[common.Hash]*StateTask // Download tasks to track previous attempts
	Timeout  time.Duration              // Maximum round trip time for this to complete
	Timer    *time.Timer                // Timer to fire when the RTT timeout expires
	Peer     PeerConnection            // Peer that we're requesting from
	Response [][]byte                   // Response data of the peer (nil for timeouts)
	Dropped  bool                       // Flag whether the peer dropped off early
}

// timedOut returns if this request timed out.
func (req *StateReq) timedOut() bool {
	return req.Response == nil
}




// stateSync schedules requests for downloading a particular state trie defined
// by a given state root.
type StateSync struct {
	D *downloader.Downloader // Downloader instance to access and manage current peerset

	sched  *trie.Sync                 // State trie sync scheduler defining the tasks
	keccak hash.Hash                  // Keccak256 hasher to verify deliveries with
	tasks  map[common.Hash]*StateTask // Set of tasks currently queued for retrieval

	numUncommitted   int
	bytesUncommitted int

	Deliver    chan *StateReq // Delivery channel multiplexing peer responses
	cancel     chan struct{}  // Channel to signal a termination request
	cancelOnce sync.Once      // Ensures cancel only ever gets called once
	Done       chan struct{}  // Channel to signal termination completion
	Err        error          // Any error hit during sync (set before completion)
}

// stateTask represents a single trie node download task, containing a set of
// peers already attempted retrieval from to detect stalled syncs and abort.
type StateTask struct {
	attempts map[string]struct{}
}

// newStateSync creates a new state trie download scheduler. This method does not
// yet start the sync. The user needs to call run to initiate.
func NewStateSync(d *downloader.Downloader, root common.Hash) *StateSync {
	return &StateSync{
		D:       d,
		sched:   state.NewStateSync(root, d.StateDB),
		keccak:  sha3.NewKeccak256(),
		tasks:   make(map[common.Hash]*StateTask),
		Deliver: make(chan *StateReq),
		cancel:  make(chan struct{}),
		Done:    make(chan struct{}),
	}
}

// run starts the task assignment and response processing loop, blocking until
// it finishes, and finally notifying any goroutines waiting for the loop to
// finish.
func (s *StateSync) Run() {
	s.Err = s.Loop()
	close(s.Done)
}

// Wait blocks until the sync is done or canceled.
func (s *StateSync) Wait() error {
	<-s.Done
	return s.Err
}

// Cancel cancels the sync and waits until it has shut down.
func (s *StateSync) Cancel() error {
	s.cancelOnce.Do(func() { close(s.cancel) })
	return s.Wait()
}

// loop is the main event loop of a state trie sync. It it responsible for the
// assignment of new tasks to peers (including sending it to them) as well as
// for the processing of inbound data. Note, that the loop does not directly
// receive data from peers, rather those are buffered up in the downloader and
// pushed here async. The reason is to decouple processing from data receipt
// and timeouts.
func (s *StateSync) Loop() (err error) {
	// Listen for new peer events to assign tasks to them
	newPeer := make(chan PeerConnection, 1024)
	peerSub := s.D.Peers.SubscribeNewPeers(newPeer)
	defer peerSub.Unsubscribe()
	defer func() {
		cerr := s.commit(true)
		if err == nil {
			err = cerr
		}
	}()

	// Keep assigning new tasks until the sync completes or aborts
	for s.sched.Pending() > 0 {
		if err = s.commit(false); err != nil {
			return err
		}
		s.assignTasks()
		// Tasks assigned, wait for something to happen
		select {
		case <-newPeer:
			// New peer arrived, try to assign it download tasks

		case <-s.cancel:
			return ErrCancelStateFetch

		case <-s.D.CancelCh:
			return ErrCancelStateFetch

		case req := <-s.Deliver:
			// Response, disconnect or timeout triggered, drop the peer if stalling
			log.Trace("Received node data response", "peer", req.Peer.GetID(), "count", len(req.Response), "dropped", req.Dropped, "timeout", !req.Dropped && req.timedOut())
			if len(req.Items) <= 2 && !req.Dropped && req.timedOut() {
				// 2 items are the minimum requested, if even that times out, we've no use of
				// this peer at the moment.
				log.Warn("Stalling state sync, dropping peer", "peer", req.Peer.GetID())
				s.D.DropPeer(req.Peer.GetID())
			}
			// Process all the received blobs and check for stale delivery
			delivered, err := s.process(req)
			if err != nil {
				log.Warn("Node data write error", "err", err)
				return err
			}
			req.Peer.SetNodeDataIdle(delivered)
		}
	}
	return nil
}

func (s *StateSync) commit(force bool) error {
	if !force && s.bytesUncommitted < ethdb.IdealBatchSize {
		return nil
	}
	start := time.Now()
	b := s.D.StateDB.NewBatch()
	if written, err := s.sched.Commit(b); written == 0 || err != nil {
		return err
	}
	if err := b.Write(); err != nil {
		return fmt.Errorf("DB write error: %v", err)
	}
	s.updateStats(s.numUncommitted, 0, 0, time.Since(start))
	s.numUncommitted = 0
	s.bytesUncommitted = 0
	return nil
}

// assignTasks attempts to assign new tasks to all idle peers, either from the
// batch currently being retried, or fetching new data from the trie sync itself.
func (s *StateSync) assignTasks() {
	// Iterate over all idle peers and try to assign them state fetches
	peers, _ := s.D.Peers.NodeDataIdlePeers()
	for _, p := range peers {
		// Assign a batch of fetches proportional to the estimated latency/bandwidth
		cap := p.NodeDataCapacity(s.D.RequestRTT())
		req := &StateReq{Peer: p, Timeout: s.D.RequestTTL()}
		s.fillTasks(cap, req)

		// If the peer was assigned tasks to fetch, send the network request
		if len(req.Items) > 0 {
			req.Peer.GetLog().Trace("Requesting new batch of data", "type", "state", "count", len(req.Items))
			select {
			case s.D.TrackStateReq <- req:
				req.Peer.FetchNodeData(req.Items)
			case <-s.cancel:
			case <-s.D.CancelCh:
			}
		}
	}
}

// fillTasks fills the given request object with a maximum of n state download
// tasks to send to the remote peer.
func (s *StateSync) fillTasks(n int, req *StateReq) {
	// Refill available tasks from the scheduler.
	if len(s.tasks) < n {
		new := s.sched.Missing(n - len(s.tasks))
		for _, hash := range new {
			s.tasks[hash] = &StateTask{make(map[string]struct{})}
		}
	}
	// Find tasks that haven't been tried with the request's peer.
	req.Items = make([]common.Hash, 0, n)
	req.tasks = make(map[common.Hash]*StateTask, n)
	for hash, t := range s.tasks {
		// Stop when we've gathered enough requests
		if len(req.Items) == n {
			break
		}
		// Skip any requests we've already tried from this peer
		if _, ok := t.attempts[req.Peer.GetID()]; ok {
			continue
		}
		// Assign the request to this peer
		t.attempts[req.Peer.GetID()] = struct{}{}
		req.Items = append(req.Items, hash)
		req.tasks[hash] = t
		delete(s.tasks, hash)
	}
}

// process iterates over a batch of delivered state data, injecting each item
// into a running state sync, re-queuing any items that were requested but not
// delivered.
// Returns whether the peer actually managed to deliver anything of value,
// and any error that occurred
func (s *StateSync) process(req *StateReq) (int, error) {
	// Collect processing stats and update progress if valid data was received
	duplicate, unexpected, successful := 0, 0, 0

	defer func(start time.Time) {
		if duplicate > 0 || unexpected > 0 {
			s.updateStats(0, duplicate, unexpected, time.Since(start))
		}
	}(time.Now())

	// Iterate over all the delivered data and inject one-by-one into the trie
	progress := false

	for _, blob := range req.Response {
		prog, hash, err := s.processNodeData(blob)
		switch err {
		case nil:
			s.numUncommitted++
			s.bytesUncommitted += len(blob)
			progress = progress || prog
			successful++
		case trie.ErrNotRequested:
			unexpected++
		case trie.ErrAlreadyProcessed:
			duplicate++
		default:
			return successful, fmt.Errorf("invalid state node %s: %v", hash.TerminalString(), err)
		}
		if _, ok := req.tasks[hash]; ok {
			delete(req.tasks, hash)
		}
	}
	// Put unfulfilled tasks back into the retry queue
	npeers := s.D.Peers.Len()
	for hash, task := range req.tasks {
		// If the node did deliver something, missing items may be due to a protocol
		// limit or a previous timeout + delayed delivery. Both cases should permit
		// the node to retry the missing items (to avoid single-peer stalls).
		if len(req.Response) > 0 || req.timedOut() {
			delete(task.attempts, req.Peer.GetID())
		}
		// If we've requested the node too many times already, it may be a malicious
		// sync where nobody has the right data. Abort.
		if len(task.attempts) >= npeers {
			return successful, fmt.Errorf("state node %s failed with all peers (%d tries, %d peers)", hash.TerminalString(), len(task.attempts), npeers)
		}
		// Missing item, place into the retry queue.
		s.tasks[hash] = task
	}
	return successful, nil
}

// processNodeData tries to inject a trie node data blob delivered from a remote
// peer into the state trie, returning whether anything useful was written or any
// error occurred.
func (s *StateSync) processNodeData(blob []byte) (bool, common.Hash, error) {
	res := trie.SyncResult{Data: blob}
	s.keccak.Reset()
	s.keccak.Write(blob)
	s.keccak.Sum(res.Hash[:0])
	committed, _, err := s.sched.Process([]trie.SyncResult{res})
	return committed, res.Hash, err
}

// updateStats bumps the various state sync progress counters and displays a log
// message for the user to see.
func (s *StateSync) updateStats(written, duplicate, unexpected int, duration time.Duration) {
	s.D.SyncStatsLock.Lock()
	defer s.D.SyncStatsLock.Unlock()

	s.D.SyncStatsState.Pending = uint64(s.sched.Pending())
	s.D.SyncStatsState.Processed += uint64(written)
	s.D.SyncStatsState.duplicate += uint64(duplicate)
	s.D.SyncStatsState.unexpected += uint64(unexpected)

	if written > 0 || duplicate > 0 || unexpected > 0 {
		log.Info("Imported new state entries", "count", written, "elapsed", common.PrettyDuration(duration), "processed", s.D.SyncStatsState.Processed, "pending", s.D.SyncStatsState.Pending, "retry", len(s.tasks), "duplicate", s.D.SyncStatsState.duplicate, "unexpected", s.D.SyncStatsState.unexpected)
	}
	if written > 0 {
		rawdb.WriteFastTrieProgress(s.D.StateDB, s.D.SyncStatsState.Processed)
	}
}

// stateSyncStats is a collection of progress stats to report during a state trie
// sync to RPC requests as well as to display in user logs.
type StateSyncStats struct {
	Processed  uint64 // Number of state entries processed
	duplicate  uint64 // Number of state entries downloaded twice
	unexpected uint64 // Number of non-requested state entries received
	Pending    uint64 // Number of still pending state entries
}