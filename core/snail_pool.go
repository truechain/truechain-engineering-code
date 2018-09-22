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
	//"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
)

const (
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	//recordChanSize = 100
	fastBlockChanSize = 100
	fruitChanSize     = 100

	fastchainHeadChanSize = 100
)

// freshFruitSize is the freshness of fruit according to the paper
var fruitFreshness *big.Int = big.NewInt(17)

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

// TxPoolConfig are the configuration parameters of the transaction pool.
type SnailPoolConfig struct {
	NoLocals  bool          // Whether local transaction handling should be disabled
	Journal   string        // Journal of local transactions to survive node restarts
	Rejournal time.Duration // Time interval to regenerate the local transaction journal

	PriceLimit uint64 // Minimum gas price to enforce for acceptance into the pool
	PriceBump  uint64 // Minimum price bump percentage to replace an already existing transaction (nonce)

	AccountSlots uint64 // Minimum number of executable transaction slots guaranteed per account
	GlobalSlots  uint64 // Maximum number of executable transaction slots for all accounts
	AccountQueue uint64 // Maximum number of non-executable transaction slots permitted per account
	GlobalQueue  uint64 // Maximum number of non-executable transaction slots for all accounts

	Lifetime time.Duration // Maximum amount of time non-executable transaction are queued
}

// DefaultTxPoolConfig contains the default configurations for the transaction
// pool.
var DefaultHybridPoolConfig = SnailPoolConfig{
	//Journal:   "records.rlp",
	Journal:   "fastBlocks.rlp",
	Rejournal: time.Hour,

	PriceLimit: 1,
	PriceBump:  10,

	AccountSlots: 16,
	GlobalSlots:  4096,
	AccountQueue: 64,
	GlobalQueue:  1024,

	Lifetime: 3 * time.Hour,
}

// sanitize checks the provided user configurations and changes anything that's
// unreasonable or unworkable.
func (config *SnailPoolConfig) sanitize() SnailPoolConfig {
	conf := *config
	if conf.Rejournal < time.Second {
		log.Warn("Sanitizing invalid txpool journal time", "provided", conf.Rejournal, "updated", time.Second)
		conf.Rejournal = time.Second
	}
	if conf.PriceLimit < 1 {
		log.Warn("Sanitizing invalid txpool price limit", "provided", conf.PriceLimit, "updated", DefaultTxPoolConfig.PriceLimit)
		conf.PriceLimit = DefaultHybridPoolConfig.PriceLimit
	}
	if conf.PriceBump < 1 {
		log.Warn("Sanitizing invalid txpool price bump", "provided", conf.PriceBump, "updated", DefaultTxPoolConfig.PriceBump)
		conf.PriceBump = DefaultHybridPoolConfig.PriceBump
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
	chain     *snailchain.SnailBlockChain
	fastchain *BlockChain
	gasPrice  *big.Int

	scope event.SubscriptionScope

	fruitFeed event.Feed
	//recordFeed event.Feed
	fastBlockFeed event.Feed
	mu            sync.RWMutex

	//chainHeadCh  chan ChainHeadEvent
	chainHeadCh  chan snailchain.ChainHeadEvent
	chainHeadSub event.Subscription

	fastchainHeadCh  chan ChainHeadEvent
	fastchainHeadSub event.Subscription

	engine consensus.Engine // Consensus engine used for validating

	muFruit     sync.RWMutex
	muFastBlock sync.RWMutex

	allFastBlocks map[common.Hash]*types.Block

	//fruitFastBlocks  map[common.Hash]*types.Block // the fastBlocks have fruit

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

	homestead bool
}

// NewSnailPool creates a new fruit/fastblock pool to gather, sort and filter inbound
// fruits/fastblock from the network.
func NewSnailPool(chainconfig *params.ChainConfig, fastBlockChain *BlockChain, chain *snailchain.SnailBlockChain, engine consensus.Engine) *SnailPool {
	// Sanitize the input to ensure no vulnerable gas prices are set
	//config = (&config).sanitize()
	config := DefaultHybridPoolConfig

	// Create the transaction pool with its initial settings
	pool := &SnailPool{
		config:      config,
		chainconfig: chainconfig,
		fastchain:   fastBlockChain,
		chain:       chain,
		engine:      engine,

		chainHeadCh:     make(chan snailchain.ChainHeadEvent, chainHeadChanSize),
		fastchainHeadCh: make(chan ChainHeadEvent, fastchainHeadChanSize),

		newFastBlockCh: make(chan *types.Block, fastBlockChanSize),

		allFastBlocks: make(map[common.Hash]*types.Block),

		//fastBlockList: 	list.New(),
		fastBlockPending: list.New(),

		newFruitCh:   make(chan *types.SnailBlock, fruitChanSize),
		allFruits:    make(map[common.Hash]*types.SnailBlock),
		fruitPending: make(map[common.Hash]*types.SnailBlock),
	}
	pool.reset(nil, chain.CurrentBlock())

	// If local transactions and journaling is enabled, load from disk
	if !config.NoLocals && config.Journal != "" {
	}
	// Subscribe events from blockchain
	pool.fastchainHeadSub = pool.fastchain.SubscribeChainHeadEvent(pool.fastchainHeadCh)
	pool.chainHeadSub = pool.chain.SubscribeChainHeadEvent(pool.chainHeadCh)

	//pool.minedFruitSub = pool.eventMux.Subscribe(NewMinedFruitEvent{})

	pool.header = pool.chain.CurrentBlock()

	//from snailchain get headchain's fb number
	var minFbNumber *big.Int
	headSnailBlock := pool.chain.CurrentBlock()
	if headSnailBlock.NumberU64() == 0 {
		/* genesis block */
		minFbNumber = new(big.Int).Set(common.Big1)
	} else {
		fruits := headSnailBlock.Fruits()
		minFbNumber = fruits[len(fruits)-1].FastNumber()
	}
	maxFbNumber := pool.fastchain.CurrentHeader().Number
	if maxFbNumber != nil && minFbNumber != nil {
		for i := new(big.Int).Add(minFbNumber, common.Big1); i.Cmp(maxFbNumber) <= 0; i = new(big.Int).Add(i, common.Big1) {
			fastblock := pool.fastchain.GetBlockByNumber(i.Uint64())
			pool.insertFastBlockWithLock(pool.fastBlockPending, fastblock)
			pool.allFastBlocks[fastblock.Hash()] = fastblock
		}
	}

	// Start the event loop and return
	pool.wg.Add(1)
	go pool.loop()

	return pool
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
	} else {
		if err := pool.chain.Validator().ValidateFruit(f, nil); err != nil {
			log.Info("update fruit validation error ", "fruit ", f.Hash(), "number", f.FastNumber(), " err: ", err)
			delete(pool.allFruits, fastBlock.Hash())
			delete(pool.fruitPending, fastBlock.Hash())
			return ErrInvalidHash
		}

		pool.fruitPending[fastBlock.Hash()] = f
	}
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
		pool.allFruits[fruit.FastHash()] = fruit
		// now can't confirm
		go pool.fruitFeed.Send(snailchain.NewFruitsEvent{types.SnailBlocks{fruit}})
		return nil
	}
	//judge is the fb exist
	fb := pool.fastchain.GetBlock(fruit.FastHash(), fruit.FastNumber().Uint64())
	if fb == nil {
		return ErrNotExist
	}

	// TODO: check signature
	//fruit validation
	if err := pool.chain.Validator().ValidateFruit(fruit, nil); err != nil {
		log.Info("add fruit validation fruit error ", "fruit ", fruit.Hash(), "number", fruit.FastNumber(), " err: ", err)
		return err
	}
	log.Info("add fruit ", "fastnumber", fruit.FastNumber(), "hash", fruit.Hash())
	// compare with allFruits's fruit
	if f, ok := pool.allFruits[fruit.FastHash()]; ok {
		if rst := pool.chain.GetBlockDifficulty(fruit).Cmp(pool.chain.GetBlockDifficulty(f)); rst < 0 {
			return nil
		} else if rst == 0 {
			if fruit.Hash().Big().Cmp(f.Hash().Big()) >= 0 {
				return nil
			}
			pool.allFruits[fruit.FastHash()] = fruit
			pool.fruitPending[fruit.FastHash()] = fruit
			go pool.fruitFeed.Send(snailchain.NewFruitsEvent{types.SnailBlocks{fruit}})
		} else {
			pool.allFruits[fruit.FastHash()] = fruit
			pool.fruitPending[fruit.FastHash()] = fruit
			go pool.fruitFeed.Send(snailchain.NewFruitsEvent{types.SnailBlocks{fruit}})
		}
	} else {
		pool.fruitPending[fruit.FastHash()] = fruit

		pool.allFruits[fruit.FastHash()] = fruit
		//the fruit already exists,so remove the fruit's fb from fastBlockPending
		log.Info("mine fruit to del fast block pending", "fb number", fruit.FastNumber())
		pool.muFastBlock.Lock()
		pool.removeFastBlockWithLock(pool.fastBlockPending, fruit.FastHash())
		pool.muFastBlock.Unlock()
		// send out
		go pool.fruitFeed.Send(snailchain.NewFruitsEvent{types.SnailBlocks{fruit}})
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

	pool.allFastBlocks[fastBlock.Hash()] = fastBlock
	//check fruit already exist
	if _, ok := pool.fruitPending[fastBlock.Hash()]; ok {
		return ErrMined
	}

	if _, ok := pool.allFruits[fastBlock.Hash()]; ok {
		if err := pool.updateFruit(fastBlock, true); err == nil {
			return ErrMined
		}
	}

	// TODO: check sign numbers

	pool.insertFastBlockWithLock(pool.fastBlockPending, fastBlock)

	go pool.fastBlockFeed.Send(snailchain.NewFastBlocksEvent{types.Blocks{fastBlock}})

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
				if pool.chainconfig.IsHomestead(ev.Block.Number()) {
					pool.homestead = true
				}
				pool.reset(head, ev.Block)
				head = ev.Block

				pool.mu.Unlock()
			}

		case ev := <-pool.fastchainHeadCh:
			if ev.Block != nil {
				pool.addFastBlock(ev.Block)
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

			// Handle local transaction journal rotation
		case <-journal.C:
			// TODO: support journal

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

// remove the record from pending list and unexecutable list
/*func (pool *SnailPool) removeRecordWithLock(fastBlockList *list.List, hash common.Hash) {
	for e := fastBlockList.Front(); e != nil; e = e.Next() {
		r := e.Value.(*types.PbftRecord)
		if r.Hash() == hash {
			fastBlockList.Remove(e)
			break
		}
	}
}*/

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
	for _, fruit := range fruits {
		//log.Info(" ********* del fruit","snail number", fruit.Number(),"fb number",fruit.FastNumber())
		delete(pool.fruitPending, fruit.FastHash())
		delete(pool.allFruits, fruit.FastHash())

		if _, ok := pool.allFastBlocks[fruit.FastHash()]; ok {
			//pool.removeFastBlockWithLock(pool.fastBlockList, fruit.FastHash())
			pool.removeFastBlockWithLock(pool.fastBlockPending, fruit.FastHash())
			delete(pool.allFastBlocks, fruit.FastHash())
		}
	}
}

// reset retrieves the current state of the blockchain and ensures the content
// of the fastblock pool is valid with regard to the chain state.
func (pool *SnailPool) reset(oldHead, newHead *types.SnailBlock) {
	// If we're reorging an old state, reinject all dropped fastblocks

	/*
		for _ , fb := range newHead.Fruits() {
			log.Info(" -----------------------------------reset fb list","sb number",newHead.Number(),"fb number",fb.FastNumber())
		}
	*/
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
				rem = pool.chain.GetBlock(oldHead.Hash(), oldHead.Number().Uint64())
				add = pool.chain.GetBlock(newHead.Hash(), newHead.Number().Uint64())
			)
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

	pool.header = pool.chain.CurrentBlock()
}

// Insert rest old fruit into allfruits and fruitPending
func (pool *SnailPool) insertRestFruits(reinject []*types.SnailBlock) error {

	pool.muFruit.Lock()
	defer pool.muFruit.Unlock()
	log.Debug("begininsertRestFruits","len(reinject)", len(reinject))
	for _, fruit := range reinject {
		pool.allFruits[fruit.FastHash()] = fruit
		pool.fruitPending[fruit.FastHash()] = fruit
		fb := pool.fastchain.GetBlock(fruit.FastHash(), fruit.FastNumber().Uint64())
		if fb == nil {
			continue
		}
		log.Debug("add to fastBlockPending","fb number",fb.Number())
		pool.insertFastBlockWithLock(pool.fastBlockPending, fb)
		log.Debug("add to allFastBlocks","fb number",fb.Number())
		pool.allFastBlocks[fruit.FastHash()] = fb

	}
	log.Debug("endinsertRestFruits","len(reinject)", len(reinject))
	return nil
}

// Stop terminates the transaction pool.
func (pool *SnailPool) Stop() {
	// Unsubscribe all subscriptions registered from txpool
	pool.scope.Close()

	// Unsubscribe subscriptions registered from blockchain
	pool.chainHeadSub.Unsubscribe()
	pool.wg.Wait()

	// TODO: journal close
	//if pool.journal != nil {
	//	pool.journal.close()
	//}
	log.Info("Transaction pool stopped")
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
		if err := pool.validateFruit(fruit); err != nil {
			errs[i] = err
			continue
		}

		f := types.CopyFruit(fruit)
		pool.newFruitCh <- f
	}

	return errs
}

// PendingFruits retrieves all currently verified fruits, sorted by fast number.
// The returned fruit set is a copy and can be freely modified by calling code.
func (pool *SnailPool) PendingFruits() ([]*types.SnailBlock, error) {
	// new flow return all fruits
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
	return rtfruits, nil
}

// SubscribeNewFruitsEvent registers a subscription of NewFruitEvent and
// starts sending event to the given channel.
func (pool *SnailPool) SubscribeNewFruitEvent(ch chan<- snailchain.NewFruitsEvent) event.Subscription {
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
	if pool.fastBlockPending.Front() != nil && pool.fastBlockPending.Back() !=nil{
		log.Info("$pending Fast Blocks","min fb num",pool.fastBlockPending.Front().Value.(*types.Block).Number()," ---- max fb num",pool.fastBlockPending.Back().Value.(*types.Block).Number())
	}
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
func (pool *SnailPool) SubscribeNewFastBlockEvent(ch chan<- snailchain.NewFastBlocksEvent) event.Subscription {
	return pool.scope.Track(pool.fastBlockFeed.Subscribe(ch))
}

func (pool *SnailPool) validateFruit(fruit *types.SnailBlock) error {

	//check integrity
	getSignHash := types.CalcSignHash(fruit.Signs())
	if fruit.Header().SignHash != getSignHash {
		return ErrInvalidSign
	}
	// check freshness
	err := pool.engine.VerifyFreshness(fruit, nil)
	if err != nil {
		log.Info("validateFruit verify freshness err","err", err, "fruit", fruit.FastNumber(), "hash", fruit.Hash())

		return nil
	}

	header := fruit.Header()
	if err := pool.engine.VerifySnailHeader(pool.chain, header, true); err != nil {
		log.Info("validateFruit verify header err", "err", err)
		return err
	}

	return nil
}
