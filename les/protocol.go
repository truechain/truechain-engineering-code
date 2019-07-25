// Copyright 2016 The go-ethereum Authors
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

package les

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/types"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/p2p/enode"
)

// Constants to match up protocol versions and messages
const (
	lpv2 = 2
)

// Supported versions of the les protocol (first is primary)
var (
	ClientProtocolVersions    = []uint{lpv2}
	ServerProtocolVersions    = []uint{lpv2}
	AdvertiseProtocolVersions = []uint{lpv2} // clients are searching for the first advertised protocol in the list
)

// Number of implemented message corresponding to different protocol versions.
var ProtocolLengths = map[uint]uint64{lpv2: 37}

const (
	NetworkId          = 1
	ProtocolMaxMsgSize = 10 * 1024 * 1024 // Maximum cap on the size of a protocol message
)

// les protocol message codes
const (
	// Protocol messages belonging to LPV1
	StatusMsg               = 0x00
	AnnounceMsg             = 0x01
	GetFastBlockHeadersMsg  = 0x02
	FastBlockHeadersMsg     = 0x03
	GetFastBlockBodiesMsg   = 0x04
	FastBlockBodiesMsg      = 0x05
	GetSnailBlockHeadersMsg = 0x06
	SnailBlockHeadersMsg    = 0x07
	GetSnailBlockBodiesMsg  = 0x08
	SnailBlockBodiesMsg     = 0x09
	GetFruitBodiesMsg       = 0x0a
	FruitBodiesMsg          = 0x0b

	GetReceiptsMsg = 0x0c
	ReceiptsMsg    = 0x0d
	// Protocol messages belonging to LPV2
	GetCodeMsg             = 0x0e
	CodeMsg                = 0x0f
	GetProofsV2Msg         = 0x10
	ProofsV2Msg            = 0x11
	GetHelperTrieProofsMsg = 0x12
	HelperTrieProofsMsg    = 0x13
	SendTxV2Msg            = 0x15
	GetTxStatusMsg         = 0x16
	TxStatusMsg            = 0x17
	// Protocol messages introduced in LPV3
	StopMsg   = 0x18
	ResumeMsg = 0x19
)

type requestInfo struct {
	name     string
	maxCount uint64
}

var requests = map[uint64]requestInfo{
	GetFastBlockHeadersMsg:  {"GetBlockHeaders", MaxHeaderFetch},
	GetFastBlockBodiesMsg:   {"GetBlockBodies", MaxBodyFetch},
	GetSnailBlockHeadersMsg: {"GetBlockHeaders", MaxHeaderFetch},
	GetSnailBlockBodiesMsg:  {"GetBlockBodies", MaxBodyFetch},
	GetFruitBodiesMsg:       {"GetBlockBodies", MaxBodyFetch},
	GetReceiptsMsg:          {"GetReceipts", MaxReceiptFetch},
	GetCodeMsg:              {"GetCode", MaxCodeFetch},
	GetProofsV2Msg:          {"GetProofsV2", MaxProofsFetch},
	GetHelperTrieProofsMsg:  {"GetHelperTrieProofs", MaxHelperTrieProofsFetch},
	SendTxV2Msg:             {"SendTxV2", MaxTxSend},
	GetTxStatusMsg:          {"GetTxStatus", MaxTxStatus},
}

type errCode int

const (
	ErrMsgTooLarge = iota
	ErrDecode
	ErrInvalidMsgCode
	ErrProtocolVersionMismatch
	ErrNetworkIdMismatch
	ErrGenesisBlockMismatch
	ErrNoStatusMsg
	ErrExtraStatusMsg
	ErrSuspendedPeer
	ErrUselessPeer
	ErrRequestRejected
	ErrUnexpectedResponse
	ErrInvalidResponse
	ErrTooManyTimeouts
	ErrMissingKey
)

func (e errCode) String() string {
	return errorToString[int(e)]
}

// XXX change once legacy code is out
var errorToString = map[int]string{
	ErrMsgTooLarge:             "Message too long",
	ErrDecode:                  "Invalid message",
	ErrInvalidMsgCode:          "Invalid message code",
	ErrProtocolVersionMismatch: "Protocol version mismatch",
	ErrNetworkIdMismatch:       "NetworkId mismatch",
	ErrGenesisBlockMismatch:    "Genesis block mismatch",
	ErrNoStatusMsg:             "No status message",
	ErrExtraStatusMsg:          "Extra status message",
	ErrSuspendedPeer:           "Suspended peer",
	ErrRequestRejected:         "Request rejected",
	ErrUnexpectedResponse:      "Unexpected response",
	ErrInvalidResponse:         "Invalid response",
	ErrTooManyTimeouts:         "Too many request timeouts",
	ErrMissingKey:              "Key missing from list",
}

type announceBlock struct {
	Hash       common.Hash // Hash of one particular block being announced
	Number     uint64      // Number of one particular block being announced
	Td         *big.Int    // Total difficulty of one particular block being announced
	FastHash   common.Hash // Hash of one particular block being announced
	FastNumber uint64      // Number of one particular block being announced
}

// announceData is the network packet for the block announcements.
type announceData struct {
	Hash       common.Hash // Hash of one particular block being announced
	Number     uint64      // Number of one particular block being announced
	Td         *big.Int    // Total difficulty of one particular block being announced
	FastHash   common.Hash // Hash of one particular block being announced
	FastNumber uint64      // Number of one particular block being announced
	ReorgDepth uint64
	Update     keyValueList
}

// sanityCheck verifies that the values are reasonable, as a DoS protection
func (a *announceData) sanityCheck() error {
	if tdlen := a.Td.BitLen(); tdlen > 100 {
		return fmt.Errorf("too large block TD: bitlen %d", tdlen)
	}
	return nil
}

// sign adds a signature to the block announcement by the given privKey
func (a *announceData) sign(privKey *ecdsa.PrivateKey) {
	rlp, _ := rlp.EncodeToBytes(announceBlock{Hash: a.Hash, Number: a.Number, Td: a.Td, FastHash: a.FastHash, FastNumber: a.FastNumber})
	sig, _ := crypto.Sign(crypto.Keccak256(rlp), privKey)
	a.Update = a.Update.add("sign", sig)
}

// checkSignature verifies if the block announcement has a valid signature by the given pubKey
func (a *announceData) checkSignature(id enode.ID, update keyValueMap) error {
	var sig []byte
	if err := update.get("sign", &sig); err != nil {
		return err
	}
	rlp, _ := rlp.EncodeToBytes(announceBlock{Hash: a.Hash, Number: a.Number, Td: a.Td, FastHash: a.FastHash, FastNumber: a.FastNumber})
	recPubkey, err := crypto.SigToPub(crypto.Keccak256(rlp), sig)
	if err != nil {
		return err
	}
	if id == enode.PubkeyToIDV4(recPubkey) {
		return nil
	}
	return errors.New("wrong signature")
}

type blockInfo struct {
	Hash       common.Hash // Hash of one particular block being announced
	Number     uint64      // Number of one particular block being announced
	Td         *big.Int    // Total difficulty of one particular block being announced
	FastHash   common.Hash // Hash of one particular block being announced
	FastNumber uint64      // Number of one particular block being announced
}

// getBlockHeadersData represents a block header query.
type getBlockHeadersData struct {
	Origin  hashOrNumber // Block from which to retrieve headers
	Amount  uint64       // Maximum number of headers to retrieve
	Skip    uint64       // Blocks to skip between consecutive headers
	Reverse bool         // Query direction (false = rising towards latest, true = falling towards genesis)
	Fast    bool
	Fruit   bool
}

// hashOrNumber is a combined field for specifying an origin block.
type hashOrNumber struct {
	Hash   common.Hash // Block hash from which to retrieve headers (excludes Number)
	Number uint64      // Block hash from which to retrieve headers (excludes Hash)
}

// EncodeRLP is a specialized encoder for hashOrNumber to encode only one of the
// two contained union fields.
func (hn *hashOrNumber) EncodeRLP(w io.Writer) error {
	if hn.Hash == (common.Hash{}) {
		return rlp.Encode(w, hn.Number)
	}
	if hn.Number != 0 {
		return fmt.Errorf("both origin hash (%x) and number (%d) provided", hn.Hash, hn.Number)
	}
	return rlp.Encode(w, hn.Hash)
}

// DecodeRLP is a specialized decoder for hashOrNumber to decode the contents
// into either a block hash or a block number.
func (hn *hashOrNumber) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	origin, err := s.Raw()
	if err == nil {
		switch {
		case size == 32:
			err = rlp.DecodeBytes(origin, &hn.Hash)
		case size <= 8:
			err = rlp.DecodeBytes(origin, &hn.Number)
		default:
			err = fmt.Errorf("invalid input size %d for origin", size)
		}
	}
	return err
}

// CodeData is the network response packet for a node data retrieval.
type CodeData []struct {
	Value []byte
}

type txStatus struct {
	Status core.TxStatus
	Lookup *rawdb.TxLookupEntry `rlp:"nil"`
	Error  string
}

// getBlockBodiesData represents a block body query.
type getBlockBodiesData struct {
	Hash []common.Hash // Block hash from which to retrieve Bodies (excludes Number)
	Type uint32        // Distinguish fetcher and downloader
}

// BlockBodiesRawData represents a block header send.
type BlockBodiesRawData struct {
	Bodies     []rlp.RawValue
	FruitHeads []*fruitHeadsData
	Type       uint32 // Distinguish fetcher and downloader
}

// blockBodiesData is the network packet for block content distribution.
type snailBlockBodiesData struct {
	Fruits     []*fruitsData
	FruitHeads []*fruitHeadsData
	Type       uint32 // Distinguish fetcher and downloader
}

// blockBody represents the data content of a single block.
type fruitHeadsData struct {
	FruitHead []*types.SnailHeader
}

// blockBody represents the data content of a single block.
type fruitsData struct {
	Fruit []*types.SnailBlock
}

// snailHeadsData is the network packet for block content distribution.
type snailHeadsData struct {
	Heads      []*types.SnailHeader
	FruitHeads []*fruitHeadsData
}

type incompleteBlocks struct {
	Blocks []*incompleteBlock
}

type incompleteBlock struct {
	Head  *types.Header
	Signs []*types.PbftSign
	Infos []*types.CommitteeMember
}

type headsWithSigns struct {
	Heads []*types.Header
	Signs [][]*types.PbftSign
}
