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

// Contains the metrics collected by the snailfetcher.

package snailfetcher

import (
	"github.com/truechain/truechain-engineering-code/metrics"
)

var (
	propBroadcastInMeter   = metrics.NewRegisteredMeter("etrue/snailfetcher/prop/broadcasts/in", nil)
	propBroadcastOutTimer  = metrics.NewRegisteredTimer("etrue/snailfetcher/prop/broadcasts/out", nil)
	propBroadcastDropMeter = metrics.NewRegisteredMeter("etrue/snailfetcher/prop/broadcasts/drop", nil)
	propBroadcastDOSMeter  = metrics.NewRegisteredMeter("etrue/snailfetcher/prop/broadcasts/dos", nil)

	headerFetchMeter = metrics.NewRegisteredMeter("etrue/snailfetcher/fetch/headers", nil)
	bodyFetchMeter   = metrics.NewRegisteredMeter("etrue/snailfetcher/fetch/bodies", nil)

	headerFilterInMeter  = metrics.NewRegisteredMeter("etrue/snailfetcher/filter/headers/in", nil)
	headerFilterOutMeter = metrics.NewRegisteredMeter("etrue/snailfetcher/filter/headers/out", nil)
	bodyFilterInMeter    = metrics.NewRegisteredMeter("etrue/snailfetcher/filter/bodies/in", nil)
	bodyFilterOutMeter   = metrics.NewRegisteredMeter("etrue/snailfetcher/filter/bodies/out", nil)
)
