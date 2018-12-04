package tbft

import (
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/go-amino"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/p2p"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/log"
	"reflect"
	"sync"
	"time"
)

const (
	StateChannel       = byte(0x20)
	DataChannel        = byte(0x21)
	VoteChannel        = byte(0x22)
	VoteSetBitsChannel = byte(0x23)

	maxMsgSize = 1048576 // 1MB; NOTE/TODO: keep in sync with ttypes.PartSet sizes.

	blocksToContributeToBecomeGoodPeer = 10000
)

//-----------------------------------------------------------------------------

// ConsensusReactor defines a reactor for the consensus service.
type ConsensusReactor struct {
	p2p.BaseReactor // BaseService + p2p.Switch

	conS *ConsensusState

	mtx      sync.RWMutex
	fastSync bool
	eventBus *ttypes.EventBus
}

// NewConsensusReactor returns a new ConsensusReactor with the given
// consensusState.
func NewConsensusReactor(consensusState *ConsensusState, fastSync bool) *ConsensusReactor {
	conR := &ConsensusReactor{
		conS:     consensusState,
		fastSync: fastSync,
	}
	conR.BaseReactor = *p2p.NewBaseReactor("ConsensusReactor", conR)
	return conR
}

// OnStart implements BaseService by subscribing to events, which later will be
// broadcasted to other peers and starting state if we're not in fast sync.
func (conR *ConsensusReactor) OnStart() error {
	log.Info("Begin ConsensusReactor start ", "fastSync", conR.FastSync())

	conR.subscribeToBroadcastEvents()

	err := conR.conS.Start()
	if err != nil {
		return err
	}
	log.Info("End ConsensusReactor start")
	return nil
}

// OnStop implements BaseService by unsubscribing from events and stopping
// state.
func (conR *ConsensusReactor) OnStop() {
	log.Info("Begin ConsensusReactor finish")
	conR.unsubscribeFromBroadcastEvents()
	running := conR.conS.IsRunning()
	conR.conS.Stop()
	if running {
		conR.conS.Wait()
	}	
	log.Info("End ConsensusReactor finish")
}

// GetChannels implements Reactor
func (conR *ConsensusReactor) GetChannels() []*p2p.ChannelDescriptor {
	// TODO optimize
	return []*p2p.ChannelDescriptor{
		{
			ID:                  StateChannel,
			Priority:            5,
			SendQueueCapacity:   100,
			RecvMessageCapacity: maxMsgSize,
		},
		{
			ID:                  DataChannel, // maybe split between gossiping current block and catchup stuff
			Priority:            10,          // once we gossip the whole block there's nothing left to send until next height or round
			SendQueueCapacity:   100,
			RecvBufferCapacity:  50 * 4096,
			RecvMessageCapacity: maxMsgSize,
		},
		{
			ID:                  VoteChannel,
			Priority:            5,
			SendQueueCapacity:   100,
			RecvBufferCapacity:  100 * 100,
			RecvMessageCapacity: maxMsgSize,
		},
		{
			ID:                  VoteSetBitsChannel,
			Priority:            1,
			SendQueueCapacity:   2,
			RecvBufferCapacity:  1024,
			RecvMessageCapacity: maxMsgSize,
		},
	}
}

// AddPeer implements Reactor
func (conR *ConsensusReactor) AddPeer(peer p2p.Peer) {
	if !conR.IsRunning() {
		return
	}

	// Create peerState for peer
	peerState := NewPeerState(peer)
	peer.Set(ttypes.PeerStateKey, peerState)

	// Begin routines for this peer.
	go conR.gossipDataRoutine(peer, peerState)
	go conR.gossipVotesRoutine(peer, peerState)
	go conR.queryMaj23Routine(peer, peerState)

	// Send our state to peer.
	// If we're fast_syncing, broadcast a RoundStepMessage later upon SwitchToConsensus().
	if !conR.FastSync() {
		conR.sendNewRoundStepMessages(peer)
	}
}

// RemovePeer implements Reactor
func (conR *ConsensusReactor) RemovePeer(peer p2p.Peer, reason interface{}) {
	if !conR.IsRunning() {
		return
	}
	// TODO
	//peer.Get(PeerStateKey).(*PeerState).Disconnect()
}

// Receive implements Reactor
// NOTE: We process these messages even when we're fast_syncing.
// Messages affect either a peer state or the consensus state.
// Peer state updates can happen in parallel, but processing of
// proposals, block parts, and votes are ordered by the receiveRoutine
// NOTE: blocks on consensus state for proposals, block parts, and votes
func (conR *ConsensusReactor) Receive(chID byte, src p2p.Peer, msgBytes []byte) {
	if !conR.IsRunning() {
		log.Debug("Receive", "src", src, "chId", chID, "bytes", msgBytes)
		return
	}
	msg, err := decodeMsg(msgBytes)
	if err != nil {
		log.Error("Error decoding message", "src", src, "chId", chID, "msg", msg, "err", err, "bytes", msgBytes)
		conR.Switch.StopPeerForError(src, err)
		return
	}
	log.Debug("Receive", "src", src, "chId", chID, "msg", msg)
	// Get peer states
	ps := src.Get(ttypes.PeerStateKey).(*PeerState)

	switch chID {
	case StateChannel:
		switch msg := msg.(type) {
		case *NewRoundStepMessage:
			ps.ApplyNewRoundStepMessage(msg)
		case *CommitStepMessage:
			ps.ApplyCommitStepMessage(msg)
		case *HasVoteMessage:
			ps.ApplyHasVoteMessage(msg)
		case *VoteSetMaj23Message:
			cs := conR.conS
			cs.mtx.Lock()
			height, votes := cs.Height, cs.Votes
			cs.mtx.Unlock()
			if height != msg.Height {
				return
			}
			// Peer claims to have a maj23 for some BlockID at H,R,S,
			err := votes.SetPeerMaj23(int(msg.Round), msg.Type, string(ps.peer.ID()), msg.BlockID)
			if err != nil {
				conR.Switch.StopPeerForError(src, err)
				return
			}
			// Respond with a VoteSetBitsMessage showing which votes we have.
			// (and consequently shows which we don't have)
			var ourVotes *help.BitArray
			switch msg.Type {
			case ttypes.VoteTypePrevote:
				ourVotes = votes.Prevotes(int(msg.Round)).BitArrayByBlockID(msg.BlockID)
			case ttypes.VoteTypePrecommit:
				ourVotes = votes.Precommits(int(msg.Round)).BitArrayByBlockID(msg.BlockID)
			default:
				log.Error("Bad VoteSetBitsMessage field Type")
				return
			}
			if d1, derr := cdc.MarshalBinaryBare(&VoteSetBitsMessage{
				Height:  msg.Height,
				Round:   msg.Round,
				Type:    msg.Type,
				BlockID: msg.BlockID,
				Votes:   ourVotes,
			}); derr != nil {
				panic(err)
			} else {
				src.TrySend(VoteSetBitsChannel, d1)
			}
		case *ProposalHeartbeatMessage:
			hb := msg.Heartbeat
			log.Debug("Received proposal heartbeat message",
				"height", hb.Height, "round", hb.Round, "sequence", hb.Sequence,
				"valIdx", hb.ValidatorIndex, "valAddr", hb.ValidatorAddress)
		default:
			log.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}

	case DataChannel:
		if conR.FastSync() {
			log.Info("Ignoring message received during fastSync", "msg", msg)
			return
		}
		switch msg := msg.(type) {
		case *ProposalMessage:
			ps.SetHasProposal(msg.Proposal)
			conR.conS.peerMsgQueue <- msgInfo{msg, string(src.ID())}
		case *ProposalPOLMessage:
			ps.ApplyProposalPOLMessage(msg)
		case *BlockPartMessage:
			ps.SetHasProposalBlockPart(msg.Height, msg.Round, uint(msg.Part.Index))
			if numBlocks := ps.RecordBlockPart(msg); numBlocks%blocksToContributeToBecomeGoodPeer == 0 {
				conR.Switch.MarkPeerAsGood(src)
			}
			conR.conS.peerMsgQueue <- msgInfo{msg, string(src.ID())}
		default:
			log.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}

	case VoteChannel:
		if conR.FastSync() {
			log.Info("Ignoring message received during fastSync", "msg", msg)
			return
		}
		switch msg := msg.(type) {
		case *VoteMessage:
			cs := conR.conS
			var height uint64
			var valSize, lastCommitSize uint
			func() {
				cs.mtx.Lock()
				defer cs.mtx.Unlock()
				height, valSize, lastCommitSize = cs.Height, cs.Validators.Size(), cs.LastCommit.Size()
			}()
			ps.EnsureVoteBitArrays(height, valSize)
			ps.EnsureVoteBitArrays(height-1, lastCommitSize)
			ps.SetHasVote(msg.Vote)
			if blocks := ps.RecordVote(msg.Vote); blocks%blocksToContributeToBecomeGoodPeer == 0 {
				conR.Switch.MarkPeerAsGood(src)
			}
			cs.peerMsgQueue <- msgInfo{msg, string(src.ID())}
		default:
			// don't punish (leave room for soft upgrades)
			log.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}
	case VoteSetBitsChannel:
		if conR.FastSync() {
			log.Info("Ignoring message received during fastSync", "msg", msg)
			return
		}
		switch msg := msg.(type) {
		case *VoteSetBitsMessage:
			cs := conR.conS
			cs.mtx.Lock()
			height, votes := cs.Height, cs.Votes
			cs.mtx.Unlock()
			if height == msg.Height {
				var ourVotes *help.BitArray
				switch msg.Type {
				case ttypes.VoteTypePrevote:
					ourVotes = votes.Prevotes(int(msg.Round)).BitArrayByBlockID(msg.BlockID)
				case ttypes.VoteTypePrecommit:
					ourVotes = votes.Precommits(int(msg.Round)).BitArrayByBlockID(msg.BlockID)
				default:
					log.Error("Bad VoteSetBitsMessage field Type")
					return
				}
				ps.ApplyVoteSetBitsMessage(msg, ourVotes)
			} else {
				ps.ApplyVoteSetBitsMessage(msg, nil)
			}
		default:
			// don't punish (leave room for soft upgrades)
			log.Error(fmt.Sprintf("Unknown message type %v", reflect.TypeOf(msg)))
		}

	default:
		log.Error(fmt.Sprintf("Unknown chId %X", chID))
	}

	if err != nil {
		log.Error("Error in Receive()", "err", err)
	}
}

// SetEventBus sets event bus.
func (conR *ConsensusReactor) SetEventBus(b *ttypes.EventBus) {
	conR.eventBus = b
	conR.conS.SetEventBus(b)
}

// FastSync returns whether the consensus reactor is in fast-sync mode.
func (conR *ConsensusReactor) FastSync() bool {
	conR.mtx.RLock()
	defer conR.mtx.RUnlock()
	return conR.fastSync
}

//--------------------------------------

// subscribeToBroadcastEvents subscribes for new round steps, votes and
// proposal heartbeats using internal pubsub defined on state to broadcast
// them to peers upon receiving.
func (conR *ConsensusReactor) subscribeToBroadcastEvents() {
	const subscriber = "consensus-reactor"
	conR.conS.evsw.AddListenerForEvent(subscriber, ttypes.EventNewRoundStep,
		func(data ttypes.EventData) {
			conR.broadcastNewRoundStepMessages(data.(*ttypes.RoundState))
		})

	conR.conS.evsw.AddListenerForEvent(subscriber, ttypes.EventVote,
		func(data ttypes.EventData) {
			conR.broadcastHasVoteMessage(data.(*ttypes.Vote))
		})

	conR.conS.evsw.AddListenerForEvent(subscriber, ttypes.EventProposalHeartbeat,
		func(data ttypes.EventData) {
			conR.broadcastProposalHeartbeatMessage(data.(*ttypes.Heartbeat))
		})
}

func (conR *ConsensusReactor) unsubscribeFromBroadcastEvents() {
	const subscriber = "consensus-reactor"
	conR.conS.evsw.RemoveListener(subscriber)
}

func (conR *ConsensusReactor) broadcastProposalHeartbeatMessage(hb *ttypes.Heartbeat) {
	log.Debug("Broadcasting proposal heartbeat message",
		"height", hb.Height, "round", hb.Round, "sequence", hb.Sequence)
	msg := &ProposalHeartbeatMessage{hb}
	conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(msg))
}

func (conR *ConsensusReactor) broadcastNewRoundStepMessages(rs *ttypes.RoundState) {
	log.Debug("broadcastNewRoundStepMessages", "makeRoundStepMessages", "in")
	nrsMsg, csMsg := makeRoundStepMessages(rs)
	if nrsMsg != nil {
		conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(nrsMsg))
	}
	if csMsg != nil {
		conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(csMsg))
	}
}

// Broadcasts HasVoteMessage to peers that care.
func (conR *ConsensusReactor) broadcastHasVoteMessage(vote *ttypes.Vote) {
	msg := &HasVoteMessage{
		Height: vote.Height,
		Round:  vote.Round,
		Type:   vote.Type,
		Index:  vote.ValidatorIndex,
	}
	conR.Switch.Broadcast(StateChannel, cdc.MustMarshalBinaryBare(msg))
	/*
		// TODO: Make this broadcast more selective.
		for _, peer := range conR.Switch.Peers().List() {
			ps := peer.Get(PeerStateKey).(*PeerState)
			prs := ps.GetRoundState()
			if prs.Height == vote.Height {
				// TODO: Also filter on round?
				peer.TrySend(StateChannel, struct{ ConsensusMessage }{msg})
			} else {
				// Height doesn't match
				// TODO: check a field, maybe CatchupCommitRound?
				// TODO: But that requires changing the struct field comment.
			}
		}
	*/
}

func makeRoundStepMessages(rs *ttypes.RoundState) (nrsMsg *NewRoundStepMessage, csMsg *CommitStepMessage) {
	nrsMsg = &NewRoundStepMessage{
		Height: rs.Height,
		Round:  uint(rs.Round),
		Step:   rs.Step,
		SecondsSinceStartTime: uint(time.Since(rs.StartTime).Seconds()),
		LastCommitRound:       uint(rs.LastCommit.Round()),
	}
	if rs.Step == ttypes.RoundStepCommit && rs.ProposalBlockParts != nil {
		csMsg = &CommitStepMessage{
			Height:           rs.Height,
			BlockPartsHeader: rs.ProposalBlockParts.Header(),
			BlockParts:       rs.ProposalBlockParts.BitArray(),
		}
	}
	return
}

func (conR *ConsensusReactor) sendNewRoundStepMessages(peer p2p.Peer) {
	log.Debug("sendNewRoundStepMessages", "makeRoundStepMessages", "in")
	rs := conR.conS.GetRoundState()
	nrsMsg, csMsg := makeRoundStepMessages(rs)
	if nrsMsg != nil {
		peer.Send(StateChannel, cdc.MustMarshalBinaryBare(nrsMsg))
	}
	if csMsg != nil {
		peer.Send(StateChannel, cdc.MustMarshalBinaryBare(csMsg))
	}
}

func (conR *ConsensusReactor) gossipDataRoutine(peer p2p.Peer, ps *PeerState) {
	//logger := log.With("peer", peer)

OUTER_LOOP:
	for {
		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !conR.IsRunning() {
			log.Info("Stopping gossipDataRoutine for peer")
			return
		}
		rs := conR.conS.GetRoundState()
		prs := ps.GetRoundState()

		// Send proposal Block parts?
		if rs.ProposalBlockParts != nil && rs.ProposalBlockParts.HasHeader(prs.ProposalBlockPartsHeader) {
			if index, ok := rs.ProposalBlockParts.BitArray().Sub(prs.ProposalBlockParts.Copy()).PickRandom(); ok {
				part := rs.ProposalBlockParts.GetPart(index)
				msg := &BlockPartMessage{
					Height: rs.Height,      // This tells peer that this part applies to us.
					Round:  uint(rs.Round), // This tells peer that this part applies to us.
					Part:   part,
				}
				log.Debug("Sending block part", "height", prs.Height, "round", prs.Round)
				if peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg)) {
					ps.SetHasProposalBlockPart(prs.Height, uint(prs.Round), index)
				}
				continue OUTER_LOOP
			}
		}

		// If the peer is on a previous height, help catch up.
		if (0 < prs.Height) && (prs.Height < rs.Height) {
			//heightLogger := logger.With("height", prs.Height)

			// if we never received the commit message from the peer, the block parts wont be initialized
			if prs.ProposalBlockParts == nil {
				blockMeta := conR.conS.blockStore.LoadBlockMeta(prs.Height)
				if blockMeta == nil {
					// help.PanicSanity(fmt.Sprintf("Failed to load block %d when blockStore is at %d",
					// 	prs.Height, conR.conS.blockStore.MaxBlockHeight()))
					time.Sleep(conR.conS.config.PeerGossipSleep())
				} else {
					ps.InitProposalBlockParts(blockMeta.BlockID.PartsHeader)
				}
				// continue the loop since prs is a copy and not effected by this initialization
				continue OUTER_LOOP
			}
			conR.gossipDataForCatchup(rs, prs, ps, peer)
			continue OUTER_LOOP
		}

		// If height and round don't match, sleep.
		if (rs.Height != prs.Height) || (rs.Round != prs.Round) {
			//logger.Info("Peer Height|Round mismatch, sleeping", "peerHeight", prs.Height, "peerRound", prs.Round, "peer", peer)
			time.Sleep(conR.conS.config.PeerGossipSleep())
			continue OUTER_LOOP
		}

		// By here, height and round match.
		// Proposal block parts were already matched and sent if any were wanted.
		// (These can match on hash so the round doesn't matter)
		// Now consider sending other things, like the Proposal itself.

		// Send Proposal && ProposalPOL BitArray?
		if rs.Proposal != nil && !prs.Proposal {
			// Proposal: share the proposal metadata with peer.
			{
				msg := &ProposalMessage{Proposal: rs.Proposal}
				log.Debug("Sending proposal", "height", prs.Height, "round", prs.Round)
				if peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg)) {
					ps.SetHasProposal(rs.Proposal)
				}
			}
			// ProposalPOL: lets peer know which POL votes we have so far.
			// Peer must receive ProposalMessage first.
			// rs.Proposal was validated, so rs.Proposal.POLRound <= rs.Round,
			// so we definitely have rs.Votes.Prevotes(rs.Proposal.POLRound).
			if 0 <= int(rs.Proposal.POLRound) {
				msg := &ProposalPOLMessage{
					Height:           rs.Height,
					ProposalPOLRound: rs.Proposal.POLRound,
					ProposalPOL:      rs.Votes.Prevotes(int(rs.Proposal.POLRound)).BitArray(),
				}
				log.Debug("Sending POL", "height", prs.Height, "round", prs.Round)
				peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg))
			}
			continue OUTER_LOOP
		}

		// Nothing to do. Sleep.
		time.Sleep(conR.conS.config.PeerGossipSleep())
		continue OUTER_LOOP
	}
}

func (conR *ConsensusReactor) gossipDataForCatchup(rs *ttypes.RoundState,
	prs *ttypes.PeerRoundState, ps *PeerState, peer p2p.Peer) {

	if index, ok := prs.ProposalBlockParts.Not().PickRandom(); ok {
		// Ensure that the peer's PartSetHeader is correct
		blockMeta := conR.conS.blockStore.LoadBlockMeta(prs.Height)
		if blockMeta == nil {
			log.Error("Failed to load block meta",
				"ourHeight", rs.Height, "blockstoreHeight", conR.conS.blockStore.MaxBlockHeight())
			time.Sleep(conR.conS.config.PeerGossipSleep())
			return
		} else if !blockMeta.BlockID.PartsHeader.Equals(prs.ProposalBlockPartsHeader) {
			log.Info("Peer ProposalBlockPartsHeader mismatch, sleeping",
				"blockPartsHeader", blockMeta.BlockID.PartsHeader, "peerBlockPartsHeader", prs.ProposalBlockPartsHeader)
			time.Sleep(conR.conS.config.PeerGossipSleep())
			return
		}
		// Load the part
		part := conR.conS.blockStore.LoadBlockPart(prs.Height, index)
		if part == nil {
			log.Error("Could not load part", "index", index,
				"blockPartsHeader", blockMeta.BlockID.PartsHeader, "peerBlockPartsHeader", prs.ProposalBlockPartsHeader)
			time.Sleep(conR.conS.config.PeerGossipSleep())
			return
		}
		// Send the part
		msg := &BlockPartMessage{
			Height: prs.Height,      // Not our height, so it doesn't matter.
			Round:  uint(prs.Round), // Not our height, so it doesn't matter.
			Part:   part,
		}
		log.Debug("Sending block part for catchup", "round", prs.Round, "index", index)
		if peer.Send(DataChannel, cdc.MustMarshalBinaryBare(msg)) {
			ps.SetHasProposalBlockPart(prs.Height, uint(prs.Round), index)
		} else {
			log.Debug("Sending block part for catchup failed")
		}
		return
	}
	//logger.Info("No parts to send in catch-up, sleeping")
	time.Sleep(conR.conS.config.PeerGossipSleep())
}

func (conR *ConsensusReactor) gossipVotesRoutine(peer p2p.Peer, ps *PeerState) {
	//logger := conR.Logger.With("peer", peer)

	// Simple hack to throttle logs upon sleep.
	var sleeping = 0

OUTER_LOOP:
	for {
		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !conR.IsRunning() {
			log.Info("Stopping gossipVotesRoutine for peer")
			return
		}
		rs := conR.conS.GetRoundState()
		prs := ps.GetRoundState()

		switch sleeping {
		case 1: // First sleep
			sleeping = 2
		case 2: // No more sleep
			sleeping = 0
		}

		//logger.Debug("gossipVotesRoutine", "rsHeight", rs.Height, "rsRound", rs.Round,
		//	"prsHeight", prs.Height, "prsRound", prs.Round, "prsStep", prs.Step)

		// If height matches, then send LastCommit, Prevotes, Precommits.
		if rs.Height == prs.Height {
			//heightLogger := logger.With("height", prs.Height)
			if conR.gossipVotesForHeight(rs, prs, ps) {
				continue OUTER_LOOP
			}
		}

		// Special catchup logic.
		// If peer is lagging by height 1, send LastCommit.
		if prs.Height != 0 && rs.Height == prs.Height+1 {
			if ps.PickSendVote(rs.LastCommit) {
				log.Debug("Picked rs.LastCommit to send", "height", prs.Height)
				continue OUTER_LOOP
			}
		}

		// Catchup logic
		// If peer is lagging by more than 1, send Commit.
		if prs.Height != 0 && rs.Height >= prs.Height+2 {
			// Load the block commit for prs.Height,
			// which contains precommit signatures for prs.Height.
			commit := conR.conS.blockStore.LoadBlockCommit(prs.Height)
			if commit != nil && ps.PickSendVote(commit) {
				log.Debug("Picked Catchup commit to send", "height", prs.Height)
				continue OUTER_LOOP
			}
		}

		if sleeping == 0 {
			// We sent nothing. Sleep...
			sleeping = 1
			log.Debug("No votes to send, sleeping", "rs.Height", rs.Height, "prs.Height", prs.Height,
				"localPV", rs.Votes.Prevotes(int(rs.Round)).BitArray(), "peerPV", prs.Prevotes,
				"localPC", rs.Votes.Precommits(int(rs.Round)).BitArray(), "peerPC", prs.Precommits)
		} else if sleeping == 2 {
			// Continued sleep...
			sleeping = 1
		}
		time.Sleep(conR.conS.config.PeerGossipSleep())
		continue OUTER_LOOP
	}
}

func (conR *ConsensusReactor) gossipVotesForHeight(rs *ttypes.RoundState, prs *ttypes.PeerRoundState, ps *PeerState) bool {
	// If there are lastCommits to send...
	if prs.Step == ttypes.RoundStepNewHeight {
		if ps.PickSendVote(rs.LastCommit) {
			log.Debug("Picked rs.LastCommit to send")
			return true
		}
	}
	// If there are POL prevotes to send...
	if prs.Step <= ttypes.RoundStepPropose && int(prs.Round) != -1 && prs.Round <= rs.Round && int(prs.ProposalPOLRound) != -1 {
		if polPrevotes := rs.Votes.Prevotes(int(prs.ProposalPOLRound)); polPrevotes != nil {
			if ps.PickSendVote(polPrevotes) {
				log.Debug("Picked rs.Prevotes(prs.ProposalPOLRound) to send",
					"round", prs.ProposalPOLRound)
				return true
			}
		}
	}
	// If there are prevotes to send...
	if prs.Step <= ttypes.RoundStepPrevoteWait && int(prs.Round) != -1 && prs.Round <= rs.Round {
		if ps.PickSendVote(rs.Votes.Prevotes(int(prs.Round))) {
			log.Debug("Picked rs.Prevotes(prs.Round) to send", "round", prs.Round)
			return true
		}
	}
	// If there are precommits to send...
	if prs.Step <= ttypes.RoundStepPrecommitWait && int(prs.Round) != -1 && prs.Round <= rs.Round {
		if ps.PickSendVote(rs.Votes.Precommits(int(prs.Round))) {
			log.Debug("Picked rs.Precommits(prs.Round) to send", "round", prs.Round)
			return true
		}
	}
	// If there are prevotes to send...Needed because of validBlock mechanism
	if int(prs.Round) != -1 && prs.Round <= rs.Round {
		if ps.PickSendVote(rs.Votes.Prevotes(int(prs.Round))) {
			log.Debug("Picked rs.Prevotes(prs.Round) to send", "round", prs.Round)
			return true
		}
	}
	// If there are POLPrevotes to send...
	if int(prs.ProposalPOLRound) != -1 {
		if polPrevotes := rs.Votes.Prevotes(int(prs.ProposalPOLRound)); polPrevotes != nil {
			if ps.PickSendVote(polPrevotes) {
				log.Debug("Picked rs.Prevotes(prs.ProposalPOLRound) to send",
					"round", prs.ProposalPOLRound)
				return true
			}
		}
	}
	return false
}

// NOTE: `queryMaj23Routine` has a simple crude design since it only comes
// into play for liveness when there's a signature DDoS attack happening.
func (conR *ConsensusReactor) queryMaj23Routine(peer p2p.Peer, ps *PeerState) {
	//logger := conR.Logger.With("peer", peer)

OUTER_LOOP:
	for {
		// Manage disconnects from self or peer.
		if !peer.IsRunning() || !conR.IsRunning() {
			log.Info("Stopping queryMaj23Routine for peer")
			return
		}

		// Maybe send Height/Round/Prevotes
		{
			rs := conR.conS.GetRoundState()
			prs := ps.GetRoundState()
			if rs.Height == prs.Height {
				if maj23, ok := rs.Votes.Prevotes(int(prs.Round)).TwoThirdsMajority(); ok {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   uint(prs.Round),
						Type:    ttypes.VoteTypePrevote,
						BlockID: maj23,
					}))
					time.Sleep(conR.conS.config.PeerQueryMaj23Sleep())
				}
			}
		}

		// Maybe send Height/Round/Precommits
		{
			rs := conR.conS.GetRoundState()
			prs := ps.GetRoundState()
			if rs.Height == prs.Height {
				if maj23, ok := rs.Votes.Precommits(int(prs.Round)).TwoThirdsMajority(); ok {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   uint(prs.Round),
						Type:    ttypes.VoteTypePrecommit,
						BlockID: maj23,
					}))
					time.Sleep(conR.conS.config.PeerQueryMaj23Sleep())
				}
			}
		}

		// Maybe send Height/Round/ProposalPOL
		{
			rs := conR.conS.GetRoundState()
			prs := ps.GetRoundState()
			if rs.Height == prs.Height && int(prs.ProposalPOLRound) >= 0 {
				if maj23, ok := rs.Votes.Prevotes(int(prs.ProposalPOLRound)).TwoThirdsMajority(); ok {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   uint(prs.ProposalPOLRound),
						Type:    ttypes.VoteTypePrevote,
						BlockID: maj23,
					}))
					time.Sleep(conR.conS.config.PeerQueryMaj23Sleep())
				}
			}
		}

		// Little point sending LastCommitRound/LastCommit,
		// These are fleeting and non-blocking.

		// Maybe send Height/CatchupCommitRound/CatchupCommit.
		{
			prs := ps.GetRoundState()
			if prs.CatchupCommitRound != -1 && 0 < prs.Height && prs.Height <= conR.conS.blockStore.MaxBlockHeight() {
				// update by iceming
				// commit := conR.conS.LoadCommit(prs.Height)
				commit := conR.conS.blockStore.LoadBlockCommit(prs.Height)
				if commit != nil {
					peer.TrySend(StateChannel, cdc.MustMarshalBinaryBare(&VoteSetMaj23Message{
						Height:  prs.Height,
						Round:   uint(commit.Round()),
						Type:    ttypes.VoteTypePrecommit,
						BlockID: commit.BlockID,
					}))
				}
				time.Sleep(conR.conS.config.PeerQueryMaj23Sleep())
			}
		}

		time.Sleep(conR.conS.config.PeerQueryMaj23Sleep())

		continue OUTER_LOOP
	}
}

// String returns a string representation of the ConsensusReactor.
// NOTE: For now, it is just a hard-coded string to avoid accessing unprotected shared variables.
// TODO: improve!
func (conR *ConsensusReactor) String() string {
	// better not to access shared variables
	return "ConsensusReactor" // conR.StringIndented("")
}

// StringIndented returns an indented string representation of the ConsensusReactor
func (conR *ConsensusReactor) StringIndented(indent string) string {
	s := "ConsensusReactor{\n"
	s += indent + "  " + conR.conS.StringIndented(indent+"  ") + "\n"
	for _, peer := range conR.Switch.Peers().List() {
		ps := peer.Get(ttypes.PeerStateKey).(*PeerState)
		s += indent + "  " + ps.StringIndented(indent+"  ") + "\n"
	}
	s += indent + "}"
	return s
}

//-----------------------------------------------------------------------------

var (
	ErrPeerStateHeightRegression = errors.New("Error peer state height regression")
	ErrPeerStateInvalidStartTime = errors.New("Error peer state invalid startTime")
)

// PeerState contains the known state of a peer, including its connection and
// threadsafe access to its PeerRoundState.
// NOTE: THIS GETS DUMPED WITH rpc/core/consensus.go.
// Be mindful of what you Expose.
type PeerState struct {
	peer   p2p.Peer
	logger log.Logger

	mtx   sync.Mutex            `json:"-"`           // NOTE: Modify below using setters, never directly.
	PRS   ttypes.PeerRoundState `json:"round_state"` // Exposed.
	Stats *peerStateStats       `json:"stats"`       // Exposed.
}

// peerStateStats holds internal statistics for a peer.
type peerStateStats struct {
	LastVoteHeight      uint64 `json:"last_vote_height"`
	Votes               uint   `json:"votes"`
	LastBlockPartHeight uint64 `json:"last_block_part_height"`
	BlockParts          uint   `json:"block_parts"`
}

func (pss peerStateStats) String() string {
	return fmt.Sprintf("peerStateStats{lvh: %d, votes: %d, lbph: %d, blockParts: %d}",
		pss.LastVoteHeight, pss.Votes, pss.LastBlockPartHeight, pss.BlockParts)
}

// NewPeerState returns a new PeerState for the given Peer
func NewPeerState(peer p2p.Peer) *PeerState {
	return &PeerState{
		peer: peer,
		//logger: log.NewNopLogger(),
		logger: nil,
		PRS: ttypes.PeerRoundState{
			Round:              ^uint(0),
			ProposalPOLRound:   ^uint(0),
			LastCommitRound:    ^uint(0),
			CatchupCommitRound: -1,
		},
		Stats: &peerStateStats{},
	}
}

// SetLogger allows to set a logger on the peer state. Returns the peer state
// itself.
func (ps *PeerState) SetLogger(logger log.Logger) *PeerState {
	ps.logger = logger
	return ps
}

// GetRoundState returns an shallow copy of the PeerRoundState.
// There's no point in mutating it since it won't change PeerState.
func (ps *PeerState) GetRoundState() *ttypes.PeerRoundState {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	prs := ps.PRS // copy
	return &prs
}

// ToJSON returns a json of PeerState, marshalled using go-amino.
func (ps *PeerState) ToJSON() ([]byte, error) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return cdc.MarshalJSON(ps)
}

// GetHeight returns an atomic snapshot of the PeerRoundState's height
// used by the mempool to ensure peers are caught up before broadcasting new txs
func (ps *PeerState) GetHeight() uint64 {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return ps.PRS.Height
}

// SetHasProposal sets the given proposal as known for the peer.
func (ps *PeerState) SetHasProposal(proposal *ttypes.Proposal) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != proposal.Height || int(ps.PRS.Round) != int(proposal.Round) {
		return
	}
	if ps.PRS.Proposal {
		return
	}

	ps.PRS.Proposal = true
	ps.PRS.ProposalBlockPartsHeader = proposal.BlockPartsHeader
	ps.PRS.ProposalBlockParts = help.NewBitArray(proposal.BlockPartsHeader.Total)
	ps.PRS.ProposalPOLRound = proposal.POLRound
	ps.PRS.ProposalPOL = nil // Nil until ProposalPOLMessage received.
}

// InitProposalBlockParts initializes the peer's proposal block parts header and bit array.
func (ps *PeerState) InitProposalBlockParts(partsHeader ttypes.PartSetHeader) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.ProposalBlockParts != nil {
		return
	}

	ps.PRS.ProposalBlockPartsHeader = partsHeader
	ps.PRS.ProposalBlockParts = help.NewBitArray(partsHeader.Total)
}

// SetHasProposalBlockPart sets the given block part index as known for the peer.
func (ps *PeerState) SetHasProposalBlockPart(height uint64, round uint, index uint) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != height || ps.PRS.Round != round {
		return
	}

	ps.PRS.ProposalBlockParts.SetIndex(index, true)
}

// PickSendVote picks a vote and sends it to the peer.
// Returns true if vote was sent.
func (ps *PeerState) PickSendVote(votes ttypes.VoteSetReader) bool {
	if vote, ok := ps.PickVoteToSend(votes); ok {
		msg := &VoteMessage{Vote: vote.Copy()}
		return ps.peer.Send(VoteChannel, cdc.MustMarshalBinaryBare(msg))
	}
	return false
}

// PickVoteToSend picks a vote to send to the peer.
// Returns true if a vote was picked.
// NOTE: `votes` must be the correct Size() for the Height().
func (ps *PeerState) PickVoteToSend(votes ttypes.VoteSetReader) (vote *ttypes.Vote, ok bool) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	if votes.Size() == 0 {
		return nil, false
	}

	height, round, type_, size := votes.Height(), votes.Round(), votes.Type(), votes.Size()

	// Lazily set data using 'votes'.
	if votes.IsCommit() {
		ps.ensureCatchupCommitRound(height, round, size)
	}
	ps.ensureVoteBitArrays(height, size)

	psVotes := ps.getVoteBitArray(height, round, type_)
	if psVotes == nil {
		return nil, false // Not something worth sending
	}
	if index, ok := votes.BitArray().Sub(psVotes).PickRandom(); ok {
		ps.setHasVote(height, round, type_, index)
		return votes.GetByIndex(index), true
	}
	return nil, false
}

func (ps *PeerState) getVoteBitArray(height uint64, round int, type_ byte) *help.BitArray {
	if !ttypes.IsVoteTypeValid(type_) {
		return nil
	}

	if ps.PRS.Height == height {
		if int(ps.PRS.Round) == round {
			switch type_ {
			case ttypes.VoteTypePrevote:
				return ps.PRS.Prevotes
			case ttypes.VoteTypePrecommit:
				return ps.PRS.Precommits
			}
		}
		if ps.PRS.CatchupCommitRound == round {
			switch type_ {
			case ttypes.VoteTypePrevote:
				return nil
			case ttypes.VoteTypePrecommit:
				return ps.PRS.CatchupCommit
			}
		}
		if int(ps.PRS.ProposalPOLRound) == round {
			switch type_ {
			case ttypes.VoteTypePrevote:
				return ps.PRS.ProposalPOL
			case ttypes.VoteTypePrecommit:
				return nil
			}
		}
		return nil
	}
	if ps.PRS.Height == height+1 {
		if int(ps.PRS.LastCommitRound) == round {
			switch type_ {
			case ttypes.VoteTypePrevote:
				return nil
			case ttypes.VoteTypePrecommit:
				return ps.PRS.LastCommit
			}
		}
		return nil
	}
	return nil
}

// 'round': A round for which we have a +2/3 commit.
func (ps *PeerState) ensureCatchupCommitRound(height uint64, round int, numValidators uint) {
	if ps.PRS.Height != height {
		return
	}
	/*
		NOTE: This is wrong, 'round' could change.
		e.g. if orig round is not the same as block LastCommit round.
		if ps.CatchupCommitRound != -1 && ps.CatchupCommitRound != round {
			help.PanicSanity(fmt.Sprintf("Conflicting CatchupCommitRound. Height: %v, Orig: %v, New: %v", height, ps.CatchupCommitRound, round))
		}
	*/
	if int(ps.PRS.CatchupCommitRound) == round {
		return // Nothing to do!
	}
	ps.PRS.CatchupCommitRound = round
	if round == int(ps.PRS.Round) {
		ps.PRS.CatchupCommit = ps.PRS.Precommits
	} else {
		ps.PRS.CatchupCommit = help.NewBitArray(numValidators)
	}
}

// EnsureVoteBitArrays ensures the bit-arrays have been allocated for tracking
// what votes this peer has received.
// NOTE: It's important to make sure that numValidators actually matches
// what the node sees as the number of validators for height.
func (ps *PeerState) EnsureVoteBitArrays(height uint64, numValidators uint) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	ps.ensureVoteBitArrays(height, numValidators)
}

func (ps *PeerState) ensureVoteBitArrays(height uint64, numValidators uint) {
	if ps.PRS.Height == height {
		if ps.PRS.Prevotes == nil {
			ps.PRS.Prevotes = help.NewBitArray(numValidators)
		}
		if ps.PRS.Precommits == nil {
			ps.PRS.Precommits = help.NewBitArray(numValidators)
		}
		if ps.PRS.CatchupCommit == nil {
			ps.PRS.CatchupCommit = help.NewBitArray(numValidators)
		}
		if ps.PRS.ProposalPOL == nil {
			ps.PRS.ProposalPOL = help.NewBitArray(numValidators)
		}
	} else if ps.PRS.Height == height+1 {
		if ps.PRS.LastCommit == nil {
			ps.PRS.LastCommit = help.NewBitArray(numValidators)
		}
	}
}

// RecordVote updates internal statistics for this peer by recording the vote.
// It returns the total number of votes (1 per block). This essentially means
// the number of blocks for which peer has been sending us votes.
func (ps *PeerState) RecordVote(vote *ttypes.Vote) uint {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.Stats.LastVoteHeight >= vote.Height {
		return ps.Stats.Votes
	}
	ps.Stats.LastVoteHeight = vote.Height
	ps.Stats.Votes++
	return ps.Stats.Votes
}

// VotesSent returns the number of blocks for which peer has been sending us
// votes.
func (ps *PeerState) VotesSent() uint {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return ps.Stats.Votes
}

// RecordBlockPart updates internal statistics for this peer by recording the
// block part. It returns the total number of block parts (1 per block). This
// essentially means the number of blocks for which peer has been sending us
// block parts.
func (ps *PeerState) RecordBlockPart(bp *BlockPartMessage) uint {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.Stats.LastBlockPartHeight >= bp.Height {
		return ps.Stats.BlockParts
	}

	ps.Stats.LastBlockPartHeight = bp.Height
	ps.Stats.BlockParts++
	return ps.Stats.BlockParts
}

// BlockPartsSent returns the number of blocks for which peer has been sending
// us block parts.
func (ps *PeerState) BlockPartsSent() uint {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return ps.Stats.BlockParts
}

// SetHasVote sets the given vote as known by the peer
func (ps *PeerState) SetHasVote(vote *ttypes.Vote) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	ps.setHasVote(vote.Height, int(vote.Round), vote.Type, vote.ValidatorIndex)
}

func (ps *PeerState) setHasVote(height uint64, round int, type_ byte, index uint) {
	psVotes := ps.getVoteBitArray(height, round, type_)
	if psVotes != nil {
		psVotes.SetIndex(index, true)
	}
}

// ApplyNewRoundStepMessage updates the peer state for the new round.
func (ps *PeerState) ApplyNewRoundStepMessage(msg *NewRoundStepMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	// Ignore duplicates or decreases
	if !(msg.Height == ps.PRS.Height && msg.Step == ttypes.RoundStepNewHeight && msg.Round == 0) {
		return 
	} else if CompareHRS(msg.Height, msg.Round, msg.Step, ps.PRS.Height, ps.PRS.Round, ps.PRS.Step) <= 0 {
		return
	}

	// Just remember these values.
	psHeight := ps.PRS.Height
	psRound := ps.PRS.Round
	//psStep := ps.PRS.Step
	psCatchupCommitRound := ps.PRS.CatchupCommitRound
	psCatchupCommit := ps.PRS.CatchupCommit

	startTime := time.Now().Add(-1 * time.Duration(msg.SecondsSinceStartTime) * time.Second)
	ps.PRS.Height = msg.Height
	ps.PRS.Round = msg.Round
	ps.PRS.Step = msg.Step
	ps.PRS.StartTime = startTime
	if psHeight != msg.Height || psRound != msg.Round {
		ps.PRS.Proposal = false
		ps.PRS.ProposalBlockPartsHeader = ttypes.PartSetHeader{}
		ps.PRS.ProposalBlockParts = nil
		ps.PRS.ProposalPOLRound = ^uint(0)
		ps.PRS.ProposalPOL = nil
		// We'll update the BitArray capacity later.
		ps.PRS.Prevotes = nil
		ps.PRS.Precommits = nil
	}
	if psHeight == msg.Height && psRound != msg.Round && int(msg.Round) == psCatchupCommitRound {
		// Peer caught up to CatchupCommitRound.
		// Preserve psCatchupCommit!
		// NOTE: We prefer to use prs.Precommits if
		// pr.Round matches pr.CatchupCommitRound.
		ps.PRS.Precommits = psCatchupCommit
	}
	if psHeight != msg.Height {
		// Shift Precommits to LastCommit.
		if psHeight+1 == msg.Height && psRound == uint(msg.LastCommitRound) {
			ps.PRS.LastCommitRound = uint(msg.LastCommitRound)
			ps.PRS.LastCommit = ps.PRS.Precommits
		} else {
			ps.PRS.LastCommitRound = uint(msg.LastCommitRound)
			ps.PRS.LastCommit = nil
		}
		// We'll update the BitArray capacity later.
		ps.PRS.CatchupCommitRound = -1
		ps.PRS.CatchupCommit = nil
	}
}

// ApplyCommitStepMessage updates the peer state for the new commit.
func (ps *PeerState) ApplyCommitStepMessage(msg *CommitStepMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != msg.Height {
		return
	}

	ps.PRS.ProposalBlockPartsHeader = msg.BlockPartsHeader
	ps.PRS.ProposalBlockParts = msg.BlockParts
}

// ApplyProposalPOLMessage updates the peer state for the new proposal POL.
func (ps *PeerState) ApplyProposalPOLMessage(msg *ProposalPOLMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != msg.Height {
		return
	}
	if ps.PRS.ProposalPOLRound != msg.ProposalPOLRound {
		return
	}

	// TODO: Merge onto existing ps.PRS.ProposalPOL?
	// We might have sent some prevotes in the meantime.
	ps.PRS.ProposalPOL = msg.ProposalPOL
}

// ApplyHasVoteMessage updates the peer state for the new vote.
func (ps *PeerState) ApplyHasVoteMessage(msg *HasVoteMessage) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if ps.PRS.Height != msg.Height {
		return
	}
	ps.setHasVote(msg.Height, int(msg.Round), msg.Type, msg.Index)
}

// ApplyVoteSetBitsMessage updates the peer state for the bit-array of votes
// it claims to have for the corresponding BlockID.
// `ourVotes` is a BitArray of votes we have for msg.BlockID
// NOTE: if ourVotes is nil (e.g. msg.Height < rs.Height),
// we conservatively overwrite ps's votes w/ msg.Votes.
func (ps *PeerState) ApplyVoteSetBitsMessage(msg *VoteSetBitsMessage, ourVotes *help.BitArray) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	votes := ps.getVoteBitArray(msg.Height, int(msg.Round), msg.Type)
	if votes != nil {
		if ourVotes == nil {
			votes.Update(msg.Votes)
		} else {
			otherVotes := votes.Sub(ourVotes)
			hasVotes := otherVotes.Or(msg.Votes)
			votes.Update(hasVotes)
		}
	}
}

// String returns a string representation of the PeerState
func (ps *PeerState) String() string {
	return ps.StringIndented("")
}

// StringIndented returns a string representation of the PeerState
func (ps *PeerState) StringIndented(indent string) string {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return fmt.Sprintf(`PeerState{
%s  Key        %v
%s  RoundState %v
%s  Stats      %v
%s}`,
		indent, ps.peer.ID(),
		indent, ps.PRS.StringIndented(indent+"  "),
		indent, ps.Stats,
		indent)
}

//-----------------------------------------------------------------------------
// Messages

// ConsensusMessage is a message that can be sent and received on the ConsensusReactor
type ConsensusMessage interface{}

func RegisterConsensusMessages(cdc *amino.Codec) {
	cdc.RegisterInterface((*ConsensusMessage)(nil), nil)
	cdc.RegisterConcrete(&NewRoundStepMessage{}, "true/NewRoundStepMessage", nil)
	cdc.RegisterConcrete(&CommitStepMessage{}, "true/CommitStep", nil)
	cdc.RegisterConcrete(&ProposalMessage{}, "true/Proposal", nil)
	cdc.RegisterConcrete(&ProposalPOLMessage{}, "true/ProposalPOL", nil)
	cdc.RegisterConcrete(&BlockPartMessage{}, "true/BlockPart", nil)
	cdc.RegisterConcrete(&VoteMessage{}, "true/Vote", nil)
	cdc.RegisterConcrete(&HasVoteMessage{}, "true/HasVote", nil)
	cdc.RegisterConcrete(&VoteSetMaj23Message{}, "true/VoteSetMaj23", nil)
	cdc.RegisterConcrete(&VoteSetBitsMessage{}, "true/VoteSetBits", nil)
	cdc.RegisterConcrete(&ProposalHeartbeatMessage{}, "true/ProposalHeartbeat", nil)
}

func decodeMsg(bz []byte) (msg ConsensusMessage, err error) {
	if len(bz) > maxMsgSize {
		return msg, fmt.Errorf("Msg exceeds max size (%d > %d)", len(bz), maxMsgSize)
	}
	err = cdc.UnmarshalBinaryBare(bz, &msg)
	return
}

//-------------------------------------

// NewRoundStepMessage is sent for every step taken in the ConsensusState.
// For every height/round/step transition
type NewRoundStepMessage struct {
	Height                uint64
	Round                 uint
	Step                  ttypes.RoundStepType
	SecondsSinceStartTime uint
	LastCommitRound       uint
}

// String returns a string representation.
func (m *NewRoundStepMessage) String() string {
	return fmt.Sprintf("[NewRoundStep H:%v R:%v S:%v LCR:%v]",
		m.Height, m.Round, m.Step, int(m.LastCommitRound))
}

//-------------------------------------

// CommitStepMessage is sent when a block is committed.
type CommitStepMessage struct {
	Height           uint64
	BlockPartsHeader ttypes.PartSetHeader
	BlockParts       *help.BitArray
}

// String returns a string representation.
func (m *CommitStepMessage) String() string {
	return fmt.Sprintf("[CommitStep H:%v BP:%v BA:%v]", m.Height, m.BlockPartsHeader, m.BlockParts)
}

//-------------------------------------

// ProposalMessage is sent when a new block is proposed.
type ProposalMessage struct {
	Proposal *ttypes.Proposal
}

// String returns a string representation.
func (m *ProposalMessage) String() string {
	return fmt.Sprintf("[Proposal %v]", m.Proposal)
}

//-------------------------------------

// ProposalPOLMessage is sent when a previous proposal is re-proposed.
type ProposalPOLMessage struct {
	Height           uint64
	ProposalPOLRound uint
	ProposalPOL      *help.BitArray
}

// String returns a string representation.
func (m *ProposalPOLMessage) String() string {
	return fmt.Sprintf("[ProposalPOL H:%v POLR:%v POL:%v]", m.Height, m.ProposalPOLRound, m.ProposalPOL)
}

//-------------------------------------

// BlockPartMessage is sent when gossipping a piece of the proposed block.
type BlockPartMessage struct {
	Height uint64
	Round  uint
	Part   *ttypes.Part
}

// String returns a string representation.
func (m *BlockPartMessage) String() string {
	return fmt.Sprintf("[BlockPart H:%v R:%v P:%v]", m.Height, m.Round, m.Part)
}

//-------------------------------------

// VoteMessage is sent when voting for a proposal (or lack thereof).
type VoteMessage struct {
	Vote *ttypes.Vote
}

// String returns a string representation.
func (m *VoteMessage) String() string {
	return fmt.Sprintf("[Vote %v]", m.Vote)
}

//-------------------------------------

// HasVoteMessage is sent to indicate that a particular vote has been received.
type HasVoteMessage struct {
	Height uint64
	Round  uint
	Type   byte
	Index  uint
}

// String returns a string representation.
func (m *HasVoteMessage) String() string {
	return fmt.Sprintf("[HasVote VI:%v V:{%v/%02d/%v}]", m.Index, m.Height, m.Round, m.Type)
}

//-------------------------------------

// VoteSetMaj23Message is sent to indicate that a given BlockID has seen +2/3 votes.
type VoteSetMaj23Message struct {
	Height  uint64
	Round   uint
	Type    byte
	BlockID ttypes.BlockID
}

// String returns a string representation.
func (m *VoteSetMaj23Message) String() string {
	return fmt.Sprintf("[VSM23 %v/%02d/%v %v]", m.Height, m.Round, m.Type, m.BlockID)
}

//-------------------------------------

// VoteSetBitsMessage is sent to communicate the bit-array of votes seen for the BlockID.
type VoteSetBitsMessage struct {
	Height  uint64
	Round   uint
	Type    byte
	BlockID ttypes.BlockID
	Votes   *help.BitArray
}

// String returns a string representation.
func (m *VoteSetBitsMessage) String() string {
	return fmt.Sprintf("[VSB %v/%02d/%v %v %v]", m.Height, m.Round, m.Type, m.BlockID, m.Votes)
}

//-------------------------------------

// ProposalHeartbeatMessage is sent to signal that a node is alive and waiting for transactions for a proposal.
type ProposalHeartbeatMessage struct {
	Heartbeat *ttypes.Heartbeat
}

// String returns a string representation.
func (m *ProposalHeartbeatMessage) String() string {
	return fmt.Sprintf("[HEARTBEAT %v]", m.Heartbeat)
}
