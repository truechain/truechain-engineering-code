// Copyright 2014 The go-ethereum Authors
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

// Package etrue implements the Truechain protocol.
package etrue

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft"
	config "github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/consensus"
	elect "github.com/truechain/truechain-engineering-code/consensus/election"
	ethash "github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/bloombits"
	chain "github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/etrue/filters"
	"github.com/truechain/truechain-engineering-code/etrue/gasprice"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/internal/trueapi"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/miner"
	"github.com/truechain/truechain-engineering-code/node"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/pbftserver"
	"github.com/truechain/truechain-engineering-code/rlp"
	"github.com/truechain/truechain-engineering-code/rpc"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *core.ChainIndexer)
}

// Truechain implements the Truechain full node service.
type Truechain struct {
	config      *Config
	chainConfig *params.ChainConfig

	// Channel for shutting down the service
	shutdownChan chan bool // Channel for shutting down the Truechain

	// Handlers
	txPool *core.TxPool

	snailPool *chain.SnailPool

	agent    *PbftAgent
	election *elect.Election

	blockchain      *core.BlockChain
	snailblockchain *chain.SnailBlockChain
	protocolManager *ProtocolManager
	lesServer       LesServer

	// DB interfaces
	chainDb ethdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	APIBackend *TrueAPIBackend

	miner     *miner.Miner
	gasPrice  *big.Int
	etherbase common.Address

	networkID     uint64
	netRPCService *trueapi.PublicNetAPI

	pbftServerOld *pbftserver.PbftServerMgr
	pbftServer    *tbft.Node

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and etherbase)
}

func (s *Truechain) AddLesServer(ls LesServer) {
	s.lesServer = ls
	ls.SetBloomBitsIndexer(s.bloomIndexer)
}

// New creates a new Truechain object (including the
// initialisation of the common Truechain object)
func New(ctx *node.ServiceContext, config *Config) (*Truechain, error) {
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run etrue.Truechain in light sync mode, use les.LightEthereum")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	chainDb, err := CreateDB(ctx, config, "chaindata")
	//chainDb, err := CreateDB(ctx, config, path)
	if err != nil {
		return nil, err
	}

	chainConfig, genesisHash, _, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	etrue := &Truechain{
		config:         config,
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, &config.Ethash, chainConfig, chainDb),
		shutdownChan:   make(chan bool),
		networkID:      config.NetworkId,
		gasPrice:       config.GasPrice,
		etherbase:      config.Etherbase,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks),
	}

	log.Info("Initialising Truechain protocol", "versions", ProtocolVersions, "network", config.NetworkId)

	if !config.SkipBcVersionCheck {
		bcVersion := rawdb.ReadDatabaseVersion(chainDb)
		if bcVersion != core.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run getrue upgradedb.\n", bcVersion, core.BlockChainVersion)
		}
		rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
	}
	var (
		vmConfig = vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
		//cacheConfig     = &core.CacheConfig{Disabled: config.NoPruning, TrieNodeLimit: config.TrieCache, TrieTimeLimit: config.TrieTimeout}
		cacheConfig      = &core.CacheConfig{Disabled: config.NoPruning, TrieNodeLimit: config.TrieCache, TrieTimeLimit: config.TrieTimeout}
		snailCacheConfig = &chain.CacheConfig{Disabled: config.NoPruning, TrieNodeLimit: config.TrieCache, TrieTimeLimit: config.TrieTimeout}
	)

	etrue.snailblockchain, err = chain.NewSnailBlockChain(chainDb, snailCacheConfig, etrue.chainConfig, etrue.engine, vmConfig)
	if err != nil {
		return nil, err
	}

	etrue.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, etrue.chainConfig, etrue.engine, vmConfig)
	if err != nil {
		return nil, err
	}

	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		etrue.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	// TODO: rewind snail if case of incompatible config
	// if compat, ok := snailErr.(*params.ConfigCompatError); ok {
	// 	log.Warn("Rewinding chain to upgrade configuration", "err", compat)
	// 	etrue.snailblockchain.SetHead(compat.RewindTo)
	// 	rawdb.WriteChainConfig(chainDb, snailHash, snailConfig)
	// }

	// TODO: start bloom indexer
	//etrue.bloomIndexer.Start(etrue.blockchain)

	sv := chain.NewBlockValidator(etrue.chainConfig, etrue.blockchain, etrue.snailblockchain, etrue.engine)
	etrue.snailblockchain.SetValidator(sv)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}

	if config.SnailPool.Journal != "" {
		config.SnailPool.Journal = ctx.ResolvePath(config.SnailPool.Journal)
	}

	etrue.txPool = core.NewTxPool(config.TxPool, etrue.chainConfig, etrue.blockchain)

	etrue.snailPool = chain.NewSnailPool(config.SnailPool, etrue.blockchain, etrue.snailblockchain, etrue.engine, sv)

	etrue.election = elect.NewElction(etrue.blockchain, etrue.snailblockchain, etrue.config)

	//etrue.snailblockchain.Validator().SetElection(etrue.election, etrue.blockchain)

	etrue.engine.SetElection(etrue.election)
	etrue.engine.SetSnailChainReader(etrue.snailblockchain)
	etrue.election.SetEngine(etrue.engine)

	coinbase, _ := etrue.Etherbase()
	etrue.agent = NewPbftAgent(etrue, etrue.chainConfig, etrue.engine, etrue.election, coinbase)
	if etrue.protocolManager, err = NewProtocolManager(
		etrue.chainConfig, config.SyncMode, config.NetworkId,
		etrue.eventMux, etrue.txPool, etrue.snailPool, etrue.engine,
		etrue.blockchain, etrue.snailblockchain,
		chainDb, etrue.agent); err != nil {
		return nil, err
	}

	etrue.miner = miner.New(etrue, etrue.chainConfig, etrue.EventMux(), etrue.engine, etrue.election, etrue.Config().MineFruit, etrue.Config().NodeType)
	etrue.miner.SetExtra(makeExtraData(config.ExtraData))

	committeeKey, err := crypto.ToECDSA(etrue.config.CommitteeKey)
	if err == nil {
		etrue.miner.SetElection(etrue.config.EnableElection, crypto.FromECDSAPub(&committeeKey.PublicKey))
	}

	etrue.APIBackend = &TrueAPIBackend{etrue, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	etrue.APIBackend.gpo = gasprice.NewOracle(etrue.APIBackend, gpoParams)
	return etrue, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"getrue",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateDB creates the chain database.
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (ethdb.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*ethdb.LDBDatabase); ok {
		db.Meter("etrue/db/chaindata/")
	}
	return db, nil
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Truechain service
func CreateConsensusEngine(ctx *node.ServiceContext, config *ethash.Config, chainConfig *params.ChainConfig,
	db ethdb.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
	// snail chain not need clique
	/*
		if chainConfig.Clique != nil {
			return clique.New(chainConfig.Clique, db)
		}*/
	// Otherwise assume proof-of-work
	switch config.PowMode {
	case ethash.ModeFake:
		log.Info("-----Fake mode")
		log.Warn("Ethash used in fake mode")
		return ethash.NewFaker()
	case ethash.ModeTest:
		log.Warn("Ethash used in test mode")
		return ethash.NewTester()
	case ethash.ModeShared:
		log.Warn("Ethash used in shared mode")
		return ethash.NewShared()
	default:
		engine := ethash.New(ethash.Config{
			CacheDir:       ctx.ResolvePath(config.CacheDir),
			CachesInMem:    config.CachesInMem,
			CachesOnDisk:   config.CachesOnDisk,
			DatasetDir:     config.DatasetDir,
			DatasetsInMem:  config.DatasetsInMem,
			DatasetsOnDisk: config.DatasetsOnDisk,
		})
		engine.SetThreads(-1) // Disable CPU mining
		return engine
	}
}

// APIs return the collection of RPC services the etrue package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Truechain) APIs() []rpc.API {
	apis := trueapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append etrue	APIs and  Eth APIs
	namespaces := []string{"etrue", "eth"}
	for _, name := range namespaces {
		apis = append(apis, []rpc.API{
			{
				Namespace: name,
				Version:   "1.0",
				Service:   NewPublicTruechainAPI(s),
				Public:    true,
			}, {
				Namespace: name,
				Version:   "1.0",
				Service:   NewPublicMinerAPI(s),
				Public:    true,
			}, {
				Namespace: name,
				Version:   "1.0",
				Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
				Public:    true,
			}, {
				Namespace: name,
				Version:   "1.0",
				Service:   filters.NewPublicFilterAPI(s.APIBackend, false),
				Public:    true,
			},
		}...)
	}
	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Truechain) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Truechain) ResetWithFastGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Truechain) Etherbase() (eb common.Address, err error) {
	s.lock.RLock()
	etherbase := s.etherbase
	s.lock.RUnlock()

	if etherbase != (common.Address{}) {
		return etherbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			etherbase := accounts[0].Address

			s.lock.Lock()
			s.etherbase = etherbase
			s.lock.Unlock()

			log.Info("Etherbase automatically configured", "address", etherbase)
			return etherbase, nil
		}
	}
	return common.Address{}, fmt.Errorf("etherbase must be explicitly specified")
}

// SetEtherbase sets the mining reward address.
func (s *Truechain) SetEtherbase(etherbase common.Address) {
	s.lock.Lock()
	s.etherbase = etherbase
	s.agent.committeeNode.Coinbase = etherbase
	s.lock.Unlock()

	s.miner.SetEtherbase(etherbase)
}

func (s *Truechain) StartMining(local bool) error {
	eb, err := s.Etherbase()
	if err != nil {
		log.Error("Cannot start mining without etherbase", "err", err)
		return fmt.Errorf("etherbase missing: %v", err)
	}

	// snail chain not need clique
	/*
		if clique, ok := s.engine.(*clique.Clique); ok {
			wallet, err := s.accountManager.Find(accounts.Account{Address: eb})
			if wallet == nil || err != nil {
				log.Error("Etherbase account unavailable locally", "err", err)
				return fmt.Errorf("signer missing: %v", err)
			}
			clique.Authorize(eb, wallet.SignHash)
		}*/

	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so none will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)
		atomic.StoreUint32(&s.protocolManager.acceptFruits, 1)

	}
	go s.miner.Start(eb)
	return nil
}

func (s *Truechain) StopMining()         { s.miner.Stop() }
func (s *Truechain) IsMining() bool      { return s.miner.Mining() }
func (s *Truechain) Miner() *miner.Miner { return s.miner }

func (s *Truechain) AccountManager() *accounts.Manager { return s.accountManager }
func (s *Truechain) BlockChain() *core.BlockChain      { return s.blockchain }
func (s *Truechain) Config() *Config                   { return s.config }

func (s *Truechain) SnailBlockChain() *chain.SnailBlockChain { return s.snailblockchain }
func (s *Truechain) TxPool() *core.TxPool                    { return s.txPool }

func (s *Truechain) SnailPool() *chain.SnailPool { return s.snailPool }

func (s *Truechain) EventMux() *event.TypeMux           { return s.eventMux }
func (s *Truechain) Engine() consensus.Engine           { return s.engine }
func (s *Truechain) ChainDb() ethdb.Database            { return s.chainDb }
func (s *Truechain) IsListening() bool                  { return true } // Always listening
func (s *Truechain) EthVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *Truechain) NetVersion() uint64                 { return s.networkID }
func (s *Truechain) Downloader() *downloader.Downloader { return s.protocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Truechain) Protocols() []p2p.Protocol {
	if s.lesServer == nil {
		return s.protocolManager.SubProtocols
	}
	return append(s.protocolManager.SubProtocols, s.lesServer.Protocols()...)
}

// Start implements node.Service, starting all internal goroutines needed by the
// Truechain protocol implementation.
func (s *Truechain) Start(srvr *p2p.Server) error {
	//start fruit journal
	s.snailPool.Start()
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers()

	// Start the RPC service
	s.netRPCService = trueapi.NewPublicNetAPI(srvr, s.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers >= srvr.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, srvr.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.protocolManager.Start(maxPeers)
	if s.lesServer != nil {
		s.lesServer.Start(srvr)
	}
	if s.config.OldTbft {
		s.startPbftServerOld()
		if s.pbftServerOld == nil {
			log.Error("start pbft server failed.")
			return errors.New("start pbft server failed.")
		}
		s.agent.server = s.pbftServerOld
		log.Info("", "server", s.agent.server)
	} else {
		s.startPbftServer()
		if s.pbftServer == nil {
			log.Error("start pbft server failed.")
			return errors.New("start pbft server failed.")
		}
		s.agent.server = s.pbftServer
		log.Info("", "server", s.agent.server)
	}
	s.agent.Start()

	s.election.Start()
	//go s.agent.SendBlock()

	// Start the networking layer and the light server if requested
	s.protocolManager.Start2(maxPeers)
	if s.lesServer != nil {
		s.lesServer.Start(srvr)
	}

	//sender := NewSender(s.snailPool, s.chainConfig, s.agent, s.blockchain)
	//sender.Start()

	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Truechain protocol.
func (s *Truechain) Stop() error {
	if s.config.OldTbft {
		s.stopPbftServerOld()
	} else {
		s.stopPbftServer()
	}
	s.bloomIndexer.Close()
	s.blockchain.Stop()
	s.snailblockchain.Stop()
	s.protocolManager.Stop()
	if s.lesServer != nil {
		s.lesServer.Stop()
	}
	s.txPool.Stop()
	s.miner.Stop()
	s.eventMux.Stop()

	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
func (s *Truechain) startPbftServerOld() error {
	priv, err := crypto.ToECDSA(s.config.CommitteeKey)
	if err != nil {
		return err
	}
	pk := &ecdsa.PublicKey{
		Curve: priv.Curve,
		X:     new(big.Int).Set(priv.X),
		Y:     new(big.Int).Set(priv.Y),
	}
	// var agent types.PbftAgentProxy
	s.pbftServerOld = pbftserver.NewPbftServerMgr(pk, priv, s.agent)
	return nil
}

func (s *Truechain) startPbftServer() error {
	priv, err := crypto.ToECDSA(s.config.CommitteeKey)
	if err != nil {
		return err
	}
	n1, err := tbft.NewNode(config.DefaultConfig(), "1", priv, s.agent)
	if err != nil {
		return err
	}
	s.pbftServer = n1
	return n1.Start()
}

func (s *Truechain) stopPbftServer() error {
	if s.pbftServerOld != nil {
		s.pbftServerOld.Finish()
	}
	return nil
}
func (s *Truechain) stopPbftServerOld() error {
	if s.pbftServer != nil {
		s.pbftServer.Stop()
	}
	return nil
}
