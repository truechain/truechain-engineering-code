package consensus

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/log"
	cfg "github.com/truechain/truechain-engineering-code/consensus/tbft/config"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/p2p/pex"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/p2p"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
)

//------------------------------------------------------------------------------

// Node is the highest level interface to a full Tendermint node.
// It includes all configuration information and running services.
type Node struct {
	help.BaseService
	// config
	config        *cfg.Config
	privValidator ttypes.PrivValidator // local node's validator key

	// network
	sw       *p2p.Switch  // p2p connections
	addrBook pex.AddrBook // known peers

	// services
	eventBus         *ttypes.EventBus // pub/sub for services
	consensusState   *ConsensusState     // latest consensus state
	consensusReactor *ConsensusReactor   // for participating in the consensus
	nodekey			 p2p.NodeKey
	chainID 		 string
}

// NewNode returns a new, ready to go, Tendermint Node.
func NewNode(config *cfg.Config, chainID string) (*Node, error) {

	// Make ConsensusReactor
	var state ttypes.StateAgent
	consensusState := NewConsensusState(config.Consensus, state)
	consensusReactor := NewConsensusReactor(consensusState, false)

	sw := p2p.NewSwitch(config.P2P)
	sw.AddReactor("CONSENSUS", consensusReactor)

	// Optionally, start the pex reactor
	// We need to set Seeds and PersistentPeers on the switch,
	// since it needs to be able to use these (and their DNS names)
	// even if the PEX is off. We can include the DNS name in the NetAddress,
	// but it would still be nice to have a clear list of the current "PersistentPeers"
	// somewhere that we can return with net_info.
	
	// If PEX is on, it should handle dialing the seeds. Otherwise the switch does it.
	// Note we currently use the addrBook regardless at least for AddOurAddress
	addrBook := pex.NewAddrBook(config.P2P.AddrBookFile(), config.P2P.AddrBookStrict)
	sw.SetAddrBook(addrBook)

	// Filter peers by addr or pubkey with an ABCI query.
	// If the query return code is OK, add peer.
	// XXX: Query format subject to change
	if config.FilterPeers {
		// NOTE: addr is ip:port
		//sw.SetAddrFilter(func(addr net.Addr) error {
		//	resQuery, err := proxyApp.Query().QuerySync(abci.RequestQuery{Path: fmt.Sprintf("/p2p/filter/addr/%s", addr.String())})
		//	if err != nil {
		//		return err
		//	}
		//	if resQuery.IsErr() {
		//		return fmt.Errorf("Error querying abci app: %v", resQuery)
		//	}
		//	return nil
		//})
		//sw.SetIDFilter(func(id p2p.ID) error {
		//	resQuery, err := proxyApp.Query().QuerySync(abci.RequestQuery{Path: fmt.Sprintf("/p2p/filter/id/%s", id)})
		//	if err != nil {
		//		return err
		//	}
		//	if resQuery.IsErr() {
		//		return fmt.Errorf("Error querying abci app: %v", resQuery)
		//	}
		//	return nil
		//})
	}

	eventBus := ttypes.NewEventBus()
	// services which will be publishing and/or subscribing for messages (events)
	// consensusReactor will set it on consensusState and blockExecutor
	consensusReactor.SetEventBus(eventBus)

	node := &Node{
		config:        		config,
		privValidator: 		state,
		sw:       			sw,
		addrBook: 			addrBook,
		consensusState:   	consensusState,
		consensusReactor: 	consensusReactor,
		eventBus:         	eventBus,
		chainID:			chainID,
	}
	node.BaseService = *help.NewBaseService("Node", node)
	return node, nil
}

// OnStart starts the Node. It implements help.Service.
func (n *Node) OnStart() error {
	err := n.eventBus.Start()
	if err != nil {
		return err
	}

	// Create & add listener
	l := p2p.NewDefaultListener(
		n.config.P2P.ListenAddress,
		n.config.P2P.ExternalAddress,
		n.config.P2P.UPNP,
		log.New("p2p"))
	n.sw.AddListener(l)

	nodeInfo := n.makeNodeInfo()
	n.sw.SetNodeInfo(nodeInfo)
	n.sw.SetNodeKey(&n.nodekey)

	// Add ourselves to addrbook to prevent dialing ourselves
	n.addrBook.AddOurAddress(nodeInfo.NetAddress())

	// Add private IDs to addrbook to block those peers being added
	n.addrBook.AddPrivateIDs(help.SplitAndTrim(n.config.P2P.PrivatePeerIDs, ",", " "))

	// Start the switch (the P2P server).
	err = n.sw.Start()
	if err != nil {
		return err
	}

	// Always connect to persistent peers
	if n.config.P2P.PersistentPeers != "" {
		err = n.sw.DialPeersAsync(n.addrBook, help.SplitAndTrim(n.config.P2P.PersistentPeers, ",", " "), true)
		if err != nil {
			return err
		}
	}
	return nil
}

// OnStop stops the Node. It implements help.Service.
func (n *Node) OnStop() {
	n.BaseService.OnStop()
	// first stop the non-reactor services
	n.eventBus.Stop()
	// now stop the reactors
	// TODO: gracefully disconnect from peers.
	n.sw.Stop()
}

// RunForever waits for an interrupt signal and stops the node.
func (n *Node) RunForever() {
	// Sleep forever and then...
	//cmn.TrapSignal(func() {
	//	n.Stop()
	//})
}

// AddListener adds a listener to accept inbound peer connections.
// It should be called before starting the Node.
// The first listener is the primary listener (in NodeInfo)
func (n *Node) AddListener(l p2p.Listener) {
	n.sw.AddListener(l)
}

// Switch returns the Node's Switch.
func (n *Node) Switch() *p2p.Switch {
	return n.sw
}
// ConsensusState returns the Node's ConsensusState.
func (n *Node) ConsensusState() *ConsensusState {
	return n.consensusState
}

// ConsensusReactor returns the Node's ConsensusReactor.
func (n *Node) ConsensusReactor() *ConsensusReactor {
	return n.consensusReactor
}

// EventBus returns the Node's EventBus.
func (n *Node) EventBus() *ttypes.EventBus {
	return n.eventBus
}

// PrivValidator returns the Node's PrivValidator.
// XXX: for convenience only!
func (n *Node) PrivValidator() ttypes.PrivValidator {
	return n.privValidator
}

func (n *Node) makeNodeInfo() p2p.NodeInfo {
	nodeInfo := p2p.NodeInfo{
		ID:      n.nodekey.ID(),
		Network: n.chainID,
		Version: "0.1.0",
		Channels: []byte{
			StateChannel,
			DataChannel,
			VoteChannel, 
			VoteSetBitsChannel,
		},
		Moniker: n.config.Moniker,
		Other: []string{
			fmt.Sprintf("p2p_version=%v", "0.1.0"),
			fmt.Sprintf("consensus_version=%v", "0.1.0"),
		},
	}

	if !n.sw.IsListening() {
		return nodeInfo
	}

	p2pListener := n.sw.Listeners()[0]
	p2pHost := p2pListener.ExternalAddressHost()
	p2pPort := p2pListener.ExternalAddress().Port
	nodeInfo.ListenAddr = fmt.Sprintf("%v:%v", p2pHost, p2pPort)

	return nodeInfo
}
