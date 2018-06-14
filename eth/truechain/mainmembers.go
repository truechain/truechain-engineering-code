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
	"encoding/hex"
	"net"
    "math/big"
    "errors"
    
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p"
)
// all function was not tread-safe
func (t *TrueHybrid) SyncMainMembers() {
	// sync current CommitteeMember 
	data,err := t.curCmm.ToByte()
	if err != nil {
		// send data 
	}
	// sync old CommitteeMember???
}
// verify the block which from pbft Committee
func (t *TrueHybrid) CheckPbftBlock(block *TruePbftBlock) error {
	// check with current committee 
	// for _,v := range block.
	// check with old committee
	return nil
}
func (t *TrueHybrid) InPbftCommittee() bool {
	_,nodeid,_ := getNodeID()
	if nodeid == t.curCmm.Nodeid {
		return true
	}
	return false
}
func getNodeID(/*server *p2p.Server*/) (string,string,string) {
	// get p2p server later
	var server *p2p.Server  // tmp
	ip := server.NodeInfo().IP
	priv := hex.EncodeToString(crypto.FromECDSA(server.PrivateKey))
	pub := hex.EncodeToString(crypto.FromECDSAPub(server.PrivateKey.Public()))
	return ip,pub,priv
}