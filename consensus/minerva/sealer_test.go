package minerva

import (
	"math/rand"
	"testing"
	"time"
	"github.com/truechain/truechain-engineering-code/core/types"
	//chain "github.com/truechain/truechain-engineering-code/core/snailchain"
	//"github.com/truechain/truechain-engineering-code/core/vm"
	//"github.com/truechain/truechain-engineering-code/ethdb"
	//"github.com/ethereum/go-ethereum/consensus/ethash"
)
type SnailChainReader interface {

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.SnailHeader

}
//TestUpdateLookup Feed datasets to update to get new matrix data
func TestUpdateLookup(t *testing.T) {
	//minerva := NewTester()

	var b [32]byte
	for i := 0; i < 32; i++ {
		b[i] = RandUint() % 255
	}
	/*
	var (
		testdb    = ethdb.NewMemDatabase()
		//gspec     = &Genesis{Config: params.TestChainConfig}
		genesis   = gspec.MustFastCommit(testdb)
		blocks, _ = GenerateChain(params.TestChainConfig, genesis, ethash.NewFaker(), testdb, 8, nil)
	)
	a, _ := chain.NewSnailBlockChain(testdb, nil, nil, nil, vm.Config{})
	//NewBlockChain(testdb, nil, params.TestChainConfig, ethash.NewFaker(), vm.Config{})
	minerva.SetSnailChainReader(a)

	dataset:= make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	//flag,dataset := minerva.updateLookupTBL(22250,dataset)
	flag,dataset := minerva.updateLookupTBL(191788,dataset)
	if flag {
		t.Log("dataset:",dataset)
	}else {
		t.Error("======update-err=====",flag)
	}
	*/
}

func RandUint() uint8 {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return uint8(random.Intn(255))
}
