package minerva

import (
	"math/rand"
	"testing"
	"time"
)

//TestUpdateLookup Feed datasets to update to get new matrix data
func TestUpdateLookup(t *testing.T) {
	//minerva := NewTester()
	var b [32]byte
	for i := 0; i < 32; i++ {
		b[i] = RandUint() % 255
	}
	//dataset:= make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32)
	//flag,dataset := minerva.updateLookupTBL(22250,dataset)
	//if flag {
	//	t.Log("dataset:",dataset)
	//}else {
	//	t.Error("======update-err=====",flag)
	//}
}

func RandUint() uint8 {
	random := rand.New(rand.NewSource(time.Now().Unix()))
	return uint8(random.Intn(255))
}
