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

package params

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/truechain/truechain-engineering-code/crypto"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/math"
)

// Genesis hashes to enforce below configs on.
var (
	MainnetGenesisHash      = common.HexToHash("0x0c6e644fcbd396f7b235ecef44551c45afd9274e87cd77ec6e9778cf8bfb46fc")
	MainnetSnailGenesisHash = common.HexToHash("0xf82fd9c0c8a53474c9e40e4f1c0583a94609eaf88dae01a5496da459398485c6")

	TestnetGenesisHash      = common.HexToHash("0x4b82a68ebbf32f2e816754f2b50eda0ae2c0a71dd5f4e0ecd93ccbfb7dba00b8")
	TestnetSnailGenesisHash = common.HexToHash("0x4ab1748c057b744de202d6ebea64e8d3a0b2ec4c19abbc59e8639967b14b7c96")

	DevnetSnailGenesisHash = common.HexToHash("0xdf819f11beead767f91a6c05d74e5f902fc2988e9039a969a023bc75e467cdeb")
)

// TrustedCheckpoints associates each known checkpoint with the genesis hash of
// the chain it belongs to.
var TrustedCheckpoints = map[common.Hash]*TrustedCheckpoint{
	MainnetSnailGenesisHash: MainnetTrustedCheckpoint,
	TestnetSnailGenesisHash: TestnetTrustedCheckpoint,
	DevnetSnailGenesisHash:  DevnetTrustedCheckpoint,
}

var (
	// MainnetChainConfig is the chain parameters to run a node on the main network.
	MainnetChainConfig = &ChainConfig{
		ChainID: big.NewInt(19330),
		Minerva: &(MinervaConfig{
			MinimumDifficulty:      big.NewInt(134217728),
			MinimumFruitDifficulty: big.NewInt(262144),
			DurationLimit:          big.NewInt(600),
		}),
		TIP3:  &BlockConfig{FastNumber: big.NewInt(1500000)},
		TIP5:  &BlockConfig{SnailNumber: big.NewInt(12800)},
		TIP7:  &BlockConfig{FastNumber: big.NewInt(6226000)},
		TIP8:  &BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(293)},
		TIP9:  &BlockConfig{SnailNumber: big.NewInt(47000)},
		TIP10: &BlockConfig{FastNumber: big.NewInt(6520000), CID: big.NewInt(302)},
		TIP11: &BlockConfig{FastNumber: big.NewInt(8996000)},
		TIP12: &BlockConfig{FastNumber: big.NewInt(155500000)},
	}

	// MainnetTrustedCheckpoint contains the light client trusted checkpoint for the main network.
	MainnetTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 227,
		SectionHead:  common.HexToHash("0xa2e0b25d72c2fc6e35a7f853cdacb193b4b4f95c606accf7f8fa8415283582c7"),
		CHTRoot:      common.HexToHash("0xf69bdd4053b95b61a27b106a0e86103d791edd8574950dc96aa351ab9b9f1aa0"),
		BloomRoot:    common.HexToHash("0xec1b454d4c6322c78ccedf76ac922a8698c3cac4d98748a84af4995b7bd3d744"),
	}

	// MainnetCheckpointOracle contains a set of configs for the main network oracle.
	MainnetCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0x9a9070028361F7AAbeB3f2F2Dc07F82C4a98A02a"),
		Signers: []common.Address{
			common.HexToAddress("0x1b2C260efc720BE89101890E4Db589b44E950527"), // Peter
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), // Martin
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), // Zsolt
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), // Gary
			common.HexToAddress("0x0DF8fa387C602AE62559cC4aFa4972A7045d6707"), // Guillaume
		},
		Threshold: 2,
	}

	// TestnetChainConfig contains the chain parameters to run a node on the Ropsten test network.
	TestnetChainConfig = &ChainConfig{
		ChainID: big.NewInt(18928),
		Minerva: &(MinervaConfig{
			MinimumDifficulty:      big.NewInt(60000),
			MinimumFruitDifficulty: big.NewInt(200),
			DurationLimit:          big.NewInt(600),
		}),
		TIP3:  &BlockConfig{FastNumber: big.NewInt(450000)},
		TIP5:  &BlockConfig{SnailNumber: big.NewInt(4000)},
		TIP7:  &BlockConfig{FastNumber: big.NewInt(4666000)},
		TIP8:  &BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(215)},
		TIP9:  &BlockConfig{SnailNumber: big.NewInt(38648)},
		TIP10: &BlockConfig{FastNumber: big.NewInt(5034600), CID: big.NewInt(229)},
		TIP11: &BlockConfig{FastNumber: big.NewInt(7552000)},
		TIP12: &BlockConfig{FastNumber: big.NewInt(0)},
	}

	// TestnetTrustedCheckpoint contains the light client trusted checkpoint for the Ropsten test network.
	TestnetTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex: 161,
		SectionHead:  common.HexToHash("0x5378afa734e1feafb34bcca1534c4d96952b754579b96a4afb23d5301ecececc"),
		CHTRoot:      common.HexToHash("0x1cf2b071e7443a62914362486b613ff30f60cea0d9c268ed8c545f876a3ee60c"),
		BloomRoot:    common.HexToHash("0x5ac25c84bd18a9cbe878d4609a80220f57f85037a112644532412ba0d498a31b"),
	}

	// TestnetCheckpointOracle contains a set of configs for the Ropsten test network oracle.
	TestnetCheckpointOracle = &CheckpointOracleConfig{
		Address: common.HexToAddress("0xEF79475013f154E6A65b54cB2742867791bf0B84"),
		Signers: []common.Address{
			common.HexToAddress("0x32162F3581E88a5f62e8A61892B42C46E2c18f7b"), // Peter
			common.HexToAddress("0x78d1aD571A1A09D60D9BBf25894b44e4C8859595"), // Martin
			common.HexToAddress("0x286834935f4A8Cfb4FF4C77D5770C2775aE2b0E7"), // Zsolt
			common.HexToAddress("0xb86e2B0Ab5A4B1373e40c51A7C712c70Ba2f9f8E"), // Gary
			common.HexToAddress("0x0DF8fa387C602AE62559cC4aFa4972A7045d6707"), // Guillaume
		},
		Threshold: 2,
	}

	// DevnetChainConfig contains the chain parameters to run a node on the Ropsten test network.
	DevnetChainConfig = &ChainConfig{
		ChainID: big.NewInt(100),
		Minerva: &(MinervaConfig{
			MinimumDifficulty:      big.NewInt(10000),
			MinimumFruitDifficulty: big.NewInt(100),
			DurationLimit:          big.NewInt(150),
		}),
		TIP3:  &BlockConfig{FastNumber: big.NewInt(380000)},
		TIP5:  &BlockConfig{SnailNumber: big.NewInt(5000)},
		TIP7:  &BlockConfig{FastNumber: big.NewInt(0)},
		TIP8:  &BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(0)},
		TIP9:  &BlockConfig{SnailNumber: big.NewInt(20)},
		TIP10: &BlockConfig{FastNumber: big.NewInt(40000), CID: big.NewInt(117)},
		TIP11: &BlockConfig{FastNumber: big.NewInt(0)},
		TIP12: &BlockConfig{FastNumber: big.NewInt(0)},
	}

	SingleNodeChainConfig = &ChainConfig{
		ChainID: big.NewInt(400),
		Minerva: &(MinervaConfig{
			MinimumDifficulty:      big.NewInt(200),
			MinimumFruitDifficulty: big.NewInt(2),
			DurationLimit:          big.NewInt(120),
		}),
		TIP3:  &BlockConfig{FastNumber: big.NewInt(380000)},
		TIP5:  &BlockConfig{SnailNumber: big.NewInt(5000)},
		TIP7:  &BlockConfig{FastNumber: big.NewInt(0)},
		TIP8:  &BlockConfig{FastNumber: big.NewInt(100), CID: big.NewInt(-1)},
		TIP9:  &BlockConfig{SnailNumber: big.NewInt(20)},
		TIP10: &BlockConfig{FastNumber: big.NewInt(0), CID: big.NewInt(1)},
		TIP11: &BlockConfig{FastNumber: big.NewInt(0)},
		TIP12: &BlockConfig{FastNumber: big.NewInt(0)},
	}

	// TestnetTrustedCheckpoint contains the light client trusted checkpoint for the Ropsten test network.
	DevnetTrustedCheckpoint = &TrustedCheckpoint{
		SectionIndex:  12,
		SectionHead:   common.HexToHash("0xa672246bf631e2ea05977c8720a7c318564e4f2251436a5edd9ea6a0ea31e423"),
		CHTRoot:       common.HexToHash("0x4f234caa182b92a792929fe6ff9aa85fe30c81b8525a1c8f73f044de1b31b2cf"),
		SectionBIndex: 34,
		SectionBHead:  common.HexToHash("0xd455c656df21d60886b45b16f28ee017bd9e48ba7d8df0997ebf282c703aca9c"),
		BloomRoot:     common.HexToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421"),
		DSRoot:        common.HexToHash("0x4f234caa182b92a792929fe6ff9aa85fe30c81b8525a1c8f73f044de1b31b2cf"),
	}

	chainId = big.NewInt(10000)
	// This configuration is intentionally not using keyed fields to force anyone
	// adding flags to the config to also have to set these fields.
	AllMinervaProtocolChanges = &ChainConfig{ChainID: chainId, Minerva: new(MinervaConfig), TIP3: &BlockConfig{FastNumber: big.NewInt(0)},
		TIP5: nil, TIP7: nil, TIP8: nil, TIP9: nil, TIP10: nil, TIP11: &BlockConfig{FastNumber: big.NewInt(0)},
	}

	// This configuration is intentionally not using keyed fields to force anyone
	// adding flags to the config to also have to set these fields.

	TestChainConfig = &ChainConfig{ChainID: chainId, Minerva: &MinervaConfig{MinimumDifficulty, MinimumFruitDifficulty, DurationLimit}, TIP3: &BlockConfig{FastNumber: big.NewInt(0)},
		TIP5: nil, TIP7: nil, TIP8: nil, TIP9: nil, TIP10: nil, TIP11: &BlockConfig{FastNumber: big.NewInt(0)},
	}
)

// TrustedCheckpoint represents a set of post-processed trie roots (CHT and
// BloomTrie) associated with the appropriate section index and head hash. It is
// used to start light syncing from this checkpoint and avoid downloading the
// entire header chain while still being able to securely access old headers/logs.
type TrustedCheckpoint struct {
	SectionIndex  uint64      `json:"sectionIndex"`
	SectionHead   common.Hash `json:"sectionHead"`
	CHTRoot       common.Hash `json:"chtRoot"`
	SectionBIndex uint64      `json:"sectionBIndex"`
	SectionBHead  common.Hash `json:"sectionBHead"`
	BloomRoot     common.Hash `json:"bloomRoot"`
	DSRoot        common.Hash `json:"datasetRoot"`
}

// HashEqual returns an indicator comparing the itself hash with given one.
func (c *TrustedCheckpoint) HashEqual(hash common.Hash) bool {
	if c.Empty() {
		return hash == common.Hash{}
	}
	return c.Hash() == hash
}

// Hash returns the hash of checkpoint's four key fields(index, sectionHead, chtRoot and bloomTrieRoot).
func (c *TrustedCheckpoint) Hash() common.Hash {
	buf := make([]byte, 8+3*common.HashLength)
	binary.BigEndian.PutUint64(buf, c.SectionIndex)
	copy(buf[8:], c.SectionHead.Bytes())
	copy(buf[8+common.HashLength:], c.CHTRoot.Bytes())
	copy(buf[8+2*common.HashLength:], c.BloomRoot.Bytes())
	return crypto.Keccak256Hash(buf)
}

// Empty returns an indicator whether the checkpoint is regarded as empty.
func (c *TrustedCheckpoint) Empty() bool {
	return c.SectionHead == (common.Hash{}) || c.CHTRoot == (common.Hash{}) || c.BloomRoot == (common.Hash{})
}

// CheckpointOracleConfig represents a set of checkpoint contract(which acts as an oracle)
// config which used for light client checkpoint syncing.
type CheckpointOracleConfig struct {
	Address   common.Address   `json:"address"`
	Signers   []common.Address `json:"signers"`
	Threshold uint64           `json:"threshold"`
}

// ChainConfig is the core config which determines the blockchain settings.
//
// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection

	// Various consensus engines
	Minerva *MinervaConfig `json:"minerva"`
	//Clique *CliqueConfig  `json:"clique,omitempty"`

	TIP3 *BlockConfig `json:"tip3"`

	TIP5  *BlockConfig `json:"tip5"`
	TIP7  *BlockConfig `json:"tip7"`
	TIP8  *BlockConfig `json:"tip8"`
	TIP9  *BlockConfig `json:"tip9"`
	TIP10 *BlockConfig `json:"tip10"`
	TIP11 *BlockConfig `json:"tip11"`
	TIP12 *BlockConfig `json:"tip12"`

	TIPStake *BlockConfig `json:"tipstake"`
}

type BlockConfig struct {
	FastNumber  *big.Int
	SnailNumber *big.Int
	CID         *big.Int
}

func (c *ChainConfig) UnmarshalJSON(input []byte) error {
	type ChainConfig struct {
		ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection

		Minerva *MinervaConfig `json:"minerva"`
	}
	var dec ChainConfig
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	c.ChainID = dec.ChainID
	if dec.Minerva == nil {
		c.Minerva = &(MinervaConfig{
			MinimumDifficulty:      MinimumDifficulty,
			MinimumFruitDifficulty: MinimumFruitDifficulty,
			DurationLimit:          DurationLimit,
		})
	} else {
		c.Minerva = dec.Minerva
	}

	return nil
}

// MinervaConfig is the consensus engine configs for proof-of-work based sealing.
type MinervaConfig struct {
	MinimumDifficulty      *big.Int `json:"minimumDifficulty"`
	MinimumFruitDifficulty *big.Int `json:"minimumFruitDifficulty"`
	DurationLimit          *big.Int `json:"durationLimit"`
}

func (c *MinervaConfig) UnmarshalJSON(input []byte) error {
	type MinervaConfig struct {
		MinimumDifficulty      *math.HexOrDecimal256 `json:"minimumDifficulty"`
		MinimumFruitDifficulty *math.HexOrDecimal256 `json:"minimumFruitDifficulty"`
		DurationLimit          *math.HexOrDecimal256 `json:"durationLimit"`
	}
	var dec MinervaConfig
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.MinimumDifficulty == nil {
		c.MinimumDifficulty = MinimumDifficulty
		//return errors.New("missing required field 'MinimumDifficulty' for Genesis")
	} else {
		c.MinimumDifficulty = (*big.Int)(dec.MinimumDifficulty)
	}
	if dec.MinimumFruitDifficulty == nil {
		c.MinimumFruitDifficulty = MinimumFruitDifficulty
		//return errors.New("missing required field 'MinimumFruitDifficulty' for Genesis")
	} else {
		c.MinimumFruitDifficulty = (*big.Int)(dec.MinimumFruitDifficulty)
	}
	if dec.DurationLimit == nil {
		c.DurationLimit = DurationLimit
		//return errors.New("missing required field 'DurationLimit' for Genesis")
	} else {
		c.DurationLimit = (*big.Int)(dec.DurationLimit)
	}
	return nil
}

func (c MinervaConfig) MarshalJSON() ([]byte, error) {
	type MinervaConfig struct {
		MinimumDifficulty      *math.HexOrDecimal256 `json:"minimumDifficulty,omitempty"`
		MinimumFruitDifficulty *math.HexOrDecimal256 `json:"minimumFruitDifficulty,omitempty"`
		DurationLimit          *math.HexOrDecimal256 `json:"durationLimit,omitempty"`
	}
	var enc MinervaConfig
	enc.MinimumDifficulty = (*math.HexOrDecimal256)(c.MinimumDifficulty)
	enc.MinimumFruitDifficulty = (*math.HexOrDecimal256)(c.MinimumFruitDifficulty)
	enc.DurationLimit = (*math.HexOrDecimal256)(c.DurationLimit)
	return json.Marshal(&enc)
}

// String implements the stringer interface, returning the consensus engine details.
func (c *MinervaConfig) String() string {
	return fmt.Sprintf("{MinimumDifficulty: %v MinimumFruitDifficulty: %v DurationLimit: %v}",
		c.MinimumDifficulty,
		c.MinimumFruitDifficulty,
		c.DurationLimit,
	)
}

// String implements the fmt.Stringer interface.
func (c *ChainConfig) String() string {
	var engine interface{}
	switch {
	case c.Minerva != nil:
		engine = c.Minerva
		// case c.Clique != nil:
		// 	engine = c.Clique
	default:
		engine = "unknown"
	}
	return fmt.Sprintf("{ChainID: %v Engine: %v}",
		c.ChainID,
		engine,
	)
}

// CheckCompatible checks whether scheduled fork transitions have been imported
// with a mismatching chain configuration.
func (c *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *ConfigCompatError {
	bhead := new(big.Int).SetUint64(height)

	// Iterate checkCompatible to find the lowest conflict.
	var lasterr *ConfigCompatError
	for {
		err := c.checkCompatible(newcfg, bhead)
		if err == nil || (lasterr != nil && err.RewindTo == lasterr.RewindTo) {
			break
		}
		lasterr = err
		bhead.SetUint64(err.RewindTo)
	}
	return lasterr
}

func (c *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	return nil
}

// isForkIncompatible returns true if a fork scheduled at s1 cannot be rescheduled to
// block s2 because head is already past the fork.
func isForkIncompatible(s1, s2, head *big.Int) bool {
	return (isForked(s1, head) || isForked(s2, head)) && !configNumEqual(s1, s2)
}

// isForked returns whether a fork scheduled at block s is active at the given head block.
func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func configNumEqual(x, y *big.Int) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return x.Cmp(y) == 0
}

// ConfigCompatError is raised if the locally-stored blockchain is initialised with a
// ChainConfig that would alter the past.
type ConfigCompatError struct {
	What string
	// block numbers of the stored and new configurations
	StoredConfig, NewConfig *big.Int
	// the block number to which the local chain must be rewound to correct the error
	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

// Rules wraps ChainConfig and is merely syntatic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainID                          *big.Int
	IsTIP3, IsTIP7, IsTIP11, IsTIP12 bool
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{
		ChainID: new(big.Int).Set(chainID),
		IsTIP3:  c.IsTIP3(num),
		IsTIP7:  c.IsTIP7(num),
		IsTIP11: c.IsTIP11(num),
		IsTIP12: c.IsTIP12(num),
	}
}

// IsTIP3 returns whether num is either equal to the IsTIP3 fork block or greater.
func (c *ChainConfig) IsTIP3(num *big.Int) bool {
	if c.TIP3 == nil {
		return false
	}
	return isForked(c.TIP3.FastNumber, num)
}

// IsTIP5 returns whether num is either equal to the IsTIP5 fork block or greater.
func (c *ChainConfig) IsTIP5(num *big.Int) bool {
	if c.TIP5 == nil {
		return false
	}
	return isForked(c.TIP5.SnailNumber, num)
}
func (c *ChainConfig) IsTIP7(num *big.Int) bool {
	if c.TIP7 == nil {
		return false
	}
	return isForked(c.TIP7.FastNumber, num)
}

func (c *ChainConfig) IsTIP8(cid, num *big.Int) bool {
	if c.TIP8 == nil {
		return false
	}
	res := cid.Cmp(c.TIP8.CID)
	if res > 0 || (res == 0 && num.Cmp(c.TIP8.FastNumber) >= 0) {
		return true
	}
	return false
}

func (c *ChainConfig) IsTIP9(num *big.Int) bool {
	if c.TIP9 == nil {
		return false
	}
	return isForked(c.TIP9.SnailNumber, num)
}
func (c *ChainConfig) IsTIP10(num *big.Int) bool {
	if c.TIP10 == nil {
		return false
	}
	return isForked(c.TIP10.FastNumber, num)
}

func (c *ChainConfig) IsTIP11(num *big.Int) bool {
	if c.TIP11 == nil {
		return false
	}
	return isForked(c.TIP11.FastNumber, num)
}

func (c *ChainConfig) IsTIP12(num *big.Int) bool {
	if c.TIP11 == nil {
		return false
	}
	return isForked(c.TIP12.FastNumber, num)
}
