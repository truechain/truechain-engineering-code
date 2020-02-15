// Copyright 2018 The TrueChain Authors
// This file is part of the truechain-engineering-code library.
//
// The truechain-engineering-code library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The truechain-engineering-code library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the truechain-engineering-code library. If not, see <http://www.gnu.org/licenses/>.

package dashboard

import (
	"time"
)

// collectCommitteeData gathers data about the committee and sends it to the clients.
func (db *Dashboard) collectCommitteeData() {
	defer db.wg.Done()
	agent := db.etrue.PbftAgent()

	for {
		select {
		case errc := <-db.quit:
			errc <- nil
			return
		case <-time.After(db.config.Refresh):
			number := agent.CommitteeNumber()
			isCommittee := agent.IsCommitteeMember()
			isLeader := agent.IsLeader()
			currentCommittee := agent.GetCurrentCommittee()
			backCommittee := agent.GetAlternativeCommittee()
			/*committeeNumber := &ChartEntry{
				Value: float64(number),
			}*/

			db.sendToAll(&Message{
				Committee: &CommitteeMessage{
					Number:            number,
					IsCommitteeMember: isCommittee,
					IsLeader:          isLeader,
					Committee:         currentCommittee,
					BackCommittee:     backCommittee,
				},
			})

		}
	}
}
