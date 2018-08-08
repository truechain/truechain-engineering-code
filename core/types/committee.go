package types

import (
	"crypto/ecdsa"
	"encoding/json"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"math/big"
)

// Committee is an committee info in the state of the genesis block.
type Committee struct {
	Address common.Address `json:"address,omitempty"`
	PubKey  []byte         `json:"pubKey,omitempty"`
}
type PbftVoteSign struct {
	Result     uint        // 0--agree,1--against
	FastHeight *big.Int    // fastblock height
	Msg        common.Hash // hash(FastHeight+fasthash+ecdsa.PublicKey+Result)
	Sig        []byte      // sign for SigHash
}

//Commission verification fast black result
type CommitteeFastSignResult struct {
	Address common.Address `json:"address,omitempty"`
	Result  bool           `json:"result,omitempty"` //sign is true
}

type CommitteeMember struct {
	ip       []string
	port     uint
	coinbase common.Hash
	pubkey   *ecdsa.PublicKey
}

func (g *Committee) UnmarshalJSON(input []byte) error {
	type Committee struct {
		Address common.Address `json:"address,omitempty"`
		PubKey  *hexutil.Bytes `json:"pubKey,omitempty"`
	}
	var dec Committee
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	//if dec.Address != nil {
	g.Address = dec.Address
	//}
	if dec.PubKey != nil {
		g.PubKey = *dec.PubKey
	}
	return nil
}
