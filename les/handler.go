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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/etrue/fastdownloader"
	"github.com/truechain/truechain-engineering-code/light/fast"
	"github.com/truechain/truechain-engineering-code/light/public"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	snaildb "github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/state"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/event"
	"github.com/truechain/truechain-engineering-code/light"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/p2p/discv5"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/trie"
)

var errTooManyInvalidRequest = errors.New("too many invalid requests made")

const (
	softResponseLimit = 2 * 1024 * 1024 // Target maximum size of returned blocks, headers or node data.
	estHeaderRlpSize  = 500             // Approximate size of an RLP encoded block header

	etrueVersion = 63 // equivalent etrue version for the downloader

	MaxHeaderFetch           = 192 // Amount of block headers to be fetched per retrieval request
	MaxBodyFetch             = 32  // Amount of block bodies to be fetched per retrieval request
	MaxSnailBodyFetch        = 128 // Amount of block bodies to be fetched per retrieval request
	MaxFruitBodyFetch        = 128 // Amount of block bodies to be fetched per retrieval request
	MaxReceiptFetch          = 128 // Amount of transaction receipts to allow fetching per request
	MaxCodeFetch             = 64  // Amount of contract codes to allow fetching per request
	MaxProofsFetch           = 64  // Amount of merkle proofs to be fetched per retrieval request
	MaxHelperTrieProofsFetch = 64  // Amount of merkle proofs to be fetched per retrieval request
	MaxTxSend                = 64  // Amount of transactions to be send per request
	MaxTxStatus              = 256 // Amount of transactions to queried per request

	disableClientRemovePeer = false
)

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

type FastBlockChain interface {
	Config() *params.ChainConfig
	HasHeader(hash common.Hash, number uint64) bool
	GetHeader(hash common.Hash, number uint64) *types.Header
	GetHeaderByHash(hash common.Hash) *types.Header
	CurrentHeader() *types.Header
	State() (*state.StateDB, error)
	InsertHeaderChain(chain []*types.Header, checkFreq int) (int, error)
	Rollback(chain []common.Hash)
	GetHeaderByNumber(number uint64) *types.Header
	GetAncestor(hash common.Hash, number, ancestor uint64, maxNonCanonical *uint64) (common.Hash, uint64)
	Genesis() *types.Block
	SubscribeChainHeadEvent(ch chan<- types.FastChainHeadEvent) event.Subscription
	SetCommitteeInfo(hash common.Hash, number uint64, infos []*types.CommitteeMember)
}

type BlockChain interface {
	Config() *params.ChainConfig
	HasHeader(hash common.Hash, number uint64) bool
	GetHeader(hash common.Hash, number uint64) *types.SnailHeader
	GetHeaderByHash(hash common.Hash) *types.SnailHeader
	CurrentHeader() *types.SnailHeader
	GetTd(hash common.Hash, number uint64) *big.Int
	InsertHeaderChain(chain []*types.SnailHeader, fruits [][]*types.SnailHeader, checkFreq int) (int, error)
	Rollback(chain []common.Hash)
	GetHeaderByNumber(number uint64) *types.SnailHeader
	GetAncestor(hash common.Hash, number, ancestor uint64, maxNonCanonical *uint64) (common.Hash, uint64)
	Genesis() *types.SnailBlock
	SubscribeChainHeadEvent(ch chan<- types.SnailChainHeadEvent) event.Subscription
}

type txPool interface {
	AddRemotes(txs []*types.Transaction) []error
	Status(hashes []common.Hash) []core.TxStatus
}

type ProtocolManager struct {
	// Configs
	chainConfig *params.ChainConfig
	iConfig     *public.IndexerConfig

	client       bool   // The indicator whether the node is light client
	maxPeers     int    // The maximum number peers allowed to connect.
	networkId    uint64 // The identity of network.
	txpool       txPool
	txrelay      *lesTxRelay
	blockchain   BlockChain
	fblockchain  FastBlockChain
	chainDb      etruedb.Database
	odr          *LesOdr
	server       *LesServer
	serverPool   *serverPool
	lesTopic     discv5.Topic
	reqDist      *requestDistributor
	retriever    *retrieveManager
	servingQueue *servingQueue
	downloader   *downloader.Downloader
	fdownloader  *fastdownloader.Downloader
	fetcher      *lightFetcher
	fastFetcher  *fastLightFetcher
	ulc          *ulc
	peers        *peerSet
	checkpoint   *params.TrustedCheckpoint
	reg          *checkpointOracle // If reg == nil, it means the checkpoint registrar is not activated

	// channels for fetcher, syncer, txsyncLoop
	newPeerCh   chan *peer
	quitSync    chan struct{}
	noMorePeers chan struct{}

	wg       *sync.WaitGroup
	eventMux *event.TypeMux
	election *Election
	// Callbacks
	synced func() bool
}

// NewProtocolManager returns a new ethereum sub protocol manager. The Ethereum sub protocol manages peers capable
// with the ethereum network.
func NewProtocolManager(chainConfig *params.ChainConfig, checkpoint *params.TrustedCheckpoint, indexerConfig *public.IndexerConfig, ulcServers []string, ulcFraction int, client bool, networkId uint64, mux *event.TypeMux, engine consensus.Engine, peers *peerSet, blockchain FastBlockChain, snailchain BlockChain, txpool txPool, chainDb etruedb.Database, odr *LesOdr, serverPool *serverPool, registrar *checkpointOracle, quitSync chan struct{}, wg *sync.WaitGroup, election *Election, synced func() bool) (*ProtocolManager, error) {
	// Create the protocol manager with the base fields
	manager := &ProtocolManager{
		client:      client,
		eventMux:    mux,
		blockchain:  snailchain,
		fblockchain: blockchain,
		chainConfig: chainConfig,
		iConfig:     indexerConfig,
		chainDb:     chainDb,
		odr:         odr,
		networkId:   networkId,
		txpool:      txpool,
		serverPool:  serverPool,
		reg:         registrar,
		peers:       peers,
		newPeerCh:   make(chan *peer),
		quitSync:    quitSync,
		wg:          wg,
		noMorePeers: make(chan struct{}),
		election:    election,
		checkpoint:  checkpoint,
		synced:      synced,
	}
	if odr != nil {
		manager.retriever = odr.retriever
		manager.reqDist = odr.retriever.dist
	}

	if ulcServers != nil {
		ulc, err := newULC(ulcServers, ulcFraction)
		if err != nil {
			log.Warn("Failed to initialize ultra light client", "err", err)
		} else {
			manager.ulc = ulc
		}
	}
	removePeer := manager.removePeer
	if disableClientRemovePeer {
		removePeer = func(id string, call uint32) {}
	}
	if client {
		var checkpointNumber uint64
		if checkpoint != nil {
			checkpointNumber = (checkpoint.SectionIndex+1)*params.CHTFrequency - 1
		}
		mode := downloader.LightSync
		fmode := fastdownloader.SyncMode(mode)
		manager.fdownloader = fastdownloader.New(fmode, chainDb, manager.eventMux, nil, blockchain, removePeer)
		manager.downloader = downloader.New(mode, checkpointNumber, chainDb, manager.eventMux, nil, snailchain, removePeer, manager.fdownloader)
		manager.peers.notify((*downloaderPeerNotify)(manager))
		manager.fastFetcher = newFastLightFetcher(manager)
		manager.fetcher = newLightFetcher(manager)
		manager.fetcher.setFastFetcher(manager.fastFetcher)

	}

	return manager, nil
}

// removePeer initiates disconnection from a peer by removing it from the peer set
func (pm *ProtocolManager) removePeer(id string, call uint32) {
	log.Debug("removePeer", "call", call, "id", id)
	pm.peers.Unregister(id)
}

func (pm *ProtocolManager) Start(maxPeers int) {
	pm.maxPeers = maxPeers
	if pm.client {
		go pm.syncer()
	} else {
		go func() {
			for range pm.newPeerCh {
			}
		}()
	}
}

func (pm *ProtocolManager) Stop() {
	// Showing a log message. During download / process this could actually
	// take between 5 to 10 seconds and therefor feedback is required.
	log.Info("Stopping light Truechain protocol")

	// Quit the sync loop.
	// After this send has completed, no new peers will be accepted.
	pm.noMorePeers <- struct{}{}

	close(pm.quitSync) // quits syncer, fetcher

	if pm.servingQueue != nil {
		pm.servingQueue.stop()
	}

	// Disconnect existing sessions.
	// This also closes the gate for any new registrations on the peer set.
	// sessions which are already established but not added to pm.peers yet
	// will exit when they try to register.
	pm.peers.Close()

	// Wait for any process action
	pm.wg.Wait()

	log.Info("Light Truechain protocol stopped")
}

// runPeer is the p2p protocol run function for the given version.
func (pm *ProtocolManager) runPeer(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) error {
	var entry *poolEntry
	peer := pm.newPeer(int(version), pm.networkId, p, rw)
	if pm.serverPool != nil {
		entry = pm.serverPool.connect(peer, peer.Node())
	}
	peer.poolEntry = entry
	select {
	case pm.newPeerCh <- peer:
		pm.wg.Add(1)
		defer pm.wg.Done()
		err := pm.handle(peer)
		if entry != nil {
			pm.serverPool.disconnect(entry)
		}
		return err
	case <-pm.quitSync:
		if entry != nil {
			pm.serverPool.disconnect(entry)
		}
		return p2p.DiscQuitting
	}
}

func (pm *ProtocolManager) newPeer(pv int, nv uint64, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	var trusted bool
	if pm.ulc != nil {
		trusted = pm.ulc.trusted(p.ID())
	}
	return newPeer(pv, nv, trusted, p, newMeteredMsgWriter(rw))
}

// handle is the callback invoked to manage the life cycle of a les peer. When
// this function terminates, the peer is disconnected.
func (pm *ProtocolManager) handle(p *peer) error {
	// Ignore maxPeers if this is a trusted peer
	// In server mode we try to check into the client pool after handshake
	if pm.client && pm.peers.Len() >= pm.maxPeers && !p.Peer.Info().Network.Trusted {
		clientRejectedMeter.Mark(1)
		return p2p.DiscTooManyPeers
	}
	// Reject light clients if server is not synced.
	if !pm.client && !pm.synced() {
		clientRejectedMeter.Mark(1)
		return p2p.DiscRequested
	}
	p.Log().Debug("Light Truechain peer connected", "name", p.Name())

	// Execute the LES handshake
	var (
		genesis    = pm.blockchain.Genesis()
		head       = pm.blockchain.CurrentHeader()
		hash       = head.Hash()
		number     = head.Number.Uint64()
		td         = pm.blockchain.GetTd(hash, number)
		fastHash   = pm.fblockchain.CurrentHeader().Hash()
		fastHeight = pm.fblockchain.CurrentHeader().Number
	)
	if err := p.Handshake(td, hash, number, genesis.Hash(), fastHash, fastHeight, pm.server); err != nil {
		p.Log().Debug("Light Truechain handshake failed", "err", err)
		clientErrorMeter.Mark(1)
		return err
	}
	if p.fcClient != nil {
		defer p.fcClient.Disconnect()
	}

	if rw, ok := p.rw.(*meteredMsgReadWriter); ok {
		rw.Init(p.version)
	}
	// Register the peer locally
	if err := pm.peers.Register(p); err != nil {
		clientErrorMeter.Mark(1)
		p.Log().Error("Light Truechain peer registration failed", "err", err)
		return err
	}
	p.Log().Debug("Light Truechain peer connected Register", "name", p.Name())
	connectedAt := time.Now()
	defer func() {
		pm.removePeer(p.id, public.Normal)
		connectionTimer.UpdateSince(connectedAt)
	}()
	// Register the peer in the downloader. If the downloader considers it banned, we disconnect
	if pm.client {
		p.lock.Lock()
		head := p.headInfo
		p.lock.Unlock()
		if pm.fetcher != nil {
			pm.fetcher.announce(p, head)
			pm.fastFetcher.announce(p, head)
		}

		if p.poolEntry != nil {
			pm.serverPool.registered(p.poolEntry)
		}
	}
	// main loop. handle incoming messages.
	p.Log().Debug("Light Truechain peer connected enter loop", "name", p.Name())
	for {
		if err := pm.handleMsg(p); err != nil {
			p.Log().Debug("Light Truechain message handling failed", "err", err)
			if p.fcServer != nil {
				p.fcServer.DumpLogs()
			}
			return err
		}
	}
}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (pm *ProtocolManager) handleMsg(p *peer) error {
	select {
	case err := <-p.errCh:
		return err
	default:
	}
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()
	watch := help.NewTWatch(3, fmt.Sprintf("peer: %s, handleMsg code:%d, err: %v", p.id, msg.Code, err))
	defer func() {
		watch.EndWatch()
		watch.Finish("end")
	}()
	if err != nil {
		return err
	}
	p.Log().Trace("Light Truechain message arrived", "code", msg.Code, "bytes", msg.Size)

	p.responseCount++
	responseCount := p.responseCount
	var (
		maxCost uint64
		task    *servingTask
	)

	accept := func(reqID, reqCnt, maxCnt uint64) bool {
		inSizeCost := func() uint64 {
			if pm.server.costTracker != nil {
				return pm.server.costTracker.realCost(0, msg.Size, 0)
			}
			return 0
		}
		if p.isFrozen() || reqCnt == 0 || p.fcClient == nil || reqCnt > maxCnt {
			p.fcClient.OneTimeCost(inSizeCost())
			return false
		}
		maxCost = p.fcCosts.getMaxCost(msg.Code, reqCnt)
		gf := float64(1)
		if pm.server.costTracker != nil {
			gf = pm.server.costTracker.globalFactor()
			if gf < 0.001 {
				p.Log().Error("Invalid global cost factor", "globalFactor", gf)
				gf = 1
			}
		}
		maxTime := uint64(float64(maxCost) / gf)

		if accepted, bufShort, servingPriority := p.fcClient.AcceptRequest(reqID, responseCount, maxCost); !accepted {
			p.freezeClient()
			p.Log().Warn("Request came too early", "remaining", common.PrettyDuration(time.Duration(bufShort*1000000/p.fcParams.MinRecharge)))
			p.fcClient.OneTimeCost(inSizeCost())
			return false
		} else {
			task = pm.servingQueue.newTask(p, maxTime, servingPriority)
		}
		if task.start() {
			return true
		}
		p.fcClient.RequestProcessed(reqID, responseCount, maxCost, inSizeCost())
		return false
	}

	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	var deliverMsg *Msg

	sendResponse := func(reqID, amount uint64, reply *reply, servingTime uint64) {
		p.responseLock.Lock()
		defer p.responseLock.Unlock()

		if p.isFrozen() {
			amount = 0
			reply = nil
		}
		var replySize uint32
		if reply != nil {
			replySize = reply.size()
		}
		var realCost uint64
		if pm.server.costTracker != nil {
			realCost = pm.server.costTracker.realCost(servingTime, msg.Size, replySize)
			if amount != 0 {
				pm.server.costTracker.updateStats(msg.Code, amount, servingTime, realCost)
			}
		} else {
			realCost = maxCost
		}
		bv := p.fcClient.RequestProcessed(reqID, responseCount, maxCost, realCost)
		if reply != nil {
			p.queueSend(func() {
				if err := reply.send(bv); err != nil {
					select {
					case p.errCh <- err:
					default:
					}
				}
			})
		}
	}

	// Handle the message depending on its contents
	switch msg.Code {
	case StatusMsg:
		p.Log().Trace("Received status message")
		// Status messages should never arrive after the handshake
		return errResp(ErrExtraStatusMsg, "uncontrolled status message")

	// Block header query, collect the requested headers and reply
	case AnnounceMsg:
		var req announceData
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		if err := req.sanityCheck(); err != nil {
			return err
		}
		update, size := req.Update.decode()
		if p.rejectUpdate(size) {
			return errResp(ErrRequestRejected, "")
		}
		p.updateFlowControl(update)

		if req.Hash != (common.Hash{}) || req.FastHash != (common.Hash{}) {
			if p.announceType == announceTypeNone {
				return errResp(ErrUnexpectedResponse, "AnnounceMsg")
			}
			if p.announceType == announceTypeSigned {
				if err := req.checkSignature(p.ID(), update); err != nil {
					p.Log().Trace("Invalid announcement signature", "err", err)
					return err
				}
				p.Log().Trace("Valid announcement signature")
			}

			if pm.fetcher != nil {
				if req.FastHash != (common.Hash{}) {
					pm.fastFetcher.announce(p, &req)
				} else {
					pm.fetcher.announce(p, &req)
				}
			}
		}

	case GetFastBlockHeadersMsg:
		// Decode the complex header query
		var req struct {
			ReqID uint64
			Query getBlockHeadersData
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}

		query := req.Query
		if accept(req.ReqID, query.Amount, MaxHeaderFetch) {
			go func() {
				hashMode := query.Origin.Hash != (common.Hash{})
				first := true
				maxNonCanonical := uint64(100)

				// Gather blocks until the fetch or network limits is reached
				var (
					bytes   common.StorageSize
					blocks  []*incompleteBlock
					unknown bool
				)
				for !unknown && len(blocks) < int(query.Amount) && bytes < softResponseLimit {

					// FastRetrieve the next header satisfying the query
					var origin *types.Header
					block := &incompleteBlock{}

					if hashMode {
						if first {
							origin = pm.fblockchain.GetHeaderByHash(query.Origin.Hash)
							if origin != nil {
								query.Origin.Number = origin.Number.Uint64()
							}
						} else {
							origin = pm.fblockchain.GetHeader(query.Origin.Hash, query.Origin.Number)
						}
					} else {
						origin = pm.fblockchain.GetHeaderByNumber(query.Origin.Number)
					}
					if origin == nil {
						atomic.AddUint32(&p.invalidCount, 1)
						break
					}
					var body *types.Body
					if origin.CommitteeHash != (types.EmptySignHash) {
						body = rawdb.ReadBody(pm.chainDb, origin.Hash(), origin.Number.Uint64())
						block.Infos = make([]*types.CommitteeMember, len(body.Infos))
						copy(block.Infos, body.Infos)
					}
					if !query.Fruit {
						if body == nil {
							body = rawdb.ReadBody(pm.chainDb, origin.Hash(), origin.Number.Uint64())
						}
						if body != nil {
							block.Signs = make([]*types.PbftSign, len(body.Signs))
							copy(block.Signs, body.Signs)
						}
					}
					block.Head = origin
					blocks = append(blocks, block)
					bytes += estHeaderRlpSize

					// Advance to the next header of the query
					switch {
					case hashMode && query.Reverse:
						// Hash based traversal towards the genesis block
						ancestor := query.Skip + 1
						if ancestor == 0 {
							unknown = true
						} else {
							query.Origin.Hash, query.Origin.Number = pm.fblockchain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
							unknown = query.Origin.Hash == common.Hash{}
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
							if header := pm.fblockchain.GetHeaderByNumber(next); header != nil {
								nextHash := header.Hash()
								expOldHash, _ := pm.fblockchain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
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
					first = false
				}
				sendResponse(req.ReqID, query.Amount, p.ReplyBlockHeaders(req.ReqID, incompleteBlocks{blocks}), task.done())
			}()
		}

	case FastBlockHeadersMsg:
		if pm.fdownloader == nil {
			return errResp(ErrUnexpectedResponse, "fdownloader")
		}

		// A batch of headers arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Headers   incompleteBlocks
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		heads := make([]*types.Header, len(resp.Headers.Blocks))
		signs := make([][]*types.PbftSign, len(resp.Headers.Blocks))
		p.Log().Trace("Received block header response message", "count", len(resp.Headers.Blocks))
		for i, block := range resp.Headers.Blocks {
			heads[i] = block.Head
			signs[i] = block.Signs
			if block.Head.CommitteeHash != (types.EmptySignHash) {
				pm.fblockchain.SetCommitteeInfo(block.Head.Hash(), block.Head.Number.Uint64(), block.Infos)
			}
		}
		if pm.fastFetcher != nil && pm.fastFetcher.requestedID(resp.ReqID) {
			pm.fastFetcher.deliverHeaders(p, resp.ReqID, &headsWithSigns{Heads: heads, Signs: signs})
		} else {
			err := pm.fdownloader.DeliverHeaders(p.id, heads, types.DownloaderCall)
			if err != nil {
				log.Debug(fmt.Sprint(err))
			}
		}

	case GetFastBlockBodiesMsg:
		// Decode the retrieval message
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather blocks until the fetch or network limits is reached
		var (
			bytes  int
			bodies []rlp.RawValue
		)
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxBodyFetch) {
			go func() {
				p.Log().Trace("Received block bodies request", "count", len(req.Hashes))
				for _, hash := range req.Hashes {
					// Retrieve the requested block body, stopping if enough was found
					if bytes >= softResponseLimit {
						break
					}
					number := rawdb.ReadHeaderNumber(pm.chainDb, hash)
					if number == nil {
						atomic.AddUint32(&p.invalidCount, 1)
						continue
					}
					if data := rawdb.ReadBodyRLP(pm.chainDb, hash, *number); len(data) != 0 {
						bodies = append(bodies, data)
						bytes += len(data)
					}
				}
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyBlockBodiesRLP(req.ReqID, bodies), task.done())
			}()
		}

	case FastBlockBodiesMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		p.Log().Trace("Received block bodies response")
		// A batch of block bodies arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Data      []*types.Body
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		deliverMsg = &Msg{
			MsgType: MsgBlockBodies,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case GetSnailBlockHeadersMsg:
		// Decode the complex header query
		var req struct {
			ReqID uint64
			Query getBlockHeadersData
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}

		query := req.Query
		if accept(req.ReqID, query.Amount, MaxHeaderFetch) {
			go func() {
				p.Log().Trace("Received snail block header request", "number", query.Origin.Number, "count", query.Amount, "skip", query.Skip, "hash", query.Origin.Hash)

				hashMode := query.Origin.Hash != (common.Hash{})
				first := true
				maxNonCanonical := uint64(100)

				// Gather headers until the fetch or network limits is reached
				var (
					bytes      common.StorageSize
					headers    []*types.SnailHeader
					fruitHeads []*fruitHeadsData
					unknown    bool
				)
				for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit {
					// Retrieve the next header satisfying the query
					var origin *types.SnailHeader
					if hashMode {
						if first {
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
						atomic.AddUint32(&p.invalidCount, 1)
						break
					}
					if hashMode && !query.Fruit {
						if number := snaildb.ReadHeaderNumber(pm.chainDb, query.Origin.Hash); number != nil {
							if body := snaildb.ReadBody(pm.chainDb, query.Origin.Hash, *number); body != nil {
								fruitHeads = append(fruitHeads, &fruitHeadsData{body.FruitsHeaders()})
							}
						}
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
							unknown = query.Origin.Hash == common.Hash{}
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
					first = false
				}
				sendResponse(req.ReqID, query.Amount, p.ReplySnailBlockHeaders(req.ReqID, snailHeadsData{Heads: headers, FruitHeads: fruitHeads}), task.done())
			}()
		}

	case SnailBlockHeadersMsg:
		if pm.downloader == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		// A batch of headers arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Headers   snailHeadsData
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		if len(resp.Headers.Heads) != 0 {
			p.Log().Trace("Received snail block header response message", "headers", len(resp.Headers.Heads), "number", resp.Headers.Heads[0].Number)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		if pm.fetcher != nil && pm.fetcher.requestedID(resp.ReqID) {
			fruitHeads := make([][]*types.SnailHeader, len(resp.Headers.FruitHeads))
			for i, head := range resp.Headers.FruitHeads {
				fruitHeads[i] = head.FruitHead
			}
			pm.fetcher.deliverHeaders(p, resp.ReqID, resp.Headers.Heads, fruitHeads)
		} else {
			err := pm.downloader.DeliverHeaders(p.id, resp.Headers.Heads)
			if err != nil {
				log.Debug(fmt.Sprint(err))
			}
		}

	case GetSnailBlockBodiesMsg:
		// Decode the retrieval message
		var req struct {
			ReqID uint64
			Data  getBlockBodiesData
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather blocks until the fetch or network limits is reached
		var (
			fruits     []*fruitsData
			fruitHeads []*fruitHeadsData
		)
		reqCnt := len(req.Data.Hash)
		if accept(req.ReqID, uint64(reqCnt), MaxSnailBodyFetch) {
			go func() {
				for _, hash := range req.Data.Hash {
					if req.Data.Type == public.Fruit {
						// Retrieve the requested block body, stopping if enough was found
						if number := snaildb.ReadHeaderNumber(pm.chainDb, hash); number != nil {
							if body := snaildb.ReadBody(pm.chainDb, hash, *number); body != nil {
								fruits = append(fruits, &fruitsData{body.Fruits})
							}
						}
					} else if req.Data.Type == public.FruitHead {
						// Retrieve the requested block body, stopping if enough was found
						if number := snaildb.ReadHeaderNumber(pm.chainDb, hash); number != nil {
							if body := snaildb.ReadBody(pm.chainDb, hash, *number); body != nil {
								fruitHeads = append(fruitHeads, &fruitHeadsData{body.FruitsHeaders()})
							}
						}
					}
				}
				p.Log().Trace("Received snail block bodies request", "type", req.Data.Type, "fruits", len(fruits), "fruitHeads", len(fruitHeads))
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplySnailBlockBodiesRLP(req.ReqID, snailBlockBodiesData{Fruits: fruits, FruitHeads: fruitHeads, Type: req.Data.Type}), task.done())
			}()
		}

	case SnailBlockBodiesMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		// A batch of block bodies arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Data      snailBlockBodiesData
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		if resp.Data.Type == public.FruitHead {
			// Deliver them all to the downloader for queuing
			blocks := make([][]*types.SnailBlock, len(resp.Data.FruitHeads))
			for i, body := range resp.Data.FruitHeads {
				fruits := make([]*types.SnailBlock, len(body.FruitHead))
				for j, fruitHead := range body.FruitHead {
					fruits[j] = types.NewSnailBlockWithHeader(fruitHead)
				}
				blocks[i] = fruits
			}
			p.Log().Trace("Received snail block bodies response", "type", resp.Data.Type, "blocks", len(blocks), "FruitHeads", len(resp.Data.FruitHeads))
			err := pm.downloader.DeliverBodies(p.id, blocks)
			if err != nil {
				log.Debug(fmt.Sprint(err))
			}
		} else {
			deliverMsg = &Msg{
				MsgType: MsgSnailBlockBodies,
				ReqID:   resp.ReqID,
				Obj:     resp.Data,
			}
		}

	case GetFruitBodiesMsg:
		p.Log().Trace("Received fruit bodies request")
		// Decode the retrieval message
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather blocks until the fetch or network limits is reached
		var (
			bytes  int
			bodies []rlp.RawValue
		)
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxFruitBodyFetch) {
			go func() {
				for _, hash := range req.Hashes {
					// Retrieve the requested fruit body, stopping if enough was found
					if bytes >= softResponseLimit {
						break
					}
					if fruit, _, _, _ := snaildb.ReadFruit(pm.chainDb, hash); fruit != nil {
						data, err := rlp.EncodeToBytes(fruit.Body())
						if err != nil {
							log.Crit("Failed to RLP encode snail body", "err", err)
						}
						bodies = append(bodies, data)
						bytes += len(data)
					}
				}
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyFruitBodiesRLP(req.ReqID, bodies), task.done())
			}()
		}

	case FruitBodiesMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		p.Log().Trace("Received fruit bodies response")
		// A batch of block bodies arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Data      []*types.SnailBody
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		deliverMsg = &Msg{
			MsgType: MsgFruitBodies,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}

	case GetCodeMsg:
		p.Log().Trace("Received code request")
		// Decode the retrieval message
		var req struct {
			ReqID uint64
			Reqs  []CodeReq
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			bytes int
			data  [][]byte
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxCodeFetch) {
			go func() {
				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Look up the root hash belonging to the request
					number := rawdb.ReadHeaderNumber(pm.chainDb, request.BHash)
					if number == nil {
						p.Log().Warn("Failed to retrieve block num for code", "hash", request.BHash)
						atomic.AddUint32(&p.invalidCount, 1)
						continue
					}
					header := rawdb.ReadHeader(pm.chainDb, request.BHash, *number)
					if header == nil {
						p.Log().Warn("Failed to retrieve header for code", "block", *number, "hash", request.BHash)
						continue
					}
					// Refuse to search stale state data in the database since looking for
					// a non-exist key is kind of expensive.
					local := pm.blockchain.CurrentHeader().Number.Uint64()
					if !pm.server.archiveMode && header.Number.Uint64()+core.TriesInMemory <= local {
						p.Log().Debug("Reject stale code request", "number", header.Number.Uint64(), "head", local)
						atomic.AddUint32(&p.invalidCount, 1)
						continue
					}
					statedb, err := pm.fblockchain.State()

					account, err := pm.getAccount(statedb, header.Root, common.BytesToHash(request.AccKey))
					if err != nil {
						p.Log().Warn("Failed to retrieve account for code", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "err", err)
						atomic.AddUint32(&p.invalidCount, 1)
						continue
					}
					code, err := statedb.Database().TrieDB().Node(common.BytesToHash(account.CodeHash))
					if err != nil {
						p.Log().Warn("Failed to retrieve account code", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "codehash", common.BytesToHash(account.CodeHash), "err", err)
						continue
					}
					// Accumulate the code and abort if enough data was retrieved
					data = append(data, code)
					if bytes += len(code); bytes >= softResponseLimit {
						break
					}
				}
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyCode(req.ReqID, data), task.done())
			}()
		}

	case CodeMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		p.Log().Trace("Received code response")
		// A batch of node state data arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Data      [][]byte
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		deliverMsg = &Msg{
			MsgType: MsgCode,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}

	case GetReceiptsMsg:
		p.Log().Trace("Received receipts request")
		// Decode the retrieval message
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			bytes    int
			receipts []rlp.RawValue
		)
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxReceiptFetch) {
			go func() {
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if bytes >= softResponseLimit {
						break
					}
					// Retrieve the requested block's receipts, skipping if unknown to us
					var results types.Receipts
					number := rawdb.ReadHeaderNumber(pm.chainDb, hash)
					if number == nil {
						atomic.AddUint32(&p.invalidCount, 1)
						continue
					}
					results = rawdb.ReadReceipts(pm.chainDb, hash, *number)
					if results == nil {
						if header := pm.fblockchain.GetHeaderByHash(hash); header == nil || header.ReceiptHash != types.EmptyRootHash {
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
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyReceiptsRLP(req.ReqID, receipts), task.done())
			}()
		}

	case ReceiptsMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		p.Log().Trace("Received receipts response")
		// A batch of receipts arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Receipts  []types.Receipts
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		deliverMsg = &Msg{
			MsgType: MsgReceipts,
			ReqID:   resp.ReqID,
			Obj:     resp.Receipts,
		}

	case GetProofsV2Msg:
		p.Log().Trace("Received les/2 proofs request")
		// Decode the retrieval message
		var req struct {
			ReqID uint64
			Reqs  []ProofReq
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			lastBHash common.Hash
			statedb   *state.StateDB
			root      common.Hash
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxProofsFetch) {
			go func() {
				nodes := public.NewNodeSet()

				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Look up the root hash belonging to the request
					var (
						number *uint64
						header *types.Header
						trie   state.Trie
					)
					if request.BHash != lastBHash {
						root, lastBHash = common.Hash{}, request.BHash

						if number = rawdb.ReadHeaderNumber(pm.chainDb, request.BHash); number == nil {
							p.Log().Warn("Failed to retrieve block num for proof", "hash", request.BHash)
							atomic.AddUint32(&p.invalidCount, 1)
							continue
						}
						if header = rawdb.ReadHeader(pm.chainDb, request.BHash, *number); header == nil {
							p.Log().Warn("Failed to retrieve header for proof", "block", *number, "hash", request.BHash)
							continue
						}
						// Refuse to search stale state data in the database since looking for
						// a non-exist key is kind of expensive.
						local := pm.blockchain.CurrentHeader().Number.Uint64()
						if !pm.server.archiveMode && header.Number.Uint64()+core.TriesInMemory <= local {
							p.Log().Debug("Reject stale trie request", "number", header.Number.Uint64(), "head", local)
							atomic.AddUint32(&p.invalidCount, 1)
							continue
						}
						root = header.Root
					}
					// If a header lookup failed (non existent), ignore subsequent requests for the same header
					if root == (common.Hash{}) {
						atomic.AddUint32(&p.invalidCount, 1)
						continue
					}
					// Open the account or storage trie for the request
					statedb, _ = pm.fblockchain.State()

					switch len(request.AccKey) {
					case 0:
						// No account key specified, open an account trie
						trie, err = statedb.Database().OpenTrie(root)
						if trie == nil || err != nil {
							p.Log().Warn("Failed to open storage trie for proof", "block", header.Number, "hash", header.Hash(), "root", root, "err", err)
							continue
						}
					default:
						// Account key specified, open a storage trie
						account, err := pm.getAccount(statedb, root, common.BytesToHash(request.AccKey))
						if err != nil {
							p.Log().Warn("Failed to retrieve account for proof", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "err", err)
							atomic.AddUint32(&p.invalidCount, 1)
							continue
						}
						trie, err = statedb.Database().OpenStorageTrie(common.BytesToHash(request.AccKey), account.Root)
						if trie == nil || err != nil {
							p.Log().Warn("Failed to open storage trie for proof", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "root", account.Root, "err", err)
							continue
						}
					}
					// Prove the user's request from the account or stroage trie
					if err := trie.Prove(request.Key, request.FromLevel, nodes); err != nil {
						p.Log().Warn("Failed to prove state request", "block", header.Number, "hash", header.Hash(), "err", err)
						continue
					}
					if nodes.DataSize() >= softResponseLimit {
						break
					}
				}
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyProofsV2(req.ReqID, nodes.NodeList()), task.done())
			}()
		}

	case ProofsV2Msg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		p.Log().Trace("Received les/2 proofs response")
		// A batch of merkle proofs arrived to one of our previous requests
		var resp struct {
			ReqID, BV uint64
			Data      public.NodeList
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		deliverMsg = &Msg{
			MsgType: MsgProofsV2,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}

	case GetHelperTrieProofsMsg:
		// Decode the retrieval message
		var req struct {
			ReqID uint64
			Reqs  []HelperTrieReq
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			auxBytes int
			auxData  [][]byte
			Heads    []*types.SnailHeader
			fHeads   []*types.Header
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxHelperTrieProofsFetch) {
			go func() {

				var (
					lastIdx  uint64
					lastType uint
					root     common.Hash
					auxTrie  *trie.Trie
				)
				nodes := public.NewNodeSet()
				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if auxTrie == nil || request.Type != lastType || request.TrieIdx != lastIdx {
						auxTrie, lastType, lastIdx = nil, request.Type, request.TrieIdx

						var prefix string
						if root, prefix = pm.getHelperTrie(request.Type, request.TrieIdx); root != (common.Hash{}) {
							p.Log().Info("Received helper trie proof request", "req", request.Start, "", request.TrieIdx, "root", root.String())
							auxTrie, _ = trie.New(root, trie.NewDatabase(etruedb.NewTable(pm.chainDb, prefix)))
						}
					}

					if request.AuxReq == auxRoot {
						var data []byte
						if root != (common.Hash{}) {
							data = root[:]
						}
						auxData = append(auxData, data)
						auxBytes += len(data)
					} else {
						if auxTrie != nil {
							auxTrie.Prove(request.Key, request.FromLevel, nodes)
						}
						if request.AuxReq != 0 {
							data, head := pm.getHelperTrieAuxData(request)
							fHeads = append(fHeads, head)
							auxData = append(auxData, data)
							auxBytes += len(data)
							if request.Start {
								blockNum := binary.BigEndian.Uint64(request.Key)
								for i := params.DifficultyPeriod.Int64() - 1; i > 0; i-- {
									Heads = append(Heads, pm.blockchain.GetHeaderByNumber(blockNum-uint64(i)))
								}
								dataSet := pm.getHelperDataSet(blockNum)
								auxData = append(auxData, dataSet...)
								auxBytes += len(dataSet)
							}
						}
					}
					if nodes.DataSize()+auxBytes >= softResponseLimit {
						break
					}
				}
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyHelperTrieProofs(req.ReqID, HelperTrieResps{Proofs: nodes.NodeList(), AuxData: auxData, Heads: Heads, Fhead: fHeads}), task.done())
			}()
		}

	case HelperTrieProofsMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		var resp struct {
			ReqID, BV uint64
			Data      HelperTrieResps
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.Log().Trace("Received helper trie proof response", "heads", len(resp.Data.Heads), "data", len(resp.Data.AuxData))
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		deliverMsg = &Msg{
			MsgType: MsgHelperTrieProofs,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}

	case SendTxV2Msg:
		if pm.txpool == nil {
			return errResp(ErrRequestRejected, "")
		}
		// Transactions arrived, parse all of them and deliver to the pool
		var req struct {
			ReqID uint64
			Txs   []*types.Transaction
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		reqCnt := len(req.Txs)
		if accept(req.ReqID, uint64(reqCnt), MaxTxSend) {
			go func() {
				stats := make([]fast.TxStatus, len(req.Txs))
				for i, tx := range req.Txs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					hash := tx.Hash()
					stats[i] = pm.txStatus(hash)
					if stats[i].Status == core.TxStatusUnknown {
						if errs := pm.txpool.AddRemotes([]*types.Transaction{tx}); errs[0] != nil {
							stats[i].Error = errs[0].Error()
							continue
						}
						stats[i] = pm.txStatus(hash)
					}
				}
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyTxStatus(req.ReqID, stats), task.done())
			}()
		}

	case GetTxStatusMsg:
		if pm.txpool == nil {
			return errResp(ErrUnexpectedResponse, "txpool")
		}
		// Transactions arrived, parse all of them and deliver to the pool
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxTxStatus) {
			go func() {
				stats := make([]fast.TxStatus, len(req.Hashes))
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					stats[i] = pm.txStatus(hash)
				}
				sendResponse(req.ReqID, uint64(reqCnt), p.ReplyTxStatus(req.ReqID, stats), task.done())
			}()
		}

	case TxStatusMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}

		p.Log().Trace("Received tx status response")
		var resp struct {
			ReqID, BV uint64
			Status    []fast.TxStatus
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)

		p.Log().Trace("Received helper trie proof response")
		deliverMsg = &Msg{
			MsgType: MsgTxStatus,
			ReqID:   resp.ReqID,
			Obj:     resp.Status,
		}

	case StopMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}
		p.freezeServer(true)
		pm.retriever.frozen(p)
		p.Log().Warn("Service stopped")

	case ResumeMsg:
		if pm.odr == nil {
			return errResp(ErrUnexpectedResponse, "")
		}
		var bv uint64
		if err := msg.Decode(&bv); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ResumeFreeze(bv)
		p.freezeServer(false)
		p.Log().Warn("Service resumed")

	default:
		p.Log().Trace("Received unknown message", "code", msg.Code)
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}

	if deliverMsg != nil {
		err := pm.retriever.deliver(p, deliverMsg)
		if err != nil {
			p.responseErrors++
			if p.responseErrors > maxResponseErrors {
				return err
			}
		}
	}
	// If the client has made too much invalid request(e.g. request a non-exist data),
	// reject them to prevent SPAM attack.
	if atomic.LoadUint32(&p.invalidCount) > maxRequestErrors {
		return errTooManyInvalidRequest
	}
	return nil
}

// getAccount retrieves an account from the state based at root.
func (pm *ProtocolManager) getAccount(statedb *state.StateDB, root, hash common.Hash) (state.Account, error) {
	trie, err := trie.New(root, statedb.Database().TrieDB())
	if err != nil {
		return state.Account{}, err
	}
	blob, err := trie.TryGet(hash[:])
	if err != nil {
		return state.Account{}, err
	}
	var account state.Account
	if err = rlp.DecodeBytes(blob, &account); err != nil {
		return state.Account{}, err
	}
	return account, nil
}

// getHelperTrie returns the post-processed trie root for the given trie ID and section index
func (pm *ProtocolManager) getHelperTrie(id uint, idx uint64) (common.Hash, string) {
	switch id {
	case htCanonical:
		num := (idx+1)*pm.iConfig.ChtSize - 1
		sectionHead := snaildb.ReadCanonicalHash(pm.chainDb, num)
		return light.GetChtRoot(pm.chainDb, idx, sectionHead), light.ChtTablePrefix
	case htBloomBits:
		sectionHead := rawdb.ReadCanonicalHash(pm.chainDb, (idx+1)*pm.iConfig.BloomTrieSize-1)
		return fast.GetBloomTrieRoot(pm.chainDb, idx, sectionHead), fast.BloomTrieTablePrefix
	}
	return common.Hash{}, ""
}

// getHelperTrieAuxData returns requested auxiliary data for the given HelperTrie request
func (pm *ProtocolManager) getHelperTrieAuxData(req HelperTrieReq) ([]byte, *types.Header) {
	if req.Type == htCanonical && req.AuxReq == auxHeader && len(req.Key) == 8 {
		blockNum := binary.BigEndian.Uint64(req.Key)
		hash := snaildb.ReadCanonicalHash(pm.chainDb, blockNum)
		body := snaildb.ReadBody(pm.chainDb, hash, blockNum)
		fruit := body.Fruits[len(body.Fruits)-1]
		head := rawdb.ReadHeader(pm.chainDb, fruit.FastHash(), fruit.FastNumber().Uint64())
		return snaildb.ReadHeaderRLP(pm.chainDb, hash, blockNum), head
	}
	return nil, nil
}

// getHelperDataSet returns requested auxiliary data for the given HelperTrie request
func (pm *ProtocolManager) getHelperDataSet(point uint64) [][]byte {

	var headerHash [][]byte
	epoch := uint64((point - 1) / minerva.UPDATABLOCKLENGTH)
	startEpochNumber := uint64((epoch-1)*minerva.UPDATABLOCKLENGTH + 1)

	if point < minerva.STARTUPDATENUM {
		for i := uint64(0); i < point; i++ {
			header := pm.blockchain.GetHeaderByNumber(uint64(i + 1))
			headerHash = append(headerHash, header.Hash().Bytes())
		}
	} else if point < minerva.UPDATABLOCKLENGTH {
		for i := 0; i < minerva.STARTUPDATENUM; i++ {
			header := pm.blockchain.GetHeaderByNumber(uint64(i + 1))
			headerHash = append(headerHash, header.Hash().Bytes())
		}
	} else {
		for i := 0; i < minerva.STARTUPDATENUM; i++ {
			header := pm.blockchain.GetHeaderByNumber(uint64(i) + startEpochNumber)
			headerHash = append(headerHash, header.Hash().Bytes())
		}
		for i := 0; i < int(point%minerva.UPDATABLOCKLENGTH); i++ {
			header := pm.blockchain.GetHeaderByNumber(uint64(i) + startEpochNumber + minerva.UPDATABLOCKLENGTH)
			headerHash = append(headerHash, header.Hash().Bytes())
		}
	}

	return headerHash
}

func (pm *ProtocolManager) txStatus(hash common.Hash) fast.TxStatus {
	var stat fast.TxStatus
	stat.Status = pm.txpool.Status([]common.Hash{hash})[0]
	// If the transaction is unknown to the pool, try looking it up locally
	if stat.Status == core.TxStatusUnknown {
		if block, number, index := rawdb.ReadTxLookupEntry(pm.chainDb, hash); block != (common.Hash{}) {
			stat.Status = core.TxStatusIncluded
			stat.Lookup = &rawdb.TxLookupEntry{BlockHash: block, BlockIndex: number, Index: index}
		}
	}
	return stat
}

// downloaderPeerNotify implements peerSetNotify
type downloaderPeerNotify ProtocolManager

type peerConnection struct {
	manager *ProtocolManager
	peer    *peer
}

func (pc *peerConnection) Head() (common.Hash, *big.Int) {
	return pc.peer.HeadAndTd()
}

func (pc *peerConnection) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool, fast bool) error {
	reqID := genReqID()
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*peer)
			if fast {
				return peer.GetRequestCost(GetFastBlockHeadersMsg, amount)
			} else {
				return peer.GetRequestCost(GetSnailBlockHeadersMsg, amount)
			}
		},
		canSend: func(dp distPeer) bool {
			return dp.(*peer) == pc.peer
		},
		request: func(dp distPeer) func() {
			peer := dp.(*peer)
			cost := uint64(0)
			if fast {
				cost = peer.GetRequestCost(GetFastBlockHeadersMsg, amount)
			} else {
				cost = peer.GetRequestCost(GetSnailBlockHeadersMsg, amount)
			}
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.RequestHeadersByHash(reqID, cost, origin, amount, skip, reverse, fast, true) }
		},
	}
	_, ok := <-pc.manager.reqDist.queue(rq)
	if !ok {
		return light.ErrNoPeers
	}
	return nil
}

func (pc *peerConnection) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool, fast bool) error {
	reqID := genReqID()
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*peer)
			if fast {
				return peer.GetRequestCost(GetFastBlockHeadersMsg, amount)
			} else {
				return peer.GetRequestCost(GetSnailBlockHeadersMsg, amount)
			}
		},
		canSend: func(dp distPeer) bool {
			return dp.(*peer) == pc.peer
		},
		request: func(dp distPeer) func() {
			peer := dp.(*peer)
			cost := uint64(0)
			if fast {
				cost = peer.GetRequestCost(GetFastBlockHeadersMsg, amount)
			} else {
				cost = peer.GetRequestCost(GetSnailBlockHeadersMsg, amount)
			}
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.RequestHeadersByNumber(reqID, cost, origin, amount, skip, reverse, fast) }
		},
	}
	_, ok := <-pc.manager.reqDist.queue(rq)
	if !ok {
		return light.ErrNoPeers
	}
	return nil
}

func (pc *peerConnection) RequestBodies(hashes []common.Hash, fast bool, call uint32) error {
	reqID := genReqID()
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*peer)
			return peer.GetRequestCost(GetSnailBlockBodiesMsg, len(hashes))
		},
		canSend: func(dp distPeer) bool {
			return dp.(*peer) == pc.peer
		},
		request: func(dp distPeer) func() {
			peer := dp.(*peer)
			cost := uint64(0)
			cost = peer.GetRequestCost(GetSnailBlockBodiesMsg, len(hashes))
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.RequestSnailBodies(reqID, cost, getBlockBodiesData{hashes, public.FruitHead}) }
		},
	}
	_, ok := <-pc.manager.reqDist.queue(rq)
	if !ok {
		return light.ErrNoPeers
	}
	return nil
}

func (d *downloaderPeerNotify) registerPeer(p *peer) {
	pm := (*ProtocolManager)(d)
	pc := &peerConnection{
		manager: pm,
		peer:    p,
	}
	pm.downloader.RegisterLightPeer(p.id, etrueVersion, p.RemoteAddr().String(), pc)
	pm.fdownloader.RegisterLightPeer(p.id, etrueVersion, pc)
}

func (d *downloaderPeerNotify) unregisterPeer(p *peer) {
	pm := (*ProtocolManager)(d)
	pm.downloader.UnregisterPeer(p.id)
	pm.fdownloader.UnregisterPeer(p.id)

}
