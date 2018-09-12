package types

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	//"sync"
	//"time"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	ctypes "github.com/truechain/truechain-engineering-code/core/types"
)

var (
	PeerStateKey = "ConsensusReactor.peerState"
)

// MakePartSet returns a PartSet containing parts of a serialized block.
// This is the form in which the block is gossipped to peers.
// CONTRACT: partSize is greater than zero.
// func (b *Block) MakePartSet(partSize int) *PartSet {
// 	if b == nil {
// 		return nil
// 	}
// 	b.mtx.Lock()
// 	defer b.mtx.Unlock()

// 	// We prefix the byte length, so that unmarshaling
// 	// can easily happen via a reader.
// 	bz, err := help.MarshalBinary(b)
// 	if err != nil {
// 		panic(err)
// 	}
// 	return NewPartSetFromData(bz, partSize)
// }

//-------------------------------------

// Commit contains the evidence that a block was committed by a set of validators.
// NOTE: Commit is empty for height 1, but never nil.
type Commit struct {
	// NOTE: The Precommits are in order of address to preserve the bonded ValidatorSet order.
	// Any peer with a block can gossip precommits by index with a peer without recalculating the
	// active ValidatorSet.
	BlockID    BlockID `json:"block_id"`
	Precommits []*Vote `json:"precommits"`

	// Volatile
	firstPrecommit *Vote
	hash           help.HexBytes
	bitArray       *help.BitArray
}

// FirstPrecommit returns the first non-nil precommit in the commit.
// If all precommits are nil, it returns an empty precommit with height 0.
func (commit *Commit) FirstPrecommit() *Vote {
	if len(commit.Precommits) == 0 {
		return nil
	}
	if commit.firstPrecommit != nil {
		return commit.firstPrecommit
	}
	for _, precommit := range commit.Precommits {
		if precommit != nil {
			commit.firstPrecommit = precommit
			return precommit
		}
	}
	return &Vote{
		Type: VoteTypePrecommit,
	}
}

// Height returns the height of the commit
func (commit *Commit) Height() int64 {
	if len(commit.Precommits) == 0 {
		return 0
	}
	return commit.FirstPrecommit().Height
}

// Round returns the round of the commit
func (commit *Commit) Round() int {
	if len(commit.Precommits) == 0 {
		return 0
	}
	return commit.FirstPrecommit().Round
}

// Type returns the vote type of the commit, which is always VoteTypePrecommit
func (commit *Commit) Type() byte {
	return VoteTypePrecommit
}

// Size returns the number of votes in the commit
func (commit *Commit) Size() int {
	if commit == nil {
		return 0
	}
	return len(commit.Precommits)
}

// BitArray returns a BitArray of which validators voted in this commit
func (commit *Commit) BitArray() *help.BitArray {
	if commit.bitArray == nil {
		commit.bitArray = help.NewBitArray(len(commit.Precommits))
		for i, precommit := range commit.Precommits {
			// TODO: need to check the BlockID otherwise we could be counting conflicts,
			// not just the one with +2/3 !
			commit.bitArray.SetIndex(i, precommit != nil)
		}
	}
	return commit.bitArray
}

// GetByIndex returns the vote corresponding to a given validator index
func (commit *Commit) GetByIndex(index int) *Vote {
	return commit.Precommits[index]
}

// IsCommit returns true if there is at least one vote
func (commit *Commit) IsCommit() bool {
	return len(commit.Precommits) != 0
}

// ValidateBasic performs basic validation that doesn't involve state data.
func (commit *Commit) ValidateBasic() error {
	if commit.BlockID.IsZero() {
		return errors.New("Commit cannot be for nil block")
	}
	if len(commit.Precommits) == 0 {
		return errors.New("No precommits in commit")
	}
	height, round := commit.Height(), commit.Round()

	// validate the precommits
	for _, precommit := range commit.Precommits {
		// It's OK for precommits to be missing.
		if precommit == nil {
			continue
		}
		// Ensure that all votes are precommits
		if precommit.Type != VoteTypePrecommit {
			return fmt.Errorf("Invalid commit vote. Expected precommit, got %v",
				precommit.Type)
		}
		// Ensure that all heights are the same
		if precommit.Height != height {
			return fmt.Errorf("Invalid commit precommit height. Expected %v, got %v",
				height, precommit.Height)
		}
		// Ensure that all rounds are the same
		if precommit.Round != round {
			return fmt.Errorf("Invalid commit precommit round. Expected %v, got %v",
				round, precommit.Round)
		}
	}
	return nil
}

// Hash returns the hash of the commit
func (commit *Commit) Hash() help.HexBytes {
	if commit == nil {
		return nil
	}
	if commit.hash == nil {
		bs := make([]common.Hash, len(commit.Precommits))
		for i, precommit := range commit.Precommits {
			bs[i] = help.RlpHash(precommit)
		}
		commit.hash = help.RlpHash(bs)[:]
	}
	return commit.hash
}

// StringIndented returns a string representation of the commit
func (commit *Commit) StringIndented(indent string) string {
	if commit == nil {
		return "nil-Commit"
	}
	precommitStrings := make([]string, len(commit.Precommits))
	for i, precommit := range commit.Precommits {
		precommitStrings[i] = precommit.String()
	}
	return fmt.Sprintf(`Commit{
%s  BlockID:    %v
%s  Precommits: %v
%s}#%v`,
		indent, commit.BlockID,
		indent, strings.Join(precommitStrings, "\n"+indent+"  "),
		indent, commit.hash)
}

//-----------------------------------------------------------------------------

// SignedHeader is a header along with the commits that prove it
// type SignedHeader struct {
// 	Header *Header `json:"header"`
// 	Commit *Commit `json:"commit"`
// }

//--------------------------------------------------------------------------------

// BlockID defines the unique ID of a block as its Hash and its PartSetHeader
type BlockID struct {
	Hash        help.HexBytes  `json:"hash"`
	PartsHeader PartSetHeader `json:"parts"`
}

// IsZero returns true if this is the BlockID for a nil-block
func (blockID BlockID) IsZero() bool {
	return len(blockID.Hash) == 0 && blockID.PartsHeader.IsZero()
}

// Equals returns true if the BlockID matches the given BlockID
func (blockID BlockID) Equals(other BlockID) bool {
	return bytes.Equal(blockID.Hash, other.Hash) &&
		blockID.PartsHeader.Equals(other.PartsHeader)
}

// Key returns a machine-readable string representation of the BlockID
func (blockID BlockID) Key() string {
	bz, err := help.MarshalBinaryBare(blockID.PartsHeader)
	if err != nil {
		panic(err)
	}
	return string(blockID.Hash) + string(bz)
}

// String returns a human readable string representation of the BlockID
func (blockID BlockID) String() string {
	return fmt.Sprintf(`%v:%v`, blockID.Hash, blockID.PartsHeader)
}

//-------------------------------------------------------
const (
	MaxLimitBlockStore = 2000
	MaxBlockBytes = 1048510			// lMB
)

type BlockMeta struct {
	Block 			*ctypes.Block
	BlockID			*BlockID
	BlockPacks		*PartSet
	SeenCommit		*Commit
}
type BlockStore struct {
	blocks 		map[int64]*BlockMeta
}

func NewBlockStore() *BlockStore {
	return &BlockStore {
		blocks:		make(map[int64]*BlockMeta),
	}
}
func (b *BlockStore) LoadBlockMeta(height int64) *BlockMeta {
	if v,ok := b.blocks[height]; ok{
		return v
	}
	return nil
}
func (b *BlockStore) LoadBlockPart(height int64, index int) *Part {
	if v,ok := b.blocks[height]; ok {
		return v.BlockPacks.GetPart(index)
	}
	return nil
}
func (b *BlockStore) MaxBlockHeight() int64 {
	// ss.blockLock.Lock()
	// defer ss.blockLock.Unlock()
	var cur int64 = 0
	//var fb *ctypes.Block = nil
	for k, _ := range b.blocks {
		if cur == 0 {
			cur = k
		}
		if cur < k {
			cur = k
		}
	}
	return cur
}
func (b *BlockStore) LoadBlockCommit(height int64) *Commit {
	if v,ok := b.blocks[height]; ok{
		return v.SeenCommit
	}
	return nil
}
func (b *BlockStore) SaveBlock(block *ctypes.Block, blockParts *PartSet, seenCommit *Commit){

}