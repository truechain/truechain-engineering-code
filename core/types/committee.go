package types

import (
	"encoding/json"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
)

// Committee is an committee info in the state of the genesis block.
type Committee struct {
	Address common.Address `json:"address,omitempty"`
	PubKey  []byte         `json:"pubKey,omitempty"`
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
