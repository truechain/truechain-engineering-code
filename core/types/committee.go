package types

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/log"
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
	// CommitteeSwitchover switch pbft committee
	CommitteeSwitchover
	// CommitteeUpdate update committee members and backups
	CommitteeUpdate
	// CommitteeOver notify current pbft committee end block
	CommitteeOver

	StateUnusedFlag    = 0xa0
	StateUsedFlag      = 0xa1
	StateSwitchingFlag = 0xa2
	StateRemovedFlag   = 0xa3
	StateAppendFlag    = 0xa4
	// health enter type
	TypeFixed  = 0xa5
	TypeWorked = 0xa6
	TypeBack   = 0xa7
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
	Flag      int32
	MType     int32
}

// ElectionCommittee defines election members result
type ElectionCommittee struct {
	Members []*CommitteeMember
	Backups []*CommitteeMember
}

func (c *CommitteeMember) Compared(d *CommitteeMember) bool {
	if c.MType == d.MType && c.Coinbase.String() != d.Coinbase.String() &&
		bytes.Compare(crypto.FromECDSAPub(c.Publickey), crypto.FromECDSAPub(d.Publickey)) == 0 {
		return true
	}
	return false
}

func (c *CommitteeMember) String() string {
	return fmt.Sprintf("F:%d,T:%d,C:%s,P:%s", c.Flag, c.MType, hexutil.Encode(c.Coinbase[:]),
		hexutil.Encode(crypto.FromECDSAPub(c.Publickey)))
}

func (c *CommitteeMember) UnmarshalJSON(input []byte) error {
	type committee struct {
		Address common.Address `json:"address,omitempty"`
		PubKey  *hexutil.Bytes `json:"publickey,omitempty"`
		Flag    int32          `json:"flag,omitempty"`
	}
	var dec committee
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	c.Coinbase = dec.Address
	c.Flag = dec.Flag

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
		hexutil.Encode(c.Coinbase[:]), hexutil.Encode(c.Publickey))
}

type PbftSigns []*PbftSign

type PbftSign struct {
	FastHeight *big.Int
	FastHash   common.Hash // fastblock hash
	Result     uint        // 0--against,1--agree
	Sign       []byte      // sign for fastblock height + hash + result
}

type PbftAgentProxy interface {
	FetchFastBlock(committeeId *big.Int, infos *SwitchInfos) (*Block, error)
	VerifyFastBlock(*Block, bool) (*PbftSign, error)
	BroadcastFastBlock(*Block)
	BroadcastConsensus(block *Block) error
	GetCurrentHeight() *big.Int
	GetSeedMember() []*CommitteeMember
}

type PbftServerProxy interface {
	PutCommittee(committeeInfo *CommitteeInfo) error
	UpdateCommittee(info *CommitteeInfo) error
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
	EndHeight   *big.Int
	Members     []*CommitteeMember
	BackMembers []*CommitteeMember
}

func (c *CommitteeInfo) String() string {
	if c.Members != nil {
		memStrings := make([]string, len(c.Members))
		backMemStrings := make([]string, len(c.BackMembers))
		for i, m := range c.Members {
			if m == nil {
				memStrings[i] = "nil-Member"
			} else {
				memStrings[i] = m.String()
			}
		}
		for i, m := range c.BackMembers {
			if m == nil {
				backMemStrings[i] = "nil-Member"
			} else {
				backMemStrings[i] = m.String()
			}
		}
		return fmt.Sprintf("CommitteeInfo{ID:%s,SH:%s,M:{%s},BM:{%s}}", c.Id, c.StartHeight,
			strings.Join(memStrings, "\n  "), strings.Join(backMemStrings, "\n  "))
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
	if e := rlp.Encode(hw, x); e != nil {
		log.Warn("RlpHash", "error", e.Error())
	}
	hw.Sum(h[:0])
	return h
}

// SwitchEnter is the enter inserted in block when committee member changed
type SwitchEnter struct {
	Pk   []byte
	Flag uint32
}

// Hash return SwitchInfos hash bytes
func (s *SwitchInfos) Hash() common.Hash {
	return rlpHash(s)
}

func (s *SwitchEnter) String() string {
	if s == nil {
		return "switchEnter-nil"
	}
	return fmt.Sprintf("p:%s,s:%d", hexutil.Encode(s.Pk), s.Flag)
}
func (s *SwitchEnter) Equal(other *SwitchEnter) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	return bytes.Equal(s.Pk, other.Pk) && s.Flag == other.Flag
}

type SwitchEnters []*SwitchEnter

// Equal will equal not require item index
func (s SwitchEnters) Equal(other SwitchEnters) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	if len(s) != len(other) {
		return false
	}

	for _, v1 := range s {
		equal := false
		for _, v2 := range other {
			if v1.Equal(v2) {
				equal = true
			}
		}
		if !equal {
			return false
		}
	}
	return true
}

// SwitchInfos is the infos inserted in block when committee member changed
type SwitchInfos struct {
	CID  uint64
	Vals []*SwitchEnter
}

func (s *SwitchInfos) String() string {
	if s == nil {
		return "switchInfo-nil"
	}
	memStrings := make([]string, len(s.Vals))
	for i, m := range s.Vals {
		if m == nil {
			memStrings[i] = "nil-Member"
		} else {
			memStrings[i] = m.String()
		}
	}
	return fmt.Sprintf("SwitchInfos{CID:%d,Vals:{%s}}", s.CID, strings.Join(memStrings, "\n  "))
}

func (s *SwitchInfos) Equal(other *SwitchInfos) bool {
	if s == nil && other == nil {
		return true
	}
	if s == nil || other == nil {
		return false
	}
	return s.CID == other.CID && SwitchEnters(s.Vals).Equal(SwitchEnters(other.Vals))
}
