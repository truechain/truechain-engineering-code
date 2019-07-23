// Copyright 2017 The go-ethereum Authors
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
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/trie"
)

var (
	ErrNoTrustedCht = errors.New("no trusted canonical hash trie")
	ErrNoHeader     = errors.New("header not found")
	chtPrefix       = []byte("chtRoot-") // chtPrefix + chtNum (uint64 big endian) -> trie root hash
	ChtTablePrefix  = "cht-"
)

// ChtNode structures are stored in the Canonical Hash Trie in an RLP encoded format
type ChtNode struct {
	Hash common.Hash
	Td   *big.Int
}

// GetChtRoot reads the CHT root assoctiated to the given section from the database
// Note that sectionIdx is specified according to LES/1 CHT section size
func GetChtRoot(db etruedb.Database, sectionIdx uint64, sectionHead common.Hash) common.Hash {
	var encNumber [8]byte
	binary.BigEndian.PutUint64(encNumber[:], sectionIdx)
	data, _ := db.Get(append(append(chtPrefix, encNumber[:]...), sectionHead.Bytes()...))
	return common.BytesToHash(data)
}

// StoreChtRoot writes the CHT root associated to the given section into the database
// Note that sectionIdx is specified according to LES/1 CHT section size.
func StoreChtRoot(db etruedb.Database, sectionIdx uint64, sectionHead, root common.Hash) {
	var encNumber [8]byte
	binary.BigEndian.PutUint64(encNumber[:], sectionIdx)
	db.Put(append(append(chtPrefix, encNumber[:]...), sectionHead.Bytes()...), root.Bytes())
}

// ChtIndexerBackend implements core.ChainIndexerBackend.
type ChtIndexerBackend struct {
	diskdb, trieTable    etruedb.Database
	odr                  OdrBackend
	triedb               *trie.Database
	section, sectionSize uint64
	lastHash             common.Hash
	trie                 *trie.Trie
}

// NewChtIndexer creates a Cht chain indexer
func NewChtIndexer(db etruedb.Database, odr OdrBackend, size, confirms uint64) *snailchain.ChainIndexer {
	trieTable := etruedb.NewTable(db, ChtTablePrefix)
	backend := &ChtIndexerBackend{
		diskdb:      db,
		odr:         odr,
		trieTable:   trieTable,
		triedb:      trie.NewDatabaseWithCache(trieTable, 1), // Use a tiny cache only to keep memory down
		sectionSize: size,
	}
	return snailchain.NewChainIndexer(db, etruedb.NewTable(db, "chtIndex-"), backend, size, confirms, time.Millisecond*100, "cht")
}

// fetchMissingNodes tries to retrieve the last entry of the latest trusted CHT from the
// ODR backend in order to be able to add new entries and calculate subsequent root hashes
func (c *ChtIndexerBackend) fetchMissingNodes(ctx context.Context, section uint64, root common.Hash) error {
	batch := c.trieTable.NewBatch()
	log.Info("fetchMissingNodes", "section", section, "sectionSize", c.sectionSize, "root", root)
	r := &ChtRequest{ChtRoot: root, ChtNum: section - 1, BlockNum: section*c.sectionSize - 1, Config: c.odr.IndexerConfig()}
	for {
		err := c.odr.Retrieve(ctx, r)
		switch err {
		case nil:
			r.Proof.Store(batch)
			return batch.Write()
		case ErrNoPeers:
			// if there are no peers to serve, retry later
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second * 10):
				// stay in the loop and try again
			}
		default:
			return err
		}
	}
}

// Reset implements core.ChainIndexerBackend
func (c *ChtIndexerBackend) Reset(ctx context.Context, section uint64, lastSectionHead common.Hash) error {
	var root common.Hash
	log.Info("Reset", "section", lastSectionHead, "lastSectionHead", lastSectionHead)
	if section > 0 {
		root = GetChtRoot(c.diskdb, section-1, lastSectionHead)
	}
	var err error
	c.trie, err = trie.New(root, c.triedb)

	if err != nil && c.odr != nil {
		err = c.fetchMissingNodes(ctx, section, root)
		if err == nil {
			c.trie, err = trie.New(root, c.triedb)
		}
	}

	c.section = section
	return err
}

// Process implements core.ChainIndexerBackend
func (c *ChtIndexerBackend) Process(ctx context.Context, header *types.SnailHeader) error {
	hash, num := header.Hash(), header.Number.Uint64()
	c.lastHash = hash
	log.Info("Process", "num", num, "hash", hash)

	td := rawdb.ReadTd(c.diskdb, hash, num)
	if td == nil {
		panic(nil)
	}
	var encNumber [8]byte
	binary.BigEndian.PutUint64(encNumber[:], num)
	data, _ := rlp.EncodeToBytes(ChtNode{hash, td})
	c.trie.Update(encNumber[:], data)
	return nil
}

// Commit implements core.ChainIndexerBackend
func (c *ChtIndexerBackend) Commit() error {
	root, err := c.trie.Commit(nil)
	if err != nil {
		return err
	}
	c.triedb.Commit(root, false)
	log.Info("Process", "section", c.section, "sectionSize", c.sectionSize, "CHTFrequencyClient", params.CHTFrequencyClient, "lastHash", c.lastHash, "root", root)

	if ((c.section+1)*c.sectionSize)%params.CHTFrequencyClient == 0 {
		log.Info("Storing CHT", "section", c.section*c.sectionSize/params.CHTFrequencyClient, "head", fmt.Sprintf("%064x", c.lastHash), "root", fmt.Sprintf("%064x", root))
	}
	StoreChtRoot(c.diskdb, c.section, c.lastHash, root)
	return nil
}
