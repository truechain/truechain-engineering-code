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
	"encoding/binary"
	"io"
	"math/big"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"
	"crypto/ecdsa"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/crypto/sha3"
	"github.com/truechain/truechain-engineering-code/rlp"
)

var (
	EmptyRootHash  = DeriveSha(Transactions{})
	EmptyUncleHash = CalcUncleHash(nil)
)



type PbftRecordHeader struct {
	Number   *big.Int
	Hash     common.Hash
	TxHash	common.Hash
	GasLimit *big.Int
	GasUsed  *big.Int
	Time     *big.Int
}


type PbftRecord struct {
	header       *PbftRecordHeader
	transactions Transactions
	sig          []*string
}
// Abtion 20180715 for handler.go and peer.go
type PbftRecords []*PbftRecord

func (r *PbftRecord) Hash() common.Hash {
	return r.header.Hash
}

func (r *PbftRecord) Number() *big.Int {
	return r.header.Number
 }

func (r *PbftRecord) Header() *PbftRecordHeader { return r.header }

func (r *PbftRecord) TxHash() common.Hash {return r.header.TxHash}

func (r *PbftRecord) Transactions() Transactions { return r.transactions }

func (r *PbftRecord) CalcHash() common.Hash {
	return rlpHash([]interface{}{
		r.header.Number,
		r.header.TxHash,
		r.header.GasLimit,
		r.header.GasUsed,
		r.header.Time,
		r.sig,
	})
}


func CopyRecord(r *PbftRecord) *PbftRecord {
	header := *r.header
	if header.Time = new(big.Int); r.header.Time != nil {
		header.Time.Set(r.header.Time)
	}
	if header.Number = new(big.Int); r.header.Number != nil {
		header.Number.Set(r.header.Number)
	}
	if header.GasLimit = new(big.Int); r.header.GasLimit != nil {
		header.GasLimit.Set(r.header.GasLimit)
	}
	if header.GasUsed = new(big.Int); r.header.GasUsed != nil {
		header.GasUsed.Set(r.header.GasUsed)
	}

	record := &PbftRecord{
		header: &header,
	}

	if len(r.transactions) != 0 {
		record.transactions = make(Transactions, len(r.transactions))
		copy(record.transactions, r.transactions)
	}

	// TODO: copy sigs

	return record
}


func NewRecord(number *big.Int, txs []*Transaction, sig []*string) *PbftRecord {

	r := &PbftRecord{
		header: &PbftRecordHeader {
			Number: number,
			Time: big.NewInt(time.Now().Unix()),
		},
	}

	if len(txs) == 0 {
		r.header.TxHash = EmptyRootHash
	} else {
		r.header.TxHash = DeriveSha(Transactions(txs))
		r.transactions = make(Transactions, len(txs))
		copy(r.transactions, txs)
	}

	r.header.Hash = r.CalcHash()

	return r
}

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

//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

// Header represents a block header in the Ethereum blockchain.
type Header struct {
	ParentHash common.Hash `json:"parentHash"       gencodec:"required"`

	PointerHash  common.Hash `json:"pointerHash"      gencodec:"required"`
	FruitsHash   common.Hash `json:"fruitSetHash"     gencodec:"required"`
	RecordHash   common.Hash
	RecordNumber *big.Int
	Fruit		bool

	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"`
	Coinbase    common.Address `json:"miner"            gencodec:"required"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        *big.Int       `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    `json:"mixHash"          gencodec:"required"`
	Nonce       BlockNonce     `json:"nonce"            gencodec:"required"`
}

// field type overrides for gencodec
type headerMarshaling struct {
	Difficulty *hexutil.Big
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

// HashNoNonce returns the hash which is used as input for the proof-of-work search.
func (h *Header) HashNoNonce() common.Hash {
	return rlpHash([]interface{}{
		h.ParentHash,
		h.PointerHash,
		h.FruitsHash,
		h.RecordHash,
		h.RecordNumber,
		h.UncleHash,
		h.Coinbase,
		h.Root,
		h.TxHash,
		h.ReceiptHash,
		h.Bloom,
		h.Difficulty,
		h.Number,
		h.GasLimit,
		h.GasUsed,
		h.Time,
		h.Extra,
	})
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *Header) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+(h.Difficulty.BitLen()+h.Number.BitLen()+h.Time.BitLen())/8)
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
	Fruits		 []*Block
	Uncles       []*Header
}

// Block represents an entire block in the Ethereum blockchain.
type Block struct {
	header       *Header
	uncles       []*Header
	fruits       []*Block // nil for fruit
	transactions Transactions

	// caches
	hash atomic.Value
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	td *big.Int

	// These fields are used by package etrue to track
	// inter-peer block relay.
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
func (b *Block) DeprecatedTd() *big.Int {
	return b.td
}

// [deprecated by eth/63]
// StorageBlock defines the RLP encoding of a Block stored in the
// state database. The StorageBlock encoding contains fields that
// would otherwise need to be recomputed.
type StorageBlock Block

// "external" block encoding. used for eth protocol, etc.
type extblock struct {
	Header *Header
	Txs    []*Transaction
	Uncles []*Header
}

// [deprecated by eth/63]
// "storage" block encoding. used for database.
type storageblock struct {
	Header *Header
	Txs    []*Transaction
	Uncles []*Header
	TD     *big.Int
}


// Fruits is a wrapper around a fruit array to implement DerivableList.
type Fruits []*Block

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



// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewBlock(header *Header, txs []*Transaction, uncles []*Header, receipts []*Receipt, fruits []*Block) *Block {
	b := &Block{header: CopyHeader(header), td: new(big.Int)}

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

	if len(fruits) == 0 {
		b.header.FruitsHash = EmptyRootHash
	}else {
		// TODO: get fruits hash
		b.header.FruitsHash = DeriveSha(Fruits(fruits))
		b.fruits = make([]*Block, len(fruits))
		for i := range fruits {
			b.fruits[i] = CopyFruit(fruits[i])
		}
	}

	if len(uncles) == 0 {
		b.header.UncleHash = EmptyUncleHash
	} else {
		b.header.UncleHash = CalcUncleHash(uncles)
		b.uncles = make([]*Header, len(uncles))
		for i := range uncles {
			b.uncles[i] = CopyHeader(uncles[i])
		}
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {
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
	if cpy.RecordNumber = new(big.Int); h.RecordNumber != nil {
		cpy.RecordNumber.Set(h.RecordNumber)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}


func CopyFruit(f *Block) *Block {
	b := &Block{header: CopyHeader(f.header), td: new(big.Int)}

	if len(f.transactions) > 0 {
		b.transactions = make(Transactions, len(f.transactions))
		copy(b.transactions, f.transactions)
	}
	return b
}

// DecodeRLP decodes the Ethereum
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.uncles, b.transactions = eb.Header, eb.Uncles, eb.Txs
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Txs:    b.transactions,
		Uncles: b.uncles,
	})
}

// [deprecated by eth/63]
func (b *StorageBlock) DecodeRLP(s *rlp.Stream) error {
	var sb storageblock
	if err := s.Decode(&sb); err != nil {
		return err
	}
	b.header, b.uncles, b.transactions, b.td = sb.Header, sb.Uncles, sb.Txs, sb.TD
	return nil
}

// TODO: copies

func (b *Block) Uncles() []*Header          { return b.uncles }
func (b *Block) Transactions() Transactions { return b.transactions }

func (b *Block) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range b.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}

func (b *Block) Fruits() []*Block { return b.fruits }

func (b *Block) Number() *big.Int     { return new(big.Int).Set(b.header.Number) }
func (b *Block) GasLimit() uint64     { return b.header.GasLimit }
func (b *Block) GasUsed() uint64      { return b.header.GasUsed }
func (b *Block) Difficulty() *big.Int { return new(big.Int).Set(b.header.Difficulty) }
func (b *Block) Time() *big.Int       { return new(big.Int).Set(b.header.Time) }

func (b *Block) RecordHash() common.Hash {return b.header.RecordHash}
func (b *Block) RecordNumber() *big.Int {return b.header.RecordNumber}
func (b *Block) IsFruit() bool           {return b.header.Fruit}
func (b *Block) PointerHash() common.Hash {return b.header.PointerHash}

func (b *Block) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *Block) MixDigest() common.Hash   { return b.header.MixDigest }
func (b *Block) Nonce() uint64            { return binary.BigEndian.Uint64(b.header.Nonce[:]) }
func (b *Block) Bloom() Bloom             { return b.header.Bloom }
func (b *Block) Coinbase() common.Address { return b.header.Coinbase }
func (b *Block) Root() common.Hash        { return b.header.Root }
func (b *Block) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *Block) TxHash() common.Hash      { return b.header.TxHash }
func (b *Block) ReceiptHash() common.Hash { return b.header.ReceiptHash }
func (b *Block) UncleHash() common.Hash   { return b.header.UncleHash }
func (b *Block) Extra() []byte            { return common.CopyBytes(b.header.Extra) }

func (b *Block) Header() *Header { return CopyHeader(b.header) }

// Body returns the non-header content of the block.
func (b *Block) Body() *Body { return &Body{b.transactions, b.fruits, b.uncles} }

func (b *Block) HashNoNonce() common.Hash {
	return b.header.HashNoNonce()
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

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

func CalcUncleHash(uncles []*Header) common.Hash {
	return rlpHash(uncles)
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
func (b *Block) WithSeal(header *Header) *Block {
	cpy := *header

	if header.Fruit {
		return &Block{
			header:       &cpy,
			transactions: b.transactions,
		}
	} else {
		return &Block{
			header: &cpy,
			fruits: b.fruits,
			uncles: b.uncles,
		}
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
// TODO: add fruits when downloading new block at eth/download,fetch
func (b *Block) WithBody(transactions []*Transaction, uncles []*Header) *Block {
	block := &Block{
		header:       CopyHeader(b.header),
		transactions: make([]*Transaction, len(transactions)),
		uncles:       make([]*Header, len(uncles)),
	}
	copy(block.transactions, transactions)
	for i := range uncles {
		block.uncles[i] = CopyHeader(uncles[i])
	}
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
//go:generate gencodec -type FastHeader -field-override headerMarshaling -out gen_header_json.go

// Header represents a block header in the true Fastblockchain.
type FastHeader struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"`
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	SnailHash   common.Hash    `json:"SnailHash"        gencodec:"required"`
	SnailNumber *big.Int       `json:"SnailNumber"      gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        *big.Int       `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
type FastBody struct {
	Transactions 	[]*Transaction
	signs       	[]*string
}

// FastBlock represents an entire block in the Ethereum blockchain.
type FastBlock struct {
	header       	*FastHeader
	signs       	[]*string
	transactions 	Transactions

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
// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *FastHeader) Hash() common.Hash {
	return rlpHash(h)
}
// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *FastHeader) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+
	(h.SnailNumber.BitLen()+h.Number.BitLen()+h.Time.BitLen())/8)
}
// NewFastBlock creates a new fastblock. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs
// and receipts.
func NewFastBlock(header *FastHeader, txs []*Transaction, signs []*string, receipts []*Receipt) *FastBlock {
	b := &FastBlock{header: CopyFastHeader(header)}

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
	b.signs = make([]*string,len(signs))
	copy(b.signs,signs)
	return b
}
// NewFastBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewFastBlockWithHeader(header *FastHeader) *FastBlock {
	return &FastBlock{header: CopyFastHeader(header)}
}
func CopyFastHeader(h *FastHeader) *FastHeader {
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
type extfastblock struct {
	Header *FastHeader
	Txs    []*Transaction
	Signs  []*string
}
// DecodeRLP decodes the Ethereum
func (b *FastBlock) DecodeRLP(s *rlp.Stream) error {
	var eb extfastblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.signs, b.transactions = eb.Header, eb.Signs, eb.Txs
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}
// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *FastBlock) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extfastblock{
		Header: b.header,
		Txs:    b.transactions,
		Signs:  b.signs,
	})
}

func (b *FastBlock) Signs() []*string           { return b.signs }
func (b *FastBlock) Transactions() Transactions { return b.transactions }
func (b *FastBlock) SignedHash() common.Hash    { return rlpHash([]interface{}{b.header,b.transactions})}
func (b *FastBlock) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range b.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}
func (b *FastBlock) Number() *big.Int      { return new(big.Int).Set(b.header.Number) }
func (b *FastBlock) GasLimit() uint64      { return b.header.GasLimit }
func (b *FastBlock) GasUsed() uint64       { return b.header.GasUsed }
func (b *FastBlock) SnailNumber() *big.Int { return new(big.Int).Set(b.header.SnailNumber) }
func (b *FastBlock) Time() *big.Int        { return new(big.Int).Set(b.header.Time) }

func (b *FastBlock) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *FastBlock) SnailHash() common.Hash   { return b.header.SnailHash }
func (b *FastBlock) Bloom() Bloom             { return b.header.Bloom }
func (b *FastBlock) Root() common.Hash        { return b.header.Root }
func (b *FastBlock) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *FastBlock) TxHash() common.Hash      { return b.header.TxHash }
func (b *FastBlock) ReceiptHash() common.Hash { return b.header.ReceiptHash }
func (b *FastBlock) Extra() []byte            { return common.CopyBytes(b.header.Extra) }

func (b *FastBlock) Header() *FastHeader { return CopyFastHeader(b.header) }

// Body returns the non-header content of the fastblock.
func (b *FastBlock) Body() *FastBody { return &FastBody{b.transactions, b.signs} }
// Size returns the true RLP encoded storage size of the fastblock, either by encoding
// and returning it, or returning a previsouly cached value.
func (b *FastBlock) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}
// WithSeal returns a new fastblock with the data from b but the header replaced with
// the sealed one.
func (b *FastBlock) WithSeal(header *FastHeader) *FastBlock {
	cpy := *header

	return &FastBlock{
		header:       &cpy,
		transactions: b.transactions,
		signs:        b.signs,
	}
}
// WithBody returns a new fastblock with the given transaction contents.
func (b *FastBlock) WithBody(transactions []*Transaction, signs []*string) *FastBlock {
	block := &FastBlock{
		header:       CopyFastHeader(b.header),
		transactions: make([]*Transaction, len(transactions)),
		signs:        make([]*string, len(signs)),
	}
	copy(block.transactions, transactions)
	copy(block.signs,signs)
	return block
}
// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *FastBlock) Hash() common.Hash {
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
	ParentHash  common.Hash    		`json:"parentHash"       gencodec:"required"`
	UncleHash   common.Hash    		`json:"sha3Uncles"       gencodec:"required"`
	Coinbase    common.Address 		`json:"miner"            gencodec:"required"`
	Root        common.Hash    		`json:"stateRoot"        gencodec:"required"`
	TxHash      common.Hash    		`json:"transactionsRoot" gencodec:"required"`
	ReceiptHash common.Hash    		`json:"receiptsRoot"     gencodec:"required"`
	PointerHash common.Hash    		`json:"PointerHash"      gencodec:"required"`
	FruitsHash  common.Hash    		`json:"FruitsHash"       gencodec:"required"`
	FastHash    common.Hash    		`json:"FastHash "        gencodec:"required"`
	FastNumber  *big.Int       		`json:"FastNumber"       gencodec:"required"`
	Bloom       Bloom          		`json:"logsBloom"        gencodec:"required"`
	Difficulty  *big.Int       		`json:"difficulty"       gencodec:"required"`
	Number      *big.Int       		`json:"number"           gencodec:"required"`
	Publickey   []byte			    `json:"Publickey"        gencodec:"required"`
	ToElect     bool         		`json:"ToElect"          gencodec:"required"`
	Time        *big.Int       		`json:"timestamp"        gencodec:"required"`
	Extra       []byte         		`json:"extraData"        gencodec:"required"`
	MixDigest   common.Hash    		`json:"mixHash"          gencodec:"required"`
	Nonce       BlockNonce     		`json:"nonce"            gencodec:"required"`
}

type SnailBody struct {
	Fruits       []*SnailHeader
}

// Block represents an entire block in the Ethereum blockchain.
type SnailBlock struct {
	header       *SnailHeader
	body       	 *SnailBody

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
	Header  *SnailHeader
	Body    *SnailBody
	Td 	    *big.Int
}
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
	len(h.Publickey) + (h.Difficulty.BitLen()+h.FastNumber.BitLen()+
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
func NewSnailBlock(header *SnailHeader, body SnailBody) *SnailBlock {
	b := &SnailBlock{header: CopySnailHeader(header), td: new(big.Int)}

	b.body.Fruits = make([]*SnailHeader,len(body.Fruits))
	for i := range body.Fruits {
		b.body.Fruits[i] = CopySnailHeader(body.Fruits[i])
	}
	return b
}
// NewSnailBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewSnailBlockWithHeader(header *SnailHeader) *SnailBlock {
	return &SnailBlock{header: CopySnailHeader(header)}
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
// DecodeRLP decodes the Ethereum
func (b *SnailBlock) DecodeRLP(s *rlp.Stream) error {
	var eb extsnailblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.td, b.body = eb.Header, eb.Td, eb.Body
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}
// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *SnailBlock) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extsnailblock{
		Header: 	b.header,
		Body:    	b.body,
		Td:			b.td,
	})
}

func (b *SnailBlock) Number() *big.Int     				  { return new(big.Int).Set(b.header.Number) }
func (b *SnailBlock) GetPubKey() (*ecdsa.PublicKey,error) { return crypto.UnmarshalPubkey(b.header.Publickey)}
func (b *SnailBlock) Difficulty() *big.Int 	   { return new(big.Int).Set(b.header.Difficulty) }
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
func (b *SnailBlock) FastNumber() *big.Int 	   { return new(big.Int).Set(b.header.FastNumber) }
func (b *SnailBlock) ToElect() bool            { return b.header.ToElect }
func (b *SnailBlock) Extra() []byte            { return common.CopyBytes(b.header.Extra) }
func (b *SnailBlock) Header() *SnailHeader 	   { return CopySnailHeader(b.header) }

// Body returns the non-header content of the snailblock.
func (b *SnailBlock) Body() *SnailBody { return b.body }
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
		header:     &cpy,
		body: 		b.body,
	}
}
// WithBody returns a new snailblock with the given transaction and uncle contents.
func (b *SnailBlock) WithBody(body *SnailBody) *SnailBlock {
	block := &SnailBlock{
		header:       b.Header(),
	}
	block.body.Fruits = make([]*SnailHeader,len(body.Fruits))
	for i := range body.Fruits {
		block.body.Fruits[i] = CopySnailHeader(body.Fruits[i])
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