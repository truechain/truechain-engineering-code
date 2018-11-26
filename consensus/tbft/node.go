package tbft

import (
	"strings"
	"sync"
	"crypto/ecdsa"
	"encoding/hex"
	// "encoding/json"
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
	sa 				 *ttypes.StateAgentImpl
	nodeTable 		 map[p2p.ID]*nodeInfo
	lock			 *sync.Mutex
	updateChan		 chan bool
	eventBus 		*ttypes.EventBus // pub/sub for services
	// network
	addrBook      	pex.AddrBook         // known peers

}

type nodeInfo struct {
	ID 			p2p.ID 
	Adrress		*p2p.NetAddress
	IP 			string 
	Port 		uint
	Enable		bool
}

const (
	Start int = iota
	Stop
	Switch
)

func NewNodeService(p2pcfg *cfg.P2PConfig,cscfg *cfg.ConsensusConfig,state ttypes.StateAgent,
	store *ttypes.BlockStore,) *service {
	return &service {
		sw:             p2p.NewSwitch(p2pcfg),
		consensusState: NewConsensusState(cscfg, state, store),
		nodeTable: 		make(map[p2p.ID]*nodeInfo),
		lock:			new(sync.Mutex),
		updateChan:		make(chan bool),
		eventBus:		ttypes.NewEventBus(),
		// If PEX is on, it should handle dialing the seeds. Otherwise the switch does it.
		// Note we currently use the addrBook regardless at least for AddOurAddress
		addrBook: 		pex.NewAddrBook(p2pcfg.AddrBookFile(), p2pcfg.AddrBookStrict),
	}
}

func (s *service) start(cid *big.Int,node *Node) error {
	err := s.eventBus.Start()
	if err != nil {
		return err
	}
	// Create & add listener
	lstr := node.config.P2P.ListenAddress2
	if cid.Uint64() % 2 == 0 {
		lstr = node.config.P2P.ListenAddress1
	}
	nodeinfo := node.nodeinfo
	_, lAddr := help.ProtocolAndAddress(lstr)
	lAddrIP, lAddrPort := p2p.SplitHostPort(lAddr)
	nodeinfo.ListenAddr = fmt.Sprintf("%v:%v", lAddrIP, lAddrPort)
	// Add ourselves to addrbook to prevent dialing ourselves
	s.addrBook.AddOurAddress(nodeinfo.NetAddress())
	// Add private IDs to addrbook to block those peers being added
	s.addrBook.AddPrivateIDs(help.SplitAndTrim(node.config.P2P.PrivatePeerIDs, ",", " "))

	s.sw.SetNodeInfo(nodeinfo)
	s.sw.SetNodeKey(&node.nodekey)
	log.Info("commitee start","node info",nodeinfo.String())
	l := p2p.NewDefaultListener(
		lstr,
		node.config.P2P.ExternalAddress,
		node.config.P2P.UPNP,
		log.New("p2p"))
	s.sw.AddListener(l)

	privValidator := ttypes.NewPrivValidator(*node.priv)
	s.consensusState.SetPrivValidator(privValidator)
	// Start the switch (the P2P server).
	err = s.sw.Start()
	if err != nil {
		return err
	}
	go func(){
		for {
			select {
			case update := <- s.updateChan :
				if update {
					s.updateNodes()	
				} else {
					return 			// exit
				}
			}
		}
	}()
	return nil
}
func (s *service) stop() error {
	if s.sw.IsRunning() {
		s.updateChan <- false
		s.eventBus.Stop()
	}
	s.sw.Stop()	
	return nil
}
func (s *service) getStateAgent() *ttypes.StateAgentImpl {
	return s.sa
}
func (s *service) putNodes(cid *big.Int,nodes []*types.CommitteeNode) {
	if nodes == nil {	return 		}
	
	s.lock.Lock()
	defer s.lock.Unlock()
	update := false
	nodeString := make([]string,len(nodes))
	for i, node := range nodes {
		nodeString[i] = node.String()
		pub, err := crypto.UnmarshalPubkey(node.Publickey)
		if err != nil {
			log.Error("putnode:",err,node.IP,node.Port)
			continue
		}
		// check node pk
		address := crypto.PubkeyToAddress(*pub)
		if ok:=s.sa.GetValidator().HasAddress(address[:]); !ok {
			log.Error("has not address:",address,node.IP,node.Port)
			continue
		}	
		port := node.Port2
		if cid.Uint64() % 2 == 0 {
			port = node.Port
		}
		id := p2p.ID(hex.EncodeToString(address[:]))
		addr, err := p2p.NewNetAddressString(p2p.IDAddressString(id,
			fmt.Sprintf("%v:%v", node.IP, port)))
		if _,ok := s.nodeTable[id]; !ok {
			s.nodeTable[id] = &nodeInfo{
				ID:			id,
				Adrress:	addr,
				IP:			node.IP,
				Port:		port,
				Enable:		false,
			}
			update = true
		}	
	}
	log.Debug("PutNodes","id",cid,"msg",strings.Join(nodeString,"\n"))
	if update {
		go func() { s.updateChan <- true }()
	}
}
func (s *service) updateNodes() {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _,v := range s.nodeTable {
		if !v.Enable {
			s.connTo(v)
		}
	}
}
func (s *service) connTo(node *nodeInfo) {
	if node.Enable { return }
	errDialErr := s.sw.DialPeerWithAddress(node.Adrress, true)
	if errDialErr != nil {
		log.Error("dail peer " + errDialErr.Error())
	} else {
		node.Enable = true
	}
}
// EventBus returns the Node's EventBus.
func (s *service) EventBus() *ttypes.EventBus {
	return s.eventBus
}
//------------------------------------------------------------------------------

// Node is the highest level interface to a full truechain node.
// It includes all configuration information and running services.
type Node struct {
	help.BaseService
	// configt
	config *cfg.Config
	Agent  types.PbftAgentProxy
	priv *ecdsa.PrivateKey			// local node's validator key

	// services
	services map[uint64]*service
	nodekey  p2p.NodeKey
	nodeinfo p2p.NodeInfo
	chainID  string
}

// NewNode returns a new, ready to go, truechain Node.
func NewNode(config *cfg.Config, chainID string, priv *ecdsa.PrivateKey,
	agent types.PbftAgentProxy) (*Node, error) {

	// Optionally, start the pex reactor
	// We need to set Seeds and PersistentPeers on the switch,
	// since it needs to be able to use these (and their DNS names)
	// even if the PEX is off. We can include the DNS name in the NetAddress,
	// but it would still be nice to have a clear list of the current "PersistentPeers"
	// somewhere that we can return with net_info.

	// services which will be publishing and/or subscribing for messages (events)
	// consensusReactor will set it on consensusState and blockExecutor
	node := &Node{
		config:        config,
		priv: 		   priv,
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
	n.nodeinfo = n.makeNodeInfo()
	return nil
}

// OnStop stops the Node. It implements help.Service.
func (n *Node) OnStop() {
	n.BaseService.OnStop()
	for i,v := range n.services {
		log.Info("begin stop tbft server ","id",i)
		v.stop()
		log.Info("end stop tbft server ","id",i)
	}
	// first stop the non-reactor services
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
	_, lAddr := help.ProtocolAndAddress(n.config.P2P.ListenAddress1)
	lAddrIP, lAddrPort := p2p.SplitHostPort(lAddr)
	nodeInfo.ListenAddr = fmt.Sprintf("%v:%v", lAddrIP, lAddrPort)
	return nodeInfo
}

func (n *Node) Notify(id *big.Int, action int) error {
	switch action {
	case Start:
		if server, ok := n.services[id.Uint64()]; ok {
			if server.consensusState == nil {
				panic(0)
			}
			log.Info("Begin start committee","id",id.Uint64(),"start",server.consensusState.Height,"stop",server.sa.EndHeight)
			server.start(id,n)
			log.Info("End start committee","id",id.Uint64(),"start",server.consensusState.Height,"stop",server.sa.EndHeight)
			return nil
		} else {
			return errors.New("wrong conmmitt ID:" + id.String())
		}
	case Stop:
		if server, ok := n.services[id.Uint64()]; ok {
			log.Info("Begin stop committee","id",id.Uint64(),"cur",server.consensusState.Height)
			server.stop()
			delete(n.services,id.Uint64())
			log.Info("End stop committee","id",id.Uint64(),"cur",server.consensusState.Height)
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
	log.Info("pbft PutCommittee","info",committeeInfo.String())
	// Make StateAgent
	lastCommitHeight := committeeInfo.StartHeight.Uint64()
	cid := id.Uint64()
	state := ttypes.NewStateAgent(n.Agent, n.chainID, MakeValidators(committeeInfo), lastCommitHeight,cid)
	if state == nil {
		return errors.New("make the nil state")
	}
	store := ttypes.NewBlockStore()
	service := NewNodeService(n.config.P2P,n.config.Consensus, state, store)
	service.sa = state
	service.consensusReactor = NewConsensusReactor(service.consensusState, false)
	service.sw.AddReactor("CONSENSUS", service.consensusReactor)
	service.sw.SetAddrBook(service.addrBook)
	service.consensusReactor.SetEventBus(service.eventBus)

	n.services[id.Uint64()] = service
	return nil
}
func (n *Node) PutNodes(id *big.Int, nodes []*types.CommitteeNode) error {
	if id == nil || len(nodes) <= 0 {
		return errors.New("wrong params...")
	}
	server, ok := n.services[id.Uint64()]
	if !ok {
		return errors.New("wrong ID:" + id.String())
	}
	server.putNodes(id,nodes)
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
func (n *Node) SetCommitteeStop(committeeId *big.Int, stop uint64) error {
	log.Info("SetCommitteeStop", "id", committeeId, "stop", stop)
	if server, ok := n.services[committeeId.Uint64()]; ok {
		server.getStateAgent().SetEndHeight(stop)
		return nil
	} else {
		return errors.New("wrong conmmitt ID:" + committeeId.String())
	}
}
func (n *Node) GetCommitteeStatus(committeeID *big.Int) map[string]interface{} {
	return nil
}

