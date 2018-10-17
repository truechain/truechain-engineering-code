package etrue

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/core/types"
	"math/big"
	"time"
)

func (self *PbftAgent) sendSubScribedEvent() {
	self.electionSub = self.election.SubscribeElectionEvent(self.electionCh)
}

func (self *PbftAgent) sendElectionEvent() {
	e := self.election
	go func() {
		members := e.snailchain.GetGenesisCommittee()[:3]
		fmt.Println("loop")
		if self.singleNode {
			time.Sleep(time.Second * 10)
			fmt.Println("len(members)", len(members))
			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(0),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(0),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(1),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStop,
				CommitteeID:      big.NewInt(0),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(1),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(2),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStop,
				CommitteeID:      big.NewInt(1),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(2),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeSwitchover,
				CommitteeID:      big.NewInt(3),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStop,
				CommitteeID:      big.NewInt(2),
				CommitteeMembers: members,
			})

			e.electionFeed.Send(types.ElectionEvent{
				Option:           types.CommitteeStart,
				CommitteeID:      big.NewInt(3),
				CommitteeMembers: members,
			})
		}
	}()
}
