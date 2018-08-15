package types

import (
	"bytes"
	"log"
	"crypto/ecdsa"
	"encoding/binary"
	"math/big"
	"encoding/json"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/crypto"
	"io"
	"github.com/truechain/truechain-engineering-code/rlp"
)


const (
	CommitteeStart      = iota // start pbft consensus
	CommitteeStop              // stop pbft consensus
	CommitteeSwitchover        //switch pbft committee
)


type CommitteeMembers []*CommitteeMember

type CommitteeMember struct {
	Coinbase  common.Address
	Publickey  *ecdsa.PublicKey
}


func (g *CommitteeMember) UnmarshalJSON(input []byte) error {
	type committee struct {
		Address common.Address `json:"address,omitempty"`
		PubKey  *hexutil.Bytes `json:"publickey,omitempty"`
	}
	var dec committee
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	g.Coinbase = dec.Address

	var err error
	if dec.PubKey != nil {
		g.Publickey, err = crypto.UnmarshalPubkey(*dec.PubKey)
		if err != nil {
			return err
		}
	}
	return nil
}

type CommitteeNode struct {
	IP   string
	Port uint
	//CM   *CommitteeMember
	Coinbase  common.Address
	Publickey  []byte
}

type PbftSigns []*PbftSign

// DecodeRLP decodes the Ethereum
func (p *PbftSign) DecodeRLP(s *rlp.Stream) error {
	err := s.Decode(p)
	return err
}

// EncodeRLP serializes b into the Ethereum RLP CryNodeInfo format.
func (p *PbftSign) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, p)
}

type PbftSign struct {
	FastHeight *big.Int
	FastHash   common.Hash // fastblock hash
	Result     uint        // 0--agree,1--against
	Sign       []byte      // sign for fastblock height + hash + result
}

type PbftAgentProxy interface {
	FetchFastBlock() (*Block, error)
	VerifyFastBlock(*Block) error
	BroadcastFastBlock(*Block) error
	BroadcastSign(sign *PbftSign,block *Block) error
}

type PbftServerProxy interface {
	PutCommittee(committeeInfo *CommitteeInfo) error
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


// Hash returns the block hash of the PbftSign, which is simply the keccak256 hash of its
// RLP encoding.
func (h *PbftSign) Hash() common.Hash {
	return rlpHash(h)
}

type CommitteeInfo struct {
	Id *big.Int
	Members []*CommitteeMember
}
