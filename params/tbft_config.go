package params

import (
	"os"
	"path/filepath"
	"time"
)

const (
	// FuzzModeDrop is a mode in which we randomly drop reads/writes, connections or sleep
	FuzzModeDrop = iota
	// FuzzModeDelay is a mode in which we randomly sleep
	FuzzModeDelay
)

// NOTE: Most of the structs & relevant comments + the
// default configuration options were used to manually
// generate the config.toml. Please reflect any changes
// made here in the defaultConfigTemplate constant in
// config/toml.go
// NOTE: tmlibs/cli must know to look in the config dir!
var (
	DefaultTBFTDir   = ".tbft"
	defaultConfigDir = "config"
	defaultDataDir   = "data"

	defaultConfigFileName = "config.toml"
	defaultAddrBookName   = "addrbook.json"

	defaultConfigFilePath = filepath.Join(defaultConfigDir, defaultConfigFileName)
	defaultAddrBookPath   = filepath.Join(defaultConfigDir, defaultAddrBookName)
)

// Config defines the top level configuration for a truechain node
type TbftConfig struct {
	// Top level options use an anonymous struct
	BaseConfig `mapstructure:",squash"`
	// Options for services
	P2P       *P2PConfig       `mapstructure:"p2p"`
	Consensus *ConsensusConfig `mapstructure:"consensus"`
}

// DefaultConfig returns a default configuration for a truechain node
func DefaultConfig() *TbftConfig {
	return &TbftConfig{
		BaseConfig: DefaultBaseConfig(),
		P2P:        DefaultP2PConfig(),
		Consensus:  DefaultConsensusConfig(),
	}
}

// TestConfig returns a configuration that can be used for testing
func TestConfig() *TbftConfig {
	return &TbftConfig{
		BaseConfig: TestBaseConfig(),
		P2P:        TestP2PConfig(),
		Consensus:  TestConsensusConfig(),
	}
}

// SetRoot sets the RootDir for all Config structs
func (cfg *TbftConfig) SetRoot(root string) *TbftConfig {
	cfg.P2P.RootDir = root
	cfg.Consensus.RootDir = root
	return cfg
}

//-----------------------------------------------------------------------------
// BaseConfig

// BaseConfig defines the base configuration for a truechain node
type BaseConfig struct {
	// A custom human readable name for this node
	Moniker string `mapstructure:"moniker"`

	// If true, query the ABCI app on connecting to a new peer
	// so the app can decide if we should keep the connection or not
	FilterPeers bool `mapstructure:"filter_peers"` // false
}

// DefaultBaseConfig returns a default base configuration for a truechain node
func DefaultBaseConfig() BaseConfig {
	return BaseConfig{
		Moniker:     defaultMoniker,
		FilterPeers: false,
	}
}

// TestBaseConfig returns a base configuration for testing a truechain node
func TestBaseConfig() BaseConfig {
	cfg := DefaultBaseConfig()
	return cfg
}

//-----------------------------------------------------------------------------
// P2PConfig

// P2PConfig defines the configuration options for the truechain peer-to-peer networking layer
type P2PConfig struct {
	RootDir string `mapstructure:"home"`

	// Address to listen for incoming connections
	ListenAddress1 string `mapstructure:"laddr"`
	ListenAddress2 string `mapstructure:"laddr"`

	// Address to advertise to peers for them to dial
	ExternalAddress string `mapstructure:"external_address"`

	// UPNP port forwarding
	UPNP bool `mapstructure:"upnp"`

	// Path to address book
	AddrBook string `mapstructure:"addr_book_file"`

	// Set true for strict address routability rules
	AddrBookStrict bool `mapstructure:"addr_book_strict"`

	// Maximum number of peers to connect to
	MaxNumPeers int `mapstructure:"max_num_peers"`

	// Time to wait before flushing messages out on the connection, in ms
	FlushThrottleTimeout int `mapstructure:"flush_throttle_timeout"`

	// Maximum size of a message packet payload, in bytes
	MaxPacketMsgPayloadSize int `mapstructure:"max_packet_msg_payload_size"`

	// Rate at which packets can be sent, in bytes/second
	SendRate int64 `mapstructure:"send_rate"`

	// Rate at which packets can be received, in bytes/second
	RecvRate int64 `mapstructure:"recv_rate"`

	// Comma separated list of peer IDs to keep private (will not be gossiped to
	// other peers)
	PrivatePeerIDs string `mapstructure:"private_peer_ids"`

	// Toggle to disable guard against peers connecting from the same ip.
	AllowDuplicateIP bool `mapstructure:"allow_duplicate_ip"`

	// Peer connection configuration.
	HandshakeTimeout time.Duration `mapstructure:"handshake_timeout"`
	DialTimeout      time.Duration `mapstructure:"dial_timeout"`

	// Testing params.
	// Force dial to fail
	TestDialFail bool `mapstructure:"test_dial_fail"`
	// FUzz connection
	TestFuzz       bool            `mapstructure:"test_fuzz"`
	TestFuzzConfig *FuzzConnConfig `mapstructure:"test_fuzz_config"`
}

// DefaultP2PConfig returns a default configuration for the peer-to-peer layer
func DefaultP2PConfig() *P2PConfig {
	return &P2PConfig{
		ListenAddress1:          "tcp://0.0.0.0:30310",
		ListenAddress2:          "tcp://0.0.0.0:30311",
		ExternalAddress:         "tcp://0.0.0.0:30411",
		UPNP:                    false,
		AddrBook:                defaultAddrBookPath,
		AddrBookStrict:          true,
		MaxNumPeers:             50,
		FlushThrottleTimeout:    100,
		MaxPacketMsgPayloadSize: 1024,    // 1 kB
		SendRate:                5120000, // 5 mB/s
		RecvRate:                5120000, // 5 mB/s
		AllowDuplicateIP:        true,    // so non-breaking yet
		HandshakeTimeout:        20 * time.Second,
		DialTimeout:             3 * time.Second,
		TestDialFail:            false,
		TestFuzz:                false,
		TestFuzzConfig:          DefaultFuzzConnConfig(),
	}
}

// TestP2PConfig returns a configuration for testing the peer-to-peer layer
func TestP2PConfig() *P2PConfig {
	cfg := DefaultP2PConfig()
	cfg.ListenAddress1 = "tcp://0.0.0.0:36656"
	cfg.ListenAddress2 = "tcp://0.0.0.0:36657"
	cfg.FlushThrottleTimeout = 10
	cfg.AllowDuplicateIP = true
	return cfg
}

// AddrBookFile returns the full path to the address book
func (cfg *P2PConfig) AddrBookFile() string {
	return rootify(cfg.AddrBook, cfg.RootDir)
}

// FuzzConnConfig is a FuzzedConnection configuration.
type FuzzConnConfig struct {
	Mode         int
	MaxDelay     time.Duration
	ProbDropRW   float64
	ProbDropConn float64
	ProbSleep    float64
}

// DefaultFuzzConnConfig returns the default config.
func DefaultFuzzConnConfig() *FuzzConnConfig {
	return &FuzzConnConfig{
		Mode:         FuzzModeDrop,
		MaxDelay:     3 * time.Second,
		ProbDropRW:   0.2,
		ProbDropConn: 0.00,
		ProbSleep:    0.00,
	}
}

//-----------------------------------------------------------------------------
// ConsensusConfig

// ConsensusConfig defines the configuration for the truechain consensus service,
// including timeouts and details about the WAL and the block structure.
type ConsensusConfig struct {
	RootDir string `mapstructure:"home"`
	WalPath string `mapstructure:"wal_file"`
	walFile string // overrides WalPath if set

	// All timeouts are in milliseconds
	TimeoutPropose        int `mapstructure:"timeout_propose"`
	TimeoutProposeDelta   int `mapstructure:"timeout_propose_delta"`
	TimeoutPrevote        int `mapstructure:"timeout_prevote"`
	TimeoutPrevoteDelta   int `mapstructure:"timeout_prevote_delta"`
	TimeoutPrecommit      int `mapstructure:"timeout_precommit"`
	TimeoutPrecommitDelta int `mapstructure:"timeout_precommit_delta"`
	TimeoutCommit         int `mapstructure:"timeout_commit"`
	TimeoutCatchup	  int `mapstructure:"timeout_consensus"`

	// Make progress as soon as we have all the precommits (as if TimeoutCommit = 0)
	SkipTimeoutCommit bool `mapstructure:"skip_timeout_commit"`

	// EmptyBlocks mode and possible interval between empty blocks in seconds
	CreateEmptyBlocks         bool `mapstructure:"create_empty_blocks"`
	CreateEmptyBlocksInterval int  `mapstructure:"create_empty_blocks_interval"`

	// Reactor sleep duration parameters are in milliseconds
	PeerGossipSleepDuration     int `mapstructure:"peer_gossip_sleep_duration"`
	PeerQueryMaj23SleepDuration int `mapstructure:"peer_query_maj23_sleep_duration"`
}

// DefaultConsensusConfig returns a default configuration for the consensus service
func DefaultConsensusConfig() *ConsensusConfig {
	return &ConsensusConfig{
		WalPath:                     filepath.Join(defaultDataDir, "cs.wal", "wal"),
		TimeoutPropose:              10000,
		TimeoutProposeDelta:         500,
		TimeoutPrevote:              3000,
		TimeoutPrevoteDelta:         500,
		TimeoutPrecommit:            3000,
		TimeoutPrecommitDelta:       500,
		TimeoutCommit:               1000,
		TimeoutCatchup:			 	 1000, 	
		SkipTimeoutCommit:           false,
		CreateEmptyBlocks:           true,
		CreateEmptyBlocksInterval:   5000,
		PeerGossipSleepDuration:     100,
		PeerQueryMaj23SleepDuration: 2000,
	}
}

// TestConsensusConfig returns a configuration for testing the consensus service
func TestConsensusConfig() *ConsensusConfig {
	cfg := DefaultConsensusConfig()
	cfg.TimeoutPropose = 100
	cfg.TimeoutProposeDelta = 1
	cfg.TimeoutPrevote = 10
	cfg.TimeoutPrevoteDelta = 1
	cfg.TimeoutPrecommit = 10
	cfg.TimeoutPrecommitDelta = 1
	cfg.TimeoutCommit = 10
	cfg.TimeoutCatchup = 500
	cfg.SkipTimeoutCommit = true
	cfg.PeerGossipSleepDuration = 5
	cfg.PeerQueryMaj23SleepDuration = 250
	
	return cfg
}

// WaitForTxs returns true if the consensus should wait for transactions before entering the propose step
func (cfg *ConsensusConfig) WaitForTxs() bool {
	return !cfg.CreateEmptyBlocks || cfg.CreateEmptyBlocksInterval > 0
}

// EmptyBlocks returns the amount of time to wait before proposing an empty block or starting the propose timer if there are no txs available
func (cfg *ConsensusConfig) EmptyBlocksInterval() time.Duration {
	return time.Duration(cfg.CreateEmptyBlocksInterval) * time.Millisecond
}

// Propose returns the amount of time to wait for a proposal
func (cfg *ConsensusConfig) Propose(round int) time.Duration {
	return time.Duration(cfg.TimeoutPropose+cfg.TimeoutProposeDelta*round) * time.Millisecond
}

// Prevote returns the amount of time to wait for straggler votes after receiving any +2/3 prevotes
func (cfg *ConsensusConfig) Prevote(round int) time.Duration {
	return time.Duration(cfg.TimeoutPrevote+cfg.TimeoutPrevoteDelta*round) * time.Millisecond
}

// Precommit returns the amount of time to wait for straggler votes after receiving any +2/3 precommits
func (cfg *ConsensusConfig) Precommit(round int) time.Duration {
	return time.Duration(cfg.TimeoutPrecommit+cfg.TimeoutPrecommitDelta*round) * time.Millisecond
}

// Commit returns the amount of time to wait for straggler votes after receiving +2/3 precommits for a single block (ie. a commit).
func (cfg *ConsensusConfig) Commit(t time.Time) time.Time {
	return t.Add(time.Duration(cfg.TimeoutCommit) * time.Millisecond)
}
// CatchupTime catch up same consensus for peer
func (cfg *ConsensusConfig) CatchupTime(t time.Time) time.Time {
	return t.Add(time.Duration(cfg.TimeoutCatchup) * time.Millisecond)
}
// PeerGossipSleep returns the amount of time to sleep if there is nothing to send from the ConsensusReactor
func (cfg *ConsensusConfig) PeerGossipSleep() time.Duration {
	return time.Duration(cfg.PeerGossipSleepDuration) * time.Millisecond
}

// PeerQueryMaj23Sleep returns the amount of time to sleep after each VoteSetMaj23Message is sent in the ConsensusReactor
func (cfg *ConsensusConfig) PeerQueryMaj23Sleep() time.Duration {
	return time.Duration(cfg.PeerQueryMaj23SleepDuration) * time.Millisecond
}

// WalFile returns the full path to the write-ahead log file
func (cfg *ConsensusConfig) WalFile() string {
	if cfg.walFile != "" {
		return cfg.walFile
	}
	return rootify(cfg.WalPath, cfg.RootDir)
}

// SetWalFile sets the path to the write-ahead log file
func (cfg *ConsensusConfig) SetWalFile(walFile string) {
	cfg.walFile = walFile
}

//-----------------------------------------------------------------------------
// Utils

// helper function to make config creation independent of root dir
func rootify(path, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

//-----------------------------------------------------------------------------
// Moniker

var defaultMoniker = getDefaultMoniker()

// getDefaultMoniker returns a default moniker, which is the host name. If runtime
// fails to get the host name, "anonymous" will be returned.
func getDefaultMoniker() string {
	moniker, err := os.Hostname()
	if err != nil {
		moniker = "anonymous"
	}
	return moniker
}
