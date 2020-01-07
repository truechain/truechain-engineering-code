package etrue

import (
	"github.com/truechain/truechain-engineering-code/common"
	"github.com/truechain/truechain-engineering-code/common/hexutil"
	"github.com/truechain/truechain-engineering-code/consensus"
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"github.com/truechain/truechain-engineering-code/core/types"
	"github.com/truechain/truechain-engineering-code/core/vm"
	"github.com/truechain/truechain-engineering-code/crypto"
	"github.com/truechain/truechain-engineering-code/etruedb"
	"github.com/truechain/truechain-engineering-code/log"
	"github.com/truechain/truechain-engineering-code/params"
	"math/big"
	"os"
	"testing"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

func DefaulGenesisBlock() *core.Genesis {
	i, _ := new(big.Int).SetString("10000000000000000000000", 10)
	key1 := hexutil.MustDecode("0x04d341c94a16b02cee86a627d0f6bc6e814741af4cab5065637aa013c9a7d9f26051bb6546030cd67e440d6df741cb65debaaba6c0835579f88a282193795ed369")
	key2 := hexutil.MustDecode("0x0496e0f18d4bf38e0b0de161edd2aa168adaf6842706e5ebf31e1d46cb79fe7b720c750a9e7a3e1a528482b0da723b5dfae739379e555a2893e8693747559f83cd")
	key3 := hexutil.MustDecode("0x0418196ee090081bdec01e8840941b9f6a141a713dd3461b78825edf0d8a7f8cdf3f612832dc9d94249c10c72629ea59fbe0bdd09bea872ddab2799748964c93a8")
	key4 := hexutil.MustDecode("0x04c4935993a3ce206318ab884871fbe2d4dce32a022795c674784f58e7faf3239631b6952b82471fe1e93ef999108a18d028e5d456cd88bb367d610c5e57c7e443")

	return &core.Genesis{
		Config:     params.TestChainConfig,
		Nonce:      928,
		ExtraData:  nil,
		GasLimit:   88080384,
		Difficulty: big.NewInt(20000),
		Alloc: map[common.Address]types.GenesisAccount{
			common.HexToAddress("0x3f9061bf173d8f096c94db95c40f3658b4c7eaad"): {Balance: i},
			common.HexToAddress("0x2cdac3658f85b5da3b70223cc3ad3b2dfe7c1930"): {Balance: i},
			common.HexToAddress("0x41acde8dd7611338c2a30e90149e682566716e9d"): {Balance: i},
			common.HexToAddress("0x0ffd116a3bf97a7112ff8779cc770b13ea3c66a5"): {Balance: i},
		},
		Committee: []*types.CommitteeMember{
			{Coinbase: common.HexToAddress("0x3f9061bf173d8f096c94db95c40f3658b4c7eaad"), Publickey: key1},
			{Coinbase: common.HexToAddress("0x2cdac3658f85b5da3b70223cc3ad3b2dfe7c1930"), Publickey: key2},
			{Coinbase: common.HexToAddress("0x41acde8dd7611338c2a30e90149e682566716e9d"), Publickey: key3},
			{Coinbase: common.HexToAddress("0x0ffd116a3bf97a7112ff8779cc770b13ea3c66a5"), Publickey: key4},
		},
	}
}

/////////////////////////////////////////////////////////////////////
func TestChainImpawnImpl(t *testing.T) {
	var (
		engine = minerva.NewFaker()
		db     = etruedb.NewMemDatabase()
		gspec  = DefaulGenesisBlock()
	)
	params.MinTimeGap = big.NewInt(0)
	params.SnailRewardInterval = big.NewInt(3)

	gspec.Config.TIP8 = &params.BlockConfig{FastNumber: big.NewInt(0)}
	gspec.Config.TIP9 = &params.BlockConfig{SnailNumber: big.NewInt(20)}

	genesis := gspec.MustFastCommit(db)
	blockchain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{})

	snailGenesis := gspec.MustSnailCommit(db)
	snailChain, _ := snailchain.NewSnailBlockChain(db, gspec.Config, engine, blockchain)
	engine.SetSnailChainReader(snailChain)

	parentFast := genesis
	parentSnail := []*types.SnailBlock{snailGenesis}
	for i := 1; i < 71; i++ {

		chain, _ := core.GenerateChain(gspec.Config, parentFast, engine, db, 60, func(i int, gen *core.BlockGen) {
			switch i {
			case 0:
				header := gen.GetHeader()
				rewardSnailBlock(snailChain, blockchain, header)

				stateDb := gen.GetStateDB()
				impl := vm.NewImpawnImpl()
				impl.Load(stateDb, vm.StakingAddress)

				for i := uint64(0); i < 5; i++ {
					value := big.NewInt(100)
					priKey, _ := crypto.GenerateKey()
					from := crypto.PubkeyToAddress(priKey.PublicKey)
					pub := crypto.FromECDSAPub(&priKey.PublicKey)
					err := impl.InsertSAccount2(header.Number.Uint64(), from, pub, value, big.NewInt(50), true)
					if err != nil {
						log.Info("InsertSAccount2", "err", err)
					}
					priKeyDA, _ := crypto.GenerateKey()
					daAddress := crypto.PubkeyToAddress(priKeyDA.PublicKey)
					err = impl.InsertDAccount2(header.Number.Uint64(), daAddress, from, value)
					if err != nil {
						log.Info("InsertDAccount2", "err", err)
					}
				}
				err := impl.Save(stateDb, vm.StakingAddress)
				if err != nil {
					log.Error("ToFastBlock IMPL Save", "error", err)
				}
			}
		})
		if _, err := blockchain.InsertChain(chain); err != nil {
			panic(err)
		}
		parentFast = blockchain.CurrentBlock()
		schain := snailchain.GenerateChain(gspec.Config, blockchain, parentSnail, 1, 7, nil)
		if _, err := snailChain.InsertChain(schain); err != nil {
			panic(err)
		}
		parentSnail = snailChain.GetBlocksFromNumber(0)
	}

}

//generate rewardSnailHegiht
func rewardSnailBlock(chain consensus.SnailChainReader, fastChain *core.BlockChain, header *types.Header) {
	rewardSnailHegiht := fastChain.NextSnailNumberReward()
	space := new(big.Int).Sub(chain.CurrentHeader().Number, rewardSnailHegiht).Int64()
	if space >= params.SnailRewardInterval.Int64() {
		header.SnailNumber = rewardSnailHegiht
		sb := chain.GetHeaderByNumber(rewardSnailHegiht.Uint64())
		if sb != nil {
			header.SnailHash = sb.Hash()
		} else {
			log.Error("cannot find snailBlock by rewardSnailHegiht.", "snailHeight", rewardSnailHegiht.Uint64())
		}
	}
}
