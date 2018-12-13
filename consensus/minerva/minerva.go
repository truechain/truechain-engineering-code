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

// Package minerva implements the truechain hybrid consensus engine.
package minerva

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/edsrzf/mmap-go"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/metrics"
	"github.com/truechain/truechain-engineering-code/params"
	"github.com/truechain/truechain-engineering-code/rpc"
)

// ErrInvalidDumpMagic errorinfo
var ErrInvalidDumpMagic = errors.New("invalid dump magic")

var (
	// maxUint256 is a big integer representing 2^256-1
	maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))

	maxUint128 = new(big.Int).Exp(big.NewInt(2), big.NewInt(128), big.NewInt(0))

	// sharedMinerva is a full instance that can be shared between multiple users.
	sharedMinerva = New(Config{"", 3, 0, "", 1, 0, ModeNormal})

	// algorithmRevision is the data structure version used for file naming.
	algorithmRevision = 1

	// dumpMagic is a dataset dump header to sanity check a data dump.
	dumpMagic = []uint32{0xbaddcafe, 0xfee1dead}

	//SnailBlockRewardsInitial Snail block rewards initial 116.48733*10^18
	SnailBlockRewardsInitial = new(big.Int).Mul(big.NewInt(11648733), big.NewInt(1e13))

	//SnailBlockRewardsBase Snail block rewards base value is 115.555555555555 * 10^12
	SnailBlockRewardsBase = 115555555555555

	// Big1e6 up to wei  SnailBlockRewardsBase * this is wei
	Big1e6 = big.NewInt(1e6)

	// SnailBlockRewardsChangeInterval Snail block rewards change interval 4500 blocks
	SnailBlockRewardsChangeInterval = 4500

	// SnailBlockRewardsChangePercentage snall block rewards change interval decrease %2
	SnailBlockRewardsChangePercentage = 2

	//BaseBig ...
	BaseBig = big.NewInt(1e18)

	//NetworkFragmentsNuber The number of main network fragments is currently fixed at 1
	NetworkFragmentsNuber = 1

	//MiningConstant Mining constant is 20
	MiningConstant = 20

	//SqrtMin pbft and miner allocation constant
	//Generating formula :TestOutSqrt
	SqrtMin = 25

	//SqrtMax ...
	SqrtMax = 6400

	//SnailBlockBodyFruitInitial Snail block body fruit initial 30*10^15
	SnailBlockBodyFruitInitial = new(big.Int).Mul(big.NewInt(30), big.NewInt(1e15))

	//SnailBlockRewardsFruitRatio Snail block rewards fruit ratio  10%
	SnailBlockRewardsFruitRatio = 0.1

	//CommitteesCount Number of committees
	CommitteesCount = new(big.Int).SetInt64(1)

	//MinerCount Miner quantity
	MinerCount = new(big.Int).SetInt64(1)
)

// ConstSqrt ...
type ConstSqrt struct {
	Num  int     `json:"num"`
	Sqrt float64 `json:"sqrt"`
}

// isLittleEndian returns whether the local system is running in little or big
// endian byte order.
func isLittleEndian() bool {
	n := uint32(0x01020304)
	return *(*byte)(unsafe.Pointer(&n)) == 0x04
}

// memoryMap tries to memory map a file of uint32s for read only access.
func memoryMap(path string) (*os.File, mmap.MMap, []uint32, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		return nil, nil, nil, err
	}
	mem, buffer, err := memoryMapFile(file, false)
	if err != nil {
		file.Close()
		return nil, nil, nil, err
	}
	for i, magic := range dumpMagic {
		if buffer[i] != magic {
			mem.Unmap()
			file.Close()
			return nil, nil, nil, ErrInvalidDumpMagic
		}
	}
	return file, mem, buffer[len(dumpMagic):], err
}

// memoryMapFile tries to memory map an already opened file descriptor.
func memoryMapFile(file *os.File, write bool) (mmap.MMap, []uint32, error) {
	// Try to memory map the file
	flag := mmap.RDONLY
	if write {
		flag = mmap.RDWR
	}
	mem, err := mmap.Map(file, flag, 0)
	if err != nil {
		return nil, nil, err
	}
	// Yay, we managed to memory map the file, here be dragons
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&mem))
	header.Len /= 4
	header.Cap /= 4

	return mem, *(*[]uint32)(unsafe.Pointer(&header)), nil
}

// memoryMapAndGenerate tries to memory map a temporary file of uint32s for write
// access, fill it with the data from a generator and then move it into the final
// path requested.
func memoryMapAndGenerate(path string, size uint64, generator func(buffer []uint32)) (*os.File, mmap.MMap, []uint32, error) {
	// Ensure the data folder exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, nil, nil, err
	}
	// Create a huge temporary empty file to fill with data
	temp := path + "." + strconv.Itoa(rand.Int())

	dump, err := os.Create(temp)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = dump.Truncate(int64(len(dumpMagic))*4 + int64(size)); err != nil {
		return nil, nil, nil, err
	}
	// Memory map the file for writing and fill it with the generator
	mem, buffer, err := memoryMapFile(dump, true)
	if err != nil {
		dump.Close()
		return nil, nil, nil, err
	}
	copy(buffer, dumpMagic)

	data := buffer[len(dumpMagic):]
	generator(data)

	if err := mem.Unmap(); err != nil {
		return nil, nil, nil, err
	}
	if err := dump.Close(); err != nil {
		return nil, nil, nil, err
	}
	if err := os.Rename(temp, path); err != nil {
		return nil, nil, nil, err
	}
	return memoryMap(path)
}

// lru tracks caches or datasets by their last use time, keeping at most N of them.
type lru struct {
	what string
	new  func(epoch uint64) interface{}
	mu   sync.Mutex
	// Items are kept in a LRU cache, but there is a special case:
	// We always keep an item for (highest seen epoch) + 1 as the 'future item'.
	cache      *simplelru.LRU
	future     uint64
	futureItem interface{}
}

// newlru create a new least-recently-used cache for either the verification caches
// or the mining datasets.
func newlru(what string, maxItems int, new func(epoch uint64) interface{}) *lru {
	if maxItems <= 1 {
		maxItems = 5
	}
	cache, _ := simplelru.NewLRU(maxItems, func(key, value interface{}) {
		log.Trace("Evicted minerva "+what, "epoch", key)
	})
	return &lru{what: what, new: new, cache: cache}
}

// get retrieves or creates an item for the given epoch. The first return value is always
// non-nil. The second return value is non-nil if lru thinks that an item will be useful in
// the near future.
func (lru *lru) get(epoch uint64) (item, future interface{}) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	// Get or create the item for the requested epoch.
	item, ok := lru.cache.Get(epoch)
	if !ok {
		if lru.future > 0 && lru.future == epoch {
			item = lru.futureItem
		} else {
			log.Trace("Requiring new minerva "+lru.what, "epoch", epoch)
			item = lru.new(epoch)
		}
		lru.cache.Add(epoch, item)
	}

	return item, future
}

// dataset wraps an truehash dataset with some metadata to allow easier concurrent use.
type dataset struct {
	epoch uint64 // Epoch for which this cache is relevant
	//dump    *os.File  // File descriptor of the memory mapped cache
	//mmap    mmap.MMap // Memory map itself to unmap before releasing
	dataset  []uint64  // The actual cache data content
	once     sync.Once // Ensures the cache is generated only once
	dateInit int
}

// newDataset creates a new truehash mining dataset
func newDataset(epoch uint64) interface{} {
	ds := &dataset{
		epoch:    epoch,
		dateInit: 0,
		dataset:  make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32),
	}
	//truehashTableInit(ds.evenDataset)

	return ds
}

// Mode defines the type and amount of PoW verification an minerva engine makes.
type Mode uint

// constant
const (
	ModeNormal Mode = iota
	ModeShared
	ModeTest
	ModeFake
	ModeFullFake
)

// Config are the configuration parameters of the minerva.
type Config struct {
	CacheDir       string
	CachesInMem    int
	CachesOnDisk   int
	DatasetDir     string
	DatasetsInMem  int
	DatasetsOnDisk int
	PowMode        Mode
}

// Minerva consensus
type Minerva struct {
	config Config

	//caches   *lru // In memory caches to avoid regenerating too often
	datasets *lru // In memory datasets to avoid regenerating too often

	// Mining related fields
	rand     *rand.Rand    // Properly seeded random source for nonces
	threads  int           // Number of threads to mine on if mining
	update   chan struct{} // Notification channel to update mining parameters
	hashrate metrics.Meter // Meter tracking the average hashrate

	// The fields below are hooks for testing
	shared    *Minerva      // Shared PoW verifier to avoid cache regeneration
	fakeFail  uint64        // Block number which fails PoW check even in fake mode
	fakeDelay time.Duration // Time delay to sleep for before returning from verify

	lock sync.Mutex // Ensures thread safety for the in-memory caches and mining fields

	sbc      consensus.SnailChainReader
	election consensus.CommitteeElection
}

//var MinervaLocal *Minerva

// New creates a full sized minerva hybrid consensus scheme.
func New(config Config) *Minerva {
	if config.CachesInMem <= 0 {
		//log.Warn("One minerva cache must always be in memory", "requested", config.CachesInMem)
		config.CachesInMem = 1
	}
	if config.CacheDir != "" && config.CachesOnDisk > 0 {
		//log.Info("Disk storage enabled for minerva caches", "dir", config.CacheDir, "count", config.CachesOnDisk)
	}
	if config.DatasetDir != "" && config.DatasetsOnDisk > 0 {
		//log.Info("Disk storage enabled for minerva DAGs", "dir", config.DatasetDir, "count", config.DatasetsOnDisk)
	}

	minerva := &Minerva{
		config: config,
		//caches:   newlru("cache", config.CachesInMem, newCache),
		datasets: newlru("dataset", config.DatasetsInMem, newDataset),
		update:   make(chan struct{}),
		hashrate: metrics.NewMeter(),
	}

	//MinervaLocal.CheckDataSetState(1)
	minerva.getDataset(1)

	return minerva
}

// NewTestData Method test usage
func (m *Minerva) NewTestData(block uint64) {
	m.getDataset(block)
}

// dataset tries to retrieve a mining dataset for the specified block number
func (m *Minerva) getDataset(block uint64) *dataset {
	// Retrieve the requested ethash dataset
	epoch := block / epochLength
	//log.Info("epoch value: ", epoch, "------", "block number is: ", block)
	currentI, _ := m.datasets.get(epoch)
	current := currentI.(*dataset)

	current.generate(block, m)

	return current
}

// generate ensures that the dataset content is generated before use.
func (d *dataset) generate(blockNum uint64, m *Minerva) {
	d.once.Do(func() {
		//fmt.Println("d.once:",blockNum)
		if d.dateInit == 0 {
			//d.dataset = make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
			// blockNum <= UPDATABLOCKLENGTH
			if blockNum <= UPDATABLOCKLENGTH {
				log.Info("TableInit is start,:blockNum is:  ", "------", blockNum)
				m.truehashTableInit(d.dataset)
			} else {
				//bn := (blockNum/UPDATABLOCKLENGTH-1)*UPDATABLOCKLENGTH + STARTUPDATENUM + 1
				bn := (blockNum/UPDATABLOCKLENGTH-1)*UPDATABLOCKLENGTH + STARTUPDATENUM + 1
				log.Info("updateLookupTBL is start,:blockNum is:  ", "------", blockNum)
				//d.Flag = 0
				flag, ds := m.updateLookupTBL(bn, d.dataset)
				if flag {
					d.dataset = ds
				} else {
					log.Error("updateLookupTBL is err  ", "blockNum is:  ", blockNum)
				}
			}
			d.dateInit = 1
		}
	})

	//go d.updateTable(blockNum, m)

}

func (d *dataset) updateTable(blockNum uint64, m *Minerva) {
	if blockNum%UPDATABLOCKLENGTH == STARTUPDATENUM+1 {
		epoch := blockNum / epochLength
		currentI, _ := m.datasets.get(epoch + 1)
		current := currentI.(*dataset)
		if current.dateInit == 0 {
			current.generate(blockNum, m)
			//fmt.Println(blockNum)
		}
	}
}

//SetSnailChainReader Append interface SnailChainReader after instantiations
func (m *Minerva) SetSnailChainReader(scr consensus.SnailChainReader) {
	m.sbc = scr
}

//SetElection Append interface CommitteeElection after instantiation
func (m *Minerva) SetElection(e consensus.CommitteeElection) {
	m.election = e
}

// GetElection return election
func (m *Minerva) GetElection() consensus.CommitteeElection {
	return m.election

}

// NewTester creates a small sized minerva scheme useful only for testing
// purposes.
func NewTester() *Minerva {
	return New(Config{CachesInMem: 1, PowMode: ModeTest})
}

// NewFaker creates a minerva consensus engine with a fake PoW scheme that accepts
// all blocks' seal as valid, though they still have to conform to the Ethereum
// consensus rules.
func NewFaker() *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFake,
		},
		election: newFakeElection(),
	}
}

// NewFakeFailer creates a minerva consensus engine with a fake PoW scheme that
// accepts all blocks as valid apart from the single one specified, though they
// still have to conform to the Ethereum consensus rules.
func NewFakeFailer(fail uint64) *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFake,
		},
		fakeFail: fail,
	}
}

// NewFakeDelayer creates a minerva consensus engine with a fake PoW scheme that
// accepts all blocks as valid, but delays verifications by some time, though
// they still have to conform to the Ethereum consensus rules.
func NewFakeDelayer(delay time.Duration) *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFake,
		},
		fakeDelay: delay,
	}
}

// NewFullFaker creates an minerva consensus engine with a full fake scheme that
// accepts all blocks as valid, without checking any consensus rules whatsoever.
func NewFullFaker() *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFullFake,
		},
	}
}

// NewShared creates a full sized minerva shared between all requesters running
// in the same process.
func NewShared() *Minerva {
	return &Minerva{shared: sharedMinerva}
}

// Threads returns the number of mining threads currently enabled. This doesn't
// necessarily mean that mining is running!
func (m *Minerva) Threads() int {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.threads
}

// SetThreads updates the number of mining threads currently enabled. Calling
// this method does not start mining, only sets the thread count. If zero is
// specified, the miner will use all cores of the machine. Setting a thread
// count below zero is allowed and will cause the miner to idle, without any
// work being done.
func (m *Minerva) SetThreads(threads int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// If we're running a shared PoW, set the thread count on that instead
	if m.shared != nil {
		m.shared.SetThreads(threads)
		return
	}
	// Update the threads and ping any running seal to pull in any changes
	m.threads = threads
	select {
	case m.update <- struct{}{}:
	default:
	}
}

// Hashrate implements PoW, returning the measured rate of the search invocations
// per second over the last minute.
func (m *Minerva) Hashrate() float64 {
	log.Debug("minerva  hashrate", "hash", m.hashrate.Rate1())
	return m.hashrate.Rate1()
}

// APIs implements consensus.Engine, returning the user facing RPC APIs. Currently
// that is empty.
func (m *Minerva) APIs(chain consensus.ChainReader) []rpc.API {
	return nil
}

// SeedHash is the seed to use for generating a verification cache and the mining
// dataset.
func SeedHash(block uint64) []byte {
	return seedHash(block)
}

type fakeElection struct {
	privates []*ecdsa.PrivateKey
	members  []*types.CommitteeMember
}

func newFakeElection() *fakeElection {
	var priKeys []*ecdsa.PrivateKey
	var members []*types.CommitteeMember

	for i := 0; int64(i) < params.MinimumCommitteeNumber.Int64(); i++ {
		priKey, err := crypto.GenerateKey()
		priKeys = append(priKeys, priKey)
		if err != nil {
			log.Error("initMembers", "error", err)
		}
		coinbase := crypto.PubkeyToAddress(priKey.PublicKey)
		m := &types.CommitteeMember{coinbase, &priKey.PublicKey, types.StateUsedFlag} //todo helei
		members = append(members, m)
	}
	return &fakeElection{privates: priKeys, members: members}
}

func (e *fakeElection) GetCommittee(fastNumber *big.Int) []*types.CommitteeMember {
	return e.members
}

func (e *fakeElection) VerifySigns(signs []*types.PbftSign) ([]*types.CommitteeMember, []error) {
	var (
		members = make([]*types.CommitteeMember, len(signs))
		errs    = make([]error, len(signs))
	)

	for i, sign := range signs {
		pubkey, _ := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
		pubkeyByte := crypto.FromECDSAPub(pubkey)
		for _, m := range e.members {
			if bytes.Equal(pubkeyByte, crypto.FromECDSAPub(m.Publickey)) {
				members[i] = m
			}
		}
	}

	return members, errs
}

func (e *fakeElection) GenerateFakeSigns(fb *types.Block) ([]*types.PbftSign, error) {
	var signs []*types.PbftSign
	for _, privateKey := range e.privates {
		voteSign := &types.PbftSign{
			Result:     types.VoteAgree,
			FastHeight: fb.Header().Number,
			FastHash:   fb.Hash(),
		}
		var err error
		signHash := voteSign.HashWithNoSign().Bytes()
		voteSign.Sign, err = crypto.Sign(signHash, privateKey)
		if err != nil {
			log.Error("fb GenerateSign error ", "err", err)
		}
		signs = append(signs, voteSign)
	}
	return signs, nil
}
