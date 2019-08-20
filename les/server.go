// Copyright 2016 The go-ethereum Authors
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

package les

import (
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/truechain/truechain-engineering-code/accounts/abi/bind"
	"github.com/truechain/truechain-engineering-code/light/fast"
	"github.com/truechain/truechain-engineering-code/light/public"
	"github.com/truechain/truechain-engineering-code/p2p/enode"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etrue"
	"github.com/truechain/truechain-engineering-code/les/flowcontrol"
	"github.com/truechain/truechain-engineering-code/light"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/p2p/discv5"
)

const bufLimitRatio = 6000 // fixed bufLimit/MRR ratio

type LesServer struct {
	lesCommons

	archiveMode bool // Flag whether the ethereum node runs in archive mode.

	fcManager    *flowcontrol.ClientManager // nil if our node is client only
	costTracker  *costTracker
	testCost     uint64
	defParams    flowcontrol.ServerParams
	lesTopics    []discv5.Topic
	privateKey   *ecdsa.PrivateKey
	quitSync     chan struct{}
	onlyAnnounce bool

	thcNormal, thcBlockProcessing int // serving thread count for normal operation and block processing mode

	maxPeers                                int
	minCapacity, maxCapacity, freeClientCap uint64
	clientPool                              *clientPool
}

func NewLesServer(etrue *etrue.Truechain, config *etrue.Config) (*LesServer, error) {
	lesTopics := make([]discv5.Topic, len(AdvertiseProtocolVersions))
	for i, pv := range AdvertiseProtocolVersions {
		lesTopics[i] = lesTopic(etrue.SnailBlockChain().Genesis().Hash(), pv)
	}
	quitSync := make(chan struct{})
	srv := &LesServer{
		lesCommons: lesCommons{
			config:           config,
			iConfig:          public.DefaultServerIndexerConfig,
			chainDb:          etrue.ChainDb(),
			chtIndexer:       light.NewChtIndexer(etrue.ChainDb(), nil, params.CHTFrequency, params.HelperTrieProcessConfirmations),
			bloomTrieIndexer: fast.NewBloomTrieIndexer(etrue.ChainDb(), nil, params.BloomBitsBlocks, params.BloomTrieFrequency),
		},
		archiveMode:  etrue.ArchiveMode(),
		quitSync:     quitSync,
		lesTopics:    lesTopics,
		onlyAnnounce: false,
	}
	srv.costTracker, srv.minCapacity = newCostTracker(etrue.ChainDb(), config)

	logger := log.New()
	srv.thcNormal = config.LightServ * 4 / 100
	if srv.thcNormal < 4 {
		srv.thcNormal = 4
	}
	srv.thcBlockProcessing = config.LightServ/100 + 1
	srv.fcManager = flowcontrol.NewClientManager(nil, &mclock.System{})

	checkpoint := srv.latestLocalCheckpoint()
	if !checkpoint.Empty() {
		logger.Info("Loaded latest checkpoint", "section", checkpoint.SectionIndex, "head", checkpoint.SectionHead,
			"chtroot", checkpoint.CHTRoot, "bloomroot", checkpoint.BloomRoot)
	}

	srv.chtIndexer.Start(etrue.SnailBlockChain())

	registrar := newCheckpointOracle(nil, srv.getLocalCheckpoint)
	// TODO(rjl493456442) Checkpoint is useless for les server, separate handler for client and server.
	pm, err := NewProtocolManager(etrue.BlockChain().Config(), nil, public.DefaultServerIndexerConfig, nil, 0, false, config.NetworkId, etrue.EventMux(), etrue.Engine(), newPeerSet(), etrue.BlockChain(), etrue.SnailBlockChain(), etrue.TxPool(), etrue.ChainDb(), nil, nil, registrar, quitSync, new(sync.WaitGroup), nil, etrue.Synced)
	if err != nil {
		return nil, err
	}
	srv.protocolManager = pm
	pm.servingQueue = newServingQueue(int64(time.Millisecond*10), float64(config.LightServ)/100)
	pm.server = srv

	chtSection, height, _ := srv.chtIndexer.Sections()
	if chtSection != 0 {
		for i := chtSection; i > chtSection-3; i-- {
			chtSectionCurrent := i - 1
			chtSectionHead := srv.chtIndexer.SectionHead(chtSectionCurrent)
			if chtSectionHead == (common.Hash{}) {
				break
			}
			chtTrieRoot := light.GetChtRoot(pm.chainDb, chtSectionCurrent, chtSectionHead)
			logger.Info("Loaded recent CHT", "section", chtSectionCurrent, "head", chtSectionHead.String(), "root", chtTrieRoot.String(),
				"height", height, "current", etrue.SnailBlockChain().GetHeaderByHash(chtSectionHead).Number)
		}
	}

	bloomSection, height, _ := srv.bloomTrieIndexer.Sections()
	if bloomSection != 0 {
		for i := bloomSection; i > bloomSection-3; i-- {
			bloomSectionCurrent := i - 1
			bloomSectionHead := srv.bloomTrieIndexer.SectionHead(bloomSectionCurrent)
			if bloomSectionHead == (common.Hash{}) {
				break
			}
			bloomTrieRoot := fast.GetBloomTrieRoot(pm.chainDb, bloomSectionCurrent, bloomSectionHead)
			logger.Info("Loaded bloom trie", "section", bloomSectionCurrent, "head", bloomSectionHead.String(), "root", bloomTrieRoot.String(),
				"height", height, "current", etrue.BlockChain().GetHeaderByHash(bloomSectionHead).Number)
		}
	}

	return srv, nil
}

func (s *LesServer) APIs() []rpc.API {
	return []rpc.API{
		{
			Namespace: "les",
			Version:   "1.0",
			Service:   NewPrivateLightAPI(&s.lesCommons, s.protocolManager.reg),
			Public:    false,
		},
	}
}

// startEventLoop starts an event handler loop that updates the recharge curve of
// the client manager and adjusts the client pool's size according to the total
// capacity updates coming from the client manager
func (s *LesServer) startEventLoop() {
	s.protocolManager.wg.Add(1)

	var (
		processing, procLast bool
		procStarted          time.Time
	)
	blockProcFeed := make(chan bool, 100)
	s.protocolManager.fblockchain.(*core.BlockChain).SubscribeBlockProcessingEvent(blockProcFeed)
	totalRechargeCh := make(chan uint64, 100)
	totalRecharge := s.costTracker.subscribeTotalRecharge(totalRechargeCh)
	totalCapacityCh := make(chan uint64, 100)
	updateRecharge := func() {
		if processing {
			if !procLast {
				procStarted = time.Now()
			}
			s.protocolManager.servingQueue.setThreads(s.thcBlockProcessing)
			s.fcManager.SetRechargeCurve(flowcontrol.PieceWiseLinear{{0, 0}, {totalRecharge, totalRecharge}})
		} else {
			if procLast {
				blockProcessingTimer.UpdateSince(procStarted)
			}
			s.protocolManager.servingQueue.setThreads(s.thcNormal)
			s.fcManager.SetRechargeCurve(flowcontrol.PieceWiseLinear{{0, 0}, {totalRecharge / 16, totalRecharge / 2}, {totalRecharge / 2, totalRecharge / 2}, {totalRecharge, totalRecharge}})
		}
		procLast = processing
	}
	updateRecharge()
	totalCapacity := s.fcManager.SubscribeTotalCapacity(totalCapacityCh)
	s.clientPool.setLimits(s.maxPeers, totalCapacity)

	var maxFreePeers uint64
	go func() {
		for {
			select {
			case processing = <-blockProcFeed:
				updateRecharge()
			case totalRecharge = <-totalRechargeCh:
				updateRecharge()
			case totalCapacity = <-totalCapacityCh:
				totalCapacityGauge.Update(int64(totalCapacity))
				newFreePeers := totalCapacity / s.freeClientCap
				if newFreePeers < maxFreePeers && newFreePeers < uint64(s.maxPeers) {
					log.Warn("Reduced total capacity", "maxFreePeers", newFreePeers, "maxFreePeers", maxFreePeers, "maxPeers", s.maxPeers)
				}
				maxFreePeers = newFreePeers
				s.clientPool.setLimits(s.maxPeers, totalCapacity)
			case <-s.protocolManager.quitSync:
				s.protocolManager.wg.Done()
				return
			}
		}
	}()
}

func (s *LesServer) Protocols() []p2p.Protocol {
	return s.makeProtocols(ServerProtocolVersions)
}

// Start starts the LES server
func (s *LesServer) Start(srvr *p2p.Server) {
	s.maxPeers = s.config.LightPeers
	totalRecharge := s.costTracker.totalRecharge()
	if s.maxPeers > 0 {
		s.freeClientCap = s.minCapacity //totalRecharge / uint64(s.maxPeers)
		if s.freeClientCap < s.minCapacity {
			s.freeClientCap = s.minCapacity
		}
		if s.freeClientCap > 0 {
			s.defParams = flowcontrol.ServerParams{
				BufLimit:    s.freeClientCap * bufLimitRatio,
				MinRecharge: s.freeClientCap,
			}
		}
	}

	s.maxCapacity = s.freeClientCap * uint64(s.maxPeers)
	if totalRecharge > s.maxCapacity {
		s.maxCapacity = totalRecharge
	}
	s.fcManager.SetCapacityLimits(s.freeClientCap, s.maxCapacity, s.freeClientCap*2)
	s.clientPool = newClientPool(s.chainDb, s.freeClientCap, 10000, mclock.System{}, func(id enode.ID, call uint32) { go s.protocolManager.removePeer(peerIdToString(id), call) })
	s.clientPool.setPriceFactors(priceFactors{0, 1, 1}, priceFactors{0, 1, 1})
	s.protocolManager.peers.notify(s.clientPool)
	s.startEventLoop()
	s.protocolManager.Start(s.config.LightPeers)
	if srvr.DiscV5 != nil {
		for _, topic := range s.lesTopics {
			topic := topic
			go func() {
				logger := log.New("topic", topic)
				logger.Info("Starting topic registration")
				defer logger.Info("Terminated topic registration")

				srvr.DiscV5.RegisterTopic(topic, s.quitSync)
			}()
		}
	}
	s.privateKey = srvr.PrivateKey
	s.protocolManager.blockLoop()
}

func (s *LesServer) SetBloomBitsIndexer(bloomIndexer *core.ChainIndexer) {
	bloomIndexer.AddChildIndexer(s.bloomTrieIndexer)
}

// SetClient sets the rpc client and starts running checkpoint contract if it is not yet watched.
func (s *LesServer) SetContractBackend(backend bind.ContractBackend) {
	if s.protocolManager.reg != nil {
		s.protocolManager.reg.start(backend)
	}
}

// Stop stops the LES service
func (s *LesServer) Stop() {
	s.fcManager.Stop()
	s.chtIndexer.Close()
	// bloom trie indexer is closed by parent bloombits indexer
	go func() {
		<-s.protocolManager.noMorePeers
	}()
	s.clientPool.stop()
	s.costTracker.stop()
	s.protocolManager.Stop()
}

// todo(rjl493456442) separate client and server implementation.
func (pm *ProtocolManager) blockLoop() {
	pm.wg.Add(1)
	headCh := make(chan types.FastChainHeadEvent, 10)
	headSub := pm.fblockchain.SubscribeChainHeadEvent(headCh)

	sheadCh := make(chan types.SnailChainHeadEvent, 10)
	sheadSub := pm.blockchain.SubscribeChainHeadEvent(sheadCh)

	go func() {
		var lastHead *types.SnailHeader
		lastBroadcastTd := common.Big0
		lastBroadcastNumber := uint64(0)
		lock := new(sync.Mutex)

		for {
			select {
			case ev := <-sheadCh:
				peers := pm.peers.AllPeers()

				if len(peers) > 0 {
					header := ev.Block.Header()
					hash := header.Hash()
					number := header.Number.Uint64()
					td := rawdb.ReadTd(pm.chainDb, hash, number)
					lock.Lock()
					if td != nil && td.Cmp(lastBroadcastTd) > 0 {
						lastBroadcastTd = new(big.Int).Set(td)
						var reorg uint64
						if lastHead != nil {
							reorg = lastHead.Number.Uint64() - rawdb.FindCommonAncestor(pm.chainDb, header, lastHead).Number.Uint64()
						}
						lastHead = header
						log.Debug("Announcing snail block to peers", "number", number, "hash", hash, "td", td, "reorg", reorg)

						announce := announceData{Hash: hash, Number: number, Td: td, ReorgDepth: reorg}
						var (
							signed         bool
							signedAnnounce announceData
						)

						for _, p := range peers {
							p := p
							switch p.announceType {
							case announceTypeSimple:
								p.queueSend(func() { p.SendAnnounce(announce) })
							case announceTypeSigned:
								if !signed {
									signedAnnounce = announce
									signedAnnounce.sign(pm.server.privateKey)
									signed = true
								}
								p.queueSend(func() { p.SendAnnounce(signedAnnounce) })
							}
						}
					}
					lock.Unlock()
				}
			case ev := <-headCh:
				peers := pm.peers.AllPeers()
				if len(peers) > 0 {
					header := ev.Block.Header()
					hash := header.Hash()
					number := header.Number.Uint64()
					lock.Lock()
					if number > lastBroadcastNumber {
						lastBroadcastNumber = header.Number.Uint64()
						if number%10 == 0 {
							log.Debug("Announcing fast block to peers", "number", number, "hash", hash, "lastBroadcastNumber", lastBroadcastNumber)
						}

						announce := announceData{FastHash: hash, FastNumber: number}
						var (
							signed         bool
							signedAnnounce announceData
						)

						for _, p := range peers {
							p := p
							switch p.announceType {
							case announceTypeSimple:
								p.queueSend(func() { p.SendAnnounce(announce) })
							case announceTypeSigned:
								if !signed {
									signedAnnounce = announce
									signedAnnounce.sign(pm.server.privateKey)
									signed = true
								}
								p.queueSend(func() { p.SendAnnounce(signedAnnounce) })
							}
						}
					}
					lock.Unlock()
				}
			case <-pm.quitSync:
				sheadSub.Unsubscribe()
				headSub.Unsubscribe()
				pm.wg.Done()
				return
			}
		}
	}()
}
