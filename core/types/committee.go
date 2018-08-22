package types

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/crypto"
	"log"
	"math/big"
)

const (
	CommitteeStart      = iota // start pbft consensus
	CommitteeStop              // stop pbft consensus
	CommitteeSwitchover        //switch pbft committee
)

type CommitteeMembers []*CommitteeMember

type CommitteeMember struct {
	Coinbase  common.Address
	Publickey *ecdsa.PublicKey
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
	IP        string
	Port      uint
	Coinbase  common.Address
	Publickey []byte
}

type PbftSigns []*PbftSign

// DecodeRLP decodes the Ethereum
//func (p *PbftSign) DecodeRLP(s *rlp.Stream) error {
//	var sign struct {
//		FastHeight *big.Int
//		FastHash   common.Hash
//		Result     uint
//		Sign       []byte
//	}
//	if err := s.Decode(&sign); err != nil {
//		return err
//	}
//	p.FastHeight, p.Result, p.FastHash, p.Sign = sign.FastHeight, sign.Result, sign.FastHash, sign.Sign
//	return nil
//}

// EncodeRLP serializes b into the Ethereum RLP CryNodeInfo format.
//func (p *PbftSign) EncodeRLP(w io.Writer) error {
//	return rlp.Encode(w,[]interface{}{ p.FastHeight,p.Result,
//		p.FastHash, p.Sign,
//	})
//}

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
	BroadcastSign(sign *PbftSign, block *Block) error
}

type PbftServerProxy interface {
	PutCommittee(committeeInfo *CommitteeInfo) error
	PutNodes(id *big.Int, nodes []*CommitteeNode) error
	Notify(id *big.Int, action int) error
}

/*func (voteSign *PbftSign) PrepareData() []byte {
	result,_ :=rlp.EncodeToBytes(voteSign.Result)
	height,_ :=rlp.EncodeToBytes(voteSign.FastHeight)
	data := bytes.Join(
		[][]byte{
			voteSign.FastHash[:],
			result,
			height,
		},
		[]byte{},
	)
	return data
}*/

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
	return h.HashWithNoSign()
}

func (h *PbftSign) HashWithNoSign() common.Hash {
	return rlpHash([]interface{}{
		h.FastHeight,
		h.FastHash,
		h.Result,
	})
}

type CommitteeInfo struct {
	Id      *big.Int
	Members []*CommitteeMember
}
