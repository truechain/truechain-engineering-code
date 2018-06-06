// BlockChain represents the canonical chain given a database with a genesis
// block. The Blockchain manages chain imports, reverts, chain reorganisations.
// BlockChain 表示了一个规范的链,这个链通过一个包含了创世区块的数据库指定. BlockChain管理了链的插入,还原,重建等操作.
// Importing blocks in to the block chain happens according to the set of rules
// defined by the two stage Validator. Processing of blocks is done using the
// Processor which processes the included transaction. The validation of the state
// is done in the second part of the Validator. Failing results in aborting of
// the import.
// 插入一个区块需要通过一系列指定的规则指定的两阶段的验证器.
// 使用Processor来对区块的交易进行处理. 状态的验证是第二阶段的验证器. 错误将导致插入终止.
// The BlockChain also helps in returning blocks from **any** chain included
// in the database as well as blocks that represents the canonical chain. It's
// important to note that GetBlock can return any block and does not need to be
// included in the canonical one where as GetBlockByNumber always represents the
// canonical chain.
// 需要注意的是GetBlock可能返回任意不在当前规范区块链中的区块,
// 但是GetBlockByNumber 总是返回当前规范区块链中的区块.

type BlockChain struct {
		config *params.ChainConfig // chain & network configuration

		hc            *HeaderChain		// 只包含了区块头的区块链
		chainDb       ethdb.Database	// 底层数据库
		rmLogsFeed    event.Feed		// 下面是很多消息通知的组件
		chainFeed     event.Feed
		chainSideFeed event.Feed
		chainHeadFeed event.Feed
		logsFeed      event.Feed
		scope         event.SubscriptionScope
		genesisBlock  *types.Block		// 创世区块

		mu      sync.RWMutex // global mutex for locking chain operations
		chainmu sync.RWMutex // blockchain insertion lock
		procmu  sync.RWMutex // block processor lock

		checkpoint       int          // checkpoint counts towards the new checkpoint
		currentBlock     *types.Block // Current head of the block chain 当前的区块头
		currentFastBlock *types.Block // Current head of the fast-sync chain (may be above the block chain!) 当前的快速同步的区块头.

		stateCache   state.Database // State database to reuse between imports (contains state cache)
		bodyCache    *lru.Cache     // Cache for the most recent block bodies
		bodyRLPCache *lru.Cache     // Cache for the most recent block bodies in RLP encoded format
		blockCache   *lru.Cache     // Cache for the most recent entire blocks
		futureBlocks *lru.Cache     // future blocks are blocks added for later processing  暂时还不能插入的区块存放位置.

		quit    chan struct{} // blockchain quit channel
		running int32         // running must be called atomically
		// procInterrupt must be atomically called
		procInterrupt int32          // interrupt signaler for block processing
		wg            sync.WaitGroup // chain processing wait group for shutting down

		engine    consensus.Engine	// 一致性引擎
		processor Processor // block processor interface  // 区块处理器接口
		validator Validator // block and state validator interface // 区块和状态验证器接口
		vmConfig  vm.Config //虚拟机的配置

		badBlocks *lru.Cache // Bad block cache  错误区块的缓存.
}


func NewBlockChain(chainDb ethdb.Database, config *params.ChainConfig, engine consensus.Engine, vmConfig vm.Config) (*BlockChain, error) {

		bodyCache, _ := lru.New(bodyCacheLimit)
		bodyRLPCache, _ := lru.New(bodyCacheLimit)
		blockCache, _ := lru.New(blockCacheLimit)
		futureBlocks, _ := lru.New(maxFutureBlocks)
		badBlocks, _ := lru.New(badBlockLimit)

		bc := &BlockChain{
		config:       config,
		chainDb:      chainDb,
		stateCache:   state.NewDatabase(chainDb),
		quit:         make(chan struct{}),
		bodyCache:    bodyCache,
		bodyRLPCache: bodyRLPCache,
		blockCache:   blockCache,
		futureBlocks: futureBlocks,
		engine:       engine,
		vmConfig:     vmConfig,
		badBlocks:    badBlocks,
		}
		bc.SetValidator(NewBlockValidator(config, bc, engine))
		bc.SetProcessor(NewStateProcessor(config, bc, engine))

		var err error
		bc.hc, err = NewHeaderChain(chainDb, config, engine, bc.getProcInterrupt)
		if err != nil {
		return nil, err
		}
		bc.genesisBlock = bc.GetBlockByNumber(0)  // 拿到创世区块
		if bc.genesisBlock == nil {
		return nil, ErrNoGenesis
		}
		if err := bc.loadLastState(); err != nil { //加载最新的状态
		return nil, err
		}
		// Check the current state of the block hashes and make sure that we do not have any of the bad blocks in our chain
		// 检查当前的状态,确认我们的区块链上面没有非法的区块.
		// BadHashes是一些手工配置的区块hash值, 用来硬分叉使用的.
		for hash := range BadHashes {
		if header := bc.GetHeaderByHash(hash); header != nil {
		// get the canonical block corresponding to the offending header's number
		// 获取规范的区块链上面同样高度的区块头,如果这个区块头确实是在我们的规范的区块链上的话,我们需要回滚到这个区块头的高度 - 1
		headerByNumber := bc.GetHeaderByNumber(header.Number.Uint64())
		// make sure the headerByNumber (if present) is in our current canonical chain
		if headerByNumber != nil && headerByNumber.Hash() == header.Hash() {
		log.Error("Found bad hash, rewinding chain", "number", header.Number, "hash", header.ParentHash)
		bc.SetHead(header.Number.Uint64() - 1)
		log.Error("Chain rewind was successful, resuming normal operation")
		}
		}
		}
		// Take ownership of this particular state
		go bc.update()
		return bc, nil
}

// loadLastState loads the last known chain state from the database. This method
// assumes that the chain manager mutex is held.
func (bc *BlockChain) loadLastState() error {
		// Restore the last known head block
		// 返回我们知道的最新的区块的hash
		head := GetHeadBlockHash(bc.chainDb)
		if head == (common.Hash{}) { // 如果获取到了空.那么认为数据库已经被破坏.那么设置区块链为创世区块.
		// Corrupt or empty database, init from scratch
		log.Warn("Empty database, resetting chain")
		return bc.Reset()
		}
		// Make sure the entire head block is available
		// 根据blockHash 来查找block
		currentBlock := bc.GetBlockByHash(head)
		if currentBlock == nil {
		// Corrupt or empty database, init from scratch
		log.Warn("Head block missing, resetting chain", "hash", head)
		return bc.Reset()
		}
		// Make sure the state associated with the block is available
		// 确认和这个区块的world state是否正确.
		if _, err := state.New(currentBlock.Root(), bc.stateCache); err != nil {
		// Dangling block without a state associated, init from scratch
		log.Warn("Head state missing, resetting chain", "number", currentBlock.Number(), "hash", currentBlock.Hash())
		return bc.Reset()
		}
		// Everything seems to be fine, set as the head block
		bc.currentBlock = currentBlock

		// Restore the last known head header
		// 获取最新的区块头的hash
		currentHeader := bc.currentBlock.Header()
		if head := GetHeadHeaderHash(bc.chainDb); head != (common.Hash{}) {
		if header := bc.GetHeaderByHash(head); header != nil {
		currentHeader = header
		}
		}
		// header chain 设置为当前的区块头.
		bc.hc.SetCurrentHeader(currentHeader)

		// Restore the last known head fast block
		bc.currentFastBlock = bc.currentBlock
		if head := GetHeadFastBlockHash(bc.chainDb); head != (common.Hash{}) {
		if block := bc.GetBlockByHash(head); block != nil {
		bc.currentFastBlock = block
		}
		}

		// Issue a status log for the user 用来打印日志.
		headerTd := bc.GetTd(currentHeader.Hash(), currentHeader.Number.Uint64())
		blockTd := bc.GetTd(bc.currentBlock.Hash(), bc.currentBlock.NumberU64())
		fastTd := bc.GetTd(bc.currentFastBlock.Hash(), bc.currentFastBlock.NumberU64())

		log.Info("Loaded most recent local header", "number", currentHeader.Number, "hash", currentHeader.Hash(), "td", headerTd)
		log.Info("Loaded most recent local full block", "number", bc.currentBlock.Number(), "hash", bc.currentBlock.Hash(), "td", blockTd)
		log.Info("Loaded most recent local fast block", "number", bc.currentFastBlock.Number(), "hash", bc.currentFastBlock.Hash(), "td", fastTd)

		return nil
}
