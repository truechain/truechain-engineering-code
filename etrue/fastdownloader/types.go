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

package fastdownloader

import (
	"fmt"

	"github.com/truechain/truechain-engineering-code/core/types"
)

// headerPack is a batch of block headers returned by a peer.
type headerPack struct {
	peerID  string
	headers []*types.Header
}

func (p *headerPack) PeerId() string { return p.peerID }
func (p *headerPack) Items() int     { return len(p.headers) }
func (p *headerPack) Stats() string  { return fmt.Sprintf("%d", len(p.headers)) }

// bodyPack is a batch of block bodies returned by a peer.
type bodyPack struct {
	peerID       string
	transactions [][]*types.Transaction
	signs        [][]*types.PbftSign
	//uncles       [][]*types.Header
}

func (p *bodyPack) PeerId() string { return p.peerID }
func (p *bodyPack) Items() int {
	//if len(p.transactions) <= len(p.uncles) {
	//	return len(p.transactions)
	//}
	//return len(p.uncles)
	return len(p.transactions)
}
func (p *bodyPack) Stats() string {
	return fmt.Sprintf("%d:%d", len(p.transactions), len(p.signs))
}

// receiptPack is a batch of receipts returned by a peer.
type receiptPack struct {
	peerID   string
	receipts [][]*types.Receipt
}

func (p *receiptPack) PeerId() string { return p.peerID }
func (p *receiptPack) Items() int     { return len(p.receipts) }
func (p *receiptPack) Stats() string  { return fmt.Sprintf("%d", len(p.receipts)) }

// statePack is a batch of states returned by a peer.
type statePack struct {
	peerID string
	states [][]byte
}

func (p *statePack) PeerId() string { return p.peerID }
func (p *statePack) Items() int     { return len(p.states) }
func (p *statePack) Stats() string  { return fmt.Sprintf("%d", len(p.states)) }
