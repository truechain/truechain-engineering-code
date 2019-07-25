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

package light

import (
	"bytes"
	"context"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
)

var sha3_nil = crypto.Keccak256Hash(nil)

func GetHeaderByNumber(ctx context.Context, odr OdrBackend, number uint64) (*types.SnailHeader, error) {
	db := odr.Database()
	hash := rawdb.ReadCanonicalHash(db, number)
	if (hash != common.Hash{}) {
		// if there is a canonical hash, there is a header too
		header := rawdb.ReadHeader(db, hash, number)
		if header == nil {
			panic("Canonical hash present but header not found")
		}
		return header, nil
	}

	var (
		chtCount, sectionHeadNum uint64
		sectionHead              common.Hash
		start                    bool
	)
	if odr.ChtIndexer() != nil {
		chtCount, sectionHeadNum, sectionHead = odr.ChtIndexer().Sections()
		canonicalHash := rawdb.ReadCanonicalHash(db, sectionHeadNum)
		log.Info("GetHeaderByNumber", "chtCount", chtCount, "sectionHeadNum", sectionHeadNum, "sectionHead", sectionHead, "canonicalHash", canonicalHash)
		// if the CHT was injected as a trusted checkpoint, we have no canonical hash yet so we accept zero hash too
		for chtCount > 0 && canonicalHash != sectionHead && canonicalHash != (common.Hash{}) {
			chtCount--
			if chtCount > 0 {
				sectionHeadNum = chtCount*odr.IndexerConfig().ChtSize - 1
				sectionHead = odr.ChtIndexer().SectionHead(chtCount - 1)
				canonicalHash = rawdb.ReadCanonicalHash(db, sectionHeadNum)
			}
		}
	}
	if number >= chtCount*odr.IndexerConfig().ChtSize {
		return nil, ErrNoTrustedCht
	}
	if hash == (common.Hash{}) {
		start = true
	}
	log.Info("GetHeaderByNumber", "number", number, "chtCount", chtCount, "ChtSize", odr.IndexerConfig().ChtSize)
	r := &ChtRequest{ChtRoot: GetChtRoot(db, chtCount-1, sectionHead), ChtNum: chtCount - 1, BlockNum: number, Config: odr.IndexerConfig(), Start: start}
	if err := odr.Retrieve(ctx, r); err != nil {
		return nil, err
	}
	return r.Header, nil
}

// GetUntrustedHeaderByNumber fetches specified block header without correctness checking.
// Note this function should only be used in light client checkpoint syncing.
func GetUntrustedHeaderByNumber(ctx context.Context, odr OdrBackend, number uint64, peerId string) (*types.Header, error) {
	r := &ChtRequest{BlockNum: number, ChtNum: number / odr.IndexerConfig().ChtSize, Untrusted: true, PeerId: peerId, Config: odr.IndexerConfig()}
	if err := odr.Retrieve(ctx, r); err != nil {
		return nil, err
	}
	return r.FHeader, nil
}

func GetCanonicalHash(ctx context.Context, odr OdrBackend, number uint64) (common.Hash, error) {
	hash := rawdb.ReadCanonicalHash(odr.Database(), number)
	if (hash != common.Hash{}) {
		return hash, nil
	}
	header, err := GetHeaderByNumber(ctx, odr, number)
	if header != nil {
		return header.Hash(), nil
	}
	return common.Hash{}, err
}

// GetBodyRLP retrieves the block body (transactions and uncles) in RLP encoding.
func GetBodyRLP(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (rlp.RawValue, error) {
	if data := rawdb.ReadBodyRLP(odr.Database(), hash, number); data != nil {
		return data, nil
	}
	r := &BlockRequest{Hash: hash, Number: number}
	if err := odr.Retrieve(ctx, r); err != nil {
		return nil, err
	} else {
		return r.Rlp, nil
	}
}

// GetBody retrieves the block body (transactons, uncles) corresponding to the
// hash.
func GetBody(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (*types.SnailBody, error) {
	data, err := GetBodyRLP(ctx, odr, hash, number)
	if err != nil {
		return nil, err
	}
	body := new(types.SnailBody)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		return nil, err
	}
	return body, nil
}

// GetBlock retrieves an entire block corresponding to the hash, assembling it
// back from the stored header and body.
func GetBlock(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (*types.SnailBlock, error) {
	// FastRetrieve the block header and body contents
	header := rawdb.ReadHeader(odr.Database(), hash, number)
	if header == nil {
		return nil, ErrNoHeader
	}
	body, err := GetBody(ctx, odr, hash, number)
	if err != nil {
		return nil, err
	}
	// Reassemble the block and return
	return types.NewSnailBlockWithHeader(header).WithBody(body.Fruits, nil), nil
}

// GetFruitBody retrieves the fruit body (transactons, uncles) corresponding to the
// hash.
func GetFruitBody(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (*types.SnailBody, error) {
	data, err := GetBodyRLP(ctx, odr, hash, number)
	if err != nil {
		return nil, err
	}
	body := new(types.SnailBody)
	if err := rlp.Decode(bytes.NewReader(data), body); err != nil {
		return nil, err
	}
	return body, nil
}

// GetFruit retrieves an entire fruit corresponding to the hash, assembling it
// back from the stored header and body.
func GetFruit(ctx context.Context, odr OdrBackend, hash common.Hash, number uint64) (*types.SnailBlock, error) {
	// FastRetrieve the block header and body contents
	header := rawdb.ReadHeader(odr.Database(), hash, number)
	if header == nil {
		return nil, ErrNoHeader
	}
	body, err := GetFruitBody(ctx, odr, hash, number)
	if err != nil {
		return nil, err
	}
	// Reassemble the block and return
	return types.NewSnailBlockWithHeader(header).WithBody(body.Fruits, nil), nil
}
