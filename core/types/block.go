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

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/rlp"
)

var (
	EmptyRootHash  = DeriveSha(Transactions{})
	EmptyUncleHash = CalcUncleHash(nil)
	EmptySignHash = CalcSignHash(nil)
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
	ParentHash  common.Hash `json:"parentHash"       gencodec:"required"`
	Root        common.Hash `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom       `json:"logsBloom"        gencodec:"required"`
	SnailHash   common.Hash `json:"snailHash"        gencodec:"required"`
	SnailNumber *big.Int    `json:"snailNumber"      gencodec:"required"`
	Number      *big.Int    `json:"number"           gencodec:"required"`
	GasLimit    uint64      `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64      `json:"gasUsed"          gencodec:"required"`
	Time        *big.Int    `json:"timestamp"        gencodec:"required"`
	Extra       []byte      `json:"extraData"        gencodec:"required"`
}

// field type overrides for gencodec
type headerMarshaling struct {
	SnailNumber *hexutil.Big
	Number     *hexutil.Big
	GasLimit   hexutil.Uint64
	GasUsed    hexutil.Uint64
	Time       *hexutil.Big
	Extra      hexutil.Bytes
	Hash       common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
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
	Signs   []*PbftSign
}

// Block Reward
type BlockReward struct {
	FastHash    common.Hash `json:"FastHash"        gencodec:"required"`
	FastNumber  *big.Int    `json:"FastNumber"      gencodec:"required"`
	SnailHash   common.Hash `json:"SnailHash"        gencodec:"required"`
	SnailNumber *big.Int    `json:"SnailNumber"      gencodec:"required"`
}

// FastBlock represents an entire block in the Ethereum blockchain.
type Block struct {
	header       *Header
	transactions Transactions

	uncles       []*Header // reserved for compile

	signs        PbftSigns

	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	// td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}



// NewFastBlock creates a new fastblock. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs
// and receipts.
func NewBlock(header *Header, txs []*Transaction, receipts []*Receipt, signs []*PbftSign) *Block {
	b := &Block{header: CopyHeader(header)}

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

	return b
}

func (b *Body) SetLeaderSign(sign *PbftSign) {
	signP := *sign
	b.Signs = []*PbftSign{}
	b.Signs = append(b.Signs,&signP)
}

func (b *Body) GetLeaderSign() *PbftSign{
	if len(b.Signs) > 0 {
		return b.Signs[0]
	}
	return nil
}

// NewFastBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}
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

// "external" block encoding. used for eth protocol, etc.
type extblock struct {
	Header *Header
	Txs    []*Transaction
}

// DecodeRLP decodes the Ethereum
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.transactions = eb.Header, eb.Txs
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Txs:    b.transactions,
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

func (b *Block) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *Block) SnailHash() common.Hash   { return b.header.SnailHash }
func (b *Block) Bloom() Bloom             { return b.header.Bloom }
func (b *Block) Coinbase() common.Address { return common.Address{} }
func (b *Block) Root() common.Hash        { return b.header.Root }
func (b *Block) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *Block) TxHash() common.Hash      { return b.header.TxHash }
func (b *Block) ReceiptHash() common.Hash { return b.header.ReceiptHash }
func (b *Block) UncleHash() common.Hash   { return common.Hash{} }
func (b *Block) Extra() []byte            { return common.CopyBytes(b.header.Extra) }

func (b *Block) Header() *Header { return CopyHeader(b.header) }

// Body returns the non-header content of the block.
func (b *Block) Body() *Body { return &Body{b.transactions,b.signs} }


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
// the sealed one.
func (b *Block) WithSeal(header *Header) *Block {
	cpy := *header

	return &Block{
		header:       &cpy,
		transactions: b.transactions,
	}
}

// WithBody returns a new block with the given transaction contents.
func (b *Block) WithBody(transactions []*Transaction, uncles []*Header) *Block {
	block := &Block{
		header:       CopyHeader(b.header),
		transactions: make([]*Transaction, len(transactions)),
	}
	copy(block.transactions, transactions)
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

// Header represents a block header in the Ethereum truechain.
type SnailHeader struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	Coinbase    common.Address `json:"miner"            gencodec:"required"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	PointerHash common.Hash    `json:"PointerHash"      gencodec:"required"`
	FruitsHash  common.Hash    `json:"fruitsHash"       gencodec:"required"`
	FastHash    common.Hash    `json:"fastHash"         gencodec:"required"`
	FastNumber  *big.Int       `json:"fastNumber"       gencodec:"required"`
	SignHash    common.Hash    `json:"signHash"  		gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	Publickey   []byte         `json:"Publickey"        gencodec:"required"`
	ToElect     bool           `json:"ToElect"          gencodec:"required"`
	Time        *big.Int       `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    `json:"mixHash"          gencodec:"required"`
	Nonce       BlockNonce     `json:"nonce"            gencodec:"required"`

	Fruit bool
}

type SnailBody struct {
	Fruits []*SnailBlock
	Signs  []*PbftSign
}

// Block represents an entire block in the Ethereum blockchain.
type SnailBlock struct {
	header *SnailHeader
	fruits SnailBlocks
	signs  PbftSigns

	uncles  []*SnailHeader

	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// "external" block encoding. used for eth protocol, etc.
type extsnailblock struct {
	Header *SnailHeader
	Fruits []*SnailBlock
	Signs  []*PbftSign
	//Body   *SnailBody
	Td     *big.Int
}


type SnailBlocks []*SnailBlock

type SnailBlockBy func(b1, b2 *SnailBlock) bool

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *SnailHeader) Hash() common.Hash {
	return rlpHash(h)
}

// HashNoNonce returns the hash which is used as input for the proof-of-work search.
func (h *SnailHeader) HashNoNonce() common.Hash {
	return rlpHash([]interface{}{
		h.ParentHash,
		h.UncleHash,
		h.Coinbase,
		h.Root,
		h.TxHash,
		h.ReceiptHash,
		h.PointerHash,
		h.FruitsHash,
		h.FastHash,
		h.FastNumber,
		h.Bloom,
		h.Difficulty,
		h.Number,
		h.Publickey,
		h.ToElect,
		h.Time,
		h.Extra,
	})
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *SnailHeader) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+
		len(h.Publickey)+(h.Difficulty.BitLen()+h.FastNumber.BitLen()+
		h.Number.BitLen()+h.Time.BitLen())/8)
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
func NewSnailBlock(header *SnailHeader, fruits []*SnailBlock, signs []*PbftSign,  uncles []*SnailHeader)*SnailBlock {
	b := &SnailBlock{
		header: CopySnailHeader(header),
		//body:   body,
		td:     new(big.Int),
	}

	if len(fruits) == 0 {
		b.header.FruitsHash = EmptyRootHash
	}else {
		// TODO: get fruits hash
		b.header.FruitsHash = DeriveSha(Fruits(fruits))
		b.fruits = make([]*SnailBlock, len(fruits))
		for i := range fruits {
			b.fruits[i] = CopyFruit(fruits[i])
		}
	}

	b.header.UncleHash = EmptyUncleHash

	if len(signs) == 0 {
		b.header.UncleHash = EmptySignHash
	} else {
		b.header.UncleHash = CalcSignHash(signs)
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


// TODO: implement copy function
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

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopySnailHeader(h *SnailHeader) *SnailHeader {
	cpy := *h
	if cpy.Time = new(big.Int); h.Time != nil {
		cpy.Time.Set(h.Time)
	}
	if cpy.Difficulty = new(big.Int); h.Difficulty != nil {
		cpy.Difficulty.Set(h.Difficulty)
	}
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if cpy.FastNumber = new(big.Int); h.FastNumber != nil {
		cpy.FastNumber.Set(h.FastNumber)
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

// DecodeRLP decodes the Ethereum
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

// EncodeRLP serializes b into the Ethereum RLP block format.
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
func (b *SnailBlock) Difficulty() *big.Int     { return new(big.Int).Set(b.header.Difficulty) }
func (b *SnailBlock) Time() *big.Int           { return new(big.Int).Set(b.header.Time) }
func (b *SnailBlock) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *SnailBlock) MixDigest() common.Hash   { return b.header.MixDigest }
func (b *SnailBlock) Nonce() uint64            { return binary.BigEndian.Uint64(b.header.Nonce[:]) }
func (b *SnailBlock) Bloom() Bloom             { return b.header.Bloom }
func (b *SnailBlock) Coinbase() common.Address { return b.header.Coinbase }
func (b *SnailBlock) Root() common.Hash        { return b.header.Root }
func (b *SnailBlock) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *SnailBlock) TxHash() common.Hash      { return b.header.TxHash }
func (b *SnailBlock) ReceiptHash() common.Hash { return b.header.ReceiptHash }
func (b *SnailBlock) UncleHash() common.Hash   { return b.header.UncleHash }
func (b *SnailBlock) PointerHash() common.Hash { return b.header.PointerHash }
func (b *SnailBlock) FruitsHash() common.Hash  { return b.header.FruitsHash }
func (b *SnailBlock) FastHash() common.Hash    { return b.header.FastHash }
func (b *SnailBlock) FastNumber() *big.Int     { return new(big.Int).Set(b.header.FastNumber) }
func (b *SnailBlock) ToElect() bool            { return b.header.ToElect }
func (b *SnailBlock) Extra() []byte            { return common.CopyBytes(b.header.Extra) }
func (b *SnailBlock) Header() *SnailHeader     { return CopySnailHeader(b.header) }
func (b *SnailBlock) IsFruit() bool            { return b.header.Fruit }
func (b *SnailBlock) Fruits() []*SnailBlock    { return b.fruits }
func (b *SnailBlock) Signs() PbftSigns    { return b.signs }

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
		signs: b.signs,
	}
}

// WithBody returns a new snailblock with the given transaction and uncle contents.
func (b *SnailBlock) WithBody(fruits []*SnailBlock, signs []*PbftSign, uncles []*SnailHeader) *SnailBlock {
	block := &SnailBlock{
		header: b.Header(),
		//body : 		body,
		fruits: make([]*SnailBlock, len(fruits)),
		signs:make([]*PbftSign, len(signs)),
	}
	copy(block.fruits, fruits)
	copy(block.signs, signs)
	for i := range uncles {
		block.uncles[i] = CopySnailHeader(uncles[i])
	}

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
