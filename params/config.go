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
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
)

// Genesis hashes to enforce below configs on.
var (
	MainnetGenesisHash      = common.HexToHash("0x0c6e644fcbd396f7b235ecef44551c45afd9274e87cd77ec6e9778cf8bfb46fc")
	MainnetSnailGenesisHash = common.HexToHash("0xf82fd9c0c8a53474c9e40e4f1c0583a94609eaf88dae01a5496da459398485c6")

	TestnetGenesisHash      = common.HexToHash("0x8559a3cad3e5702fe057fe6709234183140537504f2167338a7d851f4426c2b6")
	TestnetSnailGenesisHash = common.HexToHash("0x386389953605bf5fae7296a0523dd02b8a28c2bbbbd5c8a8f395319cbb938b74")
)

var (
	// MainnetChainConfig is the chain parameters to run a node on the main network.
	MainnetChainConfig = &ChainConfig{
		ChainID: big.NewInt(19330),
		Minerva: &(MinervaConfig{
			MinimumDifficulty:      big.NewInt(134217728),
			MinimumFruitDifficulty: big.NewInt(262144),
			DurationLimit:          big.NewInt(600),
		}),
	}

	// TestnetChainConfig contains the chain parameters to run a node on the Ropsten test network.
	TestnetChainConfig = &ChainConfig{
		ChainID: big.NewInt(18928),
		Minerva: &(MinervaConfig{
			MinimumDifficulty:      big.NewInt(60000),
			MinimumFruitDifficulty: big.NewInt(200),
			DurationLimit:          big.NewInt(600),
		}),
		TIP12Block:big.NewInt(306346),
	}

	// DevnetChainConfig contains the chain parameters to run a node on the Ropsten test network.
	DevnetChainConfig = &ChainConfig{
		ChainID: big.NewInt(100),
		Minerva: &(MinervaConfig{
			MinimumDifficulty:      big.NewInt(10000),
			MinimumFruitDifficulty: big.NewInt(100),
			DurationLimit:          big.NewInt(150),
		}),
	}


	 chainId =big.NewInt(9223372036854775790)
	// This configuration is intentionally not using keyed fields to force anyone
	// adding flags to the config to also have to set these fields.
	AllMinervaProtocolChanges = &ChainConfig{chainId, new(MinervaConfig),big.NewInt(0)}

	// This configuration is intentionally not using keyed fields to force anyone
	// adding flags to the config to also have to set these fields.

	TestChainConfig = &ChainConfig{chainId, &MinervaConfig{MinimumDifficulty, MinimumFruitDifficulty, DurationLimit},big.NewInt(0)}
)

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

	TIP12Block *big.Int
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

// GasTable returns the gas table corresponding to the current phase (homestead or homestead reprice).
//
// The returned GasTable's fields shouldn't, under any circumstances, be changed.
func (c *ChainConfig) GasTable(num *big.Int) GasTable {
	return GasTableConstantinople

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
	ChainID *big.Int
	IsTIP12 bool
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainID := c.ChainID
	if chainID == nil {
		chainID = new(big.Int)
	}
	return Rules{
		ChainID: new(big.Int).Set(chainID),
		IsTIP12: c.IsTIP12(num),
	}
}

// IsTIP12 returns whether num is either equal to the TIP12 fork block or greater.
func (c *ChainConfig) IsTIP12(num *big.Int) bool {
	return isForked(c.TIP12Block, num)
}
