package tbft

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	tcrypto "github.com/truechain/truechain-engineering-code/consensus/tbft/crypto"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p/pex"
	ttypes "github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	"github.com/truechain/truechain-engineering-code/core/types"
	cfg "github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"strconv"
	"strings"
	"sync"
)

type service struct {
	sw               *tp2p.Switch
	consensusState   *ConsensusState   // latest consensus state
	consensusReactor *ConsensusReactor // for participating in the consensus
	sa               *ttypes.StateAgentImpl
	nodeTable        map[tp2p.ID]*nodeInfo
	lock             *sync.Mutex
	updateChan       chan bool
	eventBus         *ttypes.EventBus // pub/sub for services
	addrBook         pex.AddrBook     // known peers
	healthMgr        *ttypes.HealthMgr
	selfID           tp2p.ID
}

type nodeInfo struct {
	ID      tp2p.ID
	Adrress *tp2p.NetAddress
	IP      string
	Port    uint
	Enable  bool
	Flag    int32
}

const (
	//Start is status for notify
	Start int = iota
	//Stop is status for notify
	Stop
	//Switch is status for notify
	Switch
)

func newNodeService(p2pcfg *cfg.P2PConfig, cscfg *cfg.ConsensusConfig, state *ttypes.StateAgentImpl,
	store *ttypes.BlockStore, cid uint64) *service {
	return &service{
		sw:             tp2p.NewSwitch(p2pcfg, state),
		consensusState: NewConsensusState(cscfg, state, store),
		// nodeTable:      make(map[p2p.ID]*nodeInfo),
		lock:       new(sync.Mutex),
		updateChan: make(chan bool, 2),
		eventBus:   ttypes.NewEventBus(),
		// If PEX is on, it should handle dialing the seeds. Otherwise the switch does it.
		// Note we currently use the addrBook regardless at least for AddOurAddress
		addrBook:  pex.NewAddrBook(p2pcfg.AddrBookFile(), p2pcfg.AddrBookStrict),
		healthMgr: ttypes.NewHealthMgr(cid),
	}
}

func (s *service) nodesHaveSelf() bool {
	if s.sa.Priv == nil {
		return true
	}
	v, ok := s.nodeTable[s.selfID]
	return ok && v.Flag == types.StateUsedFlag
}

func (s *service) setNodes(nodes map[tp2p.ID]*nodeInfo) {
	s.nodeTable = nodes
}
func (s *service) start(cid *big.Int, node *Node) error {
	err := s.eventBus.Start()
	if err != nil {
		return err
	}
	// Create & add listener
	lstr := node.config.P2P.ListenAddress2
	if cid.Uint64()%2 == 0 {
		lstr = node.config.P2P.ListenAddress1
	}
	nodeinfo := node.nodeinfo
	_, lAddr := help.ProtocolAndAddress(lstr)
	lAddrIP, lAddrPort := tp2p.SplitHostPort(lAddr)
	nodeinfo.ListenAddr = fmt.Sprintf("%v:%v", lAddrIP, lAddrPort)
	// Add ourselves to addrbook to prevent dialing ourselves
	s.addrBook.AddOurAddress(nodeinfo.NetAddress())
	// Add private IDs to addrbook to block those peers being added
	s.addrBook.AddPrivateIDs(help.SplitAndTrim(node.config.P2P.PrivatePeerIDs, ",", " "))

	s.sw.SetNodeInfo(nodeinfo)
	s.sw.SetNodeKey(&node.nodekey)
	log.Info("commitee start", "node info", nodeinfo.String())
	l := tp2p.NewDefaultListener(
		lstr,
		node.config.P2P.ExternalAddress,
		node.config.P2P.UPNP,
		log.New("p2p"))
	s.sw.AddListener(l)

	privValidator := ttypes.NewPrivValidator(*node.priv)
	s.consensusState.SetPrivValidator(privValidator)
	s.sa.SetPrivValidator(privValidator)
	// Start the switch (the P2P server).
	err = s.sw.Start()
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case update := <-s.updateChan:
				if update {
					s.updateNodes()
				} else {
					return // exit
				}
			}
		}
	}()
	return nil
}
func (s *service) stop() error {
	if s.sw.IsRunning() {
		s.updateChan <- false
		s.healthMgr.OnStop()
		help.CheckAndPrintError(s.eventBus.Stop())
	}
	help.CheckAndPrintError(s.sw.Stop())
	return nil
}
func (s *service) getStateAgent() *ttypes.StateAgentImpl {
	return s.sa
}
func (s *service) putNodes(cid *big.Int, nodes []*types.CommitteeNode) {
	if nodes == nil {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	update := false
	nodeString := make([]string, len(nodes))
	for i, node := range nodes {
		nodeString[i] = node.String()
		pub, err := crypto.UnmarshalPubkey(node.Publickey)
		if err != nil {
			log.Error("putnode:", "err", err, "ip", node.IP, "port", node.Port)
			continue
		}
		// check node pk
		address := crypto.PubkeyToAddress(*pub)
		port := node.Port2
		if cid.Uint64()%2 == 0 {
			port = node.Port
		}
		id := tp2p.ID(hex.EncodeToString(address[:]))
		addr, err := tp2p.NewNetAddressString(tp2p.IDAddressString(id,
			fmt.Sprintf("%v:%v", node.IP, port)))
		if v, ok := s.nodeTable[id]; ok {
			log.Info("Enter NodeInfo", "id", id, "addr", addr)
			v.Adrress, v.IP, v.Port = addr, node.IP, port
			update = true
		}
		s.healthMgr.UpdataHealthInfo(id, node.IP, port, node.Publickey)
	}
	log.Debug("PutNodes", "id", cid, "msg", strings.Join(nodeString, "\n"))
	if update && s.nodesHaveSelf() { //} ((s.sa.Priv != nil && s.consensusState.Validators.HasAddress(s.sa.Priv.GetAddress())) || s.sa.Priv == nil) {
		go func() { s.updateChan <- true }()
	}
}

//pkToP2pID pk to p2p id
func pkToP2pID(pk *ecdsa.PublicKey) tp2p.ID {
	publickey := crypto.FromECDSAPub(pk)
	pub, err := crypto.UnmarshalPubkey(publickey)
	if err != nil {
		return ""
	}
	address := crypto.PubkeyToAddress(*pub)
	return tp2p.ID(hex.EncodeToString(address[:]))
}

func (s *service) updateNodes() {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, v := range s.nodeTable {
		if v != nil {
			if !v.Enable && v.Flag == types.StateUsedFlag && v.Adrress != nil {
				s.connTo(v)
			}
		}
	}
}
func (s *service) connTo(node *nodeInfo) {
	if node.Enable {
		return
	}
	log.Info("[put nodes]connTo", "addr", node.Adrress)
	errDialErr := s.sw.DialPeerWithAddress(node.Adrress, true)
	if errDialErr != nil {
		log.Error("[connTo] dail peer " + errDialErr.Error())
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
	config *cfg.TbftConfig
	Agent  types.PbftAgentProxy
	priv   *ecdsa.PrivateKey // local node's validator key

	// services
	services map[uint64]*service
	nodekey  tp2p.NodeKey
	nodeinfo tp2p.NodeInfo
	chainID  string
	lock     *sync.Mutex
}

// NewNode returns a new, ready to go, truechain Node.
func NewNode(config *cfg.TbftConfig, chainID string, priv *ecdsa.PrivateKey,
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
		config:   config,
		priv:     priv,
		chainID:  chainID,
		Agent:    agent,
		lock:     new(sync.Mutex),
		services: make(map[uint64]*service),
		nodekey: tp2p.NodeKey{
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
	n.lock.Lock()
	defer n.lock.Unlock()
	for i, v := range n.services {
		log.Info("begin stop tbft server ", "id", i)
		help.CheckAndPrintError(v.stop())
		log.Info("end stop tbft server ", "id", i)
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

func (n *Node) makeNodeInfo() tp2p.NodeInfo {
	nodeInfo := tp2p.NodeInfo{
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
	lAddrIP, lAddrPort := tp2p.SplitHostPort(lAddr)
	nodeInfo.ListenAddr = fmt.Sprintf("%v:%v", lAddrIP, lAddrPort)
	return nodeInfo
}

//Notify is agent change server order
func (n *Node) Notify(id *big.Int, action int) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	switch action {
	case Start:
		if server, ok := n.services[id.Uint64()]; ok {
			if server.consensusState == nil {
				panic(0)
			}
			log.Info("Begin start committee", "id", id.Uint64(), "cur", server.consensusState.Height, "stop", server.sa.EndHeight)
			help.CheckAndPrintError(server.start(id, n))
			log.Info("End start committee", "id", id.Uint64(), "cur", server.consensusState.Height, "stop", server.sa.EndHeight)
			return nil
		}
		return errors.New("wrong conmmitt ID:" + id.String())

	case Stop:
		if server, ok := n.services[id.Uint64()]; ok {
			log.Info("Begin stop committee", "id", id.Uint64(), "cur", server.consensusState.Height)
			help.CheckAndPrintError(server.stop())
			//delete(n.services, id.Uint64())
			log.Info("End stop committee", "id", id.Uint64(), "cur", server.consensusState.Height)
		}
		return nil
	case Switch:
		// begin to make network..
		return nil
	}
	return nil
}

//PutCommittee is agent put all committee to server
func (n *Node) PutCommittee(committeeInfo *types.CommitteeInfo) error {
	id := committeeInfo.Id
	members := committeeInfo.Members
	if id == nil || len(members) <= 0 {
		return errors.New("wrong params")
	}
	n.lock.Lock()
	defer n.lock.Unlock()
	if _, ok := n.services[id.Uint64()]; ok {
		return errors.New("repeat ID:" + id.String())
	}
	log.Info("pbft PutCommittee", "info", committeeInfo.String())
	// Make StateAgent
	startHeight := committeeInfo.StartHeight.Uint64()
	cid := id.Uint64()
	state := ttypes.NewStateAgent(n.Agent, n.chainID, MakeValidators(committeeInfo), startHeight, cid)
	if state == nil {
		return errors.New("make the nil state")
	}
	store := ttypes.NewBlockStore()
	service := newNodeService(n.config.P2P, n.config.Consensus, state, store, cid)

	for _, v := range committeeInfo.Members {
		id := pkToP2pID(v.Publickey)
		//exclude self
		self := false
		if n.nodekey.PubKey().Equals(tcrypto.PubKeyTrue(*v.Publickey)) {
			self = true
		}
		val := ttypes.NewValidator(tcrypto.PubKeyTrue(*v.Publickey), 1)
		health := ttypes.NewHealth(id, v.MType, v.Flag, val, self)
		service.healthMgr.PutWorkHealth(health)
	}

	for _, v := range committeeInfo.BackMembers {
		id := pkToP2pID(v.Publickey)
		val := ttypes.NewValidator(tcrypto.PubKeyTrue(*v.Publickey), 1)
		health := ttypes.NewHealth(id, v.MType, v.Flag, val, false)
		service.healthMgr.PutBackHealth(health)
	}

	help.CheckAndPrintError(service.healthMgr.OnStart())
	service.consensusState.SetHealthMgr(service.healthMgr)
	nodeinfo := makeCommitteeMembers(service, committeeInfo)
	if nodeinfo == nil {
		help.CheckAndPrintError(service.stop())
		return errors.New("make the nil CommitteeMembers")
	}

	service.setNodes(nodeinfo)
	service.sa = state
	service.consensusReactor = NewConsensusReactor(service.consensusState, false)
	service.sw.AddReactor("CONSENSUS", service.consensusReactor)
	service.sw.SetAddrBook(service.addrBook)
	service.consensusReactor.SetHealthMgr(service.healthMgr)
	service.consensusReactor.SetEventBus(service.eventBus)
	service.selfID = n.nodekey.ID()
	n.services[id.Uint64()] = service
	return nil
}

//PutNodes is agent put peer's ip port
func (n *Node) PutNodes(id *big.Int, nodes []*types.CommitteeNode) error {
	if id == nil || len(nodes) <= 0 {
		return errors.New("wrong params")
	}
	n.lock.Lock()
	defer n.lock.Unlock()

	server, ok := n.services[id.Uint64()]
	if !ok {
		return errors.New("wrong ID:" + id.String())
	}
	server.putNodes(id, nodes)

	return nil
}

// UpdateCommittee update the committee info from agent when the members was changed
func (n *Node) UpdateCommittee(info *types.CommitteeInfo) error {
	log.Info("UpdateCommittee", "info", info)
	if service, ok := n.services[info.Id.Uint64()]; ok {
		//update validator
		stop, member, val := service.consensusState.UpdateValidatorSet(info)
		log.Info("UpdateCommittee", "stop", stop, "member", member, "val", val)
		service.consensusState.UpdateValidatorsSet(val, info.StartHeight.Uint64(), info.EndHeight.Uint64())

		for _, v := range member {
			pID := pkToP2pID(v.Publickey)
			p := service.sw.GetPeerForID(string(pID))
			if p != nil {
				service.sw.StopPeerForError(p, nil)
			}
		}

		//update node info
		service.lock.Lock()
		ni := makeCommitteeMembers(service, info)
		for k, v := range service.nodeTable {
			if vn, ok := ni[k]; ok {
				v.Flag = vn.Flag
			}
		}
		service.lock.Unlock()

		if stop {
			help.CheckAndPrintError(service.stop())
		}

		//update nodes
		//nodes := makeCommitteeMembersForUpdateCommittee(info)
		//service.setNodes(nodes)
		go func() { service.updateChan <- true }()
		//update health
		service.healthMgr.UpdateFromCommittee(info.Members, info.BackMembers)
		return nil

	}
	return errors.New("service not found")
}

//MakeValidators is make CommitteeInfo to ValidatorSet
func MakeValidators(cmm *types.CommitteeInfo) *ttypes.ValidatorSet {
	id := cmm.Id
	members := append(cmm.Members, cmm.BackMembers...)
	if id == nil || len(members) <= 0 {
		return nil
	}
	vals := make([]*ttypes.Validator, 0, 0)
	var power int64 = 1
	for i, m := range members {
		if m.Flag != types.StateUsedFlag {
			continue
		}
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
func makeCommitteeMembers(ss *service, cmm *types.CommitteeInfo) map[tp2p.ID]*nodeInfo {
	members := append(cmm.Members, cmm.BackMembers...)
	if ss == nil || len(members) <= 0 {
		return nil
	}
	tab := make(map[tp2p.ID]*nodeInfo)
	for i, m := range members {
		tt := tcrypto.PubKeyTrue(*m.Publickey)
		address := tt.Address()
		id := tp2p.ID(hex.EncodeToString(address))
		tab[id] = &nodeInfo{
			ID:   id,
			Flag: m.Flag,
		}
		log.Info("CommitteeMembers", "index", i, "id", id)
	}
	return tab
}

//SetCommitteeStop is stop committeeID server
func (n *Node) SetCommitteeStop(committeeID *big.Int, stop uint64) error {
	log.Info("SetCommitteeStop", "id", committeeID, "stop", stop)
	n.lock.Lock()
	defer n.lock.Unlock()

	if server, ok := n.services[committeeID.Uint64()]; ok {
		server.getStateAgent().SetEndHeight(stop)
		return nil
	}
	return errors.New("wrong conmmitt ID:" + committeeID.String())
}

func getCommittee(n *Node, cid uint64) (info *service) {
	n.lock.Lock()
	defer n.lock.Unlock()
	if server, ok := n.services[cid]; ok {
		return server
	}
	return nil
}

func getNodeStatus(s *service) map[string]interface{} {
	result := make(map[string]interface{})
	result[strconv.Itoa(int(s.consensusState.Height))] = s.consensusState.GetRoundState().Votes
	return result
}

//GetCommitteeStatus is show committee info in api
func (n *Node) GetCommitteeStatus(committeeID *big.Int) map[string]interface{} {
	log.Info("GetCommitteeStatus", "committeeID", committeeID.Uint64())
	result := make(map[string]interface{})
	s := getCommittee(n, committeeID.Uint64())
	if s != nil {
		committee := make(map[string]interface{})
		committee["id"] = committeeID.Uint64()
		committee["nodes"] = s.nodeTable
		committee["nodes_cnt"] = len(s.nodeTable)
		result["committee_now"] = committee
		result["nodeStatus"] = getNodeStatus(s)
	} else {
		log.Info("GetCommitteeStatus", "error", "server not have")
	}

	s1 := getCommittee(n, committeeID.Uint64()+1)
	if s1 != nil {
		committee := make(map[string]interface{})
		committee["id"] = committeeID.Uint64() + 1
		committee["nodes"] = s1.nodeTable
		result["committee_next"] = committee
	}
	return result
}

//check Committee
func (n *Node) verifyCommitteeInfo(cm *types.CommitteeInfo) error {
	//checkFlag
	if cm.Id.Uint64() == 0 {
		for _, v := range cm.Members {
			if v.Flag != types.StateUsedFlag ||
				v.MType != types.TypeFixed {
				return errors.New("committee member error 0")
			}
		}
		return nil
	}

	for _, v := range cm.Members {
		if v.Flag != types.StateUsedFlag &&
			v.Flag != types.StateRemovedFlag {
			return errors.New("committee member error 1")
		}
	}

	var seeds []*types.CommitteeMember

	for _, v := range cm.BackMembers {
		if v.Flag != types.StateUsedFlag &&
			v.Flag != types.StateRemovedFlag &&
			v.Flag != types.StateUnusedFlag {
			return errors.New("committee member error 2")
		}
		if v.Flag == types.TypeFixed {
			seeds = append(seeds, v)
		}
	}

	cSeeds := n.Agent.GetSeedMember()

	return n.verifySeedNode(seeds, cSeeds)
}

//check seed node
func (n *Node) verifySeedNode(seeds []*types.CommitteeMember, cSeeds []*types.CommitteeMember) error {
	if len(seeds) == 0 || len(cSeeds) == 0 || (len(seeds) != len(cSeeds)) {
		return errors.New("committee member error 3")
	}
	for i := len(seeds) - 1; i >= 0; i-- {
		if !seeds[i].Compared(cSeeds[i]) {
			return errors.New("committee member error 4")
		}
	}
	return nil
}
