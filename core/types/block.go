// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package types contains data types related to Ethereum consensus.
package types

import (
	"crypto/ecdsa"
	"encoding/binary"
	"io"
	"math/big"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"

	"bytes"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	EmptyRootHash  = DeriveSha(Transactions{})
	EmptyUncleHash = CalcUncleHash(nil)
	EmptySignHash  = CalcSignHash(nil)
)

// A BlockNonce is a 64-bit hash which proves (combined with the
// mix-hash) that a sufficient amount of computation has been carried
// out on a block.
type BlockNonce [8]byte

// EncodeNonce converts the given integer to a block nonce.
func EncodeNonce(i uint64) BlockNonce {
	var n BlockNonce
	binary.BigEndian.PutUint64(n[:], i)
	return n
}

// Uint64 returns the integer value of a block nonce.
func (n BlockNonce) Uint64() uint64 {
	return binary.BigEndian.Uint64(n[:])
}

// MarshalText encodes n as a hex string with 0x prefix.
func (n BlockNonce) MarshalText() ([]byte, error) {
	return hexutil.Bytes(n[:]).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *BlockNonce) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("BlockNonce", input, n[:])
}

// Fruits is a wrapper around a fruit array to implement DerivableList.
type Fruits []*SnailBlock

// Len returns the number of fruits in this list.
func (fs Fruits) Len() int { return len(fs) }

// GetRlp returns the RLP encoding of one fruit from the list.
func (fs Fruits) GetRlp(i int) []byte {
	bytes, err := rlp.EncodeToBytes(fs[i])
	if err != nil {
		panic(err)
	}
	return bytes
}

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

func CalcUncleHash(uncles []*Header) common.Hash {
	return rlpHash(uncles)
}

func CalcSignHash(signs []*PbftSign) common.Hash {
	return rlpHash(signs)
}

type Blocks []*Block

type BlockBy func(b1, b2 *Block) bool

func (self BlockBy) Sort(blocks Blocks) {
	bs := blockSorter{
		blocks: blocks,
		by:     self,
	}
	sort.Sort(bs)
}

type blockSorter struct {
	blocks Blocks
	by     func(b1, b2 *Block) bool
}

func (self blockSorter) Len() int { return len(self.blocks) }
func (self blockSorter) Swap(i, j int) {
	self.blocks[i], self.blocks[j] = self.blocks[j], self.blocks[i]
}
func (self blockSorter) Less(i, j int) bool { return self.by(self.blocks[i], self.blocks[j]) }

func Number(b1, b2 *Block) bool { return b1.header.Number.Cmp(b2.header.Number) < 0 }

////////////////////////////////////////////////////////////////////////////////

// fast chain block structure
//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

// Header represents a block header in the true Fastblockchain.
type Header struct {
	ParentHash    common.Hash    `json:"parentHash"       gencodec:"required"`
	Root          common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash        common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash   common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	CommitteeHash common.Hash    `json:"committeeRoot"     gencodec:"required"`
	Proposer      common.Address `json:"maker"            gencodec:"required"`
	Bloom         Bloom          `json:"logsBloom"        gencodec:"required"`
	SnailHash     common.Hash    `json:"snailHash"        gencodec:"required"`
	SnailNumber   *big.Int       `json:"snailNumber"      gencodec:"required"`
	Number        *big.Int       `json:"number"           gencodec:"required"`
	GasLimit      uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed       uint64         `json:"gasUsed"          gencodec:"required"`
	Time          *big.Int       `json:"timestamp"        gencodec:"required"`
	Extra         []byte         `json:"extraData"        gencodec:"required"`
}

// field type overrides for gencodec
type headerMarshaling struct {
	SnailNumber *hexutil.Big
	Number      *hexutil.Big
	GasLimit    hexutil.Uint64
	GasUsed     hexutil.Uint64
	Time        *hexutil.Big
	Extra       hexutil.Bytes
	Hash        common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	return rlpHash(h)
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *Header) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+
		(h.SnailNumber.BitLen()+h.Number.BitLen()+h.Time.BitLen())/8)
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
type Body struct {
	Transactions []*Transaction
	Signs        []*PbftSign
	Infos        *SwitchInfos
}

// BlockReward
type BlockReward struct {
	FastHash    common.Hash `json:"FastHash"        gencodec:"required"`
	FastNumber  *big.Int    `json:"FastNumber"      gencodec:"required"`
	SnailHash   common.Hash `json:"SnailHash"        gencodec:"required"`
	SnailNumber *big.Int    `json:"SnailNumber"      gencodec:"required"`
}

// Block represents an entire block in the Ethereum blockchain.
type Block struct {
	header       *Header
	transactions Transactions

	uncles []*Header // reserved for compile

	signs PbftSigns
	infos *SwitchInfos
	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	// td *big.Int

	// These fields are used by package etrue to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// NewBlock creates a new fast block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs
// and receipts.
func NewBlock(header *Header, txs []*Transaction, receipts []*Receipt, signs []*PbftSign, infos *SwitchInfos) *Block {
	b := &Block{
		header: CopyHeader(header),
		infos:  &SwitchInfos{},
	}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs))
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptHash = EmptyRootHash
	} else {
		b.header.ReceiptHash = DeriveSha(Receipts(receipts))
		b.header.Bloom = CreateBloom(receipts)
	}

	if len(signs) != 0 {
		b.signs = make(PbftSigns, len(signs))
		copy(b.signs, signs)
	}
	if infos != nil {
		b.infos.CID = infos.CID
		b.infos.Vals = make([]*SwitchEnter, len(infos.Vals))
		copy(b.infos.Vals, infos.Vals)
	}
	b.header.CommitteeHash = rlpHash(b.infos)
	return b
}

// SetLeaderSign keep the sign on the head for proposal
func (b *Body) SetLeaderSign(sign *PbftSign) {
	signP := *sign
	b.Signs = []*PbftSign{}
	b.Signs = append(b.Signs, &signP)
}

// GetLeaderSign get the sign for proposal
func (b *Body) GetLeaderSign() *PbftSign {
	if len(b.Signs) > 0 {
		return b.Signs[0]
	}
	return nil
}

// GetSwitchInfo get info for shift committee
func (b *Body) GetSwitchInfo() *SwitchInfos {
	return b.Infos
}

// SetSwitchInfo set info for shift committee
func (b *Body) SetSwitchInfo(infos *SwitchInfos) {
	b.Infos = infos
}

// NewBlockWithHeader creates a fast block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}

// CopyHeader creates a deep copy of a fast block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {
	cpy := *h
	if cpy.Time = new(big.Int); h.Time != nil {
		cpy.Time.Set(h.Time)
	}
	if cpy.SnailNumber = new(big.Int); h.SnailNumber != nil {
		cpy.SnailNumber.Set(h.SnailNumber)
	}
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}

// "external" block encoding. used for etrue protocol, etc.
type extblock struct {
	Header *Header
	Txs    []*Transaction
	Signs  []*PbftSign
	Infos  *SwitchInfos
}

// DecodeRLP decodes the Ethereum
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.transactions, b.signs, b.infos = eb.Header, eb.Txs, eb.Signs, eb.Infos
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Txs:    b.transactions,
		Signs:  b.signs,
		Infos:  b.infos,
	})
}

func (b *Block) Uncles() []*Header          { return b.uncles }
func (b *Block) Transactions() Transactions { return b.transactions }
func (b *Block) SignedHash() common.Hash    { return rlpHash([]interface{}{b.header, b.transactions}) }
func (b *Block) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range b.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}
func (b *Block) Number() *big.Int      { return new(big.Int).Set(b.header.Number) }
func (b *Block) GasLimit() uint64      { return b.header.GasLimit }
func (b *Block) GasUsed() uint64       { return b.header.GasUsed }
func (b *Block) SnailNumber() *big.Int { return new(big.Int).Set(b.header.SnailNumber) }
func (b *Block) Time() *big.Int        { return new(big.Int).Set(b.header.Time) }

func (b *Block) Proposer() common.Address   { return b.header.Proposer }
func (b *Block) NumberU64() uint64          { return b.header.Number.Uint64() }
func (b *Block) SnailHash() common.Hash     { return b.header.SnailHash }
func (b *Block) Bloom() Bloom               { return b.header.Bloom }
func (b *Block) Coinbase() common.Address   { return common.Address{} }
func (b *Block) Root() common.Hash          { return b.header.Root }
func (b *Block) ParentHash() common.Hash    { return b.header.ParentHash }
func (b *Block) TxHash() common.Hash        { return b.header.TxHash }
func (b *Block) ReceiptHash() common.Hash   { return b.header.ReceiptHash }
func (b *Block) UncleHash() common.Hash     { return common.Hash{} }
func (b *Block) Extra() []byte              { return common.CopyBytes(b.header.Extra) }
func (b *Block) Signs() []*PbftSign         { return b.signs }
func (b *Block) Header() *Header            { return CopyHeader(b.header) }
func (b *Block) CommitteeHash() common.Hash { return b.header.CommitteeHash }
func (b *Block) SwitchInfos() *SwitchInfos  { return b.infos }

// Body returns the non-header content of the block.
func (b *Block) Body() *Body { return &Body{b.transactions, b.signs, b.infos} }

func (b *Block) AppendSign(sign *PbftSign) {
	signP := CopyPbftSign(sign)
	b.signs = append(b.signs, signP)
}

func (b *Block) SetSign(signs []*PbftSign) {
	b.signs = append(make([]*PbftSign, 0), signs...)
}

func (b *Block) AppendSigns(signs []*PbftSign) {
	if len(b.signs) <= 0 {
		b.signs = signs
		return
	}

	signP := CopyPbftSign(b.signs[0])
	signN := make([]*PbftSign, 0, len(signs))
	signN = append(signN, signP)
	for _, sign := range signs {
		if bytes.Equal(sign.Sign, signP.Sign) {
			continue
		}
		signN = append(signN, sign)
	}

}

func (b *Block) GetLeaderSign() *PbftSign {
	if len(b.signs) > 0 {
		return b.signs[0]
	}
	return nil
}

func (b *Block) IsAward() bool {
	if b.SnailHash() != *new(common.Hash) && b.SnailNumber() != nil {
		return true
	}
	return false
}

func (b *Block) IsSwitch() bool {
	if b.infos != nil && len(b.infos.Vals) > 0 {
		return true
	}
	return false
}

//Condition when proposal block award or switch is not nil
func (b *Block) IsProposal() bool {
	if b.IsAward() || b.IsSwitch() {
		return true
	}
	return false
}

func (b *Block) SetSwitchInfo(info *SwitchInfos) {
	b.infos.CID = info.CID
	b.infos.Vals = make([]*SwitchEnter, 0, len(info.Vals))
	b.infos.Vals = append(b.infos.Vals, info.Vals...)
	b.header.CommitteeHash = rlpHash(b.infos)
}

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *Block) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one. fastchain not use
func (b *Block) WithSeal(header *Header) *Block {
	cpy := *header

	return &Block{
		header:       &cpy,
		transactions: b.transactions,
	}
}

func (b *Block) IsAward() bool {
	if b.SnailHash() != *new(common.Hash) && b.SnailNumber() != nil {
		return true
	}
	return false
}

// WithBody returns a new block with the given transaction contents.
func (b *Block) WithBody(transactions []*Transaction, signs []*PbftSign, infos *SwitchInfos) *Block {
	block := &Block{
		header:       CopyHeader(b.header),
		transactions: make([]*Transaction, len(transactions)),
		signs:        make([]*PbftSign, len(signs)),
		infos:        &SwitchInfos{},
	}

	copy(block.transactions, transactions)
	copy(block.signs, signs)
	if infos != nil {
		block.infos.CID = infos.CID
		block.infos.Vals = make([]*SwitchEnter, len(infos.Vals))
		copy(block.infos.Vals, infos.Vals)
	}
	b.header.CommitteeHash = rlpHash(b.infos)

	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

////////////////////////////////////////////////////////////////////////////////

//go:generate gencodec -type SnailHeader -field-override headerMarshaling -out gen_header_json.go

// SnailHeader represents a block header in the Ethereum truechain.
type SnailHeader struct {
	ParentHash      common.Hash    `json:"parentHash"       gencodec:"required"`
	Coinbase        common.Address `json:"miner"            gencodec:"required"`
	PointerHash     common.Hash    `json:"pointerHash"      gencodec:"required"`
	PointerNumber   *big.Int       `json:"pointerNumber"    gencodec:"required"`
	FruitsHash      common.Hash    `json:"fruitsHash"       gencodec:"required"`
	FastHash        common.Hash    `json:"fastHash"         gencodec:"required"`
	FastNumber      *big.Int       `json:"fastNumber"       gencodec:"required"`
	SignHash        common.Hash    `json:"signHash"  		gencodec:"required"`
	Difficulty      *big.Int       `json:"difficulty"       gencodec:"required"`
	FruitDifficulty *big.Int       `json:"fruitDifficulty"	gencodec:"required"`
	Number          *big.Int       `json:"number"           gencodec:"required"`
	Publickey       []byte         `json:"publicKey"        gencodec:"required"`
	Time            *big.Int       `json:"timestamp"        gencodec:"required"`
	Extra           []byte         `json:"extraData"        gencodec:"required"`
	MixDigest       common.Hash    `json:"mixHash"          gencodec:"required"`
	Nonce           BlockNonce     `json:"nonce"            gencodec:"required"`
}

type SnailBody struct {
	Fruits []*SnailBlock
	Signs  []*PbftSign
}

// SnailBlock represents an entire snail block in the TrueChain snail chain.
type SnailBlock struct {
	header *SnailHeader
	fruits SnailBlocks
	signs  PbftSigns

	// caches
	hash atomic.Value
	size atomic.Value

	difficulty atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	td *big.Int

	// These fields are used by package etrue to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// "external" block encoding. used for etrue protocol, etc.
type extsnailblock struct {
	Header *SnailHeader
	Fruits []*SnailBlock
	Signs  []*PbftSign
	//Body   *SnailBody
	Td *big.Int
}

type SnailBlocks []*SnailBlock

type SnailBlockBy func(b1, b2 *SnailBlock) bool

func (self SnailBlockBy) Sort(snailBlocks SnailBlocks) {
	bs := snailBlockSorter{
		snailBlocks: snailBlocks,
		by:          self,
	}
	sort.Sort(bs)
}

type snailBlockSorter struct {
	snailBlocks SnailBlocks
	by          func(b1, b2 *SnailBlock) bool
}

func (self snailBlockSorter) Len() int { return len(self.snailBlocks) }
func (self snailBlockSorter) Swap(i, j int) {
	self.snailBlocks[i], self.snailBlocks[j] = self.snailBlocks[j], self.snailBlocks[i]
}
func (self snailBlockSorter) Less(i, j int) bool {
	return self.by(self.snailBlocks[i], self.snailBlocks[j])
}

func SnailNumber(b1, b2 *SnailBlock) bool { return b1.header.Number.Cmp(b2.header.Number) < 0 }

func FruitNumber(b1, b2 *SnailBlock) bool { return b1.header.FastNumber.Cmp(b2.header.FastNumber) < 0 }

////////////////////////////////////////////////////////////////////////////////

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *SnailHeader) Hash() common.Hash {
	return rlpHash(h)
}

// HashNoNonce returns the hash which is used as input for the proof-of-work search.
func (h *SnailHeader) HashNoNonce() common.Hash {
	return rlpHash([]interface{}{
		h.ParentHash,
		h.Coinbase,
		h.PointerHash,
		h.PointerNumber,
		h.FruitsHash,
		h.FastHash,
		h.FastNumber,
		h.Difficulty,
		h.FruitDifficulty,
		h.Number,
		h.Publickey,
		h.Time,
		h.Extra,
	})
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *SnailHeader) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+
		len(h.Publickey)+(h.Difficulty.BitLen()+h.FastNumber.BitLen()+h.FruitDifficulty.BitLen()+
		h.PointerNumber.BitLen()+h.Number.BitLen()+h.Time.BitLen())/8)
}

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
func (b *SnailBlock) DeprecatedTd() *big.Int {
	return b.td
}

// NewSnailBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
func NewSnailBlock(header *SnailHeader, fruits []*SnailBlock, signs []*PbftSign, uncles []*SnailHeader) *SnailBlock {
	b := &SnailBlock{
		header: CopySnailHeader(header),
		//body:   body,
		td: new(big.Int),
	}

	if len(fruits) == 0 {
		b.header.FruitsHash = EmptyRootHash
	} else {
		b.header.FruitsHash = DeriveSha(Fruits(fruits))
		b.fruits = make([]*SnailBlock, len(fruits))
		for i := range fruits {
			b.fruits[i] = CopyFruit(fruits[i])
		}
	}

	if len(signs) == 0 {
		b.header.SignHash = EmptySignHash
	} else {
		b.header.SignHash = CalcSignHash(signs)
		b.signs = make([]*PbftSign, len(signs))
		for i := range signs {
			b.signs[i] = CopyPbftSign(signs[i])
		}
	}

	return b
}

// NewSnailBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewSnailBlockWithHeader(header *SnailHeader) *SnailBlock {
	return &SnailBlock{header: CopySnailHeader(header)}
}

func CopyFruit(f *SnailBlock) *SnailBlock {
	//return NewSnailBlockWithHeader(f.Header()).WithBody(f.fruits, f.signs, f.uncles)
	b := &SnailBlock{
		header: CopySnailHeader(f.Header()),
		td:     new(big.Int),
	}

	if len(f.fruits) > 0 {
		b.fruits = make([]*SnailBlock, len(f.fruits))
		for i := range f.fruits {
			b.fruits[i] = CopyFruit(f.fruits[i])
		}
	}

	if len(f.signs) > 0 {
		b.signs = make([]*PbftSign, len(f.signs))
		for i := range f.signs {
			b.signs[i] = CopyPbftSign(f.signs[i])
		}
	}

	return b
}

// CopySnailHeader creates a deep copy of a snail block header to prevent side effects from
// modifying a header variable.
func CopySnailHeader(h *SnailHeader) *SnailHeader {
	cpy := *h
	if cpy.Time = new(big.Int); h.Time != nil {
		cpy.Time.Set(h.Time)
	}
	if cpy.Difficulty = new(big.Int); h.Difficulty != nil {
		cpy.Difficulty.Set(h.Difficulty)
	}
	if cpy.FruitDifficulty = new(big.Int); h.FruitDifficulty != nil {
		cpy.FruitDifficulty.Set(h.FruitDifficulty)
	}
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if cpy.FastNumber = new(big.Int); h.FastNumber != nil {
		cpy.FastNumber.Set(h.FastNumber)
	}
	if cpy.PointerNumber = new(big.Int); h.PointerNumber != nil {
		cpy.PointerNumber.Set(h.PointerNumber)
	}
	if len(h.Publickey) > 0 {
		cpy.Publickey = make([]byte, len(h.Publickey))
		copy(cpy.Publickey, h.Publickey)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}

func CopyPbftSign(s *PbftSign) *PbftSign {
	cpy := *s
	if cpy.FastHeight = new(big.Int); s.FastHeight != nil {
		cpy.FastHeight.Set(s.FastHeight)
	}
	if len(s.Sign) > 0 {
		cpy.Sign = make([]byte, len(s.Sign))
		copy(cpy.Sign, s.Sign)
	}
	return &cpy
}

// DecodeRLP decodes the SnailBlock
func (b *SnailBlock) DecodeRLP(s *rlp.Stream) error {
	var eb extsnailblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.td, b.fruits, b.signs = eb.Header, eb.Td, eb.Fruits, eb.Signs
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the TrueChain RLP block format.
func (b *SnailBlock) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extsnailblock{
		Header: b.header,
		Fruits: b.fruits,
		Signs:  b.signs,
		Td:     b.td,
	})
}

func (b *SnailBlock) Number() *big.Int { return new(big.Int).Set(b.header.Number) }
func (b *SnailBlock) GetPubKey() (*ecdsa.PublicKey, error) {
	return crypto.UnmarshalPubkey(b.header.Publickey)
}
func (b *SnailBlock) PublicKey() []byte         { return b.header.Publickey }
func (b *SnailBlock) BlockDifficulty() *big.Int { return new(big.Int).Set(b.header.Difficulty) }
func (b *SnailBlock) FruitDifficulty() *big.Int { return new(big.Int).Set(b.header.FruitDifficulty) }
func (b *SnailBlock) Time() *big.Int            { return new(big.Int).Set(b.header.Time) }
func (b *SnailBlock) NumberU64() uint64         { return b.header.Number.Uint64() }
func (b *SnailBlock) MixDigest() common.Hash    { return b.header.MixDigest }
func (b *SnailBlock) Nonce() uint64             { return binary.BigEndian.Uint64(b.header.Nonce[:]) }
func (b *SnailBlock) Coinbase() common.Address  { return b.header.Coinbase }
func (b *SnailBlock) ParentHash() common.Hash   { return b.header.ParentHash }

func (b *SnailBlock) PointerHash() common.Hash { return b.header.PointerHash }
func (b *SnailBlock) PointNumber() *big.Int    { return new(big.Int).Set(b.header.PointerNumber) }
func (b *SnailBlock) FruitsHash() common.Hash  { return b.header.FruitsHash }
func (b *SnailBlock) FastHash() common.Hash    { return b.header.FastHash }
func (b *SnailBlock) FastNumber() *big.Int     { return new(big.Int).Set(b.header.FastNumber) }
func (b *SnailBlock) Extra() []byte            { return common.CopyBytes(b.header.Extra) }
func (b *SnailBlock) Header() *SnailHeader     { return CopySnailHeader(b.header) }
func (b *SnailBlock) IsFruit() bool {
	// params.MinimumFruits 60
	if len(b.fruits) > 0 {
		return false
	} else {
		return true
	}
}
func (b *SnailBlock) Fruits() []*SnailBlock { return b.fruits }
func (b *SnailBlock) Signs() PbftSigns      { return b.signs }

func (b *SnailBlock) ToElect() bool {
	if len(b.header.Publickey) > 0 {
		return true
	} else {
		return false
	}

}

// Body returns the non-header content of the snailblock.
//func (b *SnailBlock) Body() *SnailBody { return b.body }
func (b *SnailBlock) Body() *SnailBody { return &SnailBody{b.fruits, b.signs} }
func (b *SnailBlock) HashNoNonce() common.Hash {
	return b.header.HashNoNonce()
}

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *SnailBlock) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

// WithSeal returns a new snailblock with the data from b but the header replaced with
// the sealed one.
func (b *SnailBlock) WithSeal(header *SnailHeader) *SnailBlock {
	cpy := *header

	return &SnailBlock{
		header: &cpy,
		fruits: b.fruits,
		signs:  b.signs,
	}
}

func (b *SnailBlock) SetSnailBlockFruits(fruits Fruits) {
	if len(fruits) > 0 {
		b.fruits = make([]*SnailBlock, len(fruits))
		copy(b.fruits, fruits)
	} else {
		b.fruits = nil
	}
}

func (b *SnailBlock) SetSnailBlockSigns(signs []*PbftSign) {
	if len(signs) > 0 {
		b.signs = make([]*PbftSign, len(signs))
		copy(b.signs, signs)
	} else {
		b.signs = nil
	}
}

// WithBody returns a new snailblock with the given transaction and uncle contents.
func (b *SnailBlock) WithBody(fruits []*SnailBlock, signs []*PbftSign, uncles []*SnailHeader) *SnailBlock {
	block := &SnailBlock{
		header: b.Header(),
		//body : 		body,
		fruits: make([]*SnailBlock, len(fruits)),
		signs:  make([]*PbftSign, len(signs)),
	}
	copy(block.fruits, fruits)
	copy(block.signs, signs)

	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *SnailBlock) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

func (b *SnailBlock) Difficulty() *big.Int {
	if diff := b.difficulty.Load(); diff != nil {
		return diff.(*big.Int)
	}

	if b.IsFruit() {
		diff := b.FruitDifficulty()
		b.difficulty.Store(diff)
		return diff
	} else {
		td := big.NewInt(0)
		for _, f := range b.Fruits() {
			td.Add(td, f.Difficulty())
		}
		//td = new(big.Int).Div(td, params.FruitBlockRatio)
		td.Add(td, b.BlockDifficulty())

		b.difficulty.Store(td)

		return td
	}
}
