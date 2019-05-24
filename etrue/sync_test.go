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

package etrue

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"testing"
	"time"

	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/p2p"
	"github.com/truechain/truechain-engineering-code/p2p/enode"
)

// Tests that fast sync gets disabled as soon as a real block is successfully
// imported into the blockchain.
//func TestFastSyncDisabling(t *testing.T) {
//	// Create a pristine protocol manager, check that fast sync is left enabled
//	pmEmpty, _ := newTestProtocolManagerMust(t, downloader.FastSync, 0, 0, nil, nil, nil, nil)
//	if atomic.LoadUint32(&pmEmpty.fastSync) == 0 {
//		t.Fatalf("fast sync disabled on pristine blockchain")
//	}
//	// Create a full protocol manager, check that fast sync gets disabled
//	pmFull, _ := newTestProtocolManagerMust(t, downloader.FastSync, 1024, 0, nil, nil, nil, nil)
//	if atomic.LoadUint32(&pmFull.fastSync) == 1 {
//		t.Fatalf("fast sync not disabled on non-empty blockchain")
//	}
//	// Sync up the two peers
//	io1, io2 := p2p.MsgPipe()
//
//	go pmFull.handle(pmFull.newPeer(63, p2p.NewPeer(enode.ID{}, "empty", nil), io2))
//	go pmEmpty.handle(pmEmpty.newPeer(63, p2p.NewPeer(enode.ID{}, "full", nil), io1))
//
//	time.Sleep(250 * time.Millisecond)
//	pmEmpty.synchronise(pmEmpty.peers.BestPeer())
//
//	// Check that fast sync was disabled
//	if atomic.LoadUint32(&pmEmpty.fastSync) == 1 {
//		t.Fatalf("fast sync not disabled after successful synchronisation")
//	}
//}

// Tests that full sync gets disabled as soon as a real block is successfully
// imported into the blockchain.
func TestFullFastSync(t *testing.T) {
	// Create a pristine protocol manager, check that fast sync is left enabled
	pmEmpty, _ := newTestProtocolManagerMust(t, downloader.FullSync, 0, 0, nil, nil, nil, nil)
	// Create a full protocol manager, check that fast sync gets disabled
	pmFull, _ := newTestProtocolManagerMust(t, downloader.FullSync, 256, 0, nil, nil, nil, nil)
	// Sync up the two peers
	io1, io2 := p2p.MsgPipe()

	go pmFull.handle(pmFull.newPeer(63, p2p.NewPeer(enode.ID{0}, "empty", nil), io2))
	go pmEmpty.handle(pmEmpty.newPeer(63, p2p.NewPeer(enode.ID{1}, "full", nil), io1))

	time.Sleep(250 * time.Millisecond)
	pmEmpty.synchronise(pmEmpty.peers.BestPeer())
}

// Tests that full sync gets disabled as soon as a real block is successfully
// imported into the blockchain.All headers delayed, waiting fsHeaderContCheck
func TestFullFastTXsSync(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		signer  = types.NewTIP1Signer(params.AllMinervaProtocolChanges.ChainID)
	)
	// Create a pristine protocol manager, check that fast sync is left enabled
	pmEmpty, _ := newTestProtocolManagerMust(t, downloader.FullSync, 0, 0, nil, nil, nil, nil)
	// Create a full protocol manager, check that fast sync gets disabled
	pmFull, _ := newTestProtocolManagerMust(t, downloader.FullSync, 256, 0, func(i int, gen *core.BlockGen) {
		switch i % 10 {
		case 2:
			// In block 1, addr1 sends addr2 some ether.
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(100), params.TxGas, nil, nil), signer, key1)
			gen.AddTx(tx)
		}
	}, nil, nil, nil)
	// Sync up the two peers
	io1, io2 := p2p.MsgPipe()

	go pmFull.handle(pmFull.newPeer(63, p2p.NewPeer(enode.ID{0}, "empty", nil), io2))
	go pmEmpty.handle(pmEmpty.newPeer(63, p2p.NewPeer(enode.ID{1}, "full", nil), io1))

	time.Sleep(250 * time.Millisecond)
	pmEmpty.synchronise(pmEmpty.peers.BestPeer())
}

// Tests that full sync gets disabled as soon as a real block is successfully
// imported into the blockchain.
func TestFullSync(t *testing.T) {
	// Create a pristine protocol manager, check that fast sync is left enabled
	pmEmpty, _ := newTestProtocolManagerMust(t, downloader.FullSync, 0, 0, nil, nil, nil, nil)
	// Create a full protocol manager, check that fast sync gets disabled
	pmFull, _ := newTestProtocolManagerMust(t, downloader.FullSync, 256, 256, nil, nil, nil, nil)
	// Sync up the two peers
	io1, io2 := p2p.MsgPipe()

	go pmFull.handle(pmFull.newPeer(63, p2p.NewPeer(enode.ID{0}, "empty", nil), io2))
	go pmEmpty.handle(pmEmpty.newPeer(63, p2p.NewPeer(enode.ID{1}, "full", nil), io1))

	time.Sleep(250 * time.Millisecond)
	pmEmpty.synchronise(pmEmpty.peers.BestPeer())
}
