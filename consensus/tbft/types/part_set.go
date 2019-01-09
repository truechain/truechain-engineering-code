package types

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"io"
	"sync"
)

var (
	//ErrPartSetUnexpectedIndex is Error part set unexpected index
	ErrPartSetUnexpectedIndex = errors.New("error part set unexpected index")
	//ErrPartSetInvalidProof is Error part set invalid proof
	ErrPartSetInvalidProof = errors.New("error part set invalid proof")
)

//Part struct
type Part struct {
	Index uint             `json:"index"`
	Bytes help.HexBytes    `json:"bytes"`
	Proof help.SimpleProof `json:"proof"`

	hash []byte
}

//Hash is part's hash
func (part *Part) Hash() []byte {
	if part.hash != nil {
		return part.hash
	}
	// hasher := tmhash.New()
	// hasher.Write(part.Bytes) // nolint: errcheck, gas
	// part.hash = hasher.Sum(nil)
	// md hash tmp by iceming
	tmp := help.RlpHash(part.Bytes)
	part.hash = tmp[:]
	return part.hash
}

func (part *Part) String() string {
	return part.StringIndented("")
}

//StringIndented indent's string
func (part *Part) StringIndented(indent string) string {
	return fmt.Sprintf(`Part{#%v
%s  Bytes: %X...
%s  Proof: %v
%s}`,
		part.Index,
		indent, help.Fingerprint(part.Bytes),
		indent, "comment",
		// commented tmp by iceming
		// indent, part.Proof.StringIndented(indent+"  "),
		indent)
}

//-------------------------------------

//PartSetHeader struct
type PartSetHeader struct {
	Total uint          `json:"total"`
	Hash  help.HexBytes `json:"hash"`
}

func (psh PartSetHeader) String() string {
	return fmt.Sprintf("%v:%X", psh.Total, help.Fingerprint(psh.Hash))
}

//IsZero is have part
func (psh PartSetHeader) IsZero() bool {
	return psh.Total == 0
}

//Equals compare other partSet header's hash
func (psh PartSetHeader) Equals(other PartSetHeader) bool {
	return psh.Total == other.Total && bytes.Equal(psh.Hash, other.Hash)
}

//-------------------------------------

//PartSet struct
type PartSet struct {
	total uint
	hash  []byte

	mtx           sync.Mutex
	parts         []*Part
	partsBitArray *help.BitArray
	count         uint
}

// NewPartSetFromData Returns an immutable, full PartSet from the data bytes.
// The data bytes are split into "partSize" chunks, and merkle tree computed.
func NewPartSetFromData(data []byte, partSize uint) *PartSet {
	// divide data into 4kb parts.
	total := (len(data) + int(partSize) - 1) / int(partSize)
	parts := make([]*Part, total)
	partsBytes := make([][]byte, total)
	partsBitArray := help.NewBitArray(uint(total))
	for i := 0; i < total; i++ {
		part := &Part{
			Index: uint(i),
			Bytes: data[i*int(partSize) : help.MinInt(len(data), (i+1)*int(partSize))],
		}
		parts[i] = part
		partsBytes[i] = part.Bytes
		partsBitArray.SetIndex(uint(i), true)
	}
	// Compute merkle proofs
	root, proofs := help.SimpleProofsFromByteSlices(partsBytes)
	for i := 0; i < total; i++ {
		parts[i].Proof = *proofs[i]
	}
	return &PartSet{
		total:         uint(total),
		hash:          root,
		parts:         parts,
		partsBitArray: partsBitArray,
		count:         uint(total),
	}
}

// NewPartSetFromHeader Returns an empty PartSet ready to be populated.
func NewPartSetFromHeader(header PartSetHeader) *PartSet {
	return &PartSet{
		total:         header.Total,
		hash:          header.Hash,
		parts:         make([]*Part, header.Total),
		partsBitArray: help.NewBitArray(uint(header.Total)),
		count:         0,
	}
}

//Header get partSet's header
func (ps *PartSet) Header() PartSetHeader {
	if ps == nil {
		return PartSetHeader{}
	}
	return PartSetHeader{
		Total: ps.total,
		Hash:  ps.hash,
	}
}

//HasHeader Compare header
func (ps *PartSet) HasHeader(header PartSetHeader) bool {
	if ps == nil {
		return false
	}
	return ps.Header().Equals(header)
}

//BitArray return BitArray's Copy
func (ps *PartSet) BitArray() *help.BitArray {
	//log.Debug("bitArray", "start", "ok", "ps", ps)
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	//defer func() { log.Debug("bitArray", "finish", "ok") }()
	//prints.PrintStack()
	return ps.partsBitArray.Copy()
}

//Hash return ps'hash
func (ps *PartSet) Hash() []byte {
	if ps == nil {
		return nil
	}
	return ps.hash
}

//HashesTo Compare two hashes
func (ps *PartSet) HashesTo(hash []byte) bool {
	if ps == nil {
		return false
	}
	return bytes.Equal(ps.hash, hash)
}

//Count Count of parts
func (ps *PartSet) Count() uint {
	if ps == nil {
		return 0
	}
	return ps.count
}

//Total sum of parts
func (ps *PartSet) Total() uint {
	if ps == nil {
		return 0
	}
	return ps.total
}

//AddPart add a part to parts array
func (ps *PartSet) AddPart(part *Part) (bool, error) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	// Invalid part index
	if part.Index >= ps.total {
		return false, ErrPartSetUnexpectedIndex
	}

	// If part already exists, return false.
	if ps.parts[part.Index] != nil {
		return false, nil
	}
	// Check hash proof
	if part.Proof.Verify(ps.Hash(), part.Hash()) != nil {
		return false, ErrPartSetInvalidProof
	}
	// Add part
	ps.parts[part.Index] = part
	ps.partsBitArray.SetIndex(uint(part.Index), true)
	ps.count++
	return true, nil
}

//GetPart get a part for index
func (ps *PartSet) GetPart(index uint) *Part {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return ps.parts[index]
}

//IsComplete if get all part
func (ps *PartSet) IsComplete() bool {
	return ps.count == ps.total
}

//GetReader get a new reader if not complete
func (ps *PartSet) GetReader() io.Reader {
	if !ps.IsComplete() {
		help.PanicSanity("Cannot GetReader() on incomplete PartSet")
	}
	return NewPartSetReader(ps.parts)
}

//PartSetReader struct
type PartSetReader struct {
	i      int
	parts  []*Part
	reader *bytes.Reader
}

//NewPartSetReader return new reader
func NewPartSetReader(parts []*Part) *PartSetReader {
	return &PartSetReader{
		i:      0,
		parts:  parts,
		reader: bytes.NewReader(parts[0].Bytes),
	}
}

//Read  read a partSet bytes
func (psr *PartSetReader) Read(p []byte) (n int, err error) {
	readerLen := psr.reader.Len()
	if readerLen >= len(p) {
		return psr.reader.Read(p)
	} else if readerLen > 0 {
		n1, err := psr.Read(p[:readerLen])
		if err != nil {
			return n1, err
		}
		n2, err := psr.Read(p[readerLen:])
		return n1 + n2, err
	}

	psr.i++
	if psr.i >= len(psr.parts) {
		return 0, io.EOF
	}
	psr.reader = bytes.NewReader(psr.parts[psr.i].Bytes)
	return psr.Read(p)
}

//StringShort print partSet count and total
func (ps *PartSet) StringShort() string {
	if ps == nil {
		return "nil-PartSet"
	}
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return fmt.Sprintf("(%v of %v)", ps.Count(), ps.Total())
}

//MarshalJSON is marshal partsBitArray to json
func (ps *PartSet) MarshalJSON() ([]byte, error) {
	if ps == nil {
		return []byte("{}"), nil
	}

	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return cdc.MarshalJSON(struct {
		CountTotal    string         `json:"count/total"`
		PartsBitArray *help.BitArray `json:"parts_bit_array"`
	}{
		fmt.Sprintf("%d/%d", ps.Count(), ps.Total()),
		ps.partsBitArray,
	})
}
