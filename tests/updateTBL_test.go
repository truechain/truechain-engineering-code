package tests

import (
	"github.com/truechain/truechain-engineering-code/consensus/minerva"
	"github.com/truechain/truechain-engineering-code/core/snailchain"
	"testing"
)

func TestUptdateTBL(t *testing.T) {

	//snailChain,_ := snailchain.MakeChain(540000,9000)
	snailChain, _ := snailchain.MakeChain(180060, 3001)
	minervaL := minerva.NewTester()
	minervaL.SetSnailChainReader(snailChain)

	minervaL.NewTestData(uint64(5))
	minervaL.NewTestData(uint64(3001))
	minervaL.NewTestData(uint64(293))
	minervaL.NewTestData(uint64(4000))
	minervaL.NewTestData(uint64(1500))
}
