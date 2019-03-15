package metrics

import (
	"github.com/truechain/truechain-engineering-code/metrics"
	"time"
)

var (
	//Traffic Statistics
	TbftPacketInMeter  = metrics.NewRegisteredMeter("consensus/tbft/p2p/in/packets", nil)
	TbftBytesInMeter   = metrics.NewRegisteredMeter("consensus/tbft/p2p/in/bytes", nil)
	TbftPacketOutMeter = metrics.NewRegisteredMeter("consensus/tbft/p2p/out/packets", nil)
	TbftBytesOutMeter  = metrics.NewRegisteredMeter("consensus/tbft/p2p/out/bytes", nil)

	//Time-consuming statistics
	TBftFetchFastBlockTime     = metrics.NewRegisteredTimer("consensus/tbft/time/FetchFastBlock", nil)
	TBftVerifyFastBlockTime    = metrics.NewRegisteredTimer("consensus/tbft/time/VerifyFastBlock", nil)
	TBftBroadcastConsensusTime = metrics.NewRegisteredTimer("consensus/tbft/time/BroadcastConsensus", nil)

	TBftPreVoteTime   = NewTimeMTimer("consensus/tbft/time/PreVote")
	TBftPreCommitTime = NewTimeMTimer("consensus/tbft/time/PreCommit")

	//FetchFastBlock count statistics
	TBftFetchFastBlockTimesTime = metrics.NewRegisteredTimer("consensus/tbft/count/FetchFastBlock", nil)

	//FetchFastBlock rounds count statistics
	TBftFetchFastBlockRoundTime = metrics.NewRegisteredTimer("consensus/tbft/count/FetchFastBlockRound", nil)
)

type ConsensusTime int
type ConsensusTimes int
type TimesCount int

const (
	FetchFastBlockTime ConsensusTime = iota
	VerifyFastBlockTime
	BroadcastConsensusTime

	PreVoteTime ConsensusTimes = iota
	PreCommitTime

	FetchFastBlockTC TimesCount = iota
	FetchFastBlockRoundTC
)

type TimeMTimer struct {
	T    time.Time
	Self metrics.Timer
}

func NewTimeMTimer(name string) (T TimeMTimer) {
	var t time.Time
	T.T = t
	T.Self = metrics.NewRegisteredTimer(name, nil)
	return
}

func (tmt TimeMTimer) Update(end bool) {
	var t time.Time
	if !end {
		tmt.T = time.Now()
		return
	}
	if end && tmt.T != t {
		tmt.Self.Update(time.Now().Sub(tmt.T))
		tmt.T = t
	}
}

func MSend(b []byte) {
	TbftPacketOutMeter.Mark(1)
	TbftBytesOutMeter.Mark(int64(len(b)))
}

func MReceive(b []byte) {
	TbftPacketInMeter.Mark(1)
	TbftBytesInMeter.Mark(int64(len(b)))
}

func MTime(t ConsensusTime, d time.Duration) {
	switch t {
	case FetchFastBlockTime:
		TBftFetchFastBlockTime.Update(d)
		break
	case VerifyFastBlockTime:
		TBftVerifyFastBlockTime.Update(d)
		break
	case BroadcastConsensusTime:
		TBftBroadcastConsensusTime.Update(d)
		break
	}
}

func MTimes(t ConsensusTimes, end bool) {
	switch t {
	case PreVoteTime:
		TBftPreVoteTime.Update(end)
		break
	case PreCommitTime:
		TBftPreCommitTime.Update(end)
		break
	}

}

func MTimesCount(t TimesCount, d time.Duration) {
	switch t {
	case FetchFastBlockTC:
		TBftFetchFastBlockTimesTime.Update(time.Second * d)
		break
	case FetchFastBlockRoundTC:
		TBftFetchFastBlockRoundTime.Update(time.Second * d)
		break
	}
}
