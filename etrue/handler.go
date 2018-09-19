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

package etrue

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/etrue/fastdownloader"
	"github.com/truechain/truechain-engineering-code/etrue/fetcher"
	"github.com/truechain/truechain-engineering-code/etrue/fetcher/snail"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/p2p/discover"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rlp"
)

const (
	softResponseLimit = 2 * 1024 * 1024 // Target maximum size of returned blocks, headers or node data.
	estHeaderRlpSize  = 500             // Approximate size of an RLP encoded block header

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize    = 4096
	blockChanSize = 64
	signChanSize  = 512
	nodeChanSize  = 256
	fruitChanSize = 256
)

var (
	daoChallengeTimeout = 15 * time.Second // Time allowance for a node to reply to the DAO handshake challenge
)

// errIncompatibleConfig is returned if the requested protocols and configs are
// not compatible (low protocol version restrictions and high requirements).
var errIncompatibleConfig = errors.New("incompatible configuration")

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

type ProtocolManager struct {
	networkID uint64

	fastSync     uint32 // Flag whether fast sync is enabled (gets disabled if we already have blocks)
	acceptTxs    uint32 // Flag whether we're considered synchronised (enables transaction processing)
	acceptFruits uint32
	//acceptSnailBlocks uint32
	txpool      txPool
	SnailPool   SnailPool
	blockchain  *core.BlockChain
	snailchain  *snailchain.SnailBlockChain
	chainconfig *params.ChainConfig
	maxPeers    int

	downloader   *downloader.Downloader
	fdownloader  *fastdownloader.Downloader
	fetcherFast  *fetcher.Fetcher
	fetcherSnail *snailfetcher.Fetcher
	peers        *peerSet

	SubProtocols []p2p.Protocol

	eventMux *event.TypeMux
	txsCh    chan core.NewTxsEvent
	txsSub   event.Subscription

	//fruit
	fruitsch  chan snailchain.NewFruitsEvent
	fruitsSub event.Subscription

	//fast block
	minedFastCh  chan core.NewBlockEvent
	minedFastSub event.Subscription

	pbSignsCh     chan core.PbftSignEvent
	pbSignsSub    event.Subscription
	pbNodeInfoCh  chan core.NodeInfoEvent
	pbNodeInfoSub event.Subscription

	//minedsnailBlock
	minedSnailBlockSub *event.TypeMuxSubscription
	// channels for fetcher, syncer, txsyncLoop
	newPeerCh   chan *peer
	txsyncCh    chan *txsync
	fruitsyncCh chan *fruitsync
	quitSync    chan struct{}
	noMorePeers chan struct{}

	// wait group is used for graceful shutdowns during downloading
	// and processing
	wg         sync.WaitGroup
	agentProxy AgentNetworkProxy
}

// NewProtocolManager returns a new Truechain sub protocol manager. The Truechain sub protocol manages peers capable
// with the Truechain network.
func NewProtocolManager(config *params.ChainConfig, mode downloader.SyncMode, networkID uint64, mux *event.TypeMux, txpool txPool, SnailPool SnailPool, engine consensus.Engine, blockchain *core.BlockChain, snailchain *snailchain.SnailBlockChain, chaindb ethdb.Database, agent *PbftAgent) (*ProtocolManager, error) {
	// Create the protocol manager with the base fields
	manager := &ProtocolManager{
		networkID:   networkID,
		eventMux:    mux,
		txpool:      txpool,
		SnailPool:   SnailPool,
		snailchain:  snailchain,
		blockchain:  blockchain,
		chainconfig: config,
		peers:       newPeerSet(),
		newPeerCh:   make(chan *peer),
		noMorePeers: make(chan struct{}),
		txsyncCh:    make(chan *txsync),
		fruitsyncCh: make(chan *fruitsync),
		quitSync:    make(chan struct{}),
		agentProxy:  agent,
	}
	// Figure out whether to allow fast sync or not
	// TODO: add downloader func later

	if mode == downloader.FastSync && snailchain.CurrentBlock().NumberU64() > 0 {
		log.Warn("Blockchain not empty, fast sync disabled")
		mode = downloader.FullSync
	}
	if mode == downloader.FastSync && snailchain.CurrentBlock().NumberU64() > 0 {
		log.Warn("Blockchain not empty, fast sync disabled")
		mode = downloader.FullSync
	}
	if mode == downloader.FastSync {
		manager.fastSync = uint32(1)
	}

	// Initiate a sub-protocol for every implemented version we can handle
	manager.SubProtocols = make([]p2p.Protocol, 0, len(ProtocolVersions))
	for i, version := range ProtocolVersions {
		// Skip protocol version if incompatible with the mode of operation
		if mode == downloader.FastSync && version < eth63 {
			continue
		}
		// Compatible; initialise the sub-protocol
		version := version // Closure for the run
		manager.SubProtocols = append(manager.SubProtocols, p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  ProtocolLengths[i],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := manager.newPeer(int(version), p, rw)
				select {
				case manager.newPeerCh <- peer:
					manager.wg.Add(1)
					defer manager.wg.Done()
					return manager.handle(peer)
				case <-manager.quitSync:
					return p2p.DiscQuitting
				}
			},
			NodeInfo: func() interface{} {
				return manager.NodeInfo()
			},
			PeerInfo: func(id discover.NodeID) interface{} {
				if p := manager.peers.Peer(fmt.Sprintf("%x", id[:8])); p != nil {
					return p.Info()
				}
				return nil
			},
		})
	}
	if len(manager.SubProtocols) == 0 {
		return nil, errIncompatibleConfig
	}
	// Construct the different synchronisation mechanisms
	// TODO: support downloader func.

	fmode := fastdownloader.SyncMode(int(mode))
	manager.fdownloader = fastdownloader.New(fmode, chaindb, manager.eventMux, blockchain, nil, manager.removePeer)


	manager.downloader = downloader.New(mode, chaindb, manager.eventMux, snailchain, nil, manager.removePeer, manager.fdownloader)

	fastValidator := func(header *types.Header) error {
		//mecMark how to get ChainFastReader
		return engine.VerifyHeader(blockchain, header, true)
	}
	fastHeighter := func() uint64 {
		return blockchain.CurrentFastBlock().NumberU64()
	}
	fastInserter := func(blocks types.Blocks) (int, error) {
		// If fast sync is running, deny importing weird blocks
		if atomic.LoadUint32(&manager.fastSync) == 1 {
			log.Warn("Discarded bad propagated block", "number", blocks[0].Number(), "hash", blocks[0].Hash())
			return 0, nil
		}
		atomic.StoreUint32(&manager.acceptTxs, 1)    // Mark initial sync done on any fetcher import
		atomic.StoreUint32(&manager.acceptFruits, 1) // Mark initial sync done on any fetcher import
		//atomic.StoreUint32(&manager.acceptSnailBlocks, 1) // Mark initial sync done on any fetcher import
		return manager.blockchain.InsertChain(blocks)
	}

	snailValidator := func(header *types.SnailHeader) error {
		headers := make([]*types.SnailHeader, 1)
		headers[0] = header
		//mecMark how to get ChainFastReader
		seals := make([]bool, 1)
		seals[0] = true
		_, err := engine.VerifySnailHeaders(snailchain, headers, seals)
		return <-err
	}
	snailHeighter := func() uint64 {
		return snailchain.CurrentBlock().NumberU64()
	}
	snailInserter := func(blocks types.SnailBlocks) (int, error) {
		// If fast sync is running, deny importing weird blocks
		// if atomic.LoadUint32(&manager.fastSync) == 1 {
		// 	log.Warn("Discarded bad propagated block", "number", blocks[0].Number(), "hash", blocks[0].Hash())
		// 	return 0, nil
		// }
		// atomic.StoreUint32(&manager.acceptTxs, 1) // Mark initial sync done on any fetcher import
		return manager.snailchain.InsertChain(blocks)
	}

	manager.fetcherFast = fetcher.New(blockchain.GetBlockByHash, fastValidator, manager.BroadcastFastBlock, fastHeighter, fastInserter, manager.removePeer, agent, manager.BroadcastPbSign)
	manager.fetcherSnail = snailfetcher.New(snailchain.GetBlockByHash, snailValidator, manager.BroadcastSnailBlock, snailHeighter, snailInserter, manager.removePeer)

	return manager, nil
}

func (pm *ProtocolManager) removePeer(id string) {
	// Short circuit if the peer was already removed
	peer := pm.peers.Peer(id)
	if peer == nil {
		return
	}
	log.Debug("Removing Truechain peer", "peer", id)

	// TODO: downloader.UnregisterPeer
	// Unregister the peer from the downloader and Truechain peer set
	//pm.downloader.UnregisterPeer(id)
	if err := pm.peers.Unregister(id); err != nil {
		log.Error("Peer removal failed", "peer", id, "err", err)
	}
	// Hard disconnect at the networking layer
	if peer != nil {
		peer.Peer.Disconnect(p2p.DiscUselessPeer)
	}
}

func (pm *ProtocolManager) Start(maxPeers int) {
	pm.maxPeers = maxPeers

	// broadcast transactions
	pm.txsCh = make(chan core.NewTxsEvent, txChanSize)
	pm.txsSub = pm.txpool.SubscribeNewTxsEvent(pm.txsCh)
	go pm.txBroadcastLoop()

	//broadcast fruits
	pm.fruitsch = make(chan snailchain.NewFruitsEvent, fruitChanSize)
	pm.fruitsSub = pm.SnailPool.SubscribeNewFruitEvent(pm.fruitsch)
	go pm.fruitBroadcastLoop()

	// broadcast mined fastBlocks
	pm.minedFastCh = make(chan core.NewBlockEvent, blockChanSize)
	pm.minedFastSub = pm.agentProxy.SubscribeNewFastBlockEvent(pm.minedFastCh)
	go pm.minedFastBroadcastLoop()

	// broadcast sign
	pm.pbSignsCh = make(chan core.PbftSignEvent, signChanSize)
	pm.pbSignsSub = pm.agentProxy.SubscribeNewPbftSignEvent(pm.pbSignsCh)
	go pm.pbSignBroadcastLoop()

	// broadcast node info
	pm.pbNodeInfoCh = make(chan core.NodeInfoEvent, nodeChanSize)
	pm.pbNodeInfoSub = pm.agentProxy.SubscribeNodeInfoEvent(pm.pbNodeInfoCh)
	go pm.pbNodeInfoBroadcastLoop()

	//broadcast mined snailblock
	pm.minedSnailBlockSub = pm.eventMux.Subscribe(snailchain.NewMinedBlockEvent{})
	go pm.minedSnailBlockLoop()

	// start sync handlers
	go pm.syncer()
	go pm.txsyncLoop()
	go pm.fruitsyncLoop()
	atomic.StoreUint32(&pm.acceptTxs, 1)
	atomic.StoreUint32(&pm.acceptFruits, 1)
}

func (pm *ProtocolManager) Stop() {
	log.Info("Stopping Truechain protocol")

	pm.txsSub.Unsubscribe()       // quits txBroadcastLoop
	pm.minedFastSub.Unsubscribe() // quits minedFastBroadcastLoop
	pm.pbSignsSub.Unsubscribe()
	pm.pbNodeInfoSub.Unsubscribe()
	//fruit and minedfruit
	pm.fruitsSub.Unsubscribe() // quits fruitBroadcastLoop
	//minedSnailBlock
	pm.minedSnailBlockSub.Unsubscribe() // quits minedSnailBlockBroadcastLoop

	// Quit the sync loop.
	// After this send has completed, no new peers will be accepted.
	pm.noMorePeers <- struct{}{}

	// Quit fetcher, txsyncLoop.
	close(pm.quitSync)

	// Disconnect existing sessions.
	// This also closes the gate for any new registrations on the peer set.
	// sessions which are already established but not added to pm.peers yet
	// will exit when they try to register.
	pm.peers.Close()

	// Wait for all peer handler goroutines and the loops to come down.
	pm.wg.Wait()

	log.Info("Truechain protocol stopped")
}

func (pm *ProtocolManager) newPeer(pv int, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	return newPeer(pv, p, newMeteredMsgWriter(rw))
}

// handle is the callback invoked to manage the life cycle of an etrue peer. When
// this function terminates, the peer is disconnected.
func (pm *ProtocolManager) handle(p *peer) error {
	// Ignore maxPeers if this is a trusted peer
	if pm.peers.Len() >= pm.maxPeers && !p.Peer.Info().Network.Trusted {
		return p2p.DiscTooManyPeers
	}
	p.Log().Debug("Truechain peer connected", "name", p.Name(), "RemoteAddr", p.RemoteAddr())

	// Execute the Truechain handshake
	var (
		genesis = pm.snailchain.Genesis()
		head    = pm.snailchain.CurrentHeader()
		hash    = head.Hash()
		number  = head.Number.Uint64()
		td      = pm.snailchain.GetTd(hash, number)
	)
	if err := p.Handshake(pm.networkID, td, hash, genesis.Hash()); err != nil {
		p.Log().Debug("Truechain handshake failed", "err", err)
		return err
	}
	p.Log().Info("Peer connected success", "name", p.Name(), "RemoteAddr", p.RemoteAddr())
	if rw, ok := p.rw.(*meteredMsgReadWriter); ok {
		rw.Init(p.version)
	}
	// Register the peer locally
	if err := pm.peers.Register(p); err != nil {
		p.Log().Error("Truechain peer registration failed", "err", err)
		return err
	}

	defer pm.removePeer(p.id)

	// TODO: downloader.RegisterPeer
	// Register the peer in the downloader. If the downloader considers it banned, we disconnect
	if err := pm.downloader.RegisterPeer(p.id, p.version, p); err != nil {
		return err
	}

	if err := pm.fdownloader.RegisterPeer(p.id, p.version, p); err != nil {
		return err
	}

	// Propagate existing transactions. new transactions appearing
	// after this will be sent via broadcasts.
	pm.syncTransactions(p)
	pm.syncFruits(p)
	// If we're DAO hard-fork aware, validate any remote peer with regard to the hard-fork
	if daoBlock := pm.chainconfig.DAOForkBlock; daoBlock != nil {
		// Request the peer's DAO fork header for extra-data validation
		if err := p.RequestHeadersByNumber(daoBlock.Uint64(), 1, 0, false, false); err != nil {
			return err
		}
		// Start a timer to disconnect if the peer doesn't reply in time
		p.forkDrop = time.AfterFunc(daoChallengeTimeout, func() {
			p.Log().Debug("Timed out DAO fork-check, dropping")
			pm.removePeer(p.id)
		})
		// Make sure it's cleaned up if the peer dies off
		defer func() {
			if p.forkDrop != nil {
				p.forkDrop.Stop()
				p.forkDrop = nil
			}
		}()
	}
	// main loop. handle incoming messages.
	for {
		if err := pm.handleMsg(p); err != nil {
			p.Log().Debug("Truechain message handling failed", "err", err)
			return err
		}
	}
}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (pm *ProtocolManager) handleMsg(p *peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	// Handle the message depending on its contents
	switch {
	case msg.Code == StatusMsg:
		// Status messages should never arrive after the handshake
		return errResp(ErrExtraStatusMsg, "uncontrolled status message")

	// Block header query, collect the requested headers and reply

	case msg.Code == GetSnailBlockHeadersMsg:

		log.Debug("GetSnailBlockHeadersMsg>>>>>>>>>>>>")
		// Decode the complex header query
		var query getBlockHeadersData
		if err := msg.Decode(&query); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		hashMode := query.Origin.Hash != (common.Hash{})
		first := true
		maxNonCanonical := uint64(100)

		// Gather headers until the fetch or network limits is reached
		var (
			bytes   common.StorageSize
			headers []*types.SnailHeader
			unknown bool
		)
		for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < downloader.MaxHeaderFetch {
			// Retrieve the next header satisfying the query
			var origin *types.SnailHeader
			if hashMode {
				if first {
					first = false
					origin = pm.snailchain.GetHeaderByHash(query.Origin.Hash)
					if origin != nil {
						query.Origin.Number = origin.Number.Uint64()
					}
				} else {
					origin = pm.snailchain.GetHeader(query.Origin.Hash, query.Origin.Number)
				}
			} else {
				origin = pm.snailchain.GetHeaderByNumber(query.Origin.Number)
			}
			if origin == nil {
				break
			}
			headers = append(headers, origin)
			bytes += estHeaderRlpSize

			// Advance to the next header of the query
			switch {
			case hashMode && query.Reverse:
				// Hash based traversal towards the genesis block
				ancestor := query.Skip + 1
				if ancestor == 0 {
					unknown = true
				} else {
					query.Origin.Hash, query.Origin.Number = pm.snailchain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
					unknown = (query.Origin.Hash == common.Hash{})
				}
			case hashMode && !query.Reverse:
				// Hash based traversal towards the leaf block
				var (
					current = origin.Number.Uint64()
					next    = current + query.Skip + 1
				)
				if next <= current {
					infos, _ := json.MarshalIndent(p.Peer.Info(), "", "  ")
					p.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
					unknown = true
				} else {
					if header := pm.snailchain.GetHeaderByNumber(next); header != nil {
						nextHash := header.Hash()
						expOldHash, _ := pm.snailchain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
						if expOldHash == query.Origin.Hash {
							query.Origin.Hash, query.Origin.Number = nextHash, next
						} else {
							unknown = true
						}
					} else {
						unknown = true
					}
				}
			case query.Reverse:
				// Number based traversal towards the genesis block
				if query.Origin.Number >= query.Skip+1 {
					query.Origin.Number -= query.Skip + 1
				} else {
					unknown = true
				}

			case !query.Reverse:
				// Number based traversal towards the leaf block
				query.Origin.Number += query.Skip + 1
			}
		}
		return p.SendSnailBlockHeaders(headers)

	case msg.Code == SnailBlockHeadersMsg:
		log.Debug("SnailBlockHeadersMsg>>>>>>>>>>>>")
		// A batch of headers arrived to one of our previous requests
		var headers []*types.SnailHeader
		if err := msg.Decode(&headers); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Filter out any explicitly requested headers, deliver the rest to the downloader
		//filter := len(headers) == 1
		//if filter {
		//	// Irrelevant of the fork checks, send the header to the fetcher just in case
		//	headers = pm.fetcherFast.FilterHeaders(p.id, headers, time.Now())
		//}
		// mecMark

		err := pm.downloader.DeliverHeaders(p.id, headers)
		if err != nil {
			log.Debug("Failed to deliver headers", "err", err)
		}

	case msg.Code == GetFastBlockHeadersMsg:

		log.Debug("GetFastBlockHeadersMsg>>>>>>>>>>>>")
		// Decode the complex header query
		var query getBlockHeadersData
		if err := msg.Decode(&query); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		hashMode := query.Origin.Hash != (common.Hash{})
		first := true
		maxNonCanonical := uint64(100)

		// Gather headers until the fetch or network limits is reached
		var (
			bytes   common.StorageSize
			headers []*types.Header
			unknown bool
		)
		for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < downloader.MaxHeaderFetch {
			// Retrieve the next header satisfying the query
			var origin *types.Header
			if hashMode {
				if first {
					first = false
					origin = pm.blockchain.GetHeaderByHash(query.Origin.Hash)
					if origin != nil {
						query.Origin.Number = origin.Number.Uint64()
					}
				} else {
					origin = pm.blockchain.GetHeader(query.Origin.Hash, query.Origin.Number)
				}
			} else {
				origin = pm.blockchain.GetHeaderByNumber(query.Origin.Number)
			}
			if origin == nil {
				break
			}
			headers = append(headers, origin)
			bytes += estHeaderRlpSize

			// Advance to the next header of the query
			switch {
			case hashMode && query.Reverse:
				// Hash based traversal towards the genesis block
				ancestor := query.Skip + 1
				if ancestor == 0 {
					unknown = true
				} else {
					query.Origin.Hash, query.Origin.Number = pm.blockchain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
					unknown = (query.Origin.Hash == common.Hash{})
				}
			case hashMode && !query.Reverse:
				// Hash based traversal towards the leaf block
				var (
					current = origin.Number.Uint64()
					next    = current + query.Skip + 1
				)
				if next <= current {
					infos, _ := json.MarshalIndent(p.Peer.Info(), "", "  ")
					p.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
					unknown = true
				} else {
					if header := pm.blockchain.GetHeaderByNumber(next); header != nil {
						nextHash := header.Hash()
						expOldHash, _ := pm.blockchain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
						if expOldHash == query.Origin.Hash {
							query.Origin.Hash, query.Origin.Number = nextHash, next
						} else {
							unknown = true
						}
					} else {
						unknown = true
					}
				}
			case query.Reverse:
				// Number based traversal towards the genesis block
				if query.Origin.Number >= query.Skip+1 {
					query.Origin.Number -= query.Skip + 1
				} else {
					unknown = true
				}

			case !query.Reverse:
				// Number based traversal towards the leaf block
				query.Origin.Number += query.Skip + 1
			}
		}
		return p.SendFastBlockHeaders(headers)

	case msg.Code == FastBlockHeadersMsg:

		// A batch of headers arrived to one of our previous requests
		var headers []*types.Header
		if err := msg.Decode(&headers); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Filter out any explicitly requested headers, deliver the rest to the downloader
		//filter := len(headers) == 1
		//if filter {
		//	// Irrelevant of the fork checks, send the header to the fetcher just in case
		//	headers = pm.fetcherFast.FilterHeaders(p.id, headers, time.Now())
		//}
		// mecMark
		//if len(headers) > 0 {
		log.Debug("FastBlockHeadersMsg>>>>>>>>>>>>","headers:",len(headers))
		err := pm.fdownloader.DeliverHeaders(p.id, headers)
		if err != nil {
			log.Debug("Failed to deliver headers", "err", err)
		}
		//}

	case msg.Code == GetFastBlockBodiesMsg:
		log.Debug("GetFastBlockBodiesMsg>>>>>>>>>>>>")
		// Decode the retrieval message
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		// Gather blocks until the fetch or network limits is reached
		var (
			hash   common.Hash
			bytes  int
			bodies []rlp.RawValue
		)
		for bytes < softResponseLimit && len(bodies) < downloader.MaxBlockFetch {
			// Retrieve the hash of the next block
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			// Retrieve the requested block body, stopping if enough was found
			if data := pm.blockchain.GetBodyRLP(hash); len(data) != 0 {
				bodies = append(bodies, data)
				bytes += len(data)
			}
		}
		return p.SendFastBlockBodiesRLP(bodies)

	case msg.Code == FastBlockBodiesMsg:
		log.Debug("FastBlockBodiesMsg>>>>>>>>>>>>")
		// A batch of block bodies arrived to one of our previous requests
		var request blockBodiesData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Deliver them all to the downloader for queuing
		transactions := make([][]*types.Transaction, len(request))
		signs := make([][]*types.PbftSign, len(request))

		//Signs   []*types.PbftSign
		for i, body := range request {
			transactions[i] = body.Transactions
			signs[i] = body.Signs
		}
		// Filter out any explicitly requested bodies, deliver the rest to the downloader
		//filter := len(transactions) > 0
		//if filter {
		//	transactions = pm.fetcherFast.FilterBodies(p.id, transactions, time.Now())
		//}
		// mecMark
		//if len(transactions) > 0 || len(uncles) > 0 || !filter {
		err := pm.fdownloader.DeliverBodies(p.id, transactions, signs)
		if err != nil {
			log.Debug("Failed to deliver bodies", "err", err)
		}
		//}

	case msg.Code == GetSnailBlockBodiesMsg:
		log.Debug("GetSnailBlockBodiesMsg>>>>>>>>>>>>")
		// Decode the retrieval message
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		// Gather blocks until the fetch or network limits is reached
		var (
			hash   common.Hash
			bytes  int
			bodies []rlp.RawValue
		)
		for bytes < softResponseLimit && len(bodies) < downloader.MaxBlockFetch {
			// Retrieve the hash of the next block
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			// Retrieve the requested block body, stopping if enough was found
			if data := pm.snailchain.GetBodyRLP(hash); len(data) != 0 {
				bodies = append(bodies, data)
				bytes += len(data)
			}
		}
		return p.SendSnailBlockBodiesRLP(bodies)

	case msg.Code == SnailBlockBodiesMsg:

		// A batch of block bodies arrived to one of our previous requests
		var request snailBlockBodiesData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Deliver them all to the downloader for queuing
		fruits := make([][]*types.SnailBlock, len(request))
		signs := make([][]*types.PbftSign, len(request))


		for i, body := range request {
			fruits[i] = body.Fruits
			signs[i] = body.Signs
		}
		// Filter out any explicitly requested bodies, deliver the rest to the downloader
		//filter := len(fruits) > 0
		//if filter {
		//	fruits = pm.fetcherFast.FilterBodies(p.id, fruits, time.Now())
		//}
		// mecMark
		//if len(transactions) > 0 || len(uncles) > 0 || !filter {
		log.Debug("SnailBlockBodiesMsg>>>>>>>>>>>>","fruits",len(fruits))
		err := pm.downloader.DeliverBodies(p.id, fruits, signs,nil)
		if err != nil {
			log.Debug("Failed to deliver bodies", "err", err)
		}
		//}

	case p.version >= eth63 && msg.Code == GetNodeDataMsg:
		// Decode the retrieval message
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		// Gather state data until the fetch or network limits is reached
		var (
			hash  common.Hash
			bytes int
			data  [][]byte
		)
		for bytes < softResponseLimit && len(data) < fastdownloader.MaxStateFetch {
			// Retrieve the hash of the next state entry
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			// Retrieve the requested state entry, stopping if enough was found
			if entry, err := pm.blockchain.TrieNode(hash); err == nil {
				data = append(data, entry)
				bytes += len(entry)
			}
		}
		return p.SendNodeData(data)

	case p.version >= eth63 && msg.Code == NodeDataMsg:
		// A batch of node state data arrived to one of our previous requests
		var data [][]byte
		if err := msg.Decode(&data); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Deliver all to the downloader
		if err := pm.fdownloader.DeliverNodeData(p.id, data); err != nil {
			log.Debug("Failed to deliver node state data", "err", err)
		}

	case p.version >= eth63 && msg.Code == GetReceiptsMsg:
		// Decode the retrieval message
		msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
		if _, err := msgStream.List(); err != nil {
			return err
		}
		// Gather state data until the fetch or network limits is reached
		var (
			hash     common.Hash
			bytes    int
			receipts []rlp.RawValue
		)
		for bytes < softResponseLimit && len(receipts) < fastdownloader.MaxReceiptFetch {
			// Retrieve the hash of the next block
			if err := msgStream.Decode(&hash); err == rlp.EOL {
				break
			} else if err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			// Retrieve the requested block's receipts, skipping if unknown to us
			results := pm.blockchain.GetReceiptsByHash(hash)
			if results == nil {
				if header := pm.blockchain.GetHeaderByHash(hash); header == nil || header.ReceiptHash != types.EmptyRootHash {
					continue
				}
			}
			// If known, encode and queue for response packet
			if encoded, err := rlp.EncodeToBytes(results); err != nil {
				log.Error("Failed to encode receipt", "err", err)
			} else {
				receipts = append(receipts, encoded)
				bytes += len(encoded)
			}
		}
		return p.SendReceiptsRLP(receipts)

	case p.version >= eth63 && msg.Code == ReceiptsMsg:
		// A batch of receipts arrived to one of our previous requests
		var receipts [][]*types.Receipt
		if err := msg.Decode(&receipts); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Deliver all to the downloader
		if err := pm.fdownloader.DeliverReceipts(p.id, receipts); err != nil {
			log.Debug("Failed to deliver receipts", "err", err)
		}

	case msg.Code == NewFastBlockHashesMsg:
		var announces newBlockHashesData
		if err := msg.Decode(&announces); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		// Mark the hashes as present at the remote node
		for _, block := range announces {
			p.MarkFastBlock(block.Hash)
		}
		// Schedule all the unknown hashes for retrieval
		unknown := make(newBlockHashesData, 0, len(announces))
		for _, block := range announces {
			if !pm.blockchain.HasBlock(block.Hash, block.Number) {
				if pm.fetcherFast.GetPendingBlock(block.Hash) != nil {
					log.Debug("Has pending block", "num", block.Number, "announces", len(announces))
				} else {
					unknown = append(unknown, block)
				}
			}
		}
		//for _, block := range unknown {
		//	pm.fetcherFast.Notify(p.id, block.Hash, block.Number, block.Sign, time.Now(), p.RequestOneFastHeader, p.RequestBodies)
		//}

	case msg.Code == NewFastBlockMsg:
		// Retrieve and decode the propagated block
		var request newBlockData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		request.Block.ReceivedAt = msg.ReceivedAt
		request.Block.ReceivedFrom = p

		// Mark the peer as owning the block and schedule it for import
		p.MarkFastBlock(request.Block.Hash())
		pm.fetcherFast.Enqueue(p.id, request.Block)

		// TODO: downloader sync func
		//// Assuming the block is importable by the peer, but possibly not yet done so,
		//// calculate the head hash and TD that the peer truly must have.
		//var (
		//	trueHead = request.Block.ParentHash()
		//	trueTD   = new(big.Int).Sub(request.td, request.Block.Difficulty())
		//)
		//// Update the peers total difficulty if better than the previous
		//if _, td := p.Head(); trueTD.Cmp(td) > 0 {
		//	p.SetHead(trueHead, trueTD)
		//
		//	// Schedule a sync if above ours. Note, this will not fire a sync for a gap of
		//	// a singe block (as the true TD is below the propagated block), however this
		//	// scenario should easily be covered by the fetcher.
		//	currentBlock := pm.blockchain.CurrentBlock()
		//	if trueTD.Cmp(pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64())) > 0 {
		//		go pm.synchronise(p)
		//	}
		//}

	case msg.Code == TxMsg:
		// Transactions arrived, make sure we have a valid and fresh chain to handle them
		if atomic.LoadUint32(&pm.acceptTxs) == 0 {
			break
		}
		// Transactions can be processed, parse all of them and deliver to the pool
		var txs []*types.Transaction
		if err := msg.Decode(&txs); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		for i, tx := range txs {
			// Validate and mark the remote transaction
			if tx == nil {
				return errResp(ErrDecode, "transaction %d is nil", i)
			}
			p.MarkTransaction(tx.Hash())
		}
		pm.txpool.AddRemotes(txs)

	case msg.Code == PbftNodeInfoMsg:
		// EncryptNodeMessage can be processed, parse all of them and deliver to the queue
		var nodeInfo *types.EncryptNodeMessage
		if err := msg.Decode(&nodeInfo); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Validate and mark the remote node
		if nodeInfo == nil {
			return errResp(ErrDecode, "nodde  is nil")
		}
		p.MarkNodeInfo(nodeInfo.Hash())
		pm.agentProxy.AddRemoteNodeInfo(nodeInfo)

	case msg.Code == BlockSignMsg:
		// PbftSign can be processed, parse all of them and deliver to the queue
		var signs []*types.PbftSign
		if err := msg.Decode(&signs); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		for i, sign := range signs {
			// Validate and mark the remote transaction
			if sign == nil {
				return errResp(ErrDecode, "sign %d is nil", i)
			}
			p.MarkSign(sign.Hash())
		}
		log.Debug("Receive sign", "num", signs[0].FastHeight, "peer", p.id)
		// committee no current block
		pm.fetcherFast.EnqueueSign(p.id, signs)

	//fruit structure

	case msg.Code == FruitMsg:
		// Fruit arrived, make sure we have a valid and fresh chain to handle them
		if atomic.LoadUint32(&pm.acceptFruits) == 0 {
			break
		}
		// Transactions can be processed, parse all of them and deliver to the pool
		var fruits []*types.SnailBlock
		if err := msg.Decode(&fruits); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		for i, fruit := range fruits {
			// Validate and mark the remote fruit
			if fruit == nil {
				return errResp(ErrDecode, "fruit %d is nil", i)
			}
			p.MarkFruit(fruit.Hash())
		}
		//pm.hybridpool.AddRemoteFruits(fruits)
		pm.SnailPool.AddRemoteFruits(fruits)

	//snailBlock structure

	case msg.Code == SnailBlockMsg:
		// snailBlock arrived, make sure we have a valid and fresh chain to handle them
		//var snailBlocks []*types.SnailBlock
		log.Debug("receive SnailBlockMsg")
		var request newSnailBlockData
		if err := msg.Decode(&request); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		request.Block.ReceivedAt = msg.ReceivedAt
		request.Block.ReceivedFrom = p

		var snailBlock = request.Block
		if snailBlock == nil {
			return errResp(ErrDecode, "snailBlock  is nil")
		}
		log.Debug("enqueue SnailBlockMsg", "number", snailBlock.Number())

		p.MarkSnailBlock(snailBlock.Hash())

		pm.fetcherSnail.Enqueue(p.id, snailBlock)

		// Assuming the block is importable by the peer, but possibly not yet done so,
		// calculate the head hash and TD that the peer truly must have.
		var (
			trueHead = request.Block.ParentHash()
			trueTD   = new(big.Int).Sub(request.TD, request.Block.Difficulty())
		)
		// Update the peers total difficulty if better than the previous
		if _, td := p.Head(); trueTD.Cmp(td) > 0 {
			p.SetHead(trueHead, trueTD)

			// Schedule a sync if above ours. Note, this will not fire a sync for a gap of
			// a singe block (as the true TD is below the propagated block), however this
			// scenario should easily be covered by the fetcher.
			currentBlock := pm.snailchain.CurrentBlock()
			if trueTD.Cmp(pm.snailchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64())) > 0 {
				go pm.synchronise(p)
			}
		}

		// downloads
		//var (
		//	trueHead = request.Block.ParentHash()
		//	trueTD   = new(big.Int).Sub(request.TD, request.Block.Difficulty())
		//)
		// Update the peers total difficulty if better than the previous
		//if _, td := p.Head(); trueTD.Cmp(td) > 0 {
		//	p.SetHead(trueHead, trueTD)
		//
		//	// Schedule a sync if above ours. Note, this will not fire a sync for a gap of
		//	// a singe block (as the true TD is below the propagated block), however this
		//	// scenario should easily be covered by the fetcher.
		//	currentBlock := pm.blockchain.CurrentBlock()
		//	if trueTD.Cmp(pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64())) > 0 {
		//		go pm.synchronise(p)
		//	}
		//}

	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	return nil
}

// BroadcastFastBlock will either propagate a block to a subset of it's peers, or
// will only announce it's availability (depending what's requested).
func (pm *ProtocolManager) BroadcastFastBlock(block *types.Block, propagate bool) {
	hash := block.Hash()
	peers := pm.peers.PeersWithoutFastBlock(hash)

	// If propagation is requested, send to a subset of the peer
	if propagate {
		if parent := pm.blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1); parent == nil {
			log.Error("Propagating dangling block", "number", block.Number(), "hash", hash)
			return
		}
		// Send the block to a subset of our peers
		transfer := peers[:int(math.Sqrt(float64(len(peers))))]
		for _, peer := range transfer {
			peer.AsyncSendNewFastBlock(block)
		}
		log.Info("Propagated block", "num", block.Number(), "hash", hash.String(), "recipients", len(transfer), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
		return
	}
	// Otherwise if the block is indeed in out own chain, announce it
	if pm.blockchain.HasBlock(hash, block.NumberU64()) {
		for _, peer := range peers {
			peer.AsyncSendNewFastBlockHash(block)
		}
		log.Debug("Announced block", "num", block.Number(), "hash", hash.String(), "recipients", len(peers), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
	}
}

// BroadcastPbSigns will propagate a batch of PbftVoteSigns to all peers which are not known to
// already have the given PbftVoteSign.
func (pm *ProtocolManager) BroadcastPbSign(pbSigns []*types.PbftSign) {
	var pbSignSet = make(map[*peer][]*types.PbftSign)

	// Broadcast transactions to a batch of peers not knowing about it
	for _, pbSign := range pbSigns {
		peers := pm.peers.PeersWithoutSign(pbSign.Hash())
		for _, peer := range peers {
			pbSignSet[peer] = append(pbSignSet[peer], pbSign)
		}
		log.Debug("Broadcast sign", "number", pbSign.FastHeight, "hash", pbSign.Hash().String(), "recipients", len(peers))
	}

	// FIXME include this again: peers = peers[:int(math.Sqrt(float64(len(peers))))]
	for peer, signs := range pbSignSet {
		peer.AsyncSendSign(signs)
	}
}

// BroadcastPbNodeInfo will propagate a batch of EncryptNodeMessage to all peers which are not known to
// already have the given CryNodeInfo.
func (pm *ProtocolManager) BroadcastPbNodeInfo(nodeInfo *types.EncryptNodeMessage) {
	var nodeInfoSet = make(map[*peer]core.NodeInfoEvent)

	// Broadcast transactions to a batch of peers not knowing about it
	peers := pm.peers.PeersWithoutNodeInfo(nodeInfo.Hash())
	for _, peer := range peers {
		nodeInfoSet[peer] = core.NodeInfoEvent{nodeInfo}
	}
	log.Debug("Broadcast node info ", "hash", nodeInfo.Hash(), "recipients", len(peers), " ", len(pm.peers.peers))
	for peer, nodeInfo := range nodeInfoSet {
		peer.AsyncSendNodeInfo(nodeInfo.NodeInfo)
	}
}

// BroadcastSnailBlock will either propagate a snailBlock to a subset of it's peers, or
// will only announce it's availability (depending what's requested).
func (pm *ProtocolManager) BroadcastSnailBlock(snailBlock *types.SnailBlock, propagate bool) {
	hash := snailBlock.Hash()
	peers := pm.peers.PeersWithoutSnailBlock(hash)

	// If propagation is requested, send to a subset of the peer
	if propagate {
		// Calculate the TD of the fruit (it's not imported yet, so fruit.Td is not valid)
		var td *big.Int
		if parent := pm.snailchain.GetBlock(snailBlock.ParentHash(), snailBlock.NumberU64()-1); parent != nil {
			td = new(big.Int).Add(snailBlock.Difficulty(), pm.snailchain.GetTd(snailBlock.ParentHash(), snailBlock.NumberU64()-1))
		} else {
			log.Error("Propagating dangling block", "number", snailBlock.Number(), "hash", hash)
			return
		}
		// Send the fruit to a subset of our peers
		transfer := peers[:int(math.Sqrt(float64(len(peers))))]
		for _, peer := range transfer {
			log.Debug("AsyncSendNewSnailBlock begin", "peer", peer.RemoteAddr(), "number", snailBlock.NumberU64(), "hash", snailBlock.Hash())
			peer.AsyncSendNewSnailBlock(snailBlock, td)
		}
		log.Trace("Propagated snailBlock", "hash", hash, "recipients", len(transfer), "duration", common.PrettyDuration(time.Since(snailBlock.ReceivedAt)))
		return
	}
	// Otherwise if the block is indeed in out own chain, announce it
	if pm.snailchain.HasBlock(hash, snailBlock.NumberU64()) {
		td :=pm.snailchain.GetTd(snailBlock.Hash(), snailBlock.NumberU64())
		for _, peer := range peers {
			peer.AsyncSendNewSnailBlock(snailBlock,td)
		}
		log.Trace("Announced block", "hash", hash, "recipients", len(peers), "duration", common.PrettyDuration(time.Since(snailBlock.ReceivedAt)))
	}
}

// BroadcastTxs will propagate a batch of transactions to all peers which are not known to
// already have the given transaction.
func (pm *ProtocolManager) BroadcastTxs(txs types.Transactions) {
	var txset = make(map[*peer]types.Transactions)

	// Broadcast transactions to a batch of peers not knowing about it
	for _, tx := range txs {
		peers := pm.peers.PeersWithoutTx(tx.Hash())
		for _, peer := range peers {
			txset[peer] = append(txset[peer], tx)
		}
		log.Trace("Broadcast transaction", "hash", tx.Hash(), "recipients", len(peers))
	}
	// FIXME include this again: peers = peers[:int(math.Sqrt(float64(len(peers))))]
	for peer, txs := range txset {
		peer.AsyncSendTransactions(txs)
	}
}

//for fruits
func (pm *ProtocolManager) BroadcastFruits(fruits types.Fruits) {
	var fruitset = make(map[*peer]types.Fruits)

	// Broadcast records to a batch of peers not knowing about it
	for _, fruit := range fruits {
		peers := pm.peers.PeersWithoutFruit(fruit.Hash())
		for _, peer := range peers {
			fruitset[peer] = append(fruitset[peer], fruit)
		}
		log.Debug("Broadcast fruits", "hash", fruit.Hash(), "recipients", len(peers))
	}
	// FIXME include this again: peers = peers[:int(math.Sqrt(float64(len(peers))))]
	for peer, fruits := range fruitset {
		peer.AsyncSendFruits(fruits)
	}
}

// Mined broadcast loop
func (pm *ProtocolManager) minedFastBroadcastLoop() {
	for {
		select {
		case blockEvent := <-pm.minedFastCh:
			pm.BroadcastFastBlock(blockEvent.Block, true) // First propagate fast block to peers

			// Err() channel will be closed when unsubscribing.
		case <-pm.minedFastSub.Err():
			return
		}
	}
}

func (pm *ProtocolManager) pbSignBroadcastLoop() {
	for {
		select {
		case signEvent := <-pm.pbSignsCh:
			log.Info("Committee sign", "number", signEvent.PbftSign.FastHeight, "hash", signEvent.PbftSign.Hash().String(), "recipients", len(pm.peers.peers))
			pm.BroadcastFastBlock(signEvent.Block, true) // Only then announce to the rest
			pm.BroadcastPbSign([]*types.PbftSign{signEvent.PbftSign})
			pm.BroadcastFastBlock(signEvent.Block, false) // Only then announce to the rest

			// Err() channel will be closed when unsubscribing.
		case <-pm.pbSignsSub.Err():
			return
		}
	}
}

func (pm *ProtocolManager) pbNodeInfoBroadcastLoop() {
	for {
		select {
		case nodeInfoEvent := <-pm.pbNodeInfoCh:
			pm.BroadcastPbNodeInfo(nodeInfoEvent.NodeInfo)

			// Err() channel will be closed when unsubscribing.
		case <-pm.pbNodeInfoSub.Err():
			return
		}
	}
}

// Mined snailBlock loop
func (pm *ProtocolManager) minedSnailBlockLoop() {
	// automatically stops if unsubscribe
	for obj := range pm.minedSnailBlockSub.Chan() {
		switch ev := obj.Data.(type) {
		case snailchain.NewMinedBlockEvent:
			pm.BroadcastSnailBlock(ev.Block, true)  // First propagate fruit to peers
			pm.BroadcastSnailBlock(ev.Block, false) // Only then announce to the rest
		}
	}
}
func (pm *ProtocolManager) txBroadcastLoop() {
	for {
		select {
		case event := <-pm.txsCh:
			pm.BroadcastTxs(event.Txs)

		// Err() channel will be closed when unsubscribing.
		case <-pm.txsSub.Err():
			return
		}
	}
}

//  fruits
func (pm *ProtocolManager) fruitBroadcastLoop() {
	for {
		select {
		case fruitsEvent := <-pm.fruitsch:
			pm.BroadcastFruits(fruitsEvent.Fruits)

			// Err() channel will be closed when unsubscribing.
		case <-pm.fruitsSub.Err():
			return
		}
	}
}

// NodeInfo represents a short summary of the Truechain sub-protocol metadata
// known about the host peer.
type NodeInfo struct {
	Network    uint64              `json:"network"`    // Truechain network ID (1=Frontier, 2=Morden, Ropsten=3, Rinkeby=4)
	Difficulty *big.Int            `json:"difficulty"` // Total difficulty of the host's blockchain
	Genesis    common.Hash         `json:"genesis"`    // SHA3 hash of the host's genesis block
	Config     *params.ChainConfig `json:"config"`     // Chain configuration for the fork rules
	Head       common.Hash         `json:"head"`       // SHA3 hash of the host's best owned block
}

// NodeInfo retrieves some protocol metadata about the running host node.
func (pm *ProtocolManager) NodeInfo() *NodeInfo {
	currentBlock := pm.blockchain.CurrentBlock()
	return &NodeInfo{
		Network:    pm.networkID,
		Difficulty: pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64()),
		Genesis:    pm.blockchain.Genesis().Hash(),
		Config:     pm.blockchain.Config(),
		Head:       currentBlock.Hash(),
	}
}
