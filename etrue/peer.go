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
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/rlp"
	"gopkg.in/fatih/set.v0"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

const (
	maxKnownTxs         = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownSigns       = 1024  // Maximum signs to keep in the known list
	maxKnownNodeInfo    = 1024  // Maximum node info to keep in the known list
	maxKnownFruits      = 1024  // Maximum fruits hashes to keep in the known list (prevent DOS)
	maxKnownSnailBlocks = 1024  // Maximum snailBlocks hashes to keep in the known list (prevent DOS)
	maxKnownFastBlocks  = 1024  // Maximum block hashes to keep in the known list (prevent DOS)

	// maxQueuedTxs is the maximum number of transaction lists to queue up before
	// dropping broadcasts. This is a sensitive number as a transaction list might
	// contain a single transaction, or thousands.
	maxQueuedTxs = 128
	// maxQueuedSigns is the maximum number of sign lists to queue up before
	// dropping broadcasts. This is a sensitive number as a transaction list might
	// contain a single transaction, or thousands.
	maxQueuedSigns = 128
	// contain a single transaction, or thousands.
	maxQueuedFruits = 128
	maxQueuedSnailBlocks = 128
	//for fruitEvent
	maxQueuedFruit = 4
	// maxQueuedProps is the maximum number of block propagations to queue up before
	// dropping broadcasts. There's not much point in queueing stale blocks, so a few
	// that might cover uncles should be enough.
	maxQueuedFastProps = 4

	// maxQueuedNodeInfo is the maximum number of node info propagations to queue up before
	// dropping broadcasts. There's not much point in queueing stale blocks, so a few
	// that might cover uncles should be enough.
	maxQueuedNodeInfo = 32

	// maxQueuedAnns is the maximum number of block announcements to queue up before
	// dropping broadcasts. Similarly to block propagations, there's no point to queue
	// above some healthy uncle limit, so use that.
	maxQueuedFastAnns = 4

	handshakeTimeout = 5 * time.Second
)

// PeerInfo represents a short summary of the Truechain sub-protocol metadata known
// about a connected peer.
type PeerInfo struct {
	Version    int      `json:"version"`    // Truechain protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the peer's blockchain
	Head       string   `json:"head"`       // SHA3 hash of the peer's best owned block
}

// propEvent is a fast block propagation, waiting for its turn in the broadcast queue.
type propFastEvent struct {
	block *types.Block
}

// propEvent is a fruit propagation, waiting for its turn in the broadcast queue.
type fruitEvent struct {
	block *types.SnailBlock
	td    *big.Int
}

// propEvent is a snailBlock propagation, waiting for its turn in the broadcast queue.
type snailBlockEvent struct {
	block *types.SnailBlock
	td    *big.Int
}

type peer struct {
	id string

	*p2p.Peer
	rw p2p.MsgReadWriter

	version  int         // Protocol version negotiated
	forkDrop *time.Timer // Timed connection dropper if forks aren't validated in time

	head common.Hash
	td   *big.Int
	lock sync.RWMutex

	knownTxs          *set.Set                  // Set of transaction hashes known to be known by this peer
	knownSign         *set.Set                  // Set of sign  known to be known by this peer
	knownNodeInfos    *set.Set                  // Set of node info  known to be known by this peer
	knownFruits       *set.Set                  // Set of fruits hashes known to be known by this peer
	knownSnailBlocks  *set.Set                  // Set of snailBlocks hashes known to be known by this peer
	knownFastBlocks   *set.Set                  // Set of fast block hashes known to be known by this peer
	queuedTxs         chan []*types.Transaction // Queue of transactions to broadcast to the peer
	queuedSign        chan []*types.PbftSign    // Queue of sign to broadcast to the peer
	queuedNodeInfo    chan *CryNodeInfo         // a node info to broadcast to the peer
	queuedFruits      chan []*types.SnailBlock  // Queue of fruits to broadcast to the peer
	queuedSnailBlcoks chan []*types.SnailBlock  // Queue of snailBlocks to broadcast to the peer
	queuedFastProps   chan *propFastEvent       // Queue of fast blocks to broadcast to the peer

	queuedFruit      chan *fruitEvent      // Queue of newFruits to broadcast to the peer
	queuedSnailBlock chan *snailBlockEvent // Queue of newSnailBlock to broadcast to the peer

	queuedFastAnns chan *types.Block // Queue of fastBlocks to announce to the peer
	term           chan struct{}     // Termination channel to stop the broadcaster
}

func newPeer(version int, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	return &peer{
		Peer:            p,
		rw:              rw,
		version:         version,
		id:              fmt.Sprintf("%x", p.ID().Bytes()[:8]),
		knownTxs:        set.New(),
		knownSign:       set.New(),
		knownNodeInfos:  set.New(),
		knownFruits:     set.New(),
		knownSnailBlocks:     set.New(),
		knownFastBlocks: set.New(),
		queuedTxs:       make(chan []*types.Transaction, maxQueuedTxs),
		queuedSign:      make(chan []*types.PbftSign, maxQueuedSigns),
		queuedNodeInfo:  make(chan *CryNodeInfo, maxQueuedNodeInfo),
		queuedFruits: make(chan []*types.SnailBlock, maxQueuedFruits),
		queuedSnailBlcoks: make(chan []*types.SnailBlock, maxQueuedSnailBlocks),
		queuedFastProps: make(chan *propFastEvent, maxQueuedFastProps),

		queuedFruit:  make(chan *fruitEvent, maxQueuedFruit),
		queuedSnailBlock: make(chan *snailBlockEvent, maxQueuedFruit),

		queuedFastAnns:  make(chan *types.Block, maxQueuedFastAnns),
		term:         make(chan struct{}),
	}
}

// broadcast is a write loop that multiplexes block propagations, announcements
// and transaction broadcasts into the remote peer. The goal is to have an async
// writer that does not lock up node internals.
func (p *peer) broadcast() {
	for {
		select {
		case txs := <-p.queuedTxs:
			if err := p.SendTransactions(txs); err != nil {
				return
			}
			p.Log().Trace("Broadcast transactions", "count", len(txs))

			//add for sign
		case signs := <-p.queuedSign:
			if err := p.SendSign(signs); err != nil {
				return
			}
			p.Log().Trace("Broadcast sign")

			//add for node info
		case nodeInfo := <-p.queuedNodeInfo:
			if err := p.SendNodeInfo(nodeInfo); err != nil {
				return
			}
			p.Log().Trace("Broadcast node info ")

		//add for fruit
		case fruits := <-p.queuedFruits:
			if err := p.Sendfruits(fruits); err != nil {
				return
			}
			p.Log().Trace("Broadcast fruits", "count", len(fruits))

		//add for mined fruit
		case fruit := <-p.queuedFruit:
			if err := p.SendNewFruit(fruit.block, fruit.td); err != nil {
				return
			}
			p.Log().Trace("Propagated fruit", "number", fruit.block.Number(), "hash", fruit.block.Hash(), "td", fruit.td)

			//add for snailBlock
		case snailBlocks := <-p.queuedSnailBlcoks:
			if err := p.SendsnailBlocks(snailBlocks); err != nil {
				return
			}
			p.Log().Trace("Broadcast snailBlocks", "count", len(snailBlocks))

			//add for mined snailBlock
		case snailBlock := <-p.queuedSnailBlock:
			if err := p.SendNewSnailBlock(snailBlock.block, snailBlock.td); err != nil {
				return
			}
			p.Log().Trace("Propagated snailBlock", "number", snailBlock.block.Number(), "hash", snailBlock.block.Hash(), "td", snailBlock.td)

		case prop := <-p.queuedFastProps:
			if err := p.SendNewFastBlock(prop.block); err != nil {
				return
			}
			p.Log().Trace("Propagated fast block", "number", prop.block.Number(), "hash", prop.block.Hash())

		case block := <-p.queuedFastAnns:
			if err := p.SendNewFastBlockHashes([]common.Hash{block.Hash()}, []uint64{block.NumberU64()}, []types.PbftSign{*block.GetLeaderSign()}); err != nil {
				return
			}
			p.Log().Trace("Announced fast block", "number", block.Number(), "hash", block.Hash())

		case <-p.term:
			return
		}
	}
}

// close signals the broadcast goroutine to terminate.
func (p *peer) close() {
	close(p.term)
}

// Info gathers and returns a collection of metadata known about a peer.
func (p *peer) Info() *PeerInfo {
	hash, td := p.Head()

	return &PeerInfo{
		Version:    p.version,
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// Head retrieves a copy of the current head hash and total difficulty of the
// peer.
func (p *peer) Head() (hash common.Hash, td *big.Int) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	copy(hash[:], p.head[:])
	return hash, new(big.Int).Set(p.td)
}

// SetHead updates the head hash and total difficulty of the peer.
func (p *peer) SetHead(hash common.Hash, td *big.Int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	copy(p.head[:], hash[:])
	p.td.Set(td)
}

// MarkFastBlock marks a block as known for the peer, ensuring that the block will
// never be propagated to this particular peer.
func (p *peer) MarkFastBlock(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownFastBlocks.Size() >= maxKnownFastBlocks {
		p.knownFastBlocks.Pop()
	}
	p.knownFastBlocks.Add(hash)
}

// MarkTransaction marks a transaction as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkTransaction(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known transaction hash
	for p.knownTxs.Size() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(hash)
}

// MarkSign marks a sign as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkSign(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known sign hash
	for p.knownSign.Size() >= maxKnownSigns {
		p.knownSign.Pop()
	}
	p.knownSign.Add(hash)
}

// MarkNodeInfo marks a node info as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkNodeInfo(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known node info hash
	for p.knownNodeInfos.Size() >= maxKnownNodeInfo {
		p.knownNodeInfos.Pop()
	}
	p.knownNodeInfos.Add(hash)
}

// MarkFruit marks a fruit as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkFruit(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known transaction hash
	for p.knownFruits.Size() >= maxKnownFruits {
		p.knownFruits.Pop()
	}
	p.knownFruits.Add(hash)
}

// MarkSnailBlock marks a snailBlock as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkSnailBlock(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known transaction hash
	for p.knownSnailBlocks.Size() >= maxKnownSnailBlocks {
		p.knownSnailBlocks.Pop()
	}
	p.knownSnailBlocks.Add(hash)
}

// SendTransactions sends transactions to the peer and includes the hashes
// in its transaction hash set for future reference.
func (p *peer) SendTransactions(txs types.Transactions) error {
	for _, tx := range txs {
		p.knownTxs.Add(tx.Hash())
	}
	return p2p.Send(p.rw, TxMsg, txs)
}

// AsyncSendTransactions queues list of transactions propagation to a remote
// peer. If the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendTransactions(txs []*types.Transaction) {
	select {
	case p.queuedTxs <- txs:
		for _, tx := range txs {
			p.knownTxs.Add(tx.Hash())
		}
	default:
		p.Log().Debug("Dropping transaction propagation", "count", len(txs))
	}
}

//SendSigns sends signs to the peer and includes the hashes
// in its signs hash set for future reference.
func (p *peer) SendSign(signs []*types.PbftSign) error {
	for _, sign := range signs {
		p.knownSign.Add(sign.Hash())
	}
	return p2p.Send(p.rw, BlockSignMsg, signs)
}

func (p *peer) AsyncSendSign(signs []*types.PbftSign) {
	select {
	case p.queuedSign <- signs:
		for _, sign := range signs {
			p.knownSign.Add(sign.Hash())
		}
	default:
		p.Log().Debug("Dropping sign propagation")
	}
}

//SendNodeInfo sends node info to the peer and includes the hashes
// in its signs hash set for future reference.
func (p *peer) SendNodeInfo(nodeInfo *CryNodeInfo) error {
	p.knownNodeInfos.Add(nodeInfo.Hash())
	return p2p.Send(p.rw, PbftNodeInfoMsg, nodeInfo)
}

func (p *peer) AsyncSendNodeInfo(nodeInfo *CryNodeInfo) {
	select {
	case p.queuedNodeInfo <- nodeInfo:
		p.knownNodeInfos.Add(nodeInfo.Hash())
	default:
		p.Log().Debug("Dropping node info propagation")
	}
}

//Sendfruits sends fruits to the peer and includes the hashes
// in its fruit hash set for future reference.
func (p *peer) Sendfruits(fruits types.Fruits) error {
	for _, fruit := range fruits {
		p.knownFruits.Add(fruit.Hash())
	}
	return p2p.Send(p.rw, FruitMsg, fruits)
}

func (p *peer) SendsnailBlocks(snailBlocks types.SnailBlocks) error {
	for _, snailBlock := range snailBlocks {
		p.knownSnailBlocks.Add(snailBlock.Hash())
	}
	return p2p.Send(p.rw, SnailBlockMsg, snailBlocks)
}

//for record;the same as transactions
func (p *peer) AsyncSendFruits(fruits []*types.SnailBlock) {
	select {
	case p.queuedFruits <- fruits:
		for _, fruit := range fruits {
			p.knownFruits.Add(fruit.Hash())
		}
	default:
		p.Log().Debug("Dropping fruits propagation", "count", len(fruits))
	}
}

func (p *peer) AsyncSendSnailBlocks(snailBlocks []*types.SnailBlock) {
	select {
	case p.queuedSnailBlcoks <- snailBlocks:
		for _, snailBlock := range snailBlocks {
			p.knownSnailBlocks.Add(snailBlock.Hash())
		}
	default:
		p.Log().Debug("Dropping snailBlocks propagation", "count", len(snailBlocks))
	}
}

// SendNewBlockHashes announces the availability of a number of blocks through
// a hash notification.
func (p *peer) SendNewFastBlockHashes(hashes []common.Hash, numbers []uint64, signs []types.PbftSign) error {
	for _, hash := range hashes {
		p.knownFastBlocks.Add(hash)
	}
	request := make(newBlockHashesData, len(hashes))
	for i := 0; i < len(hashes); i++ {
		request[i].Hash = hashes[i]
		request[i].Number = numbers[i]
		request[i].Sign = signs[i]
	}
	return p2p.Send(p.rw, NewFastBlockHashesMsg, request)
}

// AsyncSendNewBlockHash queues the availability of a fast block for propagation to a
// remote peer. If the peer's broadcast queue is full, the event is silently
// dropped.
func (p *peer) AsyncSendNewFastBlockHash(block *types.Block) {
	select {
	case p.queuedFastAnns <- block:
		p.knownFastBlocks.Add(block.Hash())
	default:
		p.Log().Debug("Dropping fast block announcement", "number", block.NumberU64(), "hash", block.Hash())
	}
}

// SendNewFastBlock propagates an entire fast block to a remote peer.
func (p *peer) SendNewFastBlock(block *types.Block) error {
	p.knownFastBlocks.Add(block.Hash())
	return p2p.Send(p.rw, NewFastBlockMsg, []interface{}{block})
}

// AsyncSendNewFastBlock queues an entire block for propagation to a remote peer. If
// the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendNewFastBlock(block *types.Block) {
	select {
	case p.queuedFastProps <- &propFastEvent{block: block}:
		p.knownFastBlocks.Add(block.Hash())
	default:
		p.Log().Debug("Dropping block propagation", "number", block.NumberU64(), "hash", block.Hash())
	}
}

// SendNewFruit propagates an entire fruit to a remote peer.
func (p *peer) SendNewFruit(fruit *types.SnailBlock, td *big.Int) error {
	p.knownFruits.Add(fruit.Hash())
	return p2p.Send(p.rw, FruitMsg, []interface{}{fruit, td})
}

func (p *peer) SendNewSnailBlock(snailBlock *types.SnailBlock, td *big.Int) error {
	p.knownSnailBlocks.Add(snailBlock.Hash())
	return p2p.Send(p.rw, SnailBlockMsg, []interface{}{snailBlock, td})
}

// AsyncSendNewFruit queues an entire fruit for propagation to a remote peer. If
// the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendNewFruit(fruit *types.SnailBlock, td *big.Int) {
	select {
	case p.queuedFruit <- &fruitEvent{block: fruit, td: td}:
		p.knownFruits.Add(fruit.Hash())
	default:
		p.Log().Debug("Dropping fruit propagation", "number", fruit.NumberU64(), "hash", fruit.Hash())
	}
}

// AsyncSendNewSnailBlock queues an entire snailBlock for propagation to a remote peer. If
// the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendNewSnailBlock(snailBlock *types.SnailBlock, td *big.Int) {
	select {
	case p.queuedSnailBlock <- &snailBlockEvent{block: snailBlock, td: td}:
		p.knownSnailBlocks.Add(snailBlock.Hash())
	default:
		p.Log().Debug("Dropping snailBlock propagation", "number", snailBlock.NumberU64(), "hash", snailBlock.Hash())
	}
}

// SendFastBlockHeaders sends a batch of block headers to the remote peer.
func (p *peer) SendFastBlockHeaders(headers []*types.Header) error {
	return p2p.Send(p.rw, FastBlockHeadersMsg, headers)
}

// SendFastBlockBodiesRLP sends a batch of block contents to the remote peer from
// an already RLP encoded format.
func (p *peer) SendFastBlockBodiesRLP(bodies []rlp.RawValue) error {
	return p2p.Send(p.rw, FastBlockBodiesMsg, bodies)
}

// SendNodeDataRLP sends a batch of arbitrary internal data, corresponding to the
// hashes requested.
func (p *peer) SendNodeData(data [][]byte) error {
	return p2p.Send(p.rw, NodeDataMsg, data)
}

// SendReceiptsRLP sends a batch of transaction receipts, corresponding to the
// ones requested from an already RLP encoded format.
func (p *peer) SendReceiptsRLP(receipts []rlp.RawValue) error {
	return p2p.Send(p.rw, ReceiptsMsg, receipts)
}

// RequestOneFastHeader is a wrapper around the header query functions to fetch a
// single fast header. It is used solely by the fetcher fast.
func (p *peer) RequestOneFastHeader(hash common.Hash) error {
	p.Log().Debug("Fetching single header", "hash", hash)
	return p2p.Send(p.rw, GetFastBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false})
}

// RequestHeadersByHash fetches a batch of blocks' headers corresponding to the
// specified header query, based on the hash of an origin block.
func (p *peer) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromhash", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetFastBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestHeadersByNumber fetches a batch of blocks' headers corresponding to the
// specified header query, based on the number of an origin block.
func (p *peer) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetFastBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies corresponding to the hashes
// specified.
func (p *peer) RequestBodies(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of block bodies", "count", len(hashes))
	return p2p.Send(p.rw, GetFastBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state
// data, corresponding to the specified hashes.
func (p *peer) RequestNodeData(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of state data", "count", len(hashes))
	return p2p.Send(p.rw, GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (p *peer) RequestReceipts(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of receipts", "count", len(hashes))
	return p2p.Send(p.rw, GetReceiptsMsg, hashes)
}

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks.
func (p *peer) Handshake(network uint64, td *big.Int, head common.Hash, genesis common.Hash) error {
	// Send out own handshake in a new thread
	errc := make(chan error, 2)
	var status statusData // safe to read after two values have been received from errc

	go func() {
		errc <- p2p.Send(p.rw, StatusMsg, &statusData{
			ProtocolVersion: uint32(p.version),
			NetworkId:       network,
			TD:              td,
			CurrentBlock:    head,
			GenesisBlock:    genesis,
		})
	}()
	go func() {
		errc <- p.readStatus(network, &status, genesis)
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return err
			}
		case <-timeout.C:
			return p2p.DiscReadTimeout
		}
	}
	p.td, p.head = status.TD, status.CurrentBlock
	return nil
}

func (p *peer) readStatus(network uint64, status *statusData, genesis common.Hash) (err error) {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != StatusMsg {
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, StatusMsg)
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	// Decode the handshake and make sure everything matches
	if err := msg.Decode(&status); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}
	if status.GenesisBlock != genesis {
		return errResp(ErrGenesisBlockMismatch, "%x (!= %x)", status.GenesisBlock[:8], genesis[:8])
	}
	if status.NetworkId != network {
		return errResp(ErrNetworkIdMismatch, "%d (!= %d)", status.NetworkId, network)
	}
	if int(status.ProtocolVersion) != p.version {
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, p.version)
	}
	return nil
}

// String implements fmt.Stringer.
func (p *peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.id,
		fmt.Sprintf("eth/%2d", p.version),
	)
}

// peerSet represents the collection of active peers currently participating in
// the Truechain sub-protocol.
type peerSet struct {
	peers  map[string]*peer
	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[string]*peer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known. If a new peer it registered, its broadcast loop is also
// started.
func (ps *peerSet) Register(p *peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers[p.id]; ok {
		return errAlreadyRegistered
	}
	ps.peers[p.id] = p
	go p.broadcast()

	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	p, ok := ps.peers[id]
	if !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)
	p.close()

	return nil
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id string) *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
func (ps *peerSet) PeersWithoutFastBlock(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownFastBlocks.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutSign retrieves a list of peers that do not have a given sign
// in their set of known hashes.
func (ps *peerSet) PeersWithoutSign(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownSign.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutNodeInfo retrieves a list of peers that do not have a given node info
// in their set of known hashes.
func (ps *peerSet) PeersWithoutNodeInfo(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownNodeInfos.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
func (ps *peerSet) PeersWithoutTx(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownTxs.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutFruit retrieves a list of peers that do not have a given fruits
// in their set of known hashes.
func (ps *peerSet) PeersWithoutFruit(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownFruits.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

func (ps *peerSet) PeersWithoutSnailBlock(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownSnailBlocks.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// BestPeer retrieves the known peer with the currently highest total difficulty.
func (ps *peerSet) BestPeer() *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *peer
		bestTd   *big.Int
	)
	for _, p := range ps.peers {
		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, td
		}
	}
	return bestPeer
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *peerSet) Close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}
