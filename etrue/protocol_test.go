// Copyright 2014 The go-ethereum Authors
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
	"fmt"
	"testing"
	"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etrue/downloader"
	"github.com/truechain/truechain-engineering-code/p2p"
	"math/big"
	"crypto/ecdsa"
)

var (
	//th = truechain.NewTrueHybrid()
	privkeys = make([]*ecdsa.PrivateKey,0,0)
	keysCount = 6
	tx1 = types.NewTransaction(
		0,
		common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
		big.NewInt(0), 0, big.NewInt(0),
		nil,
	)
	tx2 = types.NewTransaction(
		0,
		common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
		big.NewInt(0), 0, big.NewInt(0),
		nil,
	)
	tx3, _ = types.NewTransaction(
		3,
		common.HexToAddress("b94f5374fce5edbc8e2a8697c15331677e6ebf0b"),
		big.NewInt(10),
		2000,
		big.NewInt(1),
		common.FromHex("5544"),
	).WithSignature(
		types.NewEIP155Signer(common.Big1),
		common.Hex2Bytes("98ff921201554726367d2be8c804a7ff89ccf285ebc57dff8ae4c44b9c19ac4a8887321be575c8095f789dd4c743dfe42c1820f9231f98a962b210e3ac2452a301"),
	)
)

func init() {
	// log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
	for i:=0;i<keysCount;i++ {
		k,_ := crypto.GenerateKey()
		privkeys = append(privkeys,k)
	}
	//th.SetCommitteeCount(3)
}

var testAccount, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")

// Tests that handshake failures are detected and reported correctly.
func TestStatusMsgErrors62(t *testing.T) { testStatusMsgErrors(t, 62) }
func TestStatusMsgErrors63(t *testing.T) { testStatusMsgErrors(t, 63) }

func testStatusMsgErrors(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, 0, nil, nil)
	var (
		genesis = pm.blockchain.Genesis()
		head    = pm.blockchain.CurrentHeader()
		td      = pm.blockchain.GetTd(head.Hash(), head.Number.Uint64())
	)
	defer pm.Stop()


	tests := []struct {
		code      uint64
		data      interface{}
		wantError error
	}{
		{
			code: TxMsg, data: []interface{}{},
			wantError: errResp(ErrNoStatusMsg, "first msg has code 2 (!= 0)"),
		},
		{
			code: StatusMsg, data: statusData{10, DefaultConfig.NetworkId, td, head.Hash(), genesis.Hash()},
			wantError: errResp(ErrProtocolVersionMismatch, "10 (!= %d)", protocol),
		},
		{
			code: StatusMsg, data: statusData{uint32(protocol), 999, td, head.Hash(), genesis.Hash()},
			wantError: errResp(ErrNetworkIdMismatch, "999 (!= 1)"),
		},
		{
			code: StatusMsg, data: statusData{uint32(protocol), DefaultConfig.NetworkId, td, head.Hash(), common.Hash{3}},
			wantError: errResp(ErrGenesisBlockMismatch, "0300000000000000 (!= %x)", genesis.Hash().Bytes()[:8]),
		},
	}

	for i, test := range tests {
		p, errc := newTestPeer(fmt.Sprintf("peer"), protocol, pm, false)
		// The send call might hang until reset because
		// the protocol might not read the payload.
		go p2p.Send(p.app, test.code, test.data)

		select {
		case err := <-errc:
			if err == nil {
				t.Errorf("test %d: protocol returned nil error, want %q", i, test.wantError)
			} else if err.Error() != test.wantError.Error() {
				t.Errorf("test %d: wrong error: got %q, want %q", i, err, test.wantError)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("protocol did not shut down within 2 seconds")
		}
		p.close()
	}
}
/*
// This test checks that received transactions are added to the local pool.
func TestRecvTransactions62(t *testing.T) { testRecvTransactions(t, 62) }
func TestRecvTransactions63(t *testing.T) { testRecvTransactions(t, 63) }

func testRecvTransactions(t *testing.T, protocol int) {
	txAdded := make(chan []*types.Transaction)
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, 0, nil, txAdded)
	pm.acceptTxs = 1 // mark synced to accept transactions
	p, _ := newTestPeer("peer", protocol, pm, true)
	defer pm.Stop()
	defer p.close()

	tx := newTestTransaction(testAccount, 0, 0)
	if err := p2p.Send(p.app, TxMsg, []interface{}{tx}); err != nil {
		t.Fatalf("send error: %v", err)
	}
	select {
	case added := <-txAdded:
		if len(added) != 1 {
			t.Errorf("wrong number of added transactions: got %d, want 1", len(added))
		} else if added[0].Hash() != tx.Hash() {
			t.Errorf("added wrong tx hash: got %v, want %v", added[0].Hash(), tx.Hash())
		}
	case <-time.After(2 * time.Second):
		t.Errorf("no NewTxsEvent received within 2 seconds")
	}
}

// This test checks that pending transactions are sent.
func TestSendTransactions62(t *testing.T) { testSendTransactions(t, 62) }
func TestSendTransactions63(t *testing.T) { testSendTransactions(t, 63) }

func testSendTransactions(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, 0, nil, nil)
	defer pm.Stop()

	// Fill the pool with big transactions.
	const txsize = txsyncPackSize / 10
	alltxs := make([]*types.Transaction, 100)
	for nonce := range alltxs {
		fmt.Println(nonce)
		alltxs[nonce] = newTestTransaction(testAccount, uint64(nonce), txsize)
	}
	pm.txpool.AddRemotes(alltxs)

	// Connect several peers. They should all receive the pending transactions.
	var wg sync.WaitGroup
	checktxs := func(p *testPeer) {
		defer wg.Done()
		defer p.close()
		seen := make(map[common.Hash]bool)
		for _, tx := range alltxs {
			seen[tx.Hash()] = false
		}
		for n := 0; n < len(alltxs) && !t.Failed(); {
			var txs []*types.Transaction
			msg, err := p.app.ReadMsg()
			fmt.Println("code 11 ",msg.Code)
			if err != nil {
				t.Errorf("%v: read error: %v", p.Peer, err)
			} else if msg.Code != TxMsg {
				t.Errorf("%v: got code %d, want TxMsg", p.Peer, msg.Code)
			}
			if err := msg.Decode(&txs); err != nil {
				t.Errorf("%v: %v", p.Peer, err)
			}
			for _, tx := range txs {
				hash := tx.Hash()
				seentx, want := seen[hash]
				fmt.Println(seentx," ",want)
				if seentx {
					t.Errorf("%v: got tx more than once: %x", p.Peer, hash)
				}
				if !want {
					t.Errorf("%v: got unexpected tx: %x", p.Peer, hash)
				}
				seen[hash] = true
				n++
			}
		}
	}
	for i := 0; i < 3; i++ {
		p, _ := newTestPeer(fmt.Sprintf("peer #%d", i), protocol, pm, true)
		wg.Add(1)
		go checktxs(p)
	}
	wg.Wait()
}

// Tests that the custom union field encoder and decoder works correctly.
func TestGetBlockHeadersDataEncodeDecode(t *testing.T) {
	// Create a "random" hash for testing
	var hash common.Hash
	for i := range hash {
		hash[i] = byte(i)
	}
	// Assemble some table driven tests
	tests := []struct {
		packet *getBlockHeadersData
		fail   bool
	}{
		// Providing the origin as either a hash or a number should both work
		{fail: false, packet: &getBlockHeadersData{Origin: hashOrNumber{Number: 314}}},
		{fail: false, packet: &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}}},

		// Providing arbitrary query field should also work
		{fail: false, packet: &getBlockHeadersData{Origin: hashOrNumber{Number: 314}, Amount: 314, Skip: 1, Reverse: true}},
		{fail: false, packet: &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: 314, Skip: 1, Reverse: true}},

		// Providing both the origin hash and origin number must fail
		{fail: true, packet: &getBlockHeadersData{Origin: hashOrNumber{Hash: hash, Number: 314}}},
	}
	// Iterate over each of the tests and try to encode and then decode
	for i, tt := range tests {
		bytes, err := rlp.EncodeToBytes(tt.packet)
		if err != nil && !tt.fail {
			t.Fatalf("test %d: failed to encode packet: %v", i, err)
		} else if err == nil && tt.fail {
			t.Fatalf("test %d: encode should have failed", i)
		}
		if !tt.fail {
			packet := new(getBlockHeadersData)
			if err := rlp.DecodeBytes(bytes, packet); err != nil {
				t.Fatalf("test %d: failed to decode packet: %v", i, err)
			}
			if packet.Origin.Hash != tt.packet.Origin.Hash || packet.Origin.Number != tt.packet.Origin.Number || packet.Amount != tt.packet.Amount ||
				packet.Skip != tt.packet.Skip || packet.Reverse != tt.packet.Reverse {
				t.Fatalf("test %d: encode decode mismatch: have %+v, want %+v", i, packet, tt.packet)
			}
		}
	}
}

// This test checks that pending PbftBlocks are sent.
func TestSendPbftBlocks62(t *testing.T) { testSendPbftBlocks(t, 62) }
func TestSendPbftBlocks63(t *testing.T) { testSendPbftBlocks(t, 63) }


func testSendPbftBlocks(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, 0, nil, nil)
	defer pm.Stop()

	th.Cmm = MakeFirstCommittee(3)
	// test make pbft block
	block := MakePbftBlock(th.Cmm)

	// Connect several peers. They should all receive the pending transactions.
	var wg sync.WaitGroup
	checkpbs := func(p *testPeer) {
		defer wg.Done()
		defer p.close()

		var pbblock *truechain.TruePbftBlock
		fmt.Println("读取 app.ReadMsg")
		msg, err := p.app.ReadMsg()
		if err != nil {
			t.Errorf("%v: read error: %v", p.Peer, err)
		} else if msg.Code != NewBftBlockMsg {
			t.Errorf("%v: got code %d, want TxMsg", p.Peer, msg.Code)
		}
		if err := msg.Decode(&pbblock); err != nil {
			t.Errorf("%v: %v", p.Peer, err)
		}
		fmt.Println("读取 app.ReadMsg 后 ",pbblock)
		// verify the pbft block
		err = th.CheckBlock(pbblock)
		if err != nil {
			fmt.Println("verify the block failed,err=",err)
			return
		}
		th.StopTrueChain()
	}
	for i := 0; i < 1; i++ {
		p, _ := newTestPeer(fmt.Sprintf("peer #%d", i), protocol, pm, true)
		wg.Add(1)
		fmt.Println("发送 消息  NewBftBlockMsg")
		p2p.Send(p.app, NewBftBlockMsg, []interface{}{block})
		go checkpbs(p)
	}
	wg.Wait()
}

func MakeFirstCommittee(cnt int) *truechain.PbftCommittee{
	if cnt > keysCount {
		return nil
	}
	curCmm := make([]*truechain.CommitteeMember,0,0)
	curCmmCount := cnt
	for i:=0;i<curCmmCount;i++ {
		cc := truechain.CommitteeMember{
			Addr:			"127.0.0.1",
			Port:			16745,
			Nodeid:			hex.EncodeToString(crypto.FromECDSAPub(GetPub(privkeys[i]))),
		}
		curCmm = append(curCmm,&cc)
	}

	cmm := truechain.PbftCommittee{
		No:				1,
		Ct:				time.Now(),
		Lastt:			time.Now(),
		Count:			curCmmCount,
		Lcount:			0,
		Comm:			curCmm,
		Lcomm:			nil,
		Sig:			make([]string,0,0),
	}
	sig := cmm.GetSig()
	for i:=0;i<keysCount/2;i++ {
		k,_ := crypto.Sign(cmm.GetHash(),privkeys[i])
		sig = append(sig,common.ToHex(k))
	}
	cmm.SetSig(sig)
	return &cmm
}

func MakePbftBlock(cmm *truechain.PbftCommittee) *truechain.TruePbftBlock {
	txs := make([]*types.Transaction,0,0)
	txs = append(txs,tx1,tx2)
	pbTxs := make([]*truechain.Transaction,0,0)
	for _,vv := range txs {
		to := make([]byte,0,0)
		if tt := vv.To(); tt != nil {
			to = tt.Bytes()
		}
		v,r,s := vv.RawSignatureValues()
		pbTxs = append(pbTxs,&truechain.Transaction{
			Data:       &truechain.TxData{
				AccountNonce:       vv.Nonce(),
				Price:              vv.GasPrice().Uint64(),
				GasLimit:           new(big.Int).SetUint64(vv.Gas()).Uint64(),
				Recipient:          to,
				Amount:             vv.Value().Uint64(),
				Payload:            vv.Data(),
				V:                  v.Uint64(),
				R:                  r.Uint64(),
				S:                  s.Uint64(),
			},
		})
	}
	// begin make pbft block
	now := time.Now().Unix()
	head := truechain.TruePbftBlockHeader{
		Number:				10,
		GasLimit:			100,
		GasUsed:			80,
		Time:				uint64(now),
	}
	block := truechain.TruePbftBlock{
		Header:			&head,
		Txs:			&truechain.Transactions{Txs:pbTxs},
	}
	msg := truechain.RlpHash(block.Txs)
	//cc := cmm.GetCmm()
	sigs := make([]string,0,0)
	// same priveatekey to sign the message
	for i:=0;i<keysCount/2;i++ {
		sig,err := crypto.Sign(msg[:],privkeys[i])
		if err == nil {
			sigs = append(sigs,common.ToHex(sig))
		}
	}
	block.Sigs = sigs
	return &block
}
*/

func GetPub(priv *ecdsa.PrivateKey) *ecdsa.PublicKey {
	pub := ecdsa.PublicKey{
		Curve: 	priv.Curve,
		X: 		new(big.Int).Set(priv.X),
		Y: 		new(big.Int).Set(priv.Y),
	}
	return &pub
}