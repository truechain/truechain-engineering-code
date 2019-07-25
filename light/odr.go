// Copyright 2015 The go-ethereum Authors
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

package light

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/log"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/light/public"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	fastDB "github.com/truechain/truechain-engineering-code/core/rawdb"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
)

// NoOdr is the default context passed to an ODR capable function when the ODR
// service is not required.
var NoOdr = context.Background()

// ErrNoPeers is returned if no peers capable of serving a queued request are available
var ErrNoPeers = errors.New("no suitable peers available")

// OdrBackend is an interface to a backend service that handles ODR retrievals type
type OdrBackend interface {
	Database() etruedb.Database
	ChtIndexer() *snailchain.ChainIndexer
	Retrieve(ctx context.Context, req OdrRequest) error
	IndexerConfig() *public.IndexerConfig
}

// OdrRequest is an interface for retrieval requests
type OdrRequest interface {
	StoreResult(db etruedb.Database)
}

// BlockRequest is the ODR request type for retrieving block bodies
type BlockRequest struct {
	OdrRequest
	Hash   common.Hash
	Number uint64
	Rlp    []byte
}

// StoreResult stores the retrieved data in local database
func (req *BlockRequest) StoreResult(db etruedb.Database) {
	rawdb.WriteBodyRLP(db, req.Hash, req.Number, req.Rlp)
}

// ChtRequest is the ODR request type for state/storage trie entries
type ChtRequest struct {
	OdrRequest
	Untrusted        bool   // Indicator whether the result retrieved is trusted or not
	PeerId           string // The specified peer id from which to retrieve data.
	Config           *public.IndexerConfig
	ChtNum, BlockNum uint64
	ChtRoot          common.Hash
	Header           *types.SnailHeader
	Td               *big.Int
	Proof            *public.NodeSet
	Headers          []*types.SnailHeader
	Start            bool
	FHeader          *types.Header
	Dataset          [][]byte
}

// StoreResult stores the retrieved data in local database
func (req *ChtRequest) StoreResult(db etruedb.Database) {
	hash, num := req.Header.Hash(), req.Header.Number.Uint64()

	if !req.Untrusted {
		rawdb.WriteHeader(db, req.Header)
		rawdb.WriteTd(db, hash, num, req.Td)
		rawdb.WriteCanonicalHash(db, hash, num)
	}
	rawdb.WriteLightCheckPoint(db, num)
	if len(req.Headers) > 0 {
		for _, head := range req.Headers {
			rawdb.WriteHeader(db, head)
		}
	}

	epoch := uint64((num - 1) / minerva.UPDATABLOCKLENGTH)
	if count := len(req.Dataset); count > 0 {
		if epoch == 0 {
			if count != minerva.STARTUPDATENUM {
				log.Info("StoreResult hash error", "count", count, "num", num, "epoch", epoch)
			}
			rawdb.WriteLastDataSet(db, epoch, req.Dataset)
		} else {
			if count != minerva.STARTUPDATENUM+int(num%uint64(minerva.UPDATABLOCKLENGTH)-1) {
				log.Info("StoreResult hash error", "count", count, "num", num)
			}
			rawdb.WriteLastDataSet(db, epoch, req.Dataset[minerva.STARTUPDATENUM:])
			rawdb.WriteLastDataSet(db, epoch-1, req.Dataset[:minerva.STARTUPDATENUM])
		}
	}

	fhash, fnum := req.FHeader.Hash(), req.FHeader.Number.Uint64()
	fastDB.WriteHeader(db, req.FHeader)
	fastDB.WriteCanonicalHash(db, fhash, fnum)
	fastDB.WriteHeadHeaderHash(db, fhash)
}

// BlockRequest is the ODR request type for retrieving block bodies
type FruitRequest struct {
	OdrRequest
	Hash   common.Hash
	Number uint64
	Rlp    []byte
}

// StoreResult stores the retrieved data in local database
func (req *FruitRequest) StoreResult(db etruedb.Database) {
	rawdb.WriteBodyRLP(db, req.Hash, req.Number, req.Rlp)
}
