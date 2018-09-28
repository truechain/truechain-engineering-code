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

package snailchain

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/common/math"
	rawdb "github.com/truechain/truechain-engineering-code/core/snailchain/rawdb"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/ethdb"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
)

//go:generate gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config     *params.ChainConfig `json:"config"`
	Nonce      uint64              `json:"nonce"`
	Timestamp  uint64              `json:"timestamp"`
	ExtraData  []byte              `json:"extraData"`
	GasLimit   uint64              `json:"gasLimit"   gencodec:"required"`
	Difficulty *big.Int            `json:"difficulty" gencodec:"required"`
	Mixhash    common.Hash         `json:"mixHash"`
	Coinbase   common.Address      `json:"coinbase"`
	Alloc      types.GenesisAlloc  `json:"alloc"      gencodec:"required"`
	//committee info
	Committee []*types.CommitteeMember `json:"committee"      gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Nonce      math.HexOrDecimal64
	Timestamp  math.HexOrDecimal64
	ExtraData  hexutil.Bytes
	GasLimit   math.HexOrDecimal64
	GasUsed    math.HexOrDecimal64
	Number     math.HexOrDecimal64
	Difficulty *math.HexOrDecimal256
	// Alloc      map[common.UnprefixedAddress]types.GenesisAccount
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return params.AllEthashProtocolChanges, common.Hash{}, errGenesisNoConfig
	}
	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		return genesis.Config, block.Hash(), err
	}

	// Check whether the genesis block is already written.
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &types.GenesisMismatchError{stored, hash}
		}
	}

	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return newcfg, stored, nil
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	if genesis == nil && stored != params.MainnetGenesisHash {
		return storedcfg, stored, nil
	}

	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	return newcfg, stored, nil
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	switch {
	case g != nil:
		return g.Config
	case ghash == params.MainnetGenesisHash:
		return params.MainnetChainConfig
	case ghash == params.TestnetGenesisSHash:
		return params.TestnetChainConfig
	default:
		return params.AllEthashProtocolChanges
	}
}

// ToBlock creates the genesis block and writes state of a genesis specification
// to the given database (or discards it if nil).
func (g *Genesis) ToBlock(db ethdb.Database) *types.SnailBlock {
	if db == nil {
		db = ethdb.NewMemDatabase()
	}

	head := &types.SnailHeader{
		Number:     new(big.Int).SetUint64(g.Number),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       new(big.Int).SetUint64(g.Timestamp),
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
	}

	if g.Difficulty == nil {
		head.Difficulty = params.GenesisDifficulty
	}

	return types.NewSnailBlock(head, nil, nil, nil)
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (g *Genesis) Commit(db ethdb.Database) (*types.SnailBlock, error) {
	block := g.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, fmt.Errorf("can't commit genesis block with number > 0")
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), g.Difficulty)
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())
	rawdb.WriteCommittee(db, block.NumberU64(), g.Committee)

	config := g.Config
	if config == nil {
		config = params.AllEthashProtocolChanges
	}
	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}

// MustCommit writes the genesis block and state to db, panicking on error.
// The block is committed as the canonical head block.
func (g *Genesis) MustCommit(db ethdb.Database) *types.SnailBlock {
	block, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}

// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.SnailBlock {
	g := Genesis{}
	return g.MustCommit(db)
}

// DefaultGenesisBlock returns the Ethereum main net genesis block.
func DefaultGenesisBlock() *Genesis {
	i, _ := new(big.Int).SetString("90000000000000000000000", 10)
	key1, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04044308742b61976de7344edb8662d6d10be1c477dd46e8e4c433c1288442a79183480894107299ff7b0706490f1fb9c9b7c9e62ae62d57bd84a1e469460d8ac1"))
	key2, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04ae5b1e301e167f9676937a2733242429ce7eb5dd2ad9f354669bc10eff23015d9810d17c0c680a1178b2f7d9abd925d5b62c7a463d157aa2e3e121d2e266bfc6"))
	key3, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04013151837b19e4b0e7402ac576e4352091892d82504450864fc9fd156ddf15d22014a0f6bf3c8f9c12d03e75f628736f0c76b72322be28e7b6f0220cf7f4f5fb"))
	key4, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04e3e59c07b320b5d35d65917d50806e1ee99e3d5ed062ed24d3435f61a47d29fb2f2ebb322011c1d2941b4853ce2dc71e8c4af57b59bbf40db66f76c3c740d41b"))

	return &Genesis{
		Config:     params.MainnetChainConfig,
		Nonce:      66,
		ExtraData:  nil,
		GasLimit:   88080384,
		Difficulty: big.NewInt(256),
		//Alloc:      decodePrealloc(mainnetAllocData),
		Alloc: map[common.Address]types.GenesisAccount{
			common.HexToAddress("0x7c357530174275dd30e46319b89f71186256e4f7"): {Balance: i},
			common.HexToAddress("0x4cf807958b9f6d9fd9331397d7a89a079ef43288"): {Balance: i},
			common.HexToAddress("0x04d2252a3e0ca7c2aa81247ca33060855a34a808"): {Balance: i},
			common.HexToAddress("0x05712ff78d08eaf3e0f1797aaf4421d9b24f8679"): {Balance: i},
			common.HexToAddress("0x764727f61dd0717a48236842435e9aefab6723c3"): {Balance: i},
			common.HexToAddress("0x764986534dba541d5061e04b9c561abe3f671178"): {Balance: i},
			common.HexToAddress("0x0fd0bbff2e5b3ddb4f030ff35eb0fe06658646cf"): {Balance: i},
			common.HexToAddress("0x40b3a743ba285a20eaeee770d37c093276166568"): {Balance: i},
			common.HexToAddress("0x9d3c4a33d3bcbd2245a1bebd8e989b696e561eae"): {Balance: i},
			common.HexToAddress("0x35c9d83c3de709bbd2cb4a8a42b89e0317abe6d4"): {Balance: i},
		},
		Committee: []*types.CommitteeMember{
			&types.CommitteeMember{Coinbase: common.HexToAddress("0x76ea2f3a002431fede1141b660dbb75c26ba6d97"), Publickey: key1},
			&types.CommitteeMember{Coinbase: common.HexToAddress("0x831151b7eb8e650dc442cd623fbc6ae20279df85"), Publickey: key2},
			&types.CommitteeMember{Coinbase: common.HexToAddress("0x1074f7deccf8c66efcd0106e034d3356b7db3f2c"), Publickey: key3},
			&types.CommitteeMember{Coinbase: common.HexToAddress("0xd985e9871d1be109af5a7f6407b1d6b686901fff"), Publickey: key4},
		},
	}
}

// DefaultTestnetGenesisBlock returns the Ropsten network genesis block.
func DefaultTestnetGenesisBlock() *Genesis {
	// amount, _ := new(big.Int).SetString("90000000000000000000000", 10)
	seedkey1, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x042afba5a6680b5361bb57761ca67a7ea309d2883bda93c5d9521078258bb97b03610002865fb27993fcea4918023144eb516706ea33c7c94fef7b2f330cb9d0a6"))
	seedkey2, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04e444bc40b6d1372a955fb9bb9a986ceb1c13a450794151fbf48033189351f6bddddcbebfa5c6d205887551e9527e6deff2cbee9f233ffe14fd15db4beb9c9f34"))
	seedkey3, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x049620df839696f4451842fd543b38d171f7f215dcd2c7fcd813c0206f097206a67b25ad719fbb62570c4a4ba467ec61aa396788e3ae79c704a62ea759beca3175"))
	seedkey4, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04f714bb815a9ecc505eae7e756b63753850df92a0fe4c99dc8b6660ba17bbcbb88000d9efb524eb38746ef4505ad2ab1895efccbcc966d4c685c811bda7c9d8ef"))
	
	seedkey5, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04574194462132a05d45923f322f13bfa12dbe2e536c3743915ef16d412353d7e060a835cded6f9883efc1ce1feec99c04c930e7561741b0da5286185328edeff5"))
	seedkey6, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04a49218f37e7640a7809f7f2b7213c0db128ace84288a23cf6ed00e2e26fa83d9c3f7ae1dfad2f4d9b28fc4ab846c32751f53a8614d253d96e217725e8ef80f68"))
	seedkey7, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04c9194f5bf60f9d51ecccc869df245ccaa313e40343625a9368d8bc284e17e034dcaa592355968bfc26dc086c9574fcf81e4f7009d70ebd8a217ead24b5a3edc6"))
	seedkey8, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x046b0cd08cf85d0bc61214033ca353b221897808eb69fc78e6be234857acda15a64445a0d47e146605683ff859db50244f8566a1d166f887102000b4e58584277d"))
	seedkey9, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04aa09d866ba5a229798d88bdb1f1774aa18ba1fbad3335319296a2d6c749f37a792677cdcb7e51db49d45725c0691cc4f592b8156774567a43a67dff2869a14bf"))
	seedkey10, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04731b126dfee319cd594c7f8c0b5831f53d53be80cf1c388802ca6b903d5328989b62abb6db5dcd5ba95a299442ebfdeeaf892d3440fbcd410aa144a24a71745d"))
	seedkey11, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04dfb7a1f0c3d51e8a3f54d9e8b60a0710e33737ff899160ea9fabba3417e610b17c91d7a434bfe1e717459ad4f9bafa01fdcc5790fa2f581c02419e8c27af645c"))
	seedkey12, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x041c96578ce1a662a8e4d158338e8ebbbc7f5f703590eeb11cfaffd0153bbffc827bbfa2a085e62d68dbc58793d9b5b5eee9af858ad732c11efbfa8413cdbc7d18"))
	seedkey13, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x0485a695f5ffaa932ca3df1cf76eb973927858e1955af93b698e662bc34b9c3ffb650753207dec97f1b37a7f6e43447823da036e005023f8db536ef7dc96c3b292"))
	seedkey14, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x0479a1b0ca954f1f92a5d090b32ebdc359b9897db9f564847ad50d40749e050429ddbd6a781c838e7b81ded83d089664f5e8434a21fa29530bfbf7351db9ad0c8a"))
	seedkey15, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04074940e9c0b9a760881f8027573483b238687a9b349a812d5bae1e2beaf0fcda007b5920804a9eae319e798bc35b71e26f105abd0980cdc6a7af4ad36860711f"))
	seedkey16, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04d8b14b77628b0843d99daa05b64feb05edf203faae7a3a1ab6ddeb2c65d97cebc1a672aa9b77a66ec02ce9820c4b2d1acd7f787081ee51528caed65bd8b0c37a"))
	seedkey17, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04cae56bbfe43e5a13ba04d0480a93ccf983ed8792c5360d05cecbe9e23209a6c89456f6a93959c6df1911ff86b97c64774620d414ddb0ee107b8ac8abe4b859b7"))
	// seedkey18, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x0427ddb370d76974851824317614d36a096d4f30cdaaf41f3def7c5273f5068ee738072c1f8bdee2d32495b1086827fc55a0aee544c09eb4d370ab960f79551e82"))
	// seedkey19, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x042cc58e015149f5f1c1deb3c44e5b714b08c7eaf8974436b750991bca99422449fbd237719c03da3d2861362f1b7cd2258b2df212ee46adc8ea302b5b656021d9"))
	// seedkey20, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x043a5bdd8ee2e9f1ee2c30fb1edaddf57bd2e89c30074f0b60bc55ef9c8592d9cb494eecc02cf790c3265c0714bbbc2f31480372d09f53835ddae022a66da9ff80"))
	// seedkey21, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x0453e40cd692a70cff4761c65917703e28bfc3bd152119a2eba38719a76a8e06de6a1e066c8426d58c123d3265a7364755a9036533992ddcc04a2256610bf3d094"))
	// seedkey22, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x047e5d4b18ed909f86877346b206e8f2a55ba3c413c16efccc8776277decdaec3029bc11e7097330728ec3a83fc5836cd271a17e994206191475f556db29d7d119"))
	// seedkey23, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04493b5c8d3b79ea613158d3618eb4d304cded8ab86c6aae426cd15ceec5a076c821757bbc079ade778419c552a088901ae01c5e3003769d8d93e0b0b1dc1fff91"))
	// seedkey24, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x040c362e304fa52540a65139298dc3ca53538f070654f232614cc5e3137ff5a5bef0fc95c722e181fb44df421fc911c68d4bf22c41a05ac91a268737238743e875"))
	// seedkey25, _ := crypto.UnmarshalPubkey(hexutil.MustDecode("0x04c7b8da951d402926d79e43d06cc12024f01be1a9e7632dc783c747e876b718affc7ec33c4de96c33f3ff3dd497165c2cb1b643cb1756a0b3b4740577e22c28e3"))

	coinbase := common.HexToAddress("0x0000000000000000000000000000000000000000")
	return &Genesis{
		Config:     params.TestnetChainConfig,
		Nonce:      66,
		ExtraData:  nil,
		GasLimit:   20971520,
		Timestamp:	1537891200,
		Difficulty: big.NewInt(600000),
		Coinbase: 	common.HexToAddress("0x0000000000000000000000000000000000000000"),
		Mixhash: 	common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		ParentHash: common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		Alloc: map[common.Address]types.GenesisAccount{
			// common.HexToAddress("0x7c357530174275dd30e46319b89f71186256e4f7"): {Balance: amount},
			// common.HexToAddress("0x4cf807958b9f6d9fd9331397d7a89a079ef43288"): {Balance: amount},
		},
		Committee: []*types.CommitteeMember{
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey1},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey2},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey3},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey4},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey5},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey6},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey7},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey8},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey9},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey10},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey11},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey12},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey13},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey14},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey15},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey16},
			&types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey17},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey18},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey19},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey20},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey21},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey22},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey23},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey24},
			// &types.CommitteeMember{Coinbase: coinbase, Publickey: seedkey25},
		},
	}
}

// DefaultRinkebyGenesisBlock returns the Rinkeby network genesis block.
func DefaultRinkebyGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.RinkebyChainConfig,
		Timestamp:  1492009146,
		ExtraData:  hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e42eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		//Alloc:      decodePrealloc(rinkebyAllocData),
	}
}

// DeveloperGenesisBlock returns the 'getrue --dev' genesis block. Note, this must
// be seeded with the
func DeveloperGenesisBlock(period uint64, faucet common.Address) *Genesis {
	// Override the default period to the user requested one
	config := *params.AllCliqueProtocolChanges
	//config.Clique.Period = period

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	return &Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, 65)...),
		GasLimit:   6283185,
		Difficulty: big.NewInt(1),
		// Alloc: map[common.Address]types.GenesisAccount{
		// 	common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
		// 	common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
		// 	common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
		// 	common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
		// 	common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
		// 	common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
		// 	common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
		// 	common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
		// 	faucet: {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		// },
	}
}
