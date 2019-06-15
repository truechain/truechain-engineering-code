// Copyright 2018 The go-ethereum Authors
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
	"fmt"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/light/fast"
	"github.com/truechain/truechain-engineering-code/light/public"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/truechain/truechain-engineering-code/etrue"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/light"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/p2p/enode"
	"github.com/truechain/truechain-engineering-code/params"
)

// lesCommons contains fields needed by both server and client.
type lesCommons struct {
	config           *etrue.Config
	iConfig          *public.IndexerConfig
	chainDb          etruedb.Database
	protocolManager  *ProtocolManager
	chtIndexer       *snailchain.ChainIndexer
	bloomTrieIndexer *core.ChainIndexer
}

// NodeInfo represents a short summary of the Ethereum sub-protocol metadata
// known about the host peer.
type NodeInfo struct {
	Network      uint64                   `json:"network"`      // Ethereum network ID (1=Frontier, 2=Morden, Ropsten=3, Rinkeby=4)
	Genesis      common.Hash              `json:"genesis"`      // SHA3 hash of the host's genesis block
	Config       *params.ChainConfig      `json:"config"`       // Chain configuration for the fork rules
	Head         common.Hash              `json:"head"`         // SHA3 hash of the host's best owned block
	CHT          params.TrustedCheckpoint `json:"cht"`          // Trused CHT checkpoint for fast catchup
	Difficulty   *big.Int                 `json:"difficulty"`   // Total difficulty of the host's blockchain
	SnailGenesis common.Hash              `json:"snailGenesis"` // SHA3 hash of the host's genesis block
	SnailConfig  *params.ChainConfig      `json:"snailConfig"`  // Chain configuration for the fork rules
	SnailHead    common.Hash              `json:"snailHead"`    // SHA3 hash of the host's best owned block
}

// makeProtocols creates protocol descriptors for the given LES versions.
func (c *lesCommons) makeProtocols(versions []uint) []p2p.Protocol {
	protos := make([]p2p.Protocol, len(versions))
	for i, version := range versions {
		version := version
		protos[i] = p2p.Protocol{
			Name:     "les",
			Version:  version,
			Length:   ProtocolLengths[version],
			NodeInfo: c.nodeInfo,
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				return c.protocolManager.runPeer(version, p, rw)
			},
			PeerInfo: func(id enode.ID) interface{} {
				if p := c.protocolManager.peers.Peer(fmt.Sprintf("%x", id[:8])); p != nil {
					return p.Info()
				}
				return nil
			},
		}
	}
	return protos
}

// nodeInfo retrieves some protocol metadata about the running host node.
func (c *lesCommons) nodeInfo() interface{} {
	var cht params.TrustedCheckpoint
	sections, _, _ := c.chtIndexer.Sections()
	sections2, _, _ := c.bloomTrieIndexer.Sections()

	if !c.protocolManager.lightSync {
		// convert to client section size if running in server mode
		sections /= c.iConfig.PairChtSize / c.iConfig.ChtSize
	}

	if sections2 < sections {
		sections = sections2
	}
	if sections > 0 {
		sectionIndex := sections - 1
		sectionHead := c.bloomTrieIndexer.SectionHead(sectionIndex)
		var chtRoot common.Hash
		if c.protocolManager.lightSync {
			chtRoot = light.GetChtRoot(c.chainDb, sectionIndex, sectionHead)
		} else {
			idxV2 := (sectionIndex+1)*c.iConfig.PairChtSize/c.iConfig.ChtSize - 1
			chtRoot = light.GetChtRoot(c.chainDb, idxV2, sectionHead)
		}
		cht = params.TrustedCheckpoint{
			SectionIndex: sectionIndex,
			SectionHead:  sectionHead,
			CHTRoot:      chtRoot,
			BloomRoot:    fast.GetBloomTrieRoot(c.chainDb, sectionIndex, sectionHead),
		}
	}

	fchain := c.protocolManager.blockchain
	chain := c.protocolManager.blockchain
	head := chain.CurrentHeader()
	hash := head.Hash()
	return &NodeInfo{
		Network:      c.config.NetworkId,
		Genesis:      fchain.Genesis().Hash(),
		Config:       fchain.Config(),
		Head:         fchain.CurrentHeader().Hash(),
		CHT:          cht,
		Difficulty:   chain.GetTd(hash, head.Number.Uint64()),
		SnailGenesis: chain.Genesis().Hash(),
		SnailConfig:  chain.Config(),
		SnailHead:    hash,
	}
}
