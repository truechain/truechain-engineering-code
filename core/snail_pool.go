// Copyright 2018 The Truechain Authors
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

package core

import (
	"errors"
	"math"
	"math/big"
	"sync"
	"time"

	"container/list"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/metrics"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	//recordChanSize = 100
	fastBlockChanSize = 1024
	fruitChanSize     = 1024

	fastchainHeadChanSize = 1024
)

// freshFruitSize is the freshness of fruit according to the paper
var fruitFreshness = big.NewInt(17)

var (
	// ErrInvalidSender is returned if the transaction contains an invalid signature.
	ErrInvalidSign = errors.New("invalid sign")

	ErrInvalidPointer = errors.New("invalid pointer block")

	ErrExist = errors.New("already exist")

	ErrNotExist = errors.New("not exist")

	ErrInvalidHash = errors.New("invalid hash")

	//ErrFreshness          = errors.New("fruit not fresh")
	ErrMined              = errors.New("already mined")
	ErrNoFastBlockToMiner = errors.New("the fastblocks is null")
)

var (
	// Metrics for the pending pool
	fruitPendingDiscardCounter = metrics.NewRegisteredCounter("fruitpool/pending/discard", nil)
	fruitpendingReplaceCounter = metrics.NewRegisteredCounter("fruitpool/pending/replace", nil)

	// Metrics for the allfruit pool
	allDiscardCounter = metrics.NewRegisteredCounter("fruitpool/all/discard", nil)
	allReplaceCounter = metrics.NewRegisteredCounter("fruitpool/all/replace", nil)
)

// SnailChain defines a small collection of methods needed to access the local snail block chain.
// Temporary interface for snail block chain
type SnailChain interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *params.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() *types.SnailHeader

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *types.SnailHeader

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.SnailHeader

	// GetHeaderByHash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash common.Hash) *types.SnailHeader

	// CurrentBlock retrieves the current block from the local chain.
	CurrentBlock() *types.SnailBlock

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *types.SnailBlock

	// GetBlockByNumber retrieves a snail block from the database by number.
	GetBlockByNumber(number uint64) *types.SnailBlock

	// GetBlockByHash retrieves a snail block from the database by its hash.
	GetBlockByHash(hash common.Hash) *types.SnailBlock

	SubscribeChainHeadEvent(ch chan<- types.ChainSnailHeadEvent) event.Subscription
}

// TxPoolConfig are the configuration parameters of the transaction pool.
type SnailPoolConfig struct {
	NoLocals  bool          // Whether local transaction handling should be disabled
	Journal   string        // Journal of local fruits to survive node restarts
	Rejournal time.Duration // Time interval to regenerate the local fruit journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Minimum number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued

	FruitCount uint64
	FastCount  uint64
}

// DefaultTxPoolConfig contains the default configurations for the transaction
// pool.
var DefaultSnailPoolConfig = SnailPoolConfig{
	//Journal: "fruits.rlp",
	Journal: "fruits.rlp",
	//Journal:   "fastBlocks.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime:   3 * time.Hour,
	FruitCount: 8192,
	FastCount:  8192,
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

// TxPool contains all currently known transactions. Transactions
// enter the pool when they are received from the network or submitted
// locally. They exit the pool when they are included in the blockchain.
//
// The pool separates processable transactions (which can be applied to the
// current state) and future transactions. Transactions move between those
// two states over time as they are received and processed.
type SnailPool struct {
	config      SnailPoolConfig
	chainconfig *params.ChainConfig
	//chain       *BlockChain
	chain     SnailChain
	fastchain *BlockChain
	gasPrice  *big.Int

	scope event.SubscriptionScope

	fruitFeed     event.Feed
	fastBlockFeed event.Feed
	mu            sync.RWMutex
	journal       *snailJournal // Journal of local fruit to back up to disk

	//chainHeadCh  chan ChainHeadEvent
	chainHeadCh  chan types.ChainSnailHeadEvent
	chainHeadSub event.Subscription

	fastchainEventCh  chan types.ChainFastEvent
	fastchainEventSub event.Subscription

	validator SnailValidator

	engine consensus.Engine // Consensus engine used for validating

	muFruit     sync.RWMutex
	muFastBlock sync.RWMutex

	allFastBlocks map[common.Hash]*types.Block

	//fastBlockList    *list.List
	fastBlockPending *list.List

	newFastBlockCh chan *types.Block

	allFruits    map[common.Hash]*types.SnailBlock
	fruitPending map[common.Hash]*types.SnailBlock

	newFruitCh chan *types.SnailBlock

	//header *types.Block
	header *types.SnailBlock

	gasUsed uint64
	gasPool *GasPool // available gas used to pack transactions

	wg sync.WaitGroup // for shutdown sync
}

// NewSnailPool creates a new fruit/fastblock pool to gather, sort and filter inbound
// fruits/fastblock from the network.
func NewSnailPool(config SnailPoolConfig, fastBlockChain *BlockChain, chain SnailChain, engine consensus.Engine, sv SnailValidator) *SnailPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	//config SnailPoolConfig
	config = (&config).sanitize()
	//config := DefaultSnailPoolConfig

	// Create the transaction pool with its initial settings
	pool := &SnailPool{
		config:    config,
		fastchain: fastBlockChain,
		chain:     chain,
		engine:    engine,

		validator: sv,

		chainHeadCh:      make(chan types.ChainSnailHeadEvent, chainHeadChanSize),
		fastchainEventCh: make(chan types.ChainFastEvent, fastchainHeadChanSize),

		newFastBlockCh: make(chan *types.Block, fastBlockChanSize),

		allFastBlocks: make(map[common.Hash]*types.Block),

		fastBlockPending: list.New(),

		newFruitCh:   make(chan *types.SnailBlock, fruitChanSize),
		allFruits:    make(map[common.Hash]*types.SnailBlock),
		fruitPending: make(map[common.Hash]*types.SnailBlock),
	}
	pool.reset(nil, chain.CurrentBlock())

	// Subscribe events from blockchain
	pool.fastchainEventSub = pool.fastchain.SubscribeChainEvent(pool.fastchainEventCh)
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	//pool.minedFruitSub = pool.eventMux.Subscribe(NewMinedFruitEvent{})

	pool.header = pool.chain.CurrentBlock()

	//from snailchain get headchain's fb number
	var minFbNumber *big.Int
	headSnailBlock := pool.chain.CurrentBlock()
	if headSnailBlock.NumberU64() == 0 {
		/* genesis block */
		minFbNumber = new(big.Int).Set(common.Big0)
	} else {
		fruits := headSnailBlock.Fruits()
		minFbNumber = fruits[len(fruits)-1].FastNumber()
	}
	maxFbNumber := pool.fastchain.CurrentBlock().Number()
	if maxFbNumber != nil && minFbNumber != nil {
		for i := new(big.Int).Add(minFbNumber, common.Big1); i.Cmp(maxFbNumber) <= 0; i = new(big.Int).Add(i, common.Big1) {
			fastblock := pool.fastchain.GetBlockByNumber(i.Uint64())

			log.Debug("add fastblock", "number", fastblock.Number())
			pool.insertFastBlockWithLock(pool.fastBlockPending, fastblock)
			pool.allFastBlocks[fastblock.Hash()] = fastblock
		}
	}

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()
	return pool
}

func (pool *SnailPool) Start() {
	// If journaling is enabled, load from disk
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

//updateFruit move the validated fruit to pending list
func (pool *SnailPool) updateFruit(fastBlock *types.Block, toLock bool) error {
	if toLock {
		pool.muFruit.Lock()
		defer pool.muFruit.Unlock()
	}

	f := pool.allFruits[fastBlock.Hash()]
	if f == nil {
		return ErrNotExist
	}
	if err := pool.validator.ValidateFruit(f, nil, true); err != nil {
		log.Info("update fruit validation error ", "fruit ", f.Hash(), "number", f.FastNumber(), " err: ", err)
		allReplaceCounter.Inc(1)
		delete(pool.allFruits, fastBlock.Hash())
		fruitpendingReplaceCounter.Inc(1)
		delete(pool.fruitPending, fastBlock.Hash())
		return ErrInvalidHash
	}

	pool.fruitPending[fastBlock.Hash()] = f

	return nil
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
		return ErrExceedNumber
	}
	pool.allFruits[fruit.FastHash()] = fruit

	if append {
		pool.fruitPending[fruit.FastHash()] = fruit

		log.Debug("addFruit to del fast block pending", "fb number", fruit.FastNumber())
		pool.muFastBlock.Lock()
		pool.removeFastBlockWithLock(pool.fastBlockPending, fruit.FastHash())
		pool.muFastBlock.Unlock()
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

	// TODO: check signature
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

func (pool *SnailPool) addFastBlock(fastBlock *types.Block) error {
	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()

	//check exist
	if _, ok := pool.allFastBlocks[fastBlock.Hash()]; ok {
		return ErrExist
	}

	if uint64(len(pool.allFastBlocks)) >= pool.config.FastCount {
		return ErrExceedNumber
	}

	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	log.Debug("addFastBlock", "fast", fastBlock.Number(), "hash", fastBlock.Hash())

	pool.allFastBlocks[fastBlock.Hash()] = fastBlock
	//check fruit already exist
	if _, ok := pool.fruitPending[fastBlock.Hash()]; ok {
		return ErrMined
	}

	if _, ok := pool.allFruits[fastBlock.Hash()]; ok {
		if err := pool.updateFruit(fastBlock, false); err == nil {
			return ErrMined
		}
	}

	// TODO: check sign numbers
	pool.insertFastBlockWithLock(pool.fastBlockPending, fastBlock)

	go pool.fastBlockFeed.Send(types.NewFastBlocksEvent{types.Blocks{fastBlock}})

	return nil
}

// loop is the transaction pool's main event loop, waiting for and reacting to
// outside blockchain events as well as for various reporting and transaction
// eviction events.
func (pool *SnailPool) loop() {
	defer pool.wg.Done()

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	evict := time.NewTicker(evictionInterval)
	defer evict.Stop()

	journal := time.NewTicker(pool.config.Rejournal)
	defer journal.Stop()

	// Track the previous head headers for transaction reorgs
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

		case ev := <-pool.fastchainEventCh:
			if ev.Block != nil {
				log.Debug("get new fastblock", "number", ev.Block.Number())
				go pool.AddRemoteFastBlock([]*types.Block{ev.Block})
				//pool.addFastBlock(ev.Block)
			}

		case fruit := <-pool.newFruitCh:
			if fruit != nil {
				pool.addFruit(fruit)
			}

		case fastBlock := <-pool.newFastBlockCh:
			if fastBlock != nil {
				pool.addFastBlock(fastBlock)
			}

			// Be unsubscribed due to system stopped
		case <-pool.chainHeadSub.Err():
			return

			// Handle stats reporting ticks
		case <-report.C:
			// TODO: pool report

			// Handle inactive account transaction eviction
		case <-evict.C:
			pool.mu.Lock()
			pool.mu.Unlock()

			// Handle local fruit journal rotation
		case <-journal.C:
			// TODO: support journal
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

// removeFastBlockWithLock remove the fastblock from pending list and unexecutable list
func (pool *SnailPool) removeFastBlockWithLock(fastBlockList *list.List, hash common.Hash) {
	for e := fastBlockList.Front(); e != nil; e = e.Next() {
		r := e.Value.(*types.Block)
		if r.Hash() == hash {
			fastBlockList.Remove(e)
			break
		}
	}
}

// remove all the fruits and fastBlocks included in the new snailblock
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
			/*if _, ok := pool.allFastBlocks[fruit.FastHash()]; ok {
				pool.removeFastBlockWithLock(pool.fastBlockPending, fruit.FastHash())
				delete(pool.allFastBlocks, fruit.FastHash())
			}*/
		}
	}
	for _, fastblcok := range pool.allFastBlocks {
		if fastblcok.Number().Cmp(maxFbNumber) < 1 {
			log.Trace(" removeWithLock del fastblcok", "fb number", fastblcok.Number())
			pool.removeFastBlockWithLock(pool.fastBlockPending, fastblcok.Hash())
			delete(pool.allFastBlocks, fastblcok.Hash())
		}
	}
}

// reset retrieves the current state of the blockchain and ensures the content
// of the fastblock pool is valid with regard to the chain state.
func (pool *SnailPool) reset(oldHead, newHead *types.SnailBlock) {
	var reinject []*types.SnailBlock

	if oldHead != nil && oldHead.Hash() != newHead.ParentHash() {
		// If the reorg is too deep, avoid doing it (will happen during fast sync)
		oldNum := oldHead.Number().Uint64()
		newNum := newHead.Number().Uint64()

		if depth := uint64(math.Abs(float64(oldNum) - float64(newNum))); depth > 64 {
			log.Debug("Skipping deep transaction reorg", "depth", depth)
		} else {
			// Reorg seems shallow enough to pull in all fastblocks into memory
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
	// Inject any fastblocks discarded due to reorgs
	log.Debug("Reinjecting stale fruits", "count", len(reinject))

	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()

	//remove all the fruits and fastBlocks included in the new snailblock
	pool.removeWithLock(newHead.Fruits())
	pool.removeUnfreshFruit()
	pool.header = pool.chain.CurrentBlock()
}

// Insert rest old fruit into allfruits and fruitPending
func (pool *SnailPool) insertRestFruits(reinject []*types.SnailBlock) error {
	pool.muFruit.Lock()
	pool.muFastBlock.Lock()

	defer pool.muFruit.Unlock()
	defer pool.muFastBlock.Unlock()

	log.Debug("begininsertRestFruits", "len(reinject)", len(reinject))
	for _, fruit := range reinject {
		pool.allFruits[fruit.FastHash()] = fruit
		pool.fruitPending[fruit.FastHash()] = fruit
		fb := pool.fastchain.GetBlock(fruit.FastHash(), fruit.FastNumber().Uint64())
		if fb == nil {
			continue
		}
		log.Debug("add to fastBlockPending", "fb number", fb.Number())
		pool.insertFastBlockWithLock(pool.fastBlockPending, fb)
		log.Debug("add to allFastBlocks", "fb number", fb.Number())
		pool.allFastBlocks[fruit.FastHash()] = fb
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

				fastblock := pool.fastchain.GetBlock(fruit.FastHash(), fruit.FastNumber().Uint64())
				if fastblock == nil {
					return
				}
				log.Debug("add fastblock", "number", fastblock.Number())
				pool.insertFastBlockWithLock(pool.fastBlockPending, fastblock)
				pool.allFastBlocks[fastblock.Hash()] = fastblock
			}
		}
	}
}

func (pool *SnailPool) RemovePendingFruitByFastHash(fasthash common.Hash) {
	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()

	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()

	fruitPendingDiscardCounter.Inc(1)
	delete(pool.fruitPending, fasthash)
	allDiscardCounter.Inc(1)
	delete(pool.allFruits, fasthash)

	fastblock := pool.fastchain.GetBlockByHash(fasthash)
	if fastblock == nil {
		return
	}
	log.Debug("add fastblock", "number", fastblock.Number())
	pool.insertFastBlockWithLock(pool.fastBlockPending, fastblock)
	pool.allFastBlocks[fastblock.Hash()] = fastblock
}

// Stop terminates the transaction pool.
func (pool *SnailPool) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

	// TODO: journal close
	if pool.journal != nil {
		pool.journal.close()
	}
	log.Info("Snail pool stopped")
}

// GasPrice returns the current gas price enforced by the transaction pool.
func (pool *SnailPool) GasPrice() *big.Int {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	return new(big.Int).Set(pool.gasPrice)
}

// AddRemoteFruits enqueues a batch of fruits into the pool if they are valid.
func (pool *SnailPool) AddRemoteFruits(fruits []*types.SnailBlock) []error {

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

// Insert fastblock into list order by fastblock number
func (pool *SnailPool) insertFastBlockWithLock(fastBlockList *list.List, fastBlock *types.Block) error {

	log.Debug("snail pool fast block pending", "number", fastBlock.Number(), "hash", fastBlock.Hash(), "count", fastBlockList.Len())

	for lr := fastBlockList.Front(); lr != nil; lr = lr.Next() {
		f := lr.Value.(*types.Block)
		if f.Number().Cmp(fastBlock.Number()) > 0 {
			fastBlockList.InsertBefore(fastBlock, lr)
			return nil
		}
	}
	fastBlockList.PushBack(fastBlock)

	return nil
}

// AddRemoteFastBlock is for test only
func (pool *SnailPool) AddRemoteFastBlock(fastBlocks []*types.Block) []error {
	errs := make([]error, len(fastBlocks))

	for _, fastBlock := range fastBlocks {
		f := types.NewBlockWithHeader(fastBlock.Header()).WithBody(fastBlock.Transactions(), fastBlock.Signs(), nil)
		pool.newFastBlockCh <- f
	}

	return errs
}

// PendingFastBlocks retrieves one currently fast block.
// The returned fast block is a copy and can be freely modified by calling code.
func (pool *SnailPool) PendingFastBlocks() ([]*types.Block, error) {
	pool.muFastBlock.Lock()
	defer pool.muFastBlock.Unlock()
	var fastblocks types.Blocks

	for fastblock := pool.fastBlockPending.Front(); fastblock != nil; fastblock = fastblock.Next() {
		block := fastblock.Value.(*types.Block)
		fastBlock := types.NewBlockWithHeader(block.Header()).WithBody(block.Transactions(), block.Signs(), nil)
		fastblocks = append(fastblocks, fastBlock)
	}
	/*
	if pool.fastBlockPending.Front() != nil && pool.fastBlockPending.Back() !=nil{
		log.Info("$pending Fast Blocks","min fb num",pool.fastBlockPending.Front().Value.(*types.Block).Number()," ---- max fb num",pool.fastBlockPending.Back().Value.(*types.Block).Number())
	}*/
	var blockby types.BlockBy = types.Number
	blockby.Sort(fastblocks)
	return fastblocks, nil

	/*
		pool.muFastBlock.Lock()
		defer pool.muFastBlock.Unlock()

		first := pool.fastBlockPending.Front()
		if first == nil {
			return nil, nil
		}
		block := first.Value.(*types.Block)
		fastBlock := types.NewBlockWithHeader(block.Header()).WithBody(block.Transactions(), block.Signs(), nil)
		return fastBlock, nil*/

}

// SubscribeNewFastBlockEvent registers a subscription of NewFastBlocksEvent
// and starts sending event to the given channel.
func (pool *SnailPool) SubscribeNewFastBlockEvent(ch chan<- types.NewFastBlocksEvent) event.Subscription {
	return pool.scope.Track(pool.fastBlockFeed.Subscribe(ch))
}

func (pool *SnailPool) validateFruit(fruit *types.SnailBlock) error {
	//check integrity
	getSignHash := types.CalcSignHash(fruit.Signs())
	if fruit.Header().SignHash != getSignHash {
		return ErrInvalidSign
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
// unVerifiedFruits fruits sorted by fast number.
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
// pending fruits count and unVerifiedFruits fruits count.
func (pool *SnailPool) Stats() (pending int, unVerified int) {

	return len(pool.fruitPending), len(pool.allFruits) - len(pool.fruitPending)
}
