package dummy

import (
	"net"

	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	tp2p "github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p"
	tmconn "github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p/conn"
)

type peer struct {
	help.BaseService
	kv map[string]interface{}
}

var _ tp2p.Peer = (*peer)(nil)

// NewPeer creates new dummy peer.
func NewPeer() *peer {
	p := &peer{
		kv: make(map[string]interface{}),
	}
	p.BaseService = *help.NewBaseService("peer", p)

	return p
}

// ID always returns dummy.
func (p *peer) ID() tp2p.ID {
	return tp2p.ID("dummy")
}

// IsOutbound always returns false.
func (p *peer) IsOutbound() bool {
	return false
}

// IsPersistent always returns false.
func (p *peer) IsPersistent() bool {
	return false
}

// NodeInfo always returns empty node info.
func (p *peer) NodeInfo() tp2p.NodeInfo {
	return tp2p.NodeInfo{}
}

// RemoteIP always returns localhost.
func (p *peer) RemoteIP() net.IP {
	return net.ParseIP("127.0.0.1")
}

// Status always returns empry connection status.
func (p *peer) Status() tmconn.ConnectionStatus {
	return tmconn.ConnectionStatus{}
}

// Send does not do anything and just returns true.
func (p *peer) Send(byte, []byte) bool {
	return true
}

// TrySend does not do anything and just returns true.
func (p *peer) TrySend(byte, []byte) bool {
	return true
}

// Set records value under key specified in the map.
func (p *peer) Set(key string, value interface{}) {
	p.kv[key] = value
}

// Get returns a value associated with the key. Nil is returned if no value
// found.
func (p *peer) Get(key string) interface{} {
	if value, ok := p.kv[key]; ok {
		return value
	}
	return nil
}

// OriginalAddr always returns nil.
func (p *peer) OriginalAddr() *tp2p.NetAddress {
	return nil
}
