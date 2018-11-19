package minerva

import (
	"math/rand"
	"testing"
	"time"
	"github.com/truechain/truechain-engineering-code/core/types"
	"fmt"
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
	for i:=10240;i<0;i++{
		if i%12000 >= 10240{
			fmt.Println("i %  12000",i%12000,"  i = ",i)
		}
	}
	fmt.Println("  (12000*3)+1%12000",(12000*3+1)%12000)
	/*

	var (
		testdb    = ethdb.NewMemDatabase()
		//gspec     = &Genesis{Config: params.TestChainConfig}
		//genesis   = gspec.MustFastCommit(testdb)
		//blocks, _ = GenerateChain(params.TestChainConfig, genesis, ethash.NewFaker(), testdb, 8, nil)
	)
	a, _ := chain.NewSnailBlockChain(testdb, nil, nil, nil, vm.Config{})
	//NewBlockChain(testdb, nil, params.TestChainConfig, ethash.NewFaker(), vm.Config{})
	minerva.SetSnailChainReader(a)
	fmt.Println("191788 %  12000",191788%12000)
	dataset:= make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	//flag,dataset := minerva.updateLookupTBL(22250,dataset)
	flag,dataset := minerva.updateLookupTBL(191788,dataset)
	if flag {
		t.Log("dataset:",dataset)
	}else {
		t.Error("======update-err=====",flag)
	}*/

}

func RandUint() uint8 {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return uint8(random.Intn(255))
}
