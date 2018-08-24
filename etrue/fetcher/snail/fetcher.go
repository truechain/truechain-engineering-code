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

// Package fetcher contains the block announcement based synchronisation.
package snailfetcher

import (
	"errors"
	"time"

	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/log"
	"gopkg.in/karalabe/cookiejar.v2/collections/prque"
)

const (
	arriveTimeout = 500 * time.Millisecond // Time allowance before an announced block is explicitly requested
	gatherSlack   = 100 * time.Millisecond // Interval used to collate almost-expired announces with fetches
	fetchTimeout  = 5 * time.Second        // Maximum allotted time to return an explicitly requested block
	maxUncleDist  = 0                      // Maximum allowed backward distance from the chain head
	maxQueueDist  = 32                     // Maximum allowed distance from the chain head to queue
	hashLimit     = 256                    // Maximum number of unique blocks a peer may have announced
	blockLimit    = 64                     // Maximum number of unique blocks a peer may have delivered
	signLimit     = 64                     // Maximum number of unique sign a peer may have delivered
	maxChanelSign = 64                     // Maximum number of unique sign a peer may have cache
)

var (
	errTerminated = errors.New("terminated")
)

// blockRetrievalFn is a callback type for retrieving a block from the local chain.
type blockRetrievalFn func(common.Hash) *types.SnailBlock

// headerRequesterFn is a callback type for sending a header retrieval request.
type headerRequesterFn func(common.Hash) error

// bodyRequesterFn is a callback type for sending a body retrieval request.
type bodyRequesterFn func([]common.Hash) error

// headerVerifierFn is a callback type to verify a block's header for fast propagation.
type headerVerifierFn func(header *types.SnailHeader) error

// blockBroadcasterFn is a callback type for broadcasting a block to connected peers.
type blockBroadcasterFn func(block *types.SnailBlock, propagate bool)

// chainHeightFn is a callback type to retrieve the current chain height.
type chainHeightFn func() uint64

// chainInsertFn is a callback type to insert a batch of blocks into the local chain.
type chainInsertFn func(types.SnailBlocks) (int, error)

// peerDropFn is a callback type for dropping a peer detected as malicious.
type peerDropFn func(id string)

// inject represents a schedules import operation.
type inject struct {
	origin string
	block  *types.SnailBlock
}

// injectMulti represents more schedules import operation.
type injectMulti struct {
	origins []string
	blocks  []*types.SnailBlock
}

// Fetcher is responsible for accumulating block announcements from various peers
// and scheduling them for retrieval.
type Fetcher struct {
	// Various event channels
	inject chan *inject

	blockFilter chan chan []*types.SnailBlock

	done chan common.Hash
	quit chan struct{}

	// Block cache
	queue  *prque.Prque            // Queue containing the import operations (block number sorted)
	queues map[string]int          // Per peer block counts to prevent memory exhaustion
	queued map[common.Hash]*inject // Set of already queued blocks (to dedupe imports)

	// Callbacks
	getBlock           blockRetrievalFn           // Retrieves a block from the local chain
	verifyHeader       headerVerifierFn           // Checks if a block's headers have a valid proof of work
	broadcastFastBlock blockBroadcasterFn         // Broadcasts a block to connected peers
	chainHeight        chainHeightFn              // Retrieves the current chain's height
	insertChain        chainInsertFn              // Injects a batch of blocks into the chain
	dropPeer           peerDropFn                 // Drops a peer for misbehaving
	blockMultiHash     map[*big.Int][]common.Hash //solve same height more block question

	// Testing hooks
	queueChangeHook func(common.Hash, bool) // Method to call upon adding or deleting a block from the import queue
	importedHook    func(*types.SnailBlock) // Method to call upon successful block import (both eth/61 and eth/62)
}

// New creates a block fetcher to retrieve blocks based on hash announcements.
func New(getBlock blockRetrievalFn, verifyHeader headerVerifierFn, broadcastFastBlock blockBroadcasterFn, chainHeight chainHeightFn, insertChain chainInsertFn, dropPeer peerDropFn) *Fetcher {
	return &Fetcher{
		inject:      make(chan *inject),
		blockFilter: make(chan chan []*types.SnailBlock),
		done:        make(chan common.Hash),
		quit:        make(chan struct{}),

		queue:  prque.New(),
		queues: make(map[string]int),
		queued: make(map[common.Hash]*inject),

		getBlock:           getBlock,
		verifyHeader:       verifyHeader,
		broadcastFastBlock: broadcastFastBlock,
		chainHeight:        chainHeight,
		insertChain:        insertChain,
		dropPeer:           dropPeer,
		blockMultiHash:     make(map[*big.Int][]common.Hash),
	}
}

// Start boots up the announcement based synchroniser, accepting and processing
// hash notifications and block fetches until termination requested.
func (f *Fetcher) Start() {
	go f.loop()
}

// Stop terminates the announcement based synchroniser, canceling all pending
// operations.
func (f *Fetcher) Stop() {
	close(f.quit)
}

// Enqueue tries to fill gaps the the fetcher's future import queue.
func (f *Fetcher) Enqueue(peer string, block *types.SnailBlock) error {
	op := &inject{
		origin: peer,
		block:  block,
	}
	select {
	case f.inject <- op:
		return nil
	case <-f.quit:
		return errTerminated
	}
}

// Loop is the main fetcher loop, checking and processing various notification
// events.
func (f *Fetcher) loop() {
	// Iterate the block fetching until a quit is requested

	for {
		finished := false
		// Import any queued blocks that could potentially fit
		height := f.chainHeight()
		for !f.queue.Empty() {

			opMulti := f.queue.PopItem().(injectMulti)
			blocks := opMulti.blocks
			peers := opMulti.origins

			if len(blocks) > 0 {
				for i := 0; i < len(blocks); i++ {
					block := blocks[i]
					hash := block.Hash()
					peer := peers[i]

					if f.queueChangeHook != nil {
						f.queueChangeHook(hash, false)
					}
					// If too high up the chain or phase, continue later
					number := block.NumberU64()
					if number > height+1 {
						f.queue.Push(opMulti, -float32(number))
						if f.queueChangeHook != nil {
							f.queueChangeHook(hash, true)
						}
						finished = true
						break
					}
					// Otherwise if fresh and still unknown, try and import
					if number+maxUncleDist < height || f.getBlock(hash) != nil {
						f.forgetBlock(hash)
						continue
					}
					if _, err := f.insertChain(types.SnailBlocks{block}); err != nil {
						log.Debug("Propagated block import failed", "peer", peer, "number", block.Number(), "hash", hash, "err", err)
						f.done <- hash
						break
					}
					f.verifyBlockBroadcast(peer, block)
				}
			}
			if finished {
				break
			}
		}

		// Wait for an outside event to occur
		select {
		case <-f.quit:
			// Fetcher terminating, abort all operations
			return

		case op := <-f.inject:
			// A direct block insertion was requested, try and fill any pending gaps
			propBroadcastInMeter.Mark(1)
			f.enqueue(op.origin, op.block)

		case hash := <-f.done:
			// A pending import finished, remove all traces of the notification
			f.forgetBlock(hash)

		}
	}
}

// enqueue schedules a new future import operation, if the block to be imported
// has not yet been seen.
func (f *Fetcher) enqueue(peer string, block *types.SnailBlock) {
	hash := block.Hash()

	// Ensure the peer isn't DOSing us
	count := f.queues[peer] + 1
	if count > blockLimit {
		log.Debug("Discarded propagated block, exceeded allowance", "peer", peer, "number", block.Number(), "hash", hash, "limit", blockLimit)
		propBroadcastDOSMeter.Mark(1)
		return
	}
	// Discard any past or too distant blocks
	if dist := int64(block.NumberU64()) - int64(f.chainHeight()); dist < -maxUncleDist || dist > maxQueueDist {
		log.Debug("Discarded propagated block, too far away", "peer", peer, "number", block.Number(), "hash", hash, "distance", dist)
		propBroadcastDropMeter.Mark(1)
		return
	}

	// Schedule the block for future importing
	if _, ok := f.queued[hash]; !ok {
		op := &inject{
			origin: peer,
			block:  block,
		}
		f.queues[peer] = count
		f.queued[hash] = op

		opMulti := injectMulti{}
		if blockHsahs, ok := f.blockMultiHash[block.Number()]; ok {
			for _, hash := range blockHsahs {
				op := f.queued[hash]
				f.blockMultiHash[block.Number()] = append(f.blockMultiHash[block.Number()], hash)
				opMulti.origins = append(opMulti.origins, op.origin)
				opMulti.blocks = append(opMulti.blocks, op.block)
			}
		} else {
			f.blockMultiHash[block.Number()] = append(f.blockMultiHash[block.Number()], hash)
			opMulti.origins = append(opMulti.origins, op.origin)
			opMulti.blocks = append(opMulti.blocks, op.block)
		}

		f.queue.Push(opMulti, -float32(block.NumberU64()))
		if f.queueChangeHook != nil {
			f.queueChangeHook(op.block.Hash(), true)
		}
		log.Debug("Queued propagated block", "peer", peer, "number", block.Number(), "hash", hash, "queued", f.queue.Size())
	}
}

func (f *Fetcher) verifyBlockBroadcast(peer string, block *types.SnailBlock) {
	hash := block.Hash()

	// Run the import on a new thread
	log.Debug("Importing propagated block", "peer", peer, "number", block.Number(), "hash", hash)
	go func() {
		// If the parent's unknown, abort insertion
		parent := f.getBlock(block.ParentHash())
		if parent == nil {
			log.Debug("Unknown parent of propagated block", "peer", peer, "number", block.Number(), "hash", hash, "parent", block.ParentHash())
			f.done <- hash
			return
		}

		// Quickly validate the header and propagate the block if it passes
		switch err := f.verifyHeader(block.Header()); err {
		case nil:
			// All ok, quickly propagate to our peers
			propBroadcastOutTimer.UpdateSince(block.ReceivedAt)
			go f.broadcastFastBlock(block, true)

		case consensus.ErrFutureBlock:
			// Weird future block, don't fail, but neither propagate

		default:
			// Something went very wrong, drop the peer
			log.Debug("Propagated block verification failed", "peer", peer, "number", block.Number(), "hash", hash, "err", err)
			f.done <- hash
			return
		}
		// If import succeeded, broadcast the block
		go f.broadcastFastBlock(block, false)

		// Invoke the testing hook if needed
		if f.importedHook != nil {
			f.importedHook(block)
		}
	}()
}

// GetPendingBlock gets a block that is not inserted locally
func (f *Fetcher) GetPendingBlock(hash common.Hash) *types.SnailBlock {
	if _, ok := f.queued[hash]; !ok {
		return nil
	} else {
		return f.queued[hash].block
	}
}

// forgetBlock removes all traces of a queued block from the fetcher's internal
// state.
func (f *Fetcher) forgetBlock(hash common.Hash) {
	if insert := f.queued[hash]; insert != nil {
		f.queues[insert.origin]--
		if f.queues[insert.origin] == 0 {
			delete(f.queues, insert.origin)
		}
		delete(f.queued, hash)
	}
}
