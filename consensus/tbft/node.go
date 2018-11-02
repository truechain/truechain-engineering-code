package consensus

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	cfg "github.com/truechain/truechain-engineering-code/consensus/tbft/config"
	tcrypto "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/p2p"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/p2p/pex"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/log"
	"math/big"
)

type service struct {
	sw               *p2p.Switch
	consensusState   *ConsensusState   // latest consensus state
	consensusReactor *ConsensusReactor // for participating in the consensus
}

const (
	Start int = iota
	Stop
	Switch
)

func (s *service) start(node *Node) error {
	// Create & add listener
	l := p2p.NewDefaultListener(
		node.config.P2P.ListenAddress,
		node.config.P2P.ExternalAddress,
		node.config.P2P.UPNP,
		log.New("p2p"))
	s.sw.AddListener(l)

	ni, _ := json.Marshal(node.nodeinfo)
	fmt.Printf("node.nodeInfo %v\n", string(ni))

	s.sw.SetNodeInfo(node.nodeinfo)
	s.sw.SetNodeKey(&node.nodekey)
	s.consensusState.SetPrivValidator(node.privValidator)
	// Start the switch (the P2P server).
	err := s.sw.Start()
	if err != nil {
		return err
	}
	return nil
}
func (s *service) stop() error {
	s.sw.Stop()
	return nil
}

//------------------------------------------------------------------------------

// Node is the highest level interface to a full truechain node.
// It includes all configuration information and running services.
type Node struct {
	help.BaseService
	// configt
	config *cfg.Config
	Agent  types.PbftAgentProxy
	// network
	addrBook      pex.AddrBook         // known peers
	privValidator ttypes.PrivValidator // local node's validator key

	// services
	services map[uint64]*service
	eventBus *ttypes.EventBus // pub/sub for services
	nodekey  p2p.NodeKey
	nodeinfo p2p.NodeInfo
	chainID  string
}

// NewNode returns a new, ready to go, truechain Node.
func NewNode(config *cfg.Config, chainID string, priv *ecdsa.PrivateKey,
	agent types.PbftAgentProxy) (*Node, error) {

	privValidator := ttypes.NewPrivValidator(*priv)
	// Optionally, start the pex reactor
	// We need to set Seeds and PersistentPeers on the switch,
	// since it needs to be able to use these (and their DNS names)
	// even if the PEX is off. We can include the DNS name in the NetAddress,
	// but it would still be nice to have a clear list of the current "PersistentPeers"
	// somewhere that we can return with net_info.

	// If PEX is on, it should handle dialing the seeds. Otherwise the switch does it.
	// Note we currently use the addrBook regardless at least for AddOurAddress
	addrBook := pex.NewAddrBook(config.P2P.AddrBookFile(), config.P2P.AddrBookStrict)

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
	node := &Node{
		config:        config,
		privValidator: privValidator,
		addrBook:      addrBook,
		eventBus:      eventBus,
		chainID:       chainID,
		Agent:         agent,
		services:      make(map[uint64]*service),
		nodekey: p2p.NodeKey{
			PrivKey: tcrypto.PrivKeyTrue(*priv),
		},
	}
	node.BaseService = *help.NewBaseService("Node", node)
	return node, nil
}

// OnStart starts the Node. It implements help.Service.
func (n *Node) OnStart() error {
	fmt.Println("tempprint Node On Start")
	err := n.eventBus.Start()
	if err != nil {
		return err
	}

	n.nodeinfo = n.makeNodeInfo()

	//m,_ := json.Marshal(n.nodeinfo)

	//fmt.Printf("tempprint Node Info %v\n", string(m))
	// Add ourselves to addrbook to prevent dialing ourselves
	n.addrBook.AddOurAddress(n.nodeinfo.NetAddress())

	// Add private IDs to addrbook to block those peers being added
	n.addrBook.AddPrivateIDs(help.SplitAndTrim(n.config.P2P.PrivatePeerIDs, ",", " "))

	return nil
}

// OnStop stops the Node. It implements help.Service.
func (n *Node) OnStop() {
	n.BaseService.OnStop()
	// first stop the non-reactor services
	n.eventBus.Stop()
	// now stop the reactors
	// TODO: gracefully disconnect from peers.
}

// RunForever waits for an interrupt signal and stops the node.
func (n *Node) RunForever() {
	// Sleep forever and then...
	//cmn.TrapSignal(func() {
	//	n.Stop()
	//})
}

// EventBus returns the Node's EventBus.
func (n *Node) EventBus() *ttypes.EventBus {
	return n.eventBus
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
	// Split protocol, address, and port.
	_, lAddr := help.ProtocolAndAddress(n.config.P2P.ListenAddress)
	lAddrIP, lAddrPort := p2p.SplitHostPort(lAddr)
	nodeInfo.ListenAddr = fmt.Sprintf("%v:%v", lAddrIP, lAddrPort)
	return nodeInfo
}

func (n *Node) Notify(id *big.Int, action int) error {
	switch action {
	case Start:
		if server, ok := n.services[id.Uint64()]; ok {
			server.start(n)
			return nil
		} else {
			return errors.New("wrong conmmitt ID:" + id.String())
		}

	case Stop:
		if server, ok := n.services[id.Uint64()]; ok {
			server.stop()
		}
		return nil
	case Switch:
		// begin to make network..
		return nil
	}
	return nil
}
func (n *Node) PutCommittee(committeeInfo *types.CommitteeInfo) error {
	id := committeeInfo.Id
	members := committeeInfo.Members
	if id == nil || len(members) <= 0 {
		return errors.New("wrong params...")
	}
	if _, ok := n.services[id.Uint64()]; ok {
		return errors.New("repeat ID:" + id.String())
	}
	// Make StateAgent
	lastCommitHeight := committeeInfo.StartHeight.Uint64()
	if lastCommitHeight >= 0 {
		lastCommitHeight--
	}
	state := ttypes.NewStateAgent(n.Agent, n.chainID, MakeValidators(committeeInfo), lastCommitHeight)
	if state == nil {
		return errors.New("make the nil state")
	}
	store := ttypes.NewBlockStore()
	service := &service{
		sw:             p2p.NewSwitch(n.config.P2P),
		consensusState: NewConsensusState(n.config.Consensus, state, store),
	}
	service.consensusReactor = NewConsensusReactor(service.consensusState, false)
	service.sw.AddReactor("CONSENSUS", service.consensusReactor)
	service.sw.SetAddrBook(n.addrBook)
	service.consensusReactor.SetEventBus(n.eventBus)

	n.services[id.Uint64()] = service
	return nil
}
func (n *Node) PutNodes(id *big.Int, nodes []*types.CommitteeNode) error {
	fmt.Println("enter PutNode")
	if id == nil || len(nodes) <= 0 {
		return errors.New("wrong params...")
	}
	server, ok := n.services[id.Uint64()]
	if !ok {
		return errors.New("wrong ID:" + id.String())
	}
	for _, v := range nodes {
		pub, err := crypto.UnmarshalPubkey(v.Publickey)
		if err != nil {
			panic(err)
		}
		address := crypto.PubkeyToAddress(*pub)
		id := p2p.ID(hex.EncodeToString(address[:]))
		addr, err := p2p.NewNetAddressString(p2p.IDAddressString(id,
			fmt.Sprintf("%v:%v", v.IP, v.Port)))
		if err == nil {
			errDialErr := server.sw.DialPeerWithAddress(addr, true)
			if errDialErr != nil {
				log.Error("dail peer " + errDialErr.Error())
			}
		} else {
			panic(err)
		}
	}
	return nil
}
func MakeValidators(cmm *types.CommitteeInfo) *ttypes.ValidatorSet {
	id := cmm.Id
	members := cmm.Members
	if id == nil || len(members) <= 0 {
		return nil
	}
	vals := make([]*ttypes.Validator, 0, 0)
	var power uint64 = 1
	for i, m := range members {
		if i == 0 {
			power = 1
		} else {
			power = 1
		}
		v := ttypes.NewValidator(tcrypto.PubKeyTrue(*m.Publickey), power)
		vals = append(vals, v)
	}
	return ttypes.NewValidatorSet(vals)
}
