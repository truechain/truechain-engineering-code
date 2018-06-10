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
	"net"
    "math/big"
    "errors"
    
    "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/common"
	// "github.com/ethereum/go-ethereum/accounts"
	// "github.com/ethereum/go-ethereum/ethdb"
	// "github.com/ethereum/go-ethereum/event"
	// "github.com/ethereum/go-ethereum/p2p"
	// "github.com/ethereum/go-ethereum/rpc"
)

// all functions of sdm not thread-safe

func (t *TrueHybrid) AddStandbyMember(node *StandbyInfo) error {
    if len(t.sdm) >= t.sdmsize {
		t.sdm = append(t.sdm[:0],t.sdm[1:])
	} 
	// verfiy and add 
	t.sdm = append(t.sdm,node)
	return nil
}
func (t *TrueHybrid) verifyCheck() error {
	return nil
}
func (t *TrueHybrid) VerifyNode(node *StandbyInfo) error {
	return nil
} 
func (t *TrueHybrid) Vote(num int) ([]*CommitteeMember,error) {
	vv := make([]*CommitteeMember,0,0)
	i := 0
	for _,v := range t.sdm {
		if i >= num {
			break
		} else {
			vv = append(vv,&CommitteeMember{
				nodeid:		v.nodeid,
				addr:		v.addr,			
				port:		v.port,			
			})
		}
	}
	return vv,nil
}
// after success pow,send the node by p2p  
func MakeSignedStandbyNode() ([]byte,error) {
	node := make([]byte,0,0)
	return node,nil
}
