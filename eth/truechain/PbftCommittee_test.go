package truechain

import (
	"testing"
	"fmt"
	"time"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common"
)

func Test1(t *testing.T) {
	member := &CommitteeMember{
		"111",
		"127.4.6.9",
		8087,
	}
	bytes,_ :=toByte(member)
	m :=&CommitteeMember{
		Nodeid:"444",
	}
	fromByte(bytes,m)
	fmt.Println(m)
}


func MakeFirstCommittee(curCmmCount int) *PbftCommittee{
	if curCmmCount > keysCount {
		return nil
	}
	curCmm := make([]*CommitteeMember,0,0)
	for i:=0;i<curCmmCount;i++ {
		nodeid :=hex.EncodeToString(crypto.FromECDSAPub(GetPub(privkeys[i])))
		cc := CommitteeMember{
			Addr:			"127.0.0.1",
			Port:			16745,
			Nodeid:			nodeid,
		}
		curCmm = append(curCmm,&cc)
	}
	cmm := PbftCommittee{
		No:				1,
		Ct:				time.Now(),
		Lastt:			time.Now(),
		Count:			curCmmCount,
		Lcount:			0,
		Comm:			curCmm,
		Lcomm:			nil,
		Sig:			make([]string,0,0),
	}
	sig := cmm.Sig
	for i:=0;i<keysCount/2;i++ {
		k,_ := crypto.Sign(cmm.GetHash(),privkeys[i])
		sig = append(sig,common.ToHex(k))
	}
	cmm.Sig =sig
	return &cmm
}