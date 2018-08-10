package types

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"log"
	"math/big"
)

// Committee is an committee info in the state of the genesis block.
type Committee struct {
	Address common.Address `json:"address,omitempty"`
	PubKey  []byte         `json:"pubKey,omitempty"`
}

const (
	CommitteeStart      = iota // start pbft consensus
	CommitteeStop              // stop pbft consensus
	CommitteeSwitchover        //switch pbft committee
)

type CommitteeMember struct {
	Coinbase  common.Address
	Publickey *ecdsa.PublicKey
}

type CommitteeNode struct {
	IP   string
	Port uint
	CM   *CommitteeMember
}

type PbftSigns []*PbftSign

type PbftSign struct {
	FastHeight *big.Int
	FastHash   common.Hash // fastblock hash
	Result     uint        // 0--agree,1--against
	Sign       []byte      // sign for fastblock height + hash + result
}

type PbftAgentProxy interface {
	FetchFastBlock() (*FastBlock, error)
	VerifyFastBlock(*FastBlock) error
	BroadcastFastBlock(*FastBlock) error
	BroadcastSign(sign []*PbftSign,block *FastBlock) error
}

type PbftServerProxy interface {
	PutCommittee(id *big.Int, members []*CommitteeMember) error
	PutNodes(id *big.Int, nodes []*CommitteeNode) error
	Notify(id *big.Int, action int) error

}

func (voteSign *PbftSign) PrepareData() []byte {
	data := bytes.Join(
		[][]byte{
			voteSign.FastHash[:],
			IntToHex(voteSign.Result),
			IntToHex(voteSign.FastHeight),
		},
		[]byte{},
	)
	return data
}

// IntToHex converts an int64 to a byte array
func IntToHex(num interface{}) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
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
