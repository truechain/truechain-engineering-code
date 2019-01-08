package tbft

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"reflect"
	"runtime/debug"
	"sync"
	"time"

	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	cfg "github.com/truechain/truechain-engineering-code/params"
	// fail "github.com/ebuchman/fail-test"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

//-----------------------------------------------------------------------------
// Errors

var (
	//ErrInvalidProposalSignature is Error invalid proposal signature
	ErrInvalidProposalSignature = errors.New("Error invalid proposal signature")
	//ErrInvalidProposalPOLRound is Error invalid proposal POL round
	ErrInvalidProposalPOLRound = errors.New("Error invalid proposal POL round")
	//ErrAddingVote is Error adding vote
	ErrAddingVote = errors.New("Error adding vote")
	//ErrVoteHeightMismatch is Error vote height mismatch
	ErrVoteHeightMismatch = errors.New("Error vote height mismatch")
)

//-----------------------------------------------------------------------------

var (
	msgQueueSize = 1000
)

// msgs from the reactor which may update the state
type msgInfo struct {
	Msg    ConsensusMessage `json:"msg"`
	PeerID string           `json:"peer_key"`
}

// internally generated messages which may update the state
type timeoutInfo struct {
	Duration time.Duration        `json:"duration"`
	Height   uint64               `json:"height"`
	Round    uint                 `json:"round"`
	Step     ttypes.RoundStepType `json:"step"`
	Wait     uint                 `json:"wait"`
}

func (ti *timeoutInfo) String() string {
	return fmt.Sprintf("%v ; %d/%d %v", ti.Duration, ti.Height, ti.Round, ti.Step)
}

// ConsensusState handles execution of the consensus algorithm.
// It processes votes and proposals, and upon reaching agreement,
// commits blocks to the chain and executes them against the application.
// The internal state machine receives input from peers, the internal validator, and from a timer.
type ConsensusState struct {
	help.BaseService

	// config details
	config        *cfg.ConsensusConfig
	privValidator ttypes.PrivValidator // for signing votes

	// services for creating and executing blocks
	// TODO: encapsulate all of this in one "BlockManager"

	// internal state
	mtx sync.RWMutex
	ttypes.RoundState
	state              ttypes.StateAgent
	blockStore         *ttypes.BlockStore
	proposalForCatchup *ttypes.Proposal

	// state changes may be triggered by: msgs from peers,
	// msgs from ourself, or by timeouts
	peerMsgQueue     chan msgInfo
	internalMsgQueue chan msgInfo
	timeoutTicker    TimeoutTicker
	timeoutTask      TimeoutTicker
	taskTimeOut      time.Duration
	// we use eventBus to trigger msg broadcasts in the reactor,
	// and to notify external subscribers, eg. through a websocket
	eventBus *ttypes.EventBus

	// for tests where we want to limit the number of transitions the state makes
	nSteps int

	// some functions can be overwritten for testing
	decideProposal func(height uint64, round int, blk *types.Block, parts *ttypes.PartSet)
	doPrevote      func(height uint64, round int)
	setProposal    func(proposal *ttypes.Proposal) error

	// closed when we finish shutting down
	done chan struct{}

	// synchronous pubsub between consensus state and reactor.
	// state only emits EventNewRoundStep and EventVote
	evsw         ttypes.EventSwitch
	svs          []*ttypes.SwitchValidator
	svsBlackDoor []*ttypes.SwitchValidator
	hm           *ttypes.HealthMgr
}

// CSOption sets an optional parameter on the ConsensusState.
type CSOption func(*ConsensusState)

// NewConsensusState returns a new ConsensusState.
func NewConsensusState(
	config *cfg.ConsensusConfig,
	state ttypes.StateAgent,
	store *ttypes.BlockStore,
	options ...CSOption,
) *ConsensusState {
	cs := &ConsensusState{
		config:           config,
		blockStore:       store,
		peerMsgQueue:     make(chan msgInfo, msgQueueSize),
		internalMsgQueue: make(chan msgInfo, msgQueueSize),
		timeoutTicker:    NewTimeoutTicker("TimeoutTicker"),
		timeoutTask:      NewTimeoutTicker("TimeoutTask"),
		done:             make(chan struct{}),
		state:            state,
		evsw:             ttypes.NewEventSwitch(),
		svs:              make([]*ttypes.SwitchValidator, 0, 0),
		svsBlackDoor:     make([]*ttypes.SwitchValidator, 0, 0),
	}
	// set function defaults (may be overwritten before calling Start)
	cs.decideProposal = cs.defaultDecideProposal
	cs.doPrevote = cs.defaultDoPrevote
	cs.setProposal = cs.defaultSetProposal
	cs.taskTimeOut = config.Propose(0)

	cs.updateToState(state)
	log.Info("NewConsensusState", "Height", cs.Height)
	// Don't call scheduleRound0 yet.
	// We do that upon Start().
	cs.reconstructLastCommit()
	cs.BaseService = *help.NewBaseService("ConsensusState", cs)
	for _, option := range options {
		option(cs)
	}
	return cs
}

//----------------------------------------
// Public interface

// SetLogger implements Service.
func (cs *ConsensusState) SetLogger(l log.Logger) {
	// cs.BaseService.Logger = l
	//cs.timeoutTicker.SetLogger(l)
}

// SetEventBus sets event bus.
func (cs *ConsensusState) SetEventBus(b *ttypes.EventBus) {
	cs.eventBus = b
}

//SetHealthMgr sets peer  health
func (cs *ConsensusState) SetHealthMgr(h *ttypes.HealthMgr) {
	cs.hm = h
}

// String returns a string.
func (cs *ConsensusState) String() string {
	// better not to access shared variables
	// return fmt.Sprintf("ConsensusState") //(H:%v R:%v S:%v", cs.Height, cs.Round, cs.Step)
	return "ConsensusState"
}

// GetRoundState returns a shallow copy of the internal consensus state.
func (cs *ConsensusState) GetRoundState() *ttypes.RoundState {
	cs.mtx.RLock()
	defer cs.mtx.RUnlock()

	rs := cs.RoundState // copy
	return &rs
}

// GetRoundStateJSON returns a json of RoundState, marshalled using go-amino.
func (cs *ConsensusState) GetRoundStateJSON() ([]byte, error) {
	cs.mtx.RLock()
	defer cs.mtx.RUnlock()
	return cdc.MarshalJSON(cs.RoundState)
}

// GetRoundStateSimpleJSON returns a json of RoundStateSimple, marshalled using go-amino.
func (cs *ConsensusState) GetRoundStateSimpleJSON() ([]byte, error) {
	cs.mtx.RLock()
	defer cs.mtx.RUnlock()
	return cdc.MarshalJSON(cs.RoundState.RoundStateSimple())
}

// SetPrivValidator sets the private validator account for signing votes.
func (cs *ConsensusState) SetPrivValidator(priv ttypes.PrivValidator) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.privValidator = priv
}

// SetTimeoutTicker sets the local timer. It may be useful to overwrite for testing.
func (cs *ConsensusState) SetTimeoutTicker(timeoutTicker TimeoutTicker) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	cs.timeoutTicker = timeoutTicker
}

// OnStart implements help.Service.
// It loads the latest state via the WAL, and starts the timeout and receive routines.
func (cs *ConsensusState) OnStart() error {
	log.Info("Begin ConsensusState start")
	if cs.hm == nil {
		return errors.New("healthMgr not init")
	}
	if err := cs.evsw.Start(); err != nil {
		return err
	}
	// we need the timeoutRoutine for replay so
	// we don't block on the tick chan.
	// NOTE: we will get a build up of garbage go routines
	// firing on the tockChan until the receiveRoutine is started
	// to deal with them (by that point, at most one will be valid)
	if err := cs.timeoutTicker.Start(); err != nil {
		return err
	}
	if err := cs.timeoutTask.Start(); err != nil {
		return err
	}
	cs.updateToState(cs.state)
	// now start the receiveRoutine
	go cs.receiveRoutine(0)

	// schedule the first round!
	// use GetRoundState so we don't race the receiveRoutine for access
	cs.scheduleRound0(cs.GetRoundState())
	log.Info("End ConsensusState start")
	return nil
}

// timeoutRoutine: receive requests for timeouts on tickChan and fire timeouts on tockChan
// receiveRoutine: serializes processing of proposoals, block parts, votes; coordinates state transitions
func (cs *ConsensusState) startRoutines(maxSteps int) {
	err := cs.timeoutTicker.Start()
	if err != nil {
		log.Error("Error starting timeout ticker", "err", err)
		return
	}
	if err = cs.timeoutTask.Start(); err != nil {
		log.Error("Error starting timeoutTask ticker", "err", err)
		return
	}
	go cs.receiveRoutine(maxSteps)
}

// OnStop implements help.Service. It stops all routines and waits for the WAL to finish.
func (cs *ConsensusState) OnStop() {
	log.Info("Begin ConsensusState finish")
	cs.evsw.Stop()
	cs.timeoutTicker.Stop()
	cs.timeoutTask.Stop()
	log.Info("End ConsensusState finish")
}

// Wait waits for the the main routine to return.
// NOTE: be sure to Stop() the event switch and drain
// any event channels or this may deadlock
func (cs *ConsensusState) Wait() {
	<-cs.done
}

//------------------------------------------------------------
// Public interface for passing messages into the consensus state, possibly causing a state transition.
// If peerID == "", the msg is considered internal.
// Messages are added to the appropriate queue (peer or internal).
// If the queue is full, the function may block.
// TODO: should these return anything or let callers just use events?

// AddVote inputs a vote. not used
func (cs *ConsensusState) AddVote(vote *ttypes.Vote, peerID string) (added bool, err error) {
	if peerID == "" {
		cs.internalMsgQueue <- msgInfo{&VoteMessage{vote}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&VoteMessage{vote}, peerID}
	}
	return false, nil
}

// SetProposal inputs a proposal. not used
func (cs *ConsensusState) SetProposal(proposal *ttypes.Proposal, peerID string) error {

	if peerID == "" {
		cs.internalMsgQueue <- msgInfo{&ProposalMessage{proposal}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&ProposalMessage{proposal}, peerID}
	}

	// TODO: wait for event?!
	return nil
}

// UpdateValidatorsSet is Set when the committee member is chaneged
func (cs *ConsensusState) UpdateValidatorsSet(vset *ttypes.ValidatorSet, uHeight, eHeight uint64) {
	go func() {
		cs.internalMsgQueue <- msgInfo{&ValidatorUpdateMessage{vset, uHeight, eHeight}, ""}
	}()
}

// AddProposalBlockPart inputs a part of the proposal block. not used
func (cs *ConsensusState) AddProposalBlockPart(height uint64, round uint, part *ttypes.Part, peerID string) error {

	if peerID == "" {
		cs.internalMsgQueue <- msgInfo{&BlockPartMessage{height, round, part}, ""}
	} else {
		cs.peerMsgQueue <- msgInfo{&BlockPartMessage{height, round, part}, peerID}
	}

	// TODO: wait for event?!
	return nil
}

// SetProposalAndBlock inputs the proposal and all block parts. not used
func (cs *ConsensusState) SetProposalAndBlock(proposal *ttypes.Proposal, block *types.Block, parts *ttypes.PartSet, peerID string) error {
	if err := cs.SetProposal(proposal, peerID); err != nil {
		return err
	}
	for i := 0; i < int(parts.Total()); i++ {
		part := parts.GetPart(uint(i))
		if err := cs.AddProposalBlockPart(proposal.Height, proposal.Round, part, peerID); err != nil {
			return err
		}
	}
	return nil
}

//------------------------------------------------------------
// internal functions for managing the state

func (cs *ConsensusState) updateHeight(height uint64) {
	// cs.metrics.Height.Set(float64(height))
	cs.Height = height
}

func (cs *ConsensusState) updateRoundStep(round int, step ttypes.RoundStepType) {
	cs.Round = uint(round)
	cs.Step = step
}

// enterNewRound(height, 0) at cs.StartTime.
func (cs *ConsensusState) scheduleRound0(rs *ttypes.RoundState) {
	//log.Info("scheduleRound0", "now", time.Now(), "startTime", cs.StartTime)
	sleepDuration := rs.StartTime.Sub(time.Now()) // nolint: gotype, gosimple
	cs.scheduleTimeout(sleepDuration, rs.Height, 0, ttypes.RoundStepNewHeight)
	var d = cs.taskTimeOut
	cs.timeoutTask.ScheduleTimeout(timeoutInfo{d, rs.Height, uint(rs.Round), ttypes.RoundStepBlockSync, 0})
}

// Attempt to schedule a timeout (by sending timeoutInfo on the tickChan)
func (cs *ConsensusState) scheduleTimeout(duration time.Duration, height uint64, round int, step ttypes.RoundStepType) {
	cs.timeoutTicker.ScheduleTimeout(timeoutInfo{duration, height, uint(round), step, 1})
}

func (cs *ConsensusState) scheduleTimeoutWithWait(ti timeoutInfo) {
	cs.timeoutTicker.ScheduleTimeout(ti)
}

//UpdateStateForSync is sync update state
func (cs *ConsensusState) updateStateForSync() {
	log.Debug("begin updateStateForSync", "height", cs.Height)
	oldH := cs.Height
	newH := cs.state.GetLastBlockHeight() + 1
	if oldH != newH {
		cs.updateToState(cs.state)
		log.Info("Reset privValidator", "height", cs.Height)
		cs.state.PrivReset()
		sleepDuration := time.Duration(1) * time.Millisecond
		cs.timeoutTicker.ScheduleTimeout(timeoutInfo{sleepDuration, cs.Height, uint(0), ttypes.RoundStepNewHeight, 1})
	}
	var d = cs.taskTimeOut
	cs.timeoutTask.ScheduleTimeout(timeoutInfo{d, cs.Height, uint(cs.Round), ttypes.RoundStepBlockSync, 0})
	log.Debug("end updateStateForSync", "newHeight", newH)
}

// send a msg into the receiveRoutine regarding our own proposal, block part, or vote
func (cs *ConsensusState) sendInternalMessage(mi msgInfo) {
	select {
	case cs.internalMsgQueue <- mi:
	default:
		// NOTE: using the go-routine means our votes can
		// be processed out of order.
		// TODO: use CList here for strict determinism and
		// attempt push to internalMsgQueue in receiveRoutine
		log.Info("Internal msg queue is full. Using a go-routine")
		go func() { cs.internalMsgQueue <- mi }()
	}
}

// Reconstruct LastCommit from SeenCommit, which we saved along with the block,
// (which happens even before saving the state)
func (cs *ConsensusState) reconstructLastCommit() {
	LastBlockHeight := cs.state.GetLastBlockHeight()
	seenCommit := cs.blockStore.LoadBlockCommit(LastBlockHeight)
	if LastBlockHeight == 0 || seenCommit == nil {
		return
	}
	lastPrecommits := ttypes.NewVoteSet(cs.state.GetChainID(), LastBlockHeight, seenCommit.Round(),
		ttypes.VoteTypePrecommit, cs.state.GetLastValidator())
	for _, precommit := range seenCommit.Precommits {
		if precommit == nil {
			continue
		}
		added, err := lastPrecommits.AddVote(precommit)
		if !added || err != nil {
			help.PanicSanity(fmt.Sprintf("Failed to reconstruct LastCommit: %v", err))
		}
	}
	if !lastPrecommits.HasTwoThirdsMajority() {
		help.PanicSanity("Failed to reconstruct LastCommit: Does not have +2/3 maj")
	}
	cs.LastCommit = lastPrecommits
}

// Updates ConsensusState and increments height to match that of state.
// The round becomes 0 and cs.Step becomes ttypes.RoundStepNewHeight.
func (cs *ConsensusState) updateToState(state ttypes.StateAgent) {
	LastBlockHeight := state.GetLastBlockHeight()
	// Reset fields based on state.
	validators := state.GetValidator()
	lastPrecommits := (*ttypes.VoteSet)(nil)
	if int(cs.CommitRound) > -1 && cs.Votes != nil {
		if !cs.Votes.Precommits(int(cs.CommitRound)).HasTwoThirdsMajority() {
			help.PanicSanity("updateToState(state) called but last Precommit round didn't have +2/3")
		}
		lastPrecommits = cs.Votes.Precommits(int(cs.CommitRound))
	}

	// Next desired block height
	height := LastBlockHeight + 1

	// RoundState fields
	cs.updateHeight(height)
	cs.updateRoundStep(0, ttypes.RoundStepNewHeight)
	if cs.CommitTime.IsZero() || cs.Proposal == nil {
		// "Now" makes it easier to sync up dev nodes.
		// We add timeoutCommit to allow transactions
		// to be gathered for the first block.
		// And alternative solution that relies on clocks:
		//  cs.StartTime = state.LastBlockTime.Add(timeoutCommit)
		cs.StartTime = cs.config.Commit(time.Now())
	} else {
		if cs.Proposal != nil && cs.StartTime.After(cs.config.CatchupTime(time.Unix(cs.Proposal.Timestamp.Unix(), 0))) {
			cs.StartTime = time.Now()
		} else {
			cs.StartTime = cs.config.Commit(cs.CommitTime)
		}
	}

	cs.Validators = validators
	cs.proposalForCatchup = nil
	cs.Proposal = nil
	cs.ProposalBlock = nil
	cs.ProposalBlockParts = nil
	cs.LockedRound = 0
	cs.LockedBlock = nil
	cs.LockedBlockParts = nil
	cs.ValidRound = 0
	cs.ValidBlock = nil
	cs.ValidBlockParts = nil
	cs.Votes = ttypes.NewHeightVoteSet(state.GetChainID(), height, validators)
	cs.CommitRound = ^uint(0)
	cs.LastCommit = lastPrecommits

	cs.state = state

	// Finally, broadcast RoundState
	cs.newStep()
}

func (cs *ConsensusState) newStep() {
	rs := cs.RoundStateEvent()
	cs.nSteps++
	// newStep is called by updateToState in NewConsensusState before the eventBus is set!
	if cs.eventBus != nil {
		cs.eventBus.PublishEventNewRoundStep(rs)
		cs.evsw.FireEvent(ttypes.EventNewRoundStep, &cs.RoundState)
	}
}
func (cs *ConsensusState) validatorUpdate(msg *ValidatorUpdateMessage) {
	log.Info("ValidatorUpdate", "uHeight", msg.uHeight, "eHeight", msg.eHeight, "cHeight", cs.Height, "Round", cs.Round)
	round, oldHeight := cs.Round, cs.Height

	cs.state.UpdateValidator(msg.vset)
	cs.updateToState(cs.state)
	cs.state.PrivReset()
	cs.state.SetEndHeight(msg.eHeight)
	cs.state.SetBeginHeight(msg.uHeight)
	newHeight := cs.Height
	log.Info("ValidatorUpdate,Update state")

	if newHeight == oldHeight && round > 0 {
		log.Warn("ValidatorUpdate,has same height in current consensus", "oldHeight", oldHeight, "newHeight", newHeight)
		round = round + 1
	} else {
		round = 0
	}
	var d = cs.taskTimeOut
	cs.timeoutTask.ScheduleTimeout(timeoutInfo{d, newHeight, round, ttypes.RoundStepBlockSync, 0})
	log.Debug("end ValidatorUpdate", "newHeight", newHeight)
	cs.enterNewRound(newHeight, int(round))
}

//-----------------------------------------
// the main go routines

// receiveRoutine handles messages which may cause state transitions.
// it's argument (n) is the number of messages to process before exiting - use 0 to run forever
// It keeps the RoundState and is the only thing that updates it.
// Updates (state transitions) happen on timeouts, complete proposals, and 2/3 majorities.
// ConsensusState must be locked before any internal state is updated.
func (cs *ConsensusState) receiveRoutine(maxSteps int) {
	onExit := func(cs *ConsensusState) {
		// NOTE: the internalMsgQueue may have signed messages from our
		// priv_val that haven't hit the WAL, but its ok because
		// priv_val tracks LastSig
		log.Info("Exit receiveRoutine")
		close(cs.done)
	}

	defer func() {
		if r := recover(); r != nil {
			log.Error("CONSENSUS FAILURE!!!", "err", r, "stack", string(debug.Stack()))
			// stop gracefully
			//
			// NOTE: We most probably shouldn't be running any further when there is
			// some unexpected panic. Some unknown error happened, and so we don't
			// know if that will result in the validator signing an invalid thing. It
			// might be worthwhile to explore a mechanism for manual resuming via
			// some console or secure RPC system, but for now, halting the chain upon
			// unexpected consensus bugs sounds like the better option.
			onExit(cs)
		}
	}()

	for {
		if maxSteps > 0 {
			if cs.nSteps >= maxSteps {
				log.Info("reached max steps. exiting receive routine")
				cs.nSteps = 0
				return
			}
		}
		rs := cs.RoundState
		var mi msgInfo

		select {
		case mi = <-cs.peerMsgQueue:
			// handles proposals, block parts, votes
			// may generate internal events (votes, complete proposals, 2/3 majorities)
			cs.handleMsg(mi)
		case mi = <-cs.internalMsgQueue:
			// handles proposals, block parts, votes
			cs.handleMsg(mi)
		case ti := <-cs.timeoutTicker.Chan(): // tockChan:
			// if the timeout is relevant to the rs
			// go to the next step
			cs.handleTimeout(ti, rs)
		case ti := <-cs.timeoutTask.Chan():
			cs.handleTimeoutForTask(ti, rs)
		case ms := <-cs.hm.ChanTo():
			cs.switchHandle(ms)
		case <-cs.Quit():
			onExit(cs)
			return
		}
	}
}

// state transitions on complete-proposal, 2/3-any, 2/3-one
func (cs *ConsensusState) handleMsg(mi msgInfo) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	var err error
	msg, peerID := mi.Msg, mi.PeerID
	switch msg := msg.(type) {
	case *ProposalMessage:
		// will not cause transition.
		// once proposal is set, we can receive block parts
		err = cs.setProposal(msg.Proposal)
		if err != nil {
			log.Warn("SetProposal", "Warning", err)
		}
	case *BlockPartMessage:
		// if the proposal is complete, we'll enterPrevote or tryFinalizeCommit
		_, err = cs.addProposalBlockPart(msg, peerID)
		if err != nil && msg.Round != cs.Round {
			log.Debug("Received block part from wrong round", "height", cs.Height, "csRound", cs.Round, "blockRound", msg.Round)
			err = nil
		}
	case *VoteMessage:
		// attempt to add the vote and dupeout the validator if its a duplicate signature
		// if the vote gives us a 2/3-any or 2/3-one, we transition
		log.Info("VoteMessage", "peerID", peerID, "round", msg.Vote.Round, "height", msg.Vote.Height, "type", msg.Vote.Type)
		err := cs.tryAddVote(msg.Vote, peerID)
		if err == ErrAddingVote {
			// TODO: punish peer
			// We probably don't want to stop the peer here. The vote does not
			// necessarily comes from a malicious peer but can be just broadcasted by
			// a typical peer.
		}

		// NOTE: the vote is broadcast to peers by the reactor listening
		// for vote events

		// TODO: If rs.Height == vote.Height && rs.Round < vote.Round,
		// the peer is sending us CatchupCommit precommits.
		// We could make note of this and help filter in broadcastHasVoteMessage().
	case *ValidatorUpdateMessage:
		cs.validatorUpdate(msg)
	default:
		log.Error("Unknown msg type", reflect.TypeOf(msg))
	}
	if err != nil {
		log.Error("Error with msg", "height", cs.Height, "round", cs.Round, "type", reflect.TypeOf(msg), "peer", peerID, "err", err, "msg", msg)
	}
}

func (cs *ConsensusState) handleTimeout(ti timeoutInfo, rs ttypes.RoundState) {
	log.Debug("Received tock", "timeout", ti.Duration, "height", ti.Height, "round", ti.Round, "step", ti.Step)

	// timeouts must be for current height, round, step
	if ti.Height != rs.Height || ti.Round < rs.Round || (ti.Round == rs.Round && ti.Step < rs.Step) {
		log.Debug("Ignoring tock because we're ahead", "height", rs.Height, "round", rs.Round, "step", rs.Step)
		return
	}

	// the timeout will now cause a state transition
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	switch ti.Step {
	case ttypes.RoundStepNewHeight:
		// NewRound event fired from enterNewRound.
		// XXX: should we fire timeout here (for timeout commit)?
		cs.enterNewRound(ti.Height, 0)
	case ttypes.RoundStepNewRound:
		cs.tryEnterProposal(ti.Height, 0, ti.Wait)
	case ttypes.RoundStepPropose:
		cs.eventBus.PublishEventTimeoutPropose(cs.RoundStateEvent())
		cs.enterPrevote(ti.Height, int(ti.Round))
	case ttypes.RoundStepPrevoteWait:
		cs.eventBus.PublishEventTimeoutWait(cs.RoundStateEvent())
		cs.enterPrecommit(ti.Height, int(ti.Round))
	case ttypes.RoundStepPrecommitWait:
		cs.eventBus.PublishEventTimeoutWait(cs.RoundStateEvent())
		cs.enterNewRound(ti.Height, int(ti.Round)+1)
	default:
		panic(fmt.Sprintf("Invalid timeout step: %v", ti.Step))
	}
}
func (cs *ConsensusState) handleTimeoutForTask(ti timeoutInfo, rs ttypes.RoundState) {
	log.Debug("Received task tock", "timeout", ti.Duration, "height", ti.Height, "round", ti.Round, "cs.height", cs.Height)
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	// timeouts must be for current height, round, step
	cs.updateStateForSync()
	log.Debug("Received task tock End")
}

//-----------------------------------------------------------------------------
// State functions
// Used internally by handleTimeout and handleMsg to make state transitions

// Enter: `timeoutNewHeight` by startTime (commitTime+timeoutCommit),
// 	or, if SkipTimeout==true, after receiving all precommits from (height,round-1)
// Enter: `timeoutPrecommits` after any +2/3 precommits from (height,round-1)
// Enter: +2/3 precommits for nil at (height,round-1)
// Enter: +2/3 prevotes any or +2/3 precommits for block or any from (height, round)
// NOTE: cs.StartTime was already set for height.
func (cs *ConsensusState) enterNewRound(height uint64, round int) {
	//logger := log.With("height", height, "round", round)
	if cs.Height != height || round < int(cs.Round) || (int(cs.Round) == round && cs.Step != ttypes.RoundStepNewHeight) {
		log.Debug(fmt.Sprintf("enterNewRound(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
		return
	}

	if now := time.Now(); cs.StartTime.After(now) {
		log.Info("Need to set a buffer and log message here for sanity.", "startTime", cs.StartTime, "now", now)
	}

	log.Info(fmt.Sprintf("enterNewRound(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	// Increment validators if necessary
	validators := cs.Validators
	if int(cs.Round) < round {
		validators = validators.Copy()
		validators.IncrementAccum(uint(round - int(cs.Round)))
	}

	// Setup new round
	// we don't fire newStep for this step,
	// but we fire an event, so update the round step first
	cs.updateRoundStep(round, ttypes.RoundStepNewRound)
	cs.Validators = validators
	if round == 0 {
		// We've already reset these upon new height,
		// and meanwhile we might have received a proposal
		// for round 0.
	} else {
		log.Info("Resetting Proposal info")
		cs.Proposal = nil
		cs.ProposalBlock = nil
		cs.ProposalBlockParts = nil
	}
	cs.Votes.SetRound(round + 1) // also track next round (round+1) to allow round-skipping

	cs.eventBus.PublishEventNewRound(cs.RoundStateEvent())
	// cs.metrics.Rounds.Set(float64(round))
	cs.tryEnterProposal(height, round, 1)
}

// Enter (CreateEmptyBlocks): from enterNewRound(height,round)
// Enter (CreateEmptyBlocks, CreateEmptyBlocksInterval > 0 ): after enterNewRound(height,round), after timeout of CreateEmptyBlocksInterval
// Enter (!CreateEmptyBlocks) : after enterNewRound(height,round), once txs are in the mempool
func (cs *ConsensusState) tryEnterProposal(height uint64, round int, wait uint) {
	if cs.Height != height || round < int(cs.Round) || (int(cs.Round) == round && ttypes.RoundStepPropose <= cs.Step) {
		log.Debug(fmt.Sprintf("tryenterPropose(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
		return
	}
	log.Info(fmt.Sprintf("tryenterPropose(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
	doing := true
	var estr string

	if cs.privValidator == nil {
		estr = fmt.Sprint("This node is not a validator")
		doing = false
	} else {
		if !cs.Validators.HasAddress(cs.privValidator.GetAddress()) {
			estr = fmt.Sprint(estr, " This node is not a validator", "addr", common.ToHex(cs.privValidator.GetAddress()), "vals", cs.Validators)
			doing = false
			log.Error(estr)
		} else if !cs.isProposer() {
			estr = fmt.Sprint(estr, "Not our turn to propose ", "proposer", common.ToHex(cs.Validators.GetProposer().Address), "privValidator", cs.privValidator)
			doing = false
			log.Info(estr)
		}
	}
	var block *types.Block
	var blockParts *ttypes.PartSet
	var err error

	if doing {
		// get block
		block, blockParts, err = cs.createProposalBlock()
		if err != nil || block == nil {
			log.Error("createProposalBlock", "height:", height, "round:", round, "makeblock:", err)
			doing = false
		} else if block != nil {
			if height != block.NumberU64() {
				log.Error("State Wrong,height not match", "cs.Height", height, "block.height", block.NumberU64())
				doing = false
			}
		}
	}
	if !doing {
		cs.scheduleTimeout(cs.config.Propose(int(round)), height, round, ttypes.RoundStepPropose)
		cs.updateRoundStep(round, ttypes.RoundStepPropose)
		cs.newStep()
		if cs.isProposalComplete() {
			cs.enterPrevote(height, int(cs.Round))
		}
		return
	}

	// Wait for txs to be available in the txpool and we tryenterPropose in round 0.
	empty := len(block.Transactions()) == 0
	if empty && cs.config.CreateEmptyBlocks && round == 0 && cs.config.WaitForEmptyBlocks(int(wait)) && !block.IsAward() {
		dd := cs.config.EmptyBlocksIntervalForPer(int(wait))
		wait++
		cs.scheduleTimeoutWithWait(timeoutInfo{dd, height, uint(round), ttypes.RoundStepNewRound, wait})
	} else {
		cs.enterPropose(height, round, block, blockParts)
	}
}

func (cs *ConsensusState) enterPropose(height uint64, round int, blk *types.Block, bparts *ttypes.PartSet) {
	//logger := log.With("height", height, "round", round)
	if cs.Height != height || round < int(cs.Round) || (int(cs.Round) == round && ttypes.RoundStepPropose <= cs.Step) {
		log.Debug(fmt.Sprintf("enterPropose(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
		return
	}
	log.Info(fmt.Sprintf("enterPropose(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	defer func() {
		// Done enterPropose:
		cs.updateRoundStep(round, ttypes.RoundStepPropose)
		cs.newStep()

		// If we have the whole proposal + POL, then goto Prevote now.
		// else, we'll enterPrevote when the rest of the proposal is received (in AddProposalBlockPart),
		// or else after timeoutPropose
		if cs.isProposalComplete() {
			cs.enterPrevote(height, int(cs.Round))
		}
	}()

	// If we don't get the proposal and all block parts quick enough, enterPrevote
	cs.scheduleTimeout(cs.config.Propose(int(round)), height, round, ttypes.RoundStepPropose)
	log.Debug("This node is a validator")

	if cs.isProposer() {
		log.Info("enterPropose: Our turn to propose", "proposer", cs.Validators.GetProposer().Address, "privValidator", cs.privValidator)
		cs.decideProposal(height, round, blk, bparts)
	}
}

func (cs *ConsensusState) isProposer() bool {
	return bytes.Equal(cs.Validators.GetProposer().Address, cs.privValidator.GetAddress())
}

func (cs *ConsensusState) defaultDecideProposal(height uint64, round int, blk *types.Block, bparts *ttypes.PartSet) {
	var block = blk
	var blockParts = bparts

	// Decide on block
	if cs.LockedBlock != nil {
		// If we're locked onto a block, just choose that.
		block, blockParts = cs.LockedBlock, cs.LockedBlockParts
	} else if cs.ValidBlock != nil {
		// If there is valid block, choose that.
		block, blockParts = cs.ValidBlock, cs.ValidBlockParts
	} else {
		if block == nil || blockParts == nil { // on error
			log.Info("proposal(block or blockParts is nil)", "height:", height, "round:", round)
			return
		}
	}

	// Make proposal
	polRound, polBlockID := cs.Votes.POLInfo()
	proposal := ttypes.NewProposal(height, round, blockParts.Header(), uint(polRound), polBlockID)
	if err := cs.privValidator.SignProposal(cs.state.GetChainID(), proposal); err == nil {
		// Set fields
		/*  fields set by setProposal and addBlockPart
		cs.Proposal = proposal
		cs.ProposalBlock = block
		cs.ProposalBlockParts = blockParts
		*/

		// send proposal and block parts on internal msg queue
		cs.sendInternalMessage(msgInfo{&ProposalMessage{proposal}, ""})
		for i := 0; i < int(blockParts.Total()); i++ {
			part := blockParts.GetPart(uint(i))
			cs.sendInternalMessage(msgInfo{&BlockPartMessage{cs.Height, cs.Round, part}, ""})
		}
		log.Info("Signed proposal", "height", height, "round", round, "proposal", proposal)
		log.Debug(fmt.Sprintf("Signed proposal block: %v", block))
	} else {
		log.Error("enterPropose: Error signing proposal", "height", height, "round", round, "err", err)
	}
}

// Returns true if the proposal block is complete &&
// (if POLRound was proposed, we have +2/3 prevotes from there).
func (cs *ConsensusState) isProposalComplete() bool {
	if cs.Proposal == nil || cs.ProposalBlock == nil {
		return false
	}
	// we have the proposal. if there's a POLRound,
	// make sure we have the prevotes from it too
	if int(cs.Proposal.POLRound) < 0 {
		return true
	}
	// if this is false the proposer is lying or we haven't received the POL yet
	return cs.Votes.Prevotes(int(cs.Proposal.POLRound)).HasTwoThirdsMajority()

}

// Create the next block to propose and return it.
// We really only need to return the parts, but the block
// is returned for convenience so we can log the proposal block.
// Returns nil block upon error.
// NOTE: keep it side-effect free for clarity.
func (cs *ConsensusState) createProposalBlock() (*types.Block, *ttypes.PartSet, error) {
	// remove commit in block
	var v *ttypes.SwitchValidator
	if len(cs.svs) == 0 {
		cs.moveAndReduceDoorCount() // move to svs from svsBlockDoor,when DoorCount==0 and svs is empty
	}
	if len(cs.svs) > 0 {
		v = cs.svs[0]
		cs.svs = append(cs.svs[:0], cs.svs[1:]...)
		v.DoorCount = ttypes.BlackDoorCount - 1
		cs.svsBlackDoor = append(cs.svsBlackDoor, v)
		log.Info("Make Proposal and move item", "item", v)
	}

	block, err := cs.state.MakeBlock(v)
	if block != nil && err == nil {
		parts, err2 := cs.state.MakePartSet(ttypes.BlockPartSizeBytes, block)
		return block, parts, err2
	}
	return block, nil, err
}
func (cs *ConsensusState) moveAndReduceDoorCount() {
	for _, val := range cs.svsBlackDoor {
		if val.DoorCount > 0 {
			val.DoorCount--
		}
		if val.DoorCount == 0 {
			cs.svs = append(cs.svs, val)
		}
	}
}
func movetail(s []*ttypes.SwitchValidator) {
	if len(s) > 0 {
		tmp := s[0]
		s = append(append(s[:0], s[1:]...), tmp)
	}
}

// Enter: `timeoutPropose` after entering Propose.
// Enter: proposal block and POL is ready.
// Enter: any +2/3 prevotes for future round.
// Prevote for LockedBlock if we're locked, or ProposalBlock if valid.
// Otherwise vote nil.
func (cs *ConsensusState) enterPrevote(height uint64, round int) {
	if cs.Height != height || round < int(cs.Round) || (int(cs.Round) == round && ttypes.RoundStepPrevote <= cs.Step) {
		log.Debug(fmt.Sprintf("enterPrevote(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
		return
	}

	defer func() {
		// Done enterPrevote:
		cs.updateRoundStep(round, ttypes.RoundStepPrevote)
		cs.newStep()
	}()

	// fire event for how we got here
	if cs.isProposalComplete() {
		cs.eventBus.PublishEventCompleteProposal(cs.RoundStateEvent())
	} else {
		// we received +2/3 prevotes for a future round
		// TODO: catchup event?
	}

	log.Info(fmt.Sprintf("enterPrevote(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	// Sign and broadcast vote as necessary
	cs.doPrevote(height, round)

	// Once `addVote` hits any +2/3 prevotes, we will go to PrevoteWait
	// (so we have more time to try and collect +2/3 prevotes for a single block)
}

func (cs *ConsensusState) defaultDoPrevote(height uint64, round int) {
	//logger := log.With("height", height, "round", round)
	// If a block is locked, prevote that.
	if cs.LockedBlock != nil {
		log.Info("enterPrevote: Block was locked")
		ksign, err := cs.validateBlock(cs.LockedBlock)
		tmp := cs.LockedBlock.Hash()
		if ksign == nil {
			// ProposalBlock is invalid, prevote nil.
			log.Error("enterPrevote: LockedBlock is invalid", "err", err)
			cs.signAddVote(ttypes.VoteTypePrevote, nil, ttypes.PartSetHeader{}, nil)
			return
		}
		cs.signAddVote(ttypes.VoteTypePrevote, tmp[:], cs.LockedBlockParts.Header(), ksign)
		return
	}

	// If ProposalBlock is nil, prevote nil.
	if cs.ProposalBlock == nil {
		log.Info("enterPrevote: ProposalBlock is nil")
		cs.signAddVote(ttypes.VoteTypePrevote, nil, ttypes.PartSetHeader{}, nil)
		return
	}

	// Validate proposal block
	ksign, err := cs.validateBlock(cs.ProposalBlock)
	if ksign == nil {
		// ProposalBlock is invalid, prevote nil.
		log.Error("enterPrevote: ProposalBlock is invalid", "err", err)
		cs.signAddVote(ttypes.VoteTypePrevote, nil, ttypes.PartSetHeader{}, nil)
		return
	}

	// Prevote cs.ProposalBlock
	// NOTE: the proposal signature is validated when it is received,
	// and the proposal block parts are validated as they are received (against the merkle hash in the proposal)
	log.Info("enterPrevote: ProposalBlock is valid")
	tmp := cs.ProposalBlock.Hash()
	cs.signAddVote(ttypes.VoteTypePrevote, tmp[:], cs.ProposalBlockParts.Header(), ksign)
}

// Enter: any +2/3 prevotes at next round.
func (cs *ConsensusState) enterPrevoteWait(height uint64, round int) {
	//logger := log.With("height", height, "round", round)

	if cs.Height != height || round < int(cs.Round) || (int(cs.Round) == round && ttypes.RoundStepPrevoteWait <= cs.Step) {
		log.Debug(fmt.Sprintf("enterPrevoteWait(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
		return
	}
	if !cs.Votes.Prevotes(round).HasTwoThirdsAny() {
		help.PanicSanity(fmt.Sprintf("enterPrevoteWait(%v/%v), but Prevotes does not have any +2/3 votes", height, round))
	}
	log.Info(fmt.Sprintf("enterPrevoteWait(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	defer func() {
		// Done enterPrevoteWait:
		cs.updateRoundStep(round, ttypes.RoundStepPrevoteWait)
		cs.newStep()
	}()

	// Wait for some more prevotes; enterPrecommit
	cs.scheduleTimeout(cs.config.Prevote(int(round)), height, round, ttypes.RoundStepPrevoteWait)
}

// Enter: `timeoutPrevote` after any +2/3 prevotes.
// Enter: +2/3 precomits for block or nil.
// Enter: any +2/3 precommits for next round.
// Lock & precommit the ProposalBlock if we have enough prevotes for it (a POL in this round)
// else, unlock an existing lock and precommit nil if +2/3 of prevotes were nil,
// else, precommit nil otherwise.
func (cs *ConsensusState) enterPrecommit(height uint64, round int) {
	//logger := log.With("height", height, "round", round)

	if cs.Height != height || round < int(cs.Round) || (int(cs.Round) == round && ttypes.RoundStepPrecommit <= cs.Step) {
		log.Debug(fmt.Sprintf("enterPrecommit(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
		return
	}

	log.Info(fmt.Sprintf("enterPrecommit(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	defer func() {
		// Done enterPrecommit:
		cs.updateRoundStep(round, ttypes.RoundStepPrecommit)
		cs.newStep()
	}()

	// check for a polka
	blockID, ok := cs.Votes.Prevotes(round).TwoThirdsMajority()

	// If we don't have a polka, we must precommit nil.
	if !ok {
		if cs.LockedBlock != nil {
			log.Info("enterPrecommit: No +2/3 prevotes during enterPrecommit while we're locked. Precommitting nil")
		} else {
			log.Info("enterPrecommit: No +2/3 prevotes during enterPrecommit. Precommitting nil.")
		}
		cs.signAddVote(ttypes.VoteTypePrecommit, nil, ttypes.PartSetHeader{}, nil)
		return
	}

	// At this point +2/3 prevoted for a particular block or nil.
	cs.eventBus.PublishEventPolka(cs.RoundStateEvent())

	// the latest POLRound should be this round.
	polRound, _ := cs.Votes.POLInfo()
	if polRound < int(round) {
		help.PanicSanity(fmt.Sprintf("This POLRound should be %v but got %v", round, polRound))
	}

	// +2/3 prevoted nil. Unlock and precommit nil.
	if len(blockID.Hash) == 0 {
		if cs.LockedBlock == nil {
			log.Info("enterPrecommit: +2/3 prevoted for nil.")
		} else {
			log.Info("enterPrecommit: +2/3 prevoted for nil. Unlocking")
			cs.LockedRound, cs.LockedBlock = 0, nil
			cs.LockedBlockParts, cs.proposalForCatchup = nil, nil
			cs.eventBus.PublishEventUnlock(cs.RoundStateEvent())
		}
		cs.signAddVote(ttypes.VoteTypePrecommit, nil, ttypes.PartSetHeader{}, nil)
		return
	}

	// At this point, +2/3 prevoted for a particular block.

	// If we're already locked on that block, precommit it, and update the LockedRound
	if (cs.LockedBlock != nil) && func() bool {
		hash := cs.LockedBlock.Hash()
		return help.EqualHashes(hash[:], blockID.Hash)
	}() {
		log.Info("enterPrecommit: +2/3 prevoted locked block. Relocking")
		cs.LockedRound = uint(round)
		cs.eventBus.PublishEventRelock(cs.RoundStateEvent())
		ksign, err := cs.validateBlock(cs.LockedBlock)
		if err != nil {
			log.Info("ValidateBlock faild will vote VoteAgreeAgainst", "hash", common.ToHex(blockID.Hash), "err", err)
		}
		if ksign == nil {
			cs.signAddVote(ttypes.VoteTypePrecommit, nil, ttypes.PartSetHeader{}, nil)
		} else {
			cs.signAddVote(ttypes.VoteTypePrecommit, blockID.Hash, blockID.PartsHeader, ksign)
		}
		return
	}

	// If +2/3 prevoted for proposal block, stage and precommit it

	if cs.ProposalBlock != nil && func() bool {
		tmpPro := cs.ProposalBlock.Hash()
		return help.EqualHashes(tmpPro[:], blockID.Hash)
	}() {
		log.Info("enterPrecommit: +2/3 prevoted proposal block. Locking", "hash", common.ToHex(blockID.Hash))
		// Validate the block.
		ksign, err := cs.validateBlock(cs.ProposalBlock)
		if err != nil {
			log.Info("ValidateBlock faild will vote VoteAgreeAgainst", "hash", common.ToHex(blockID.Hash), "err", err)
		}
		if ksign != nil {
			if ksign.Result == types.VoteAgree {
				cs.LockedRound, cs.LockedBlock = uint(round), cs.ProposalBlock
				cs.LockedBlockParts, cs.proposalForCatchup = cs.ProposalBlockParts, cs.Proposal
			}
			cs.eventBus.PublishEventLock(cs.RoundStateEvent())
			cs.signAddVote(ttypes.VoteTypePrecommit, blockID.Hash, blockID.PartsHeader, ksign)
		} else {
			cs.signAddVote(ttypes.VoteTypePrecommit, nil, ttypes.PartSetHeader{}, nil)
		}
		return
	}

	// There was a polka in this round for a block we don't have.
	// Fetch that block, unlock, and precommit nil.
	// The +2/3 prevotes for this round is the POL for our unlock.
	// TODO: In the future save the POL prevotes for justification.
	cs.LockedRound, cs.LockedBlock = 0, nil
	cs.LockedBlockParts, cs.proposalForCatchup = nil, nil
	if !cs.ProposalBlockParts.HasHeader(blockID.PartsHeader) {
		cs.ProposalBlock = nil
		cs.ProposalBlockParts = ttypes.NewPartSetFromHeader(blockID.PartsHeader)
	}
	cs.eventBus.PublishEventUnlock(cs.RoundStateEvent())
	cs.signAddVote(ttypes.VoteTypePrecommit, nil, ttypes.PartSetHeader{}, nil)
}

// Enter: any +2/3 precommits for next round.
func (cs *ConsensusState) enterPrecommitWait(height uint64, round int) {
	//logger := log.With("height", height, "round", round)

	if cs.Height != height || round < int(cs.Round) || (int(cs.Round) == round && ttypes.RoundStepPrecommitWait <= cs.Step) {
		log.Debug(fmt.Sprintf("enterPrecommitWait(%v/%v): Invalid args. Current step: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))
		return
	}
	if !cs.Votes.Precommits(round).HasTwoThirdsAny() {
		help.PanicSanity(fmt.Sprintf("enterPrecommitWait(%v/%v), but Precommits does not have any +2/3 votes", height, round))
	}
	log.Info(fmt.Sprintf("enterPrecommitWait(%v/%v). Current: %v/%v/%v", height, round, cs.Height, cs.Round, cs.Step))

	defer func() {
		// Done enterPrecommitWait:
		cs.updateRoundStep(round, ttypes.RoundStepPrecommitWait)
		cs.newStep()
	}()

	// Wait for some more precommits; enterNewRound
	cs.scheduleTimeout(cs.config.Precommit(int(round)), height, round, ttypes.RoundStepPrecommitWait)

}

// Enter: +2/3 precommits for block
func (cs *ConsensusState) enterCommit(height uint64, commitRound int) {
	//logger := log.With("height", height, "commitRound", commitRound)

	if cs.Height != height || ttypes.RoundStepCommit <= cs.Step {
		log.Debug(fmt.Sprintf("enterCommit(%v/%v): Invalid args. Current step: %v/%v/%v", height, commitRound, cs.Height, cs.Round, cs.Step))
		return
	}
	log.Info(fmt.Sprintf("enterCommit(%v/%v). Current: %v/%v/%v", height, commitRound, cs.Height, cs.Round, cs.Step))

	defer func() {
		// Done enterCommit:
		// keep cs.Round the same, commitRound points to the right Precommits set.
		cs.updateRoundStep(int(cs.Round), ttypes.RoundStepCommit)
		cs.CommitRound = uint(commitRound)
		cs.CommitTime = time.Now()
		cs.newStep()

		// Maybe finalize immediately.
		cs.tryFinalizeCommit(height)
	}()

	blockID, ok := cs.Votes.Precommits(commitRound).TwoThirdsMajority()
	if !ok {
		help.PanicSanity("RunActionCommit() expects +2/3 precommits")
	}

	// The Locked* fields no longer matter.
	// Move them over to ProposalBlock if they match the commit hash,
	// otherwise they'll be cleared in updateToState
	if cs.LockedBlock != nil {
		lock := cs.LockedBlock.Hash()
		if help.EqualHashes(lock[:], blockID.Hash) {
			log.Info("Commit is for locked block. Set ProposalBlock=LockedBlock", "blockHash", common.ToHex(blockID.Hash))
			cs.ProposalBlock = cs.LockedBlock
			cs.ProposalBlockParts = cs.LockedBlockParts
		}
	}
	// If we don't have the block being committed, set up to get it.
	if cs.ProposalBlock != nil {
		pro := cs.ProposalBlock.Hash()
		if !help.EqualHashes(pro[:], blockID.Hash) {
			if !cs.ProposalBlockParts.HasHeader(blockID.PartsHeader) {
				hash := cs.ProposalBlock.Hash()
				log.Info("Commit is for a block we don't know about. Set ProposalBlock=nil", "proposal", common.ToHex(hash[:]), "commit", common.ToHex(blockID.Hash))
				// We're getting the wrong block.
				// Set up ProposalBlockParts and keep waiting.
				cs.ProposalBlock = nil
				cs.ProposalBlockParts = ttypes.NewPartSetFromHeader(blockID.PartsHeader)
			} else {
				// We just need to keep waiting.
			}
		}
	} else {
		log.Info("Attempt enterCommit failed. There was ProposalBlock, that for <nil>.")
	}
}

// If we have the block AND +2/3 commits for it, finalize.
func (cs *ConsensusState) tryFinalizeCommit(height uint64) {
	//logger := log.With("height", height)

	if cs.Height != height {
		help.PanicSanity(fmt.Sprintf("tryFinalizeCommit() cs.Height: %v vs height: %v", cs.Height, height))
	}

	blockID, ok := cs.Votes.Precommits(int(cs.CommitRound)).TwoThirdsMajority()
	if !ok || len(blockID.Hash) == 0 {
		log.Error("Attempt to finalize failed. There was no +2/3 majority, or +2/3 was for <nil>.")
		return
	}
	if cs.ProposalBlock == nil {
		log.Error("Attempt to finalize failed. There was ProposalBlock, that for <nil>.")
		return
	}
	block := cs.ProposalBlock.Hash()
	if !help.EqualHashes(block[:], blockID.Hash) {
		// TODO: this happens every time if we're not a validator (ugly logs)
		// TODO: ^^ wait, why does it matter that we're a validator?
		log.Info("Attempt to finalize failed. We don't have the commit block.", "proposal-block", cs.ProposalBlock.Hash(), "commit-block", blockID.Hash)
		return
	}

	//	go
	cs.finalizeCommit(height)
}

// Increment height and goto ttypes.RoundStepNewHeight
func (cs *ConsensusState) finalizeCommit(height uint64) {
	if cs.Height != height || cs.Step != ttypes.RoundStepCommit {
		log.Debug(fmt.Sprintf("finalizeCommit(%v): Invalid args. Current step: %v/%v/%v", height, cs.Height, cs.Round, cs.Step))
		return
	}
	voteset := cs.Votes.Precommits(int(cs.CommitRound))
	blockID, ok := voteset.TwoThirdsMajority()
	block, blockParts := cs.ProposalBlock, cs.ProposalBlockParts
	hash := block.Hash()
	signs, ierr := voteset.MakePbftSigns(hash[:])

	if !ok {
		help.PanicSanity(fmt.Sprintf("Cannot finalizeCommit, commit does not have two thirds majority"))
	}
	if !blockParts.HasHeader(blockID.PartsHeader) {
		help.PanicSanity(fmt.Sprintf("Expected ProposalBlockParts header to be commit header"))
	}
	if ierr != nil || signs == nil {
		help.PanicSanity(fmt.Sprintf("Cannot finalizeCommit, make signs error=%s", ierr.Error()))
	}

	if !help.EqualHashes(hash[:], blockID.Hash) {
		help.PanicSanity(fmt.Sprintf("Cannot finalizeCommit, ProposalBlock does not hash to commit hash"))
	}
	log.Info(fmt.Sprint("Finalizing commit of block,height:", block.NumberU64(), "hash:", common.ToHex(hash[:])))
	// fail.Fail() // XXX

	// Execute and commit the block, update and save the state, and update the mempool.
	// NOTE The block.AppHash wont reflect these txs until the next block.
	var err error
	block.SetSign(signs)
	err = cs.state.ConsensusCommit(block)
	if err != nil {
		log.Error("Error on ApplyBlock. Did the application crash? Please restart getrue", "err", err)
		return
	}
	// Save to blockStore.
	if cs.blockStore.MaxBlockHeight() < block.NumberU64() {
		// NOTE: the seenCommit is local justification to commit this block,
		// but may differ from the LastCommit included in the next block
		precommits := cs.Votes.Precommits(int(cs.CommitRound))
		seenCommit := precommits.MakeCommit()
		proposal := cs.Proposal
		if proposal == nil {
			proposal = cs.proposalForCatchup
		}
		cs.blockStore.SaveBlock(block, blockParts, seenCommit, cs.Proposal)
	} else {
		// Happens during replay if we already saved the block but didn't commit
		log.Info("Calling finalizeCommit on already stored block", "height", block.NumberU64())
	}

	cs.swithResult(block)
	// NewHeightStep!
	cs.updateToState(cs.state)

	// fail.Fail() // XXX

	// cs.StartTime is already set.
	// Schedule Round0 to start soon.
	cs.scheduleRound0(&cs.RoundState)

	// By here,
	// * cs.Height has been increment to height+1
	// * cs.Step is now ttypes.RoundStepNewHeight
	// * cs.StartTime is set to when we will start round0.
}

//-----------------------------------------------------------------------------

func (cs *ConsensusState) defaultSetProposal(proposal *ttypes.Proposal) error {
	// Already have one
	// TODO: possibly catch double proposals
	if cs.Proposal != nil || proposal == nil {
		return nil
	}

	// Does not apply
	if proposal.Height != cs.Height || proposal.Round != cs.Round {
		return nil
	}

	// We don't care about the proposal if we're already in ttypes.RoundStepCommit.
	if ttypes.RoundStepCommit <= cs.Step {
		return nil
	}

	// Verify POLRound, which must be -1 or between 0 and proposal.Round exclusive.
	if int(proposal.POLRound) != -1 &&
		(int(proposal.POLRound) < 0 || proposal.Round <= proposal.POLRound) {
		return ErrInvalidProposalPOLRound
	}

	// Verify signature
	if !cs.Validators.GetProposer().PubKey.VerifyBytes(proposal.SignBytes(cs.state.GetChainID()),
		proposal.Signature) {
		return ErrInvalidProposalSignature
	}

	cs.Proposal = proposal
	cs.ProposalBlockParts = ttypes.NewPartSetFromHeader(proposal.BlockPartsHeader)
	log.Info("Received proposal", "proposal", proposal)
	return nil
}

// NOTE: block is not necessarily valid.
// Asynchronously triggers either enterPrevote (before we timeout of propose) or tryFinalizeCommit, once we have the full block.
func (cs *ConsensusState) addProposalBlockPart(msg *BlockPartMessage, peerID string) (added bool, err error) {
	height, round, part := msg.Height, msg.Round, msg.Part

	// Blocks might be reused, so round mismatch is OK
	if cs.Height != height {
		log.Debug("Received block part from wrong height", "height", height, "round", round)
		return false, nil
	}

	// We're not expecting a block part.
	if cs.ProposalBlockParts == nil {
		// NOTE: this can happen when we've gone to a higher round and
		// then receive parts from the previous round - not necessarily a bad peer.
		log.Info("Received a block part when we're not expecting any",
			"height", height, "round", round, "index", part.Index, "peer", peerID)
		return false, nil
	}

	added, err = cs.ProposalBlockParts.AddPart(part)
	if err != nil {
		return added, err
	}
	if added && cs.ProposalBlockParts.IsComplete() {
		// Added and completed!
		cs.ProposalBlock, err = ttypes.MakeBlockFromPartSet(cs.ProposalBlockParts)
		if err != nil {
			return true, err
		}
		// NOTE: it's possible to receive complete proposal blocks for future rounds without having the proposal
		log.Info("Received complete proposal block", "height", cs.ProposalBlock.NumberU64(), "hash", cs.ProposalBlock.Hash())

		// Update Valid* if we can.
		prevotes := cs.Votes.Prevotes(int(cs.Round))
		blockID, hasTwoThirds := prevotes.TwoThirdsMajority()
		if hasTwoThirds && !blockID.IsZero() && (cs.ValidRound < cs.Round) {
			pro := cs.ProposalBlock.Hash()
			if help.EqualHashes(pro[:], blockID.Hash) {
				log.Info("Updating valid block to new proposal block",
					"valid-round", cs.Round, "valid-block-hash", cs.ProposalBlock.Hash())
				cs.ValidRound = cs.Round
				cs.ValidBlock = cs.ProposalBlock
				cs.ValidBlockParts = cs.ProposalBlockParts
			}
			// TODO: In case there is +2/3 majority in Prevotes set for some
			// block and cs.ProposalBlock contains different block, either
			// proposer is faulty or voting power of faulty processes is more
			// than 1/3. We should trigger in the future accountability
			// procedure at this point.
		}

		if cs.Step <= ttypes.RoundStepPropose && cs.isProposalComplete() {
			// Move onto the next step
			cs.enterPrevote(height, int(cs.Round))
		} else if cs.Step == ttypes.RoundStepCommit {
			// If we're waiting on the proposal block...
			cs.tryFinalizeCommit(height)
		}
		return true, nil
	}
	return added, nil
}

// Attempt to add the vote. if its a duplicate signature, dupeout the validator
func (cs *ConsensusState) tryAddVote(vote *ttypes.Vote, peerID string) error {
	_, err := cs.addVote(vote, peerID)
	if err != nil {
		// If the vote height is off, we'll just ignore it,
		// But if it's a conflicting sig, add it to the cs.evpool.
		// If it's otherwise invalid, punish peer.
		if err == ErrVoteHeightMismatch {
			return err
		}
		if err == ttypes.ErrVoteConflictingVotes {
			if bytes.Equal(vote.ValidatorAddress, cs.privValidator.GetAddress()) {
				log.Error("Found conflicting vote from ourselves. Did you unsafe_reset a validator?", "height", vote.Height, "round", vote.Round, "type", vote.Type)
				return err
			}
			log.Error("Found conflicting vote.", "height", vote.Height, "round", vote.Round, "type", vote.Type)
			return err
		}
		// Probably an invalid signature / Bad peer.
		// Seems this can also err sometimes with "Unexpected step" - perhaps not from a bad peer ?
		log.Error("Error attempting to add vote", "err", err, "height", vote.Height, "round", vote.Round, "type", vote.Type, "peerid", peerID, "address", vote.ValidatorAddress)
		return ErrAddingVote

	}

	return nil
}

//-----------------------------------------------------------------------------

func (cs *ConsensusState) addVote(vote *ttypes.Vote, peerID string) (added bool, err error) {
	log.Debug("addVote", "voteHeight", vote.Height, "voteType", vote.Type, "valIndex", vote.ValidatorIndex, "csHeight", cs.Height)

	// A precommit for the previous height?
	// These come in while we wait timeoutCommit
	if vote.Height+1 == cs.Height {
		if !(cs.Step == ttypes.RoundStepNewHeight && vote.Type == ttypes.VoteTypePrecommit) {
			// TODO: give the reason ..
			// fmt.Errorf("tryAddVote: Wrong height, not a LastCommit straggler commit.")
			return added, ErrVoteHeightMismatch
		}
		if cs.LastCommit != nil {
			added, err = cs.LastCommit.AddVote(vote)
			if !added {
				return added, err
			}
			log.Info(fmt.Sprintf("Added to lastPrecommits: %v", cs.LastCommit.StringShort()))
		}
		cs.eventBus.PublishEventVote(ttypes.EventDataVote{vote})
		cs.evsw.FireEvent(ttypes.EventVote, vote)

		// if we can skip timeoutCommit and have all the votes now,
		if cs.config.SkipTimeoutCommit && (cs.LastCommit != nil && cs.LastCommit.HasAll()) {
			// go straight to new round (skip timeout commit)
			// cs.scheduleTimeout(time.Duration(0), cs.Height, 0, ttypes.RoundStepNewHeight)
			cs.enterNewRound(cs.Height, 0)
		}
		return
	}

	// Height mismatch is ignored.
	// Not necessarily a bad peer, but not favourable behaviour.
	if vote.Height != cs.Height {
		err = ErrVoteHeightMismatch
		log.Info("Vote ignored and not added", "voteHeight", vote.Height, "csHeight", cs.Height, "err", err)
		return
	}

	height := cs.Height
	added, err = cs.Votes.AddVote(vote, peerID)
	if !added {
		// Either duplicate, or error upon cs.Votes.AddByIndex()
		return
	}

	cs.eventBus.PublishEventVote(ttypes.EventDataVote{vote})
	cs.evsw.FireEvent(ttypes.EventVote, vote)

	switch vote.Type {
	case ttypes.VoteTypePrevote:
		prevotes := cs.Votes.Prevotes(int(vote.Round))
		log.Info("Added to prevote", "vote", vote, "prevotes", prevotes.StringShort())

		// If +2/3 prevotes for a block or nil for *any* round:
		if blockID, ok := prevotes.TwoThirdsMajority(); ok {

			// There was a polka!
			// If we're locked but this is a recent polka, unlock.
			// If it matches our ProposalBlock, update the ValidBlock

			// Unlock if `cs.LockedRound < vote.Round <= cs.Round`
			// NOTE: If vote.Round > cs.Round, we'll deal with it when we get to vote.Round
			if (cs.LockedBlock != nil) &&
				(cs.LockedRound < vote.Round) &&
				(vote.Round <= cs.Round) &&
				!func() bool {
					hash := cs.LockedBlock.Hash()
					return help.EqualHashes(hash[:], blockID.Hash)
				}() {

				log.Info("Unlocking because of POL.", "lockedRound", cs.LockedRound, "POLRound", vote.Round)
				cs.LockedRound = 0
				cs.LockedBlock = nil
				cs.LockedBlockParts = nil
				cs.proposalForCatchup = nil
				cs.eventBus.PublishEventUnlock(cs.RoundStateEvent())
			}

			// Update Valid* if we can.
			// NOTE: our proposal block may be nil or not what received a polka..
			// TODO: we may want to still update the ValidBlock and obtain it via gossipping
			if !blockID.IsZero() &&
				(cs.ValidRound < vote.Round) &&
				(vote.Round <= cs.Round) &&
				func() bool {
					if cs.ProposalBlock == nil {
						return false
					}
					hash := cs.ProposalBlock.Hash()
					return help.EqualHashes(hash[:], blockID.Hash)
				}() {

				log.Info("Updating ValidBlock because of POL.", "validRound", cs.ValidRound, "POLRound", vote.Round)
				cs.ValidRound = vote.Round
				cs.ValidBlock = cs.ProposalBlock
				cs.ValidBlockParts = cs.ProposalBlockParts
			}
		}

		// If +2/3 prevotes for *anything* for this or future round:
		if cs.Round <= vote.Round && prevotes.HasTwoThirdsAny() {
			// Round-skip over to PrevoteWait or goto Precommit.
			cs.enterNewRound(height, int(vote.Round)) // if the vote is ahead of us
			if prevotes.HasTwoThirdsMajority() {
				cs.enterPrecommit(height, int(vote.Round))
			} else {
				cs.enterPrevote(height, int(vote.Round)) // if the vote is ahead of us
				cs.enterPrevoteWait(height, int(vote.Round))
			}
		} else if cs.Proposal != nil && 0 <= int(cs.Proposal.POLRound) && cs.Proposal.POLRound == vote.Round {
			// If the proposal is now complete, enter prevote of cs.Round.
			if cs.isProposalComplete() {
				cs.enterPrevote(height, int(cs.Round))
			}
		}

	case ttypes.VoteTypePrecommit:
		precommits := cs.Votes.Precommits(int(vote.Round))
		log.Info("Added to precommit", "vote", vote, "precommits", precommits.StringShort())
		blockID, ok := precommits.TwoThirdsMajority()
		if ok {
			if len(blockID.Hash) == 0 {
				cs.enterNewRound(height, int(vote.Round)+1)
			} else {
				cs.enterNewRound(height, int(vote.Round))
				cs.enterPrecommit(height, int(vote.Round))
				cs.enterCommit(height, int(vote.Round))

				if cs.config.SkipTimeoutCommit && precommits.HasAll() {
					// if we have all the votes now,
					// go straight to new round (skip timeout commit)
					// cs.scheduleTimeout(time.Duration(0), cs.Height, 0, ttypes.RoundStepNewHeight)
					cs.enterNewRound(cs.Height, 0)
				}
			}
		} else if precommits.HasAll() {
			cs.enterNewRound(height, int(cs.Round+1))
		} else if cs.Round <= vote.Round && precommits.HasTwoThirdsAny() {
			cs.enterNewRound(height, int(vote.Round))
			cs.enterPrecommit(height, int(vote.Round))
			cs.enterPrecommitWait(height, int(vote.Round))
		}
	default:
		panic(fmt.Sprintf("Unexpected vote type %X", vote.Type)) // go-wire should prevent this.
	}

	return
}

func (cs *ConsensusState) signVote(typeB byte, hash []byte, header ttypes.PartSetHeader) (*ttypes.Vote, error) {
	addr := cs.privValidator.GetAddress()
	valIndex, _ := cs.Validators.GetByAddress(addr)
	vote := &ttypes.Vote{
		ValidatorAddress: addr,
		ValidatorIndex:   uint(valIndex),
		Height:           cs.Height,
		Round:            cs.Round,
		Timestamp:        time.Now().UTC(),

		Type:    typeB,
		Result:  types.VoteAgree,
		BlockID: ttypes.BlockID{hash, header},
	}
	err := cs.privValidator.SignVote(cs.state.GetChainID(), vote)
	return vote, err
}

// sign the vote and publish on internalMsgQueue

func (cs *ConsensusState) signAddVote(typeB byte, hash []byte, header ttypes.PartSetHeader, keepsign *ttypes.KeepBlockSign) *ttypes.Vote {
	// if we don't have a key or we're not in the validator set, do nothing
	if cs.privValidator == nil || !cs.Validators.HasAddress(cs.privValidator.GetAddress()) {
		return nil
	}

	vote, err := cs.signVote(typeB, hash, header)
	if err == nil {
		if hash != nil && keepsign == nil {
			keepsign = cs.Votes.GetSignsFromVote(int(cs.Round), hash, cs.privValidator.GetAddress())
			// if prevote := cs.Votes.Prevotes(int(cs.Round)); prevote != nil {
			// 	keepsign = prevote.GetSignByAddress(cs.privValidator.GetAddress())
			// }
		}
		if hash != nil && keepsign != nil && bytes.Equal(hash, keepsign.Hash[:]) {
			vote.Result = keepsign.Result
			vote.ResultSign = make([]byte, len(keepsign.Sign))
			copy(vote.ResultSign, keepsign.Sign)
		}
		cs.sendInternalMessage(msgInfo{&VoteMessage{vote}, ""})
		log.Info("Signed and pushed vote", "height", cs.Height, "round", cs.Round, "vote", vote, "err", err)
		return vote
	}
	log.Error("Error signing vote", "height", cs.Height, "round", cs.Round, "vote", vote, "err", err)
	return nil
}

//---------------------------------------------------------
func (cs *ConsensusState) hasSwitchValidator(infos *types.SwitchInfos, s []*ttypes.SwitchValidator) bool {
	if len(infos.Vals) <= 2 {
		return false
	}
	aEnter, rEnter := infos.Vals[0], infos.Vals[1]
	exist := false
	for _, v := range s {
		if len(v.Infos.Vals) > 2 {
			if (aEnter.Flag == v.Infos.Vals[0].Flag && bytes.Equal(aEnter.Pk, v.Infos.Vals[0].Pk)) &&
				(rEnter.Flag == v.Infos.Vals[1].Flag && bytes.Equal(rEnter.Pk, v.Infos.Vals[1].Pk)) {
				exist = true
				break
			}
		}
	}
	return exist
}
func (cs *ConsensusState) switchHandle(s *ttypes.SwitchValidator) {
	if s != nil {
		if s.From == 0 { // add
			exist := cs.hasSwitchValidator(s.Infos, cs.svsBlackDoor)
			if !exist {
				exist = cs.hasSwitchValidator(s.Infos, cs.svs)
			}
			if !exist {
				cs.svs = append(cs.svs, s)
			}
		} else if s.From == 1 { // remove
			cs.pickSwitchValidator(s.Infos)
		}
	}
}

func (cs *ConsensusState) swithResult(block *types.Block) {

	sw := block.SwitchInfos()
	log.Info("swithResult", "sw", sw)
	if sw == nil || len(sw.Vals) < 2 {
		return
	}
	log.Info("swithResult", "sw", sw, "vals", sw.Vals)
	// stop fetch until update committee members
	cs.state.SetEndHeight(block.NumberU64())

	aEnter, rEnter := sw.Vals[0], sw.Vals[1]
	var add, remove *ttypes.Health
	if aEnter.Flag == types.StateAppendFlag {
		add = cs.hm.GetHealth(aEnter.Pk)
		if rEnter.Flag == types.StateRemovedFlag {
			remove = cs.hm.GetHealth(rEnter.Pk)
		}
	} else if aEnter.Flag == types.StateRemovedFlag {
		remove = cs.hm.GetHealth(aEnter.Pk)
	}
	if remove == nil {
		log.Error("swithResult,remove is nil")
		return
	}

	sv := cs.pickSwitchValidator(sw)
	if sv == nil {
		sv = &ttypes.SwitchValidator{
			Infos:  sw,
			Resion: "",
			Remove: remove,
			Add:    add,
		}
	} else {
		log.Error("swithResult,not found sv", "sw", sw)
		return
	}

	// notify to healthMgr
	go func() {
		select {
		case cs.hm.ChanFrom() <- sv:
		default:
		}
	}()

	log.Info("Switch Result,SetEndHeight", "EndHight", block.NumberU64())
}

func (cs *ConsensusState) switchVerify(block *types.Block) bool {
	sw := block.SwitchInfos()
	if sw != nil {
		if len(sw.Vals) > 2 {
			add, remove := sw.Vals[0], sw.Vals[1]
			if (add.Flag == types.StateAppendFlag && remove.Flag == types.StateRemovedFlag) ||
				(add.Flag == types.StateRemovedFlag && remove.Flag == types.StateUsedFlag) {
				if add.Flag == types.StateRemovedFlag {
					remove = add
					add = nil
				}
				err := cs.hm.VerifySwitch(remove, add)
				if err == nil {
					return true
				}
				log.Info("switchVerify", "result", err)
			} else {
				log.Info("switchVerify", "Type Error,add", add, "remove", remove)
			}
		}
	}
	return false
}

func (cs *ConsensusState) validateBlock(block *types.Block) (*ttypes.KeepBlockSign, error) {
	if block == nil {
		return nil, errors.New("block is nil")
	}
	res := cs.switchVerify(block)
	if len(block.SwitchInfos().Vals) == 0 {
		res = true
	}
	log.Info("validateBlock", "res", res)
	return cs.state.ValidateBlock(block, res)
}

func (cs *ConsensusState) pickSwitchValidator(info *types.SwitchInfos) *ttypes.SwitchValidator {
	log.Info("pickSwitchValidator", "info", info.Vals)
	if info == nil || len(info.Vals) < 2 {
		return nil
	}
	aEnter, rEnter := info.Vals[0], info.Vals[1]
	for i, v := range cs.svsBlackDoor {
		if len(v.Infos.Vals) > 2 {
			if (aEnter.Flag == v.Infos.Vals[0].Flag && bytes.Equal(aEnter.Pk, v.Infos.Vals[0].Pk)) &&
				(rEnter.Flag == v.Infos.Vals[1].Flag && bytes.Equal(rEnter.Pk, v.Infos.Vals[1].Pk)) {
				cs.svsBlackDoor = append(cs.svsBlackDoor[:i], cs.svsBlackDoor[i+1:]...)
				log.Info("pickSwitchValidator", "svsLen", len(cs.svsBlackDoor), "svs", cs.svsBlackDoor)
				return v
			}
		}
	}
	for i, v := range cs.svs {
		if len(v.Infos.Vals) > 2 {
			log.Info("pickSwitchValidator", "aEnter.Flag", aEnter.Flag, "v.Infos.Vals[0].Flag", v.Infos.Vals[0].Flag,
				"aEnter.Pk", aEnter.Pk, "v.Infos.Vals[0].Pk", v.Infos.Vals[0].Pk, "rEnter.Flag", rEnter.Flag, "v.Infos.Vals[1].Flag", v.Infos.Vals[1].Flag,
				"rEnter.Pk", rEnter.Pk, "v.Infos.Vals[1].Pk", v.Infos.Vals[1].Pk)
			log.Info("pickSwitchValidator", "if", (aEnter.Flag == v.Infos.Vals[0].Flag && bytes.Equal(aEnter.Pk, v.Infos.Vals[0].Pk)) && (rEnter.Flag == v.Infos.Vals[1].Flag && bytes.Equal(rEnter.Pk, v.Infos.Vals[1].Pk)))
			if (aEnter.Flag == v.Infos.Vals[0].Flag && bytes.Equal(aEnter.Pk, v.Infos.Vals[0].Pk)) &&
				(rEnter.Flag == v.Infos.Vals[1].Flag && bytes.Equal(rEnter.Pk, v.Infos.Vals[1].Pk)) {
				cs.svs = append(cs.svs[:i], cs.svs[i+1:]...)
				log.Info("pickSwitchValidator", "svsLen", len(cs.svs), "svs", cs.svs)
				return v
			}
		}
	}
	return nil
}

// CompareHRS is compare msg'and peerSet's height round Step
func CompareHRS(h1 uint64, r1 uint, s1 ttypes.RoundStepType, h2 uint64, r2 uint, s2 ttypes.RoundStepType) int {
	if h1 < h2 {
		return -1
	} else if h1 > h2 {
		return 1
	}
	if r1 < r2 {
		return -1
	} else if r1 > r2 {
		return 1
	}
	if s1 < s2 {
		return -1
	} else if s1 > s2 {
		return 1
	}
	return 0
}

//UpdateValidatorSet committee change
func (cs *ConsensusState) UpdateValidatorSet(info *types.CommitteeInfo) (selfStop bool, remove []*types.CommitteeMember,
	validator *ttypes.ValidatorSet) {
	allMember := append(info.Members, info.BackMembers...)
	validator = cs.GetRoundState().Validators
	for _, v := range allMember {
		if v.Flag == types.StateUsedFlag {
			vTemp := ttypes.NewValidator(crypto.PubKeyTrue(*v.Publickey), 1)
			b := validator.Add(vTemp)
			log.Info("UpdateValidatorSet", "add", b)
		}
		if v.Flag == types.StateRemovedFlag {
			if cs.state.GetPubKey().Equals(crypto.PubKeyTrue(*v.Publickey)) {
				selfStop = true
			}
			remove = append(remove, v)
			v, b := validator.RemoveForPK(*v.Publickey)
			log.Info("UpdateValidatorSet", "va", v, "remove", b)
		}
	}
	return
}
