/*
Copyright (c) 2018 TrueChain Foundation
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package truechain

import (
	// "reflect"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	// "github.com/ethereum/go-ethereum/accounts"
	// "github.com/ethereum/go-ethereum/ethdb"
	// "github.com/ethereum/go-ethereum/event"
	// "github.com/ethereum/go-ethereum/p2p"
	// "github.com/ethereum/go-ethereum/rpc"
)


type TruePbftNode struct {
	Addr 		string 		// node ip like 127.0.0.1,the port use default
	Pubkey  	string		// 
	Privkey		string  	//
}
type TruePbftBlockHeader struct {
	Number      *big.Int       // block height out of pbft 
	GasLimit    *big.Int       // gaslimit in block include bonus tx
	GasUsed     *big.Int       // gasused in block
	Time        *big.Int       // generate time
}

type TruePbftBlock struct {
	header       *TruePbftBlockHeader
	Transactions []*types.Transaction		// raw tx（include bonus tx）
	sig		     []*string					// sign with all members
}

type StandbyInfo struct {
	nodeid		string			// pubkey节点的公钥
	coinbase	string			// 出块节点的奖励地址
	addr		string 			
	port		int
	height		*big.Int		// 对应于块高度
	comfire		bool			// 经过12块确认
}
type CommitteeMember struct {
	nodeid		string			// pubkey节点的公钥
	addr		string 			
	port		int
}

// type HybridConsensus interface {
// 	// main chain set node to the py-pbft
// 	MembersNodes(nodes []*TruePbftNode) error
// 	// main chain set node to the py-pbft
// 	SetTransactions(txs []*types.Transaction) error

// 	PutBlock(block *TruePbftBlock)  error

// 	ViewChange() error

// 	Start() error

// 	Stop() error
// 	// tx validation in py-pbft, Temporary slightly
// }