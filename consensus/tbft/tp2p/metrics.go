package tp2p

import (
	"github.com/truechain/truechain-engineering-code/metrics"
)

var (
	TbftPacketInMeter  = metrics.NewRegisteredMeter("consensus/tbft/p2p/in/packets", nil)
	TbftBytesInMeter   = metrics.NewRegisteredMeter("consensus/tbft/p2p/in/bytes", nil)
	TbftPacketOutMeter = metrics.NewRegisteredMeter("consensus/tbft/p2p/out/packets", nil)
	TbftBytesOutMeter  = metrics.NewRegisteredMeter("consensus/tbft/p2p/out/bytes", nil)
)

func MSend(b []byte) {
	TbftPacketOutMeter.Mark(1)
	TbftBytesOutMeter.Mark(int64(len(b)))
}

func MReceive(b []byte) {
	TbftPacketInMeter.Mark(1)
	TbftBytesInMeter.Mark(int64(len(b)))
}
