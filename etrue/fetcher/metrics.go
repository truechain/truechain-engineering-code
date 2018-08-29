// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Contains the metrics collected by the fetcher.

package fetcher

import (
	"github.com/truechain/truechain-engineering-code/metrics"
)

var (
	propAnnounceInMeter   = metrics.NewRegisteredMeter("etrue/fetcher/prop/announces/in", nil)
	propAnnounceOutTimer  = metrics.NewRegisteredTimer("etrue/fetcher/prop/announces/out", nil)
	propAnnounceDropMeter = metrics.NewRegisteredMeter("etrue/fetcher/prop/announces/drop", nil)
	propAnnounceDOSMeter  = metrics.NewRegisteredMeter("etrue/fetcher/prop/announces/dos", nil)

	propBroadcastInMeter   = metrics.NewRegisteredMeter("etrue/fetcher/prop/broadcasts/in", nil)
	propBroadcastOutTimer  = metrics.NewRegisteredTimer("etrue/fetcher/prop/broadcasts/out", nil)
	propBroadcastDropMeter = metrics.NewRegisteredMeter("etrue/fetcher/prop/broadcasts/drop", nil)
	propBroadcastDOSMeter  = metrics.NewRegisteredMeter("etrue/fetcher/prop/broadcasts/dos", nil)

	propSignInMeter   = metrics.NewRegisteredMeter("etrue/fetcher/prop/signs/in", nil)
	propSignDropMeter = metrics.NewRegisteredMeter("etrue/fetcher/prop/signs/drop", nil)
	propSignInvaildMeter = metrics.NewRegisteredMeter("etrue/fetcher/prop/signs/invaild", nil)
	propSignDOSMeter  = metrics.NewRegisteredMeter("etrue/fetcher/prop/signs/dos", nil)
	propSignFarDropMeter = metrics.NewRegisteredMeter("etrue/fetcher/signs/far", nil)

	propReadyInMeter   = metrics.NewRegisteredMeter("etrue/fetcher/prop/ready/in", nil)

	headerFetchMeter = metrics.NewRegisteredMeter("etrue/fetcher/fetch/headers", nil)
	bodyFetchMeter   = metrics.NewRegisteredMeter("etrue/fetcher/fetch/bodies", nil)

	headerFilterInMeter  = metrics.NewRegisteredMeter("etrue/fetcher/filter/headers/in", nil)
	headerFilterOutMeter = metrics.NewRegisteredMeter("etrue/fetcher/filter/headers/out", nil)
	bodyFilterInMeter    = metrics.NewRegisteredMeter("etrue/fetcher/filter/bodies/in", nil)
	bodyFilterOutMeter   = metrics.NewRegisteredMeter("etrue/fetcher/filter/bodies/out", nil)
)
