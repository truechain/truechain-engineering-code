package types

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
	"strings"
	"time"
)

const (
	// CommitteeStart start pbft consensus
	CommitteeStart = iota
	// CommitteeStop stop pbft consensus
	CommitteeStop
	//CommitteeSwitchover switch pbft committee
	CommitteeSwitchover
	// CommitteeOver notify current pbft committee end block
	CommitteeOver
)

const (
	//VoteAgreeAgainst vote sign with against
	VoteAgreeAgainst = iota
	//VoteAgree vote sign with agree
	VoteAgree
)

//CommitteeMembers committee members
type CommitteeMembers []*CommitteeMember

type CommitteeMember struct {
	Coinbase  common.Address
	Publickey *ecdsa.PublicKey
}

func (c *CommitteeMember) String() string {
	return fmt.Sprintf("C:%s,P:%s", common.ToHex(c.Coinbase[:]),
		common.ToHex(crypto.FromECDSAPub(c.Publickey)))
}

func (c *CommitteeMember) UnmarshalJSON(input []byte) error {
	type committee struct {
		Address common.Address `json:"address,omitempty"`
		PubKey  *hexutil.Bytes `json:"publickey,omitempty"`
	}
	var dec committee
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	c.Coinbase = dec.Address

	var err error
	if dec.PubKey != nil {
		c.Publickey, err = crypto.UnmarshalPubkey(*dec.PubKey)
		if err != nil {
			return err
		}
	}
	return nil
}

//CommitteeNode contains  main info of committee node
type CommitteeNode struct {
	IP        string
	Port      uint
	Port2     uint
	Coinbase  common.Address
	Publickey []byte
}

func (c *CommitteeNode) String() string {
	return fmt.Sprintf("NodeInfo:{IP:%s,P1:%v,P2:%v,Coinbase:%s,P:%s}", c.IP, c.Port, c.Port2,
		common.ToHex(c.Coinbase[:]), common.ToHex(c.Publickey))
}

type PbftSigns []*PbftSign

type PbftSign struct {
	FastHeight *big.Int
	FastHash   common.Hash // fastblock hash
	Result     uint        // 0--against,1--agree
	Sign       []byte      // sign for fastblock height + hash + result
}

type PbftAgentProxy interface {
	FetchFastBlock(committeeId *big.Int) (*Block, error)
	VerifyFastBlock(*Block) (*PbftSign, error)
	BroadcastFastBlock(*Block)
	BroadcastConsensus(block *Block) error
	GetCurrentHeight() *big.Int
}

type PbftServerProxy interface {
	PutCommittee(committeeInfo *CommitteeInfo) error
	PutNodes(id *big.Int, nodes []*CommitteeNode) error
	Notify(id *big.Int, action int) error
	SetCommitteeStop(committeeId *big.Int, stop uint64) error
	GetCommitteeStatus(committeeID *big.Int) map[string]interface{}
}

// Hash returns the block hash of the PbftSign, which is simply the keccak256 hash of its
// RLP encoding.
func (h *PbftSign) Hash() common.Hash {
	return rlpHash(h)
}

//HashWithNoSign returns the hash which PbftSign without sign
func (h *PbftSign) HashWithNoSign() common.Hash {
	return rlpHash([]interface{}{
		h.FastHeight,
		h.FastHash,
		h.Result,
	})
}

type CommitteeInfo struct {
	Id          *big.Int
	StartHeight *big.Int
	Members     []*CommitteeMember
}

func (c *CommitteeInfo) String() string {
	if c.Members != nil {
		memStrings := make([]string, len(c.Members))
		for i, m := range c.Members {
			if m == nil {
				memStrings[i] = "nil-Member"
			} else {
				memStrings[i] = m.String()
			}
		}
		return fmt.Sprintf("CommitteeInfo{ID:%s,SH:%s,M:{%s}}", c.Id, c.StartHeight, strings.Join(memStrings, "\n  "))
	}
	return fmt.Sprintf("CommitteeInfo{ID:%s,SH:%s}", c.Id, c.StartHeight)
}

//EncryptCommitteeNode represent a committee member encrypt info
// which encrypt committeeNode with member Publickey
type EncryptCommitteeNode []byte
type Sign []byte

//EncryptNodeMessage  all information of the committee
type EncryptNodeMessage struct {
	CreatedAt   time.Time
	CommitteeID *big.Int
	Nodes       []EncryptCommitteeNode
	Sign        //sign msg
}

func (c *EncryptNodeMessage) HashWithoutSign() common.Hash {
	return RlpHash([]interface{}{
		c.Nodes,
		c.CommitteeID,
	})
}

func (c *EncryptNodeMessage) Hash() common.Hash {
	return RlpHash(c)
}

func RlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}
