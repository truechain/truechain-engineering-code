package tbft

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/types"
	types2 "github.com/truechain/truechain-engineering-code/core/types"
	"math/rand"
	"testing"
)

func BinaryBareTest(o interface{}, o2 interface{}) {
	byte2, _ := cdc.MarshalBinaryBare(o)
	if err := cdc.UnmarshalBinaryBare(byte2, o2); err == nil {
		fmt.Println(o)
		fmt.Println(o2)
	} else {
		fmt.Println(err.Error())
	}
}

func BinaryTest(o interface{}, o2 interface{}) {
	byte2, _ := cdc.MarshalBinary(o)
	if err := cdc.UnmarshalBinary(byte2, o2); err == nil {
		fmt.Println(o)
		fmt.Println(o2)
	} else {
		fmt.Println(err.Error())
	}
}

func Tests(o interface{}, o2 interface{}) {
	BinaryBareTest(o, o2)
	BinaryTest(o, o2)
}

func TestVoteSetBitsMessage(t *testing.T) {
	msg := &VoteSetBitsMessage{Height: 1, Round: 1, Type: 1, BlockID: types.BlockID{}, Votes: &help.BitArray{}}
	var msgOut VoteSetBitsMessage
	Tests(msg, &msgOut)
}

func TestNewRoundStepMessage(t *testing.T) {
	msg1 := &NewRoundStepMessage{Height: 1, Round: 1, SecondsSinceStartTime: 1, Step: 1, LastCommitRound: 1}
	var msg1Out NewRoundStepMessage
	Tests(msg1, &msg1Out)
}

func TestCommitStepMessage(t *testing.T) {
	msg1 := &CommitStepMessage{Height: 1, BlockParts: &help.BitArray{}, BlockPartsHeader: types.PartSetHeader{}}
	var msg1Out CommitStepMessage
	Tests(msg1, &msg1Out)
}

func TestHasVoteMessage(t *testing.T) {
	msg1 := &HasVoteMessage{Height: 1, Round: 1, Type: 1, Index: 1}
	var msg1Out HasVoteMessage
	Tests(msg1, &msg1Out)
}

func TestBlockPartMessage(t *testing.T) {
	msg1 := &BlockPartMessage{Height: 1, Round: 1, Part: &types.Part{}}
	var msg1Out BlockPartMessage
	Tests(msg1, &msg1Out)
}

func TestVoteSetMaj23Message(t *testing.T) {
	msg1 := &VoteSetMaj23Message{Height: 1, Round: 1, Type: 1, BlockID: types.BlockID{Hash: RandHexBytes(), PartsHeader: types.PartSetHeader{Total: 1, Hash: RandHexBytes()}}}
	var msg1Out VoteSetMaj23Message
	Tests(msg1, &msg1Out)
}

func RandUint() uint8 {
	//random := rand.New(rand.NewSource(time.Now().Unix()))
	return uint8(rand.Intn(255))
}

func RandHexBytes() []byte {
	b := make([]byte, 20)
	for i := 0; i < 20; i++ {
		b[i] = RandUint() % 255
	}
	return b
}

func RandHexBytes32() [32]byte {
	var b [32]byte
	for i := 0; i < 32; i++ {
		b[i] = RandUint() % 255
	}
	return b
}

func TestVoteRlpCommitteeMember(t *testing.T) {
	var si types2.SwitchInfos
	si.CID = common.Big1
	var m types2.CommitteeMember
	m.CommitteeBase = common.Address{0}
	m.Coinbase = common.Address{0}
	m.Publickey = []byte{1, 2}
	m.MType = 1
	m.Flag = 1
	si.Members = append(si.Members, &m)
	si.BackMembers = append(si.Members, &m)

	si.Vals = append(si.Vals, &types2.SwitchEnter{CommitteeBase: common.Address{0}, Flag: 1})
	c, e := rlp.EncodeToBytes(si)
	fmt.Println(c, e)
}
