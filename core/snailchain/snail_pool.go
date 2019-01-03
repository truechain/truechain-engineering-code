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

package snailchain

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/metrics"
)

const (
	fruitChanSize     = 1024
	chainHeadChanSize = 10
)

var (
	// ErrNotExist is returned if the fast block not exist in fastchain.
	ErrNotExist = errors.New("not exist")
)

var (
	// Metrics for the pending pool
	fruitPendingDiscardCounter = metrics.NewRegisteredCounter("fruitpool/pending/discard", nil)
	fruitpendingReplaceCounter = metrics.NewRegisteredCounter("fruitpool/pending/replace", nil)

	// Metrics for the allfruit pool
	allDiscardCounter = metrics.NewRegisteredCounter("fruitpool/all/discard", nil)
	allReplaceCounter = metrics.NewRegisteredCounter("fruitpool/all/replace", nil)

	evictionInterval    = time.Minute     // Time interval to check for evictable fruits
	statsReportInterval = 8 * time.Second // Time interval to report fruits pool stats
)

// SnailPoolConfig are the configuration parameters of the fruit pool.
type SnailPoolConfig struct {
	Journal    string        // Journal of local fruits to survive node restarts
	Rejournal  time.Duration // Time interval to regenerate the local fruit journal
	FruitCount uint64
}

// DefaultSnailPoolConfig contains the default configurations for the fruit
// pool.
var DefaultSnailPoolConfig = SnailPoolConfig{
	Journal:    "fruits.rlp",
	Rejournal:  time.Hour,
	FruitCount: 8192,
}

// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (config *SnailPoolConfig) sanitize() SnailPoolConfig {
	conf := *config
	if conf.Rejournal < time.Second {
		log.Warn("Sanitizing invalid snailpool journal time", "provided", conf.Rejournal, "updated", time.Second)
		conf.Rejournal = time.Second
	}
	return conf
}

// SnailPool contains all currently known fruit. fruits
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable fruits (which can be applied to the
// current state) and future fruits. fruits move between those
// two states over time as they are received and processed.
type SnailPool struct {
	config    SnailPoolConfig
	chain     core.SnailChain
	fastchain *core.BlockChain

	scope event.SubscriptionScope

	fruitFeed event.Feed
	mu        sync.RWMutex
	journal   *snailJournal // Journal of local fruit to back up to disk

	//chainHeadCh  chan ChainHeadEvent
	chainHeadCh  chan types.ChainSnailHeadEvent
	chainHeadSub event.Subscription

	validator core.SnailValidator

	engine consensus.Engine // Consensus engine used for validating

	muFruit sync.RWMutex

	allFruits    map[common.Hash]*types.SnailBlock
	fruitPending map[common.Hash]*types.SnailBlock

	newFruitCh chan *types.SnailBlock

	//header *types.Block
	header *types.SnailBlock
	wg     sync.WaitGroup // for shutdown sync
}

// NewSnailPool creates a new fruit pool to gather, sort and filter inbound
// fruits from the network.
func NewSnailPool(config SnailPoolConfig, fastBlockChain *core.BlockChain, chain core.SnailChain, engine consensus.Engine, sv core.SnailValidator) *SnailPool {

	//config SnailPoolConfig
	config = (&config).sanitize()

	// Create the fruit pool with its initial settings
	pool := &SnailPool{
		config:    config,
		fastchain: fastBlockChain,
		chain:     chain,
		engine:    engine,

		validator: sv,

		chainHeadCh: make(chan types.ChainSnailHeadEvent, chainHeadChanSize),

		newFruitCh:   make(chan *types.SnailBlock, fruitChanSize),
		allFruits:    make(map[common.Hash]*types.SnailBlock),
		fruitPending: make(map[common.Hash]*types.SnailBlock),
	}
	pool.reset(nil, chain.CurrentBlock())

	// Subscribe events from blockchain
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	//pool.minedFruitSub = pool.eventMux.Subscribe(NewMinedFruitEvent{})

	pool.header = pool.chain.CurrentBlock()

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()
	return pool
}

//Start load and  rotate Journal
func (pool *SnailPool) Start() {
	// If journaling is enabled, load fruit from disk
	if pool.config.Journal != "" {
		pool.journal = newSnailJournal(pool.config.Journal)
		if err := pool.journal.load(pool.AddLocals); err != nil {
			log.Warn("Failed to load fruit journal", "err", err)
		}
		if err := pool.journal.rotate(pool.local()); err != nil {
			log.Warn("Failed to rotate fruit journal", "err", err)
		}
	}
}

func (pool *SnailPool) compareFruit(f1, f2 *types.SnailBlock) int {
	if rst := f1.FruitDifficulty().Cmp(f2.FruitDifficulty()); rst < 0 {
		return -1
	} else if rst == 0 {
		if f1.Hash().Big().Cmp(f2.Hash().Big()) >= 0 {
			return -1
		}
	}

	return 1
}

func (pool *SnailPool) appendFruit(fruit *types.SnailBlock, append bool) error {
	if uint64(len(pool.allFruits)) >= pool.config.FruitCount {
		return core.ErrExceedNumber
	}
	pool.allFruits[fruit.FastHash()] = fruit

	if append {
		pool.fruitPending[fruit.FastHash()] = fruit
		log.Debug("addFruit", "fb number", fruit.FastNumber())
	}

	go pool.fruitFeed.Send(types.NewFruitsEvent{types.SnailBlocks{fruit}})

	return nil
}

// addFruit
func (pool *SnailPool) addFruit(fruit *types.SnailBlock) error {
	//if the new fruit's fbnumber less than,don't add
	headSnailBlock := pool.chain.CurrentBlock()
	if headSnailBlock.NumberU64() > 0 {
		fruits := headSnailBlock.Fruits()
		if fruits[len(fruits)-1].FastNumber().Cmp(fruit.FastNumber()) >= 0 {
			return nil
		}
	}

	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	//check number(fb)
	currentNumber := pool.fastchain.CurrentBlock().Number()
	if fruit.FastNumber().Cmp(currentNumber) > 0 {
		return pool.appendFruit(fruit, false)
	}

	//judge is the fb exist
	fb := pool.fastchain.GetBlock(fruit.FastHash(), fruit.FastNumber().Uint64())
	if fb == nil {
		log.Info("addFruit get block failed.", "number", fruit.FastNumber(), "hash", fruit.Hash(), "fHash", fruit.FastHash())
		return ErrNotExist
	}

	log.Debug("add fruit ", "fastnumber", fruit.FastNumber(), "hash", fruit.Hash())
	// compare with allFruits's fruit
	if f, ok := pool.allFruits[fruit.FastHash()]; ok {
		if err := pool.validator.ValidateFruit(fruit, nil, true); err != nil {
			log.Debug("addFruit validation fruit error ", "fruit ", fruit.Hash(), "number", fruit.FastNumber(), " err: ", err)
			return err
		}

		if rst := fruit.Difficulty().Cmp(f.Difficulty()); rst < 0 {
			return nil
		} else if rst == 0 {
			if fruit.Hash().Big().Cmp(f.Hash().Big()) >= 0 {
				return nil
			}
			return pool.appendFruit(fruit, true)
		} else {
			return pool.appendFruit(fruit, true)
		}
	} else {
		if err := pool.validator.ValidateFruit(fruit, nil, true); err != nil {
			if err == types.ErrSnailHeightNotYet {
				return pool.appendFruit(fruit, false)
			}
			log.Debug("addFruit validation fruit error ", "fruit ", fruit.Hash(), "number", fruit.FastNumber(), " err: ", err)
			return err
		}

		return pool.appendFruit(fruit, true)
	}

	return nil
}

// journalFruit adds the specified fruit to the local disk journal
func (pool *SnailPool) journalFruit(fruit *types.SnailBlock) {
	// Only journal if it's enabled
	if pool.journal == nil {
		return
	}
	if err := pool.journal.insert(fruit); err != nil {
		log.Warn("Failed to journal fruit", "err", err)
	}
}

// loop is the fruit pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and fruit
// eviction events.
func (pool *SnailPool) loop() {
	defer pool.wg.Done()

	// Start the stats reporting and fruit eviction tickers
	var prevPending, prevUnverified int

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	journal := time.NewTicker(pool.config.Rejournal)
	defer journal.Stop()

	// Track the previous head headers for fruit reorgs
	head := pool.chain.CurrentBlock()

	// Keep waiting for and reacting to the various events
	for {
		select {
		// Handle ChainHeadEvent
		case ev := <-pool.chainHeadCh:
			if ev.Block != nil {
				pool.mu.Lock()
				pool.reset(head, ev.Block)
				head = ev.Block

				pool.mu.Unlock()
			}

		case fruit := <-pool.newFruitCh:
			if fruit != nil {
				pool.addFruit(fruit)
			}

			// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return

			// Handle stats reporting ticks
		case <-report.C:
			pool.mu.RLock()
			pending, unverified := pool.stats()
			pool.mu.RUnlock()

			if pending != prevPending || unverified != prevUnverified {
				log.Debug("fruit pool status report", "pending", pending, "unverified", unverified)
				prevPending, prevUnverified = pending, unverified
			}

			// Handle local fruit journal rotation
		case <-journal.C:
			if pool.journal != nil {
				pool.mu.Lock()
				if err := pool.journal.rotate(pool.local()); err != nil {
					log.Warn("Failed to rotate local tx journal", "err", err)
				}
				pool.mu.Unlock()
			}
		}
	}
}

//get the old snailchian's fruits which need to be remined
func fruitsDifference(a, b []*types.SnailBlock) []*types.SnailBlock {
	keep := make([]*types.SnailBlock, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, f := range b {
		remove[f.Hash()] = struct{}{}
	}

	for _, f := range a {
		if _, ok := remove[f.Hash()]; !ok {
			keep = append(keep, f)
		}
	}

	return keep
}

// remove all the fruits included in the new snailblock
func (pool *SnailPool) removeWithLock(fruits []*types.SnailBlock) {
	if len(fruits) == 0 {
		return
	}
	maxFbNumber := fruits[len(fruits)-1].FastNumber()
	for _, fruit := range pool.allFruits {
		if fruit.FastNumber().Cmp(maxFbNumber) < 1 {
			log.Trace(" removeWithLock del fruit", "fb number", fruit.FastNumber())
			fruitPendingDiscardCounter.Inc(1)
			delete(pool.fruitPending, fruit.FastHash())
			allDiscardCounter.Inc(1)
			delete(pool.allFruits, fruit.FastHash())
		}
	}
}

// reset retrieves the current state of the blockchain and ensures the content
// of the fruit pool is valid with regard to the chain state.
func (pool *SnailPool) reset(oldHead, newHead *types.SnailBlock) {
	var reinject []*types.SnailBlock

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash() {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.Number().Uint64()
		newNum := newHead.Number().Uint64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep fruit reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all fruits into memory
			var discarded, included []*types.SnailBlock

			var (
			//rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.Number().Uint64())
			//add = pool.chain.GetBlock(newHead.Hash(), newHead.Number().Uint64())
			)
			rem := oldHead
			add := newHead
			//log.Debug("branching","oldHeadNumber",rem.NumberU64(),"newHeadNumber",add.NumberU64(),"oldHeadMaxFastNumber",rem.Fruits()[len(rem.Fruits())-1].FastNumber(),"newHeadMaxFastNumber",add.Fruits()[len(add.Fruits())-1].FastNumber())
			for rem.NumberU64() > add.NumberU64() {
				discarded = append(discarded, rem.Fruits()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by snail pool", "block", oldHead.Number(), "hash", oldHead.Hash())
					return
				}
			}
			for add.NumberU64() > rem.NumberU64() {
				included = append(included, add.Fruits()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by snail pool", "block", newHead.Number(), "hash", newHead.Hash())
					return
				}
			}
			for rem.Hash() != add.Hash() {
				discarded = append(discarded, rem.Fruits()...)
				if rem = pool.chain.GetBlock(rem.ParentHash(), rem.NumberU64()-1); rem == nil {
					log.Error("Unrooted old chain seen by snail pool", "block", oldHead.Number(), "hash", oldHead.Hash())
					return
				}
				included = append(included, add.Fruits()...)
				if add = pool.chain.GetBlock(add.ParentHash(), add.NumberU64()-1); add == nil {
					log.Error("Unrooted new chain seen by snail pool", "block", newHead.Number(), "hash", newHead.Hash())
					return
				}
			}
			//get the old snailchian's fruits which need to be remined
			reinject = fruitsDifference(discarded, included)
			pool.insertRestFruits(reinject)
		}
	}
	// Initialize the internal state to the current head
	if newHead == nil {
		newHead = pool.chain.CurrentBlock() // Special case during testing
	}
	// Inject any fruits discarded due to reorgs
	log.Debug("Reinjecting stale fruits", "count", len(reinject))

	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	//remove all the fruits included in the new snailblock
	pool.removeWithLock(newHead.Fruits())
	pool.removeUnfreshFruit()
	pool.header = pool.chain.CurrentBlock()
}

// Insert rest old fruit into allfruits and fruitPending
func (pool *SnailPool) insertRestFruits(reinject []*types.SnailBlock) error {
	pool.muFruit.Lock()

	defer pool.muFruit.Unlock()

	log.Debug("begininsertRestFruits", "len(reinject)", len(reinject))
	for _, fruit := range reinject {
		pool.allFruits[fruit.FastHash()] = fruit
		pool.fruitPending[fruit.FastHash()] = fruit
		fb := pool.fastchain.GetBlock(fruit.FastHash(), fruit.FastNumber().Uint64())
		if fb == nil {
			continue
		}
	}

	log.Debug("endinsertRestFruits", "len(reinject)", len(reinject))
	return nil
}

//remove unfresh fruit after rest
func (pool *SnailPool) removeUnfreshFruit() {
	for _, fruit := range pool.allFruits {
		// check freshness
		err := pool.engine.VerifyFreshness(pool.chain, fruit.Header(), nil, false)
		if err != nil {
			if err != types.ErrSnailHeightNotYet {
				log.Debug(" removeUnfreshFruit del fruit", "fb number", fruit.FastNumber())
				fruitPendingDiscardCounter.Inc(1)
				delete(pool.fruitPending, fruit.FastHash())
				allDiscardCounter.Inc(1)
				delete(pool.allFruits, fruit.FastHash())
			}
		}
	}
}

//RemovePendingFruitByFastHash remove unVerifyFreshness fruit
func (pool *SnailPool) RemovePendingFruitByFastHash(fasthash common.Hash) {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	fruitPendingDiscardCounter.Inc(1)
	delete(pool.fruitPending, fasthash)
	allDiscardCounter.Inc(1)
	delete(pool.allFruits, fasthash)
}

// Stop terminates the fruit pool.
func (pool *SnailPool) Stop() {
	// Unsubscribe all subscriptions registered from snailpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

	if pool.journal != nil {
		pool.journal.close()
	}
	log.Info("Snail pool stopped")
}

// AddRemoteFruits enqueues a batch of fruits into the pool if they are valid.
func (pool *SnailPool) AddRemoteFruits(fruits []*types.SnailBlock, local bool) []error {

	errs := make([]error, len(fruits))

	for i, fruit := range fruits {
		log.Trace("AddRemoteFruits", "number", fruit.FastNumber(), "diff", fruit.FruitDifficulty(), "pointer", fruit.PointNumber())
		if err := pool.validateFruit(fruit); err != nil {
			log.Debug("AddRemoteFruits validate fruit failed", "err fruit fb num", fruit.FastNumber(), "err", err)
			errs[i] = err
			continue
		}

		f := types.CopyFruit(fruit)
		pool.newFruitCh <- f
		if local {
			pool.journalFruit(fruit)
		}
	}

	return errs
}

// addLocalFruits enqueues a batch of fruits into the pool if they are valid.
func (pool *SnailPool) addLocalFruits(fruits []*types.SnailBlock) []error {

	errs := make([]error, len(fruits))

	for i, fruit := range fruits {
		log.Trace("addLocalFruits", "number", fruit.FastNumber(), "diff", fruit.FruitDifficulty(), "pointer", fruit.PointNumber())
		if err := pool.validateFruit(fruit); err != nil {
			log.Debug("addLocalFruits validate fruit failed", "err fruit fb num", fruit.FastNumber(), "err", err)
			errs[i] = err
			continue
		}

		f := types.CopyFruit(fruit)
		pool.newFruitCh <- f
	}

	return errs
}

// AddLocals enqueues a batch of fruits into the pool if they are valid,
// marking the senders as a local ones in the mean time, ensuring they go around
// the local pricing constraints.
func (pool *SnailPool) AddLocals(fruits []*types.SnailBlock) []error {
	return pool.addLocalFruits(fruits)
}

// local retrieves all currently known local fruits sorted by fast number. The returned fruit set is a copy and can be
// freely modified by calling code.
func (pool *SnailPool) local() []*types.SnailBlock {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	var fruits types.SnailBlocks
	var rtfruits types.SnailBlocks

	for _, fruit := range pool.allFruits {
		fruits = append(fruits, types.CopyFruit(fruit))
	}

	var blockby types.SnailBlockBy = types.FruitNumber
	blockby.Sort(fruits)

	for _, v := range fruits {
		rtfruits = append(rtfruits, v)
	}
	return rtfruits
}

// PendingFruits retrieves all currently verified fruits.
// The returned fruit set is a copy and can be freely modified by calling code.
func (pool *SnailPool) PendingFruits() map[common.Hash]*types.SnailBlock {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	rtfruits := make(map[common.Hash]*types.SnailBlock)
	for _, fruit := range pool.fruitPending {
		rtfruits[fruit.FastHash()] = types.CopyFruit(fruit)
	}
	return rtfruits
}

// SubscribeNewFruitEvent registers a subscription of NewFruitEvent and
// starts sending event to the given channel.
func (pool *SnailPool) SubscribeNewFruitEvent(ch chan<- types.NewFruitsEvent) event.Subscription {
	return pool.scope.Track(pool.fruitFeed.Subscribe(ch))
}

func (pool *SnailPool) validateFruit(fruit *types.SnailBlock) error {
	//check integrity
	getSignHash := types.CalcSignHash(fruit.Signs())
	if fruit.Header().SignHash != getSignHash {
		return ErrInvalidSignHash
	}
	// check freshness
	/*
		err := pool.engine.VerifyFreshness(fruit.Header(), nil)
		if err != nil {
			log.Debug("validateFruit verify freshness err","err", err, "fruit", fruit.FastNumber(), "hash", fruit.Hash())

			return nil
		}*/

	/*
		header := fruit.Header()
		if err := pool.engine.VerifySnailHeader(pool.chain, pool.fastchain, header, true); err != nil {
			log.Info("validateFruit verify header err", "err", err, "fruit", fruit.FastNumber(), "hash", fruit.Hash())
			return err
		}*/

	return nil
}

// Content returning all the
// pending fruits sorted by fast number.
func (pool *SnailPool) Content() []*types.SnailBlock {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	var fruits types.SnailBlocks
	var rtfruits types.SnailBlocks

	for _, fruit := range pool.fruitPending {
		fruits = append(fruits, types.CopyFruit(fruit))
	}

	var blockby types.SnailBlockBy = types.FruitNumber
	blockby.Sort(fruits)

	for _, v := range fruits {
		rtfruits = append(rtfruits, v)
	}
	return fruits
}

// Inspect returning all the
// unverifiedFruits fruits sorted by fast number.
func (pool *SnailPool) Inspect() []*types.SnailBlock {

	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	var fruits types.SnailBlocks
	var rtfruits types.SnailBlocks

	for _, fruit := range pool.allFruits {
		if _, ok := pool.fruitPending[fruit.FastHash()]; !ok {
			fruits = append(fruits, types.CopyFruit(fruit))
		}
	}

	var blockby types.SnailBlockBy = types.FruitNumber
	blockby.Sort(fruits)

	for _, v := range fruits {
		rtfruits = append(rtfruits, v)
	}
	return rtfruits
}

// Stats returning all the
// pending fruits count and unverifiedFruits fruits count.
func (pool *SnailPool) Stats() (int, int) {
	pool.mu.RLock()
	defer pool.mu.RUnlock()
	return pool.stats()
}

func (pool *SnailPool) stats() (int, int) {

	return len(pool.fruitPending), len(pool.allFruits) - len(pool.fruitPending)
}
