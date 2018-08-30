// Copyright 2016 The go-ethereum Authors
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

package les

import (
	"github.com/truechain/truechain-engineering-code/metrics"
	"github.com/truechain/truechain-engineering-code/p2p"
)

var (
	/*	propTxnInPacketsMeter     = metrics.NewMeter("etrue/prop/txns/in/packets")
		propTxnInTrafficMeter     = metrics.NewMeter("etrue/prop/txns/in/traffic")
		propTxnOutPacketsMeter    = metrics.NewMeter("etrue/prop/txns/out/packets")
		propTxnOutTrafficMeter    = metrics.NewMeter("etrue/prop/txns/out/traffic")
		propHashInPacketsMeter    = metrics.NewMeter("etrue/prop/hashes/in/packets")
		propHashInTrafficMeter    = metrics.NewMeter("etrue/prop/hashes/in/traffic")
		propHashOutPacketsMeter   = metrics.NewMeter("etrue/prop/hashes/out/packets")
		propHashOutTrafficMeter   = metrics.NewMeter("etrue/prop/hashes/out/traffic")
		propBlockInPacketsMeter   = metrics.NewMeter("etrue/prop/blocks/in/packets")
		propBlockInTrafficMeter   = metrics.NewMeter("etrue/prop/blocks/in/traffic")
		propBlockOutPacketsMeter  = metrics.NewMeter("etrue/prop/blocks/out/packets")
		propBlockOutTrafficMeter  = metrics.NewMeter("etrue/prop/blocks/out/traffic")
		reqHashInPacketsMeter     = metrics.NewMeter("etrue/req/hashes/in/packets")
		reqHashInTrafficMeter     = metrics.NewMeter("etrue/req/hashes/in/traffic")
		reqHashOutPacketsMeter    = metrics.NewMeter("etrue/req/hashes/out/packets")
		reqHashOutTrafficMeter    = metrics.NewMeter("etrue/req/hashes/out/traffic")
		reqBlockInPacketsMeter    = metrics.NewMeter("etrue/req/blocks/in/packets")
		reqBlockInTrafficMeter    = metrics.NewMeter("etrue/req/blocks/in/traffic")
		reqBlockOutPacketsMeter   = metrics.NewMeter("etrue/req/blocks/out/packets")
		reqBlockOutTrafficMeter   = metrics.NewMeter("etrue/req/blocks/out/traffic")
		reqHeaderInPacketsMeter   = metrics.NewMeter("etrue/req/headers/in/packets")
		reqHeaderInTrafficMeter   = metrics.NewMeter("etrue/req/headers/in/traffic")
		reqHeaderOutPacketsMeter  = metrics.NewMeter("etrue/req/headers/out/packets")
		reqHeaderOutTrafficMeter  = metrics.NewMeter("etrue/req/headers/out/traffic")
		reqBodyInPacketsMeter     = metrics.NewMeter("etrue/req/bodies/in/packets")
		reqBodyInTrafficMeter     = metrics.NewMeter("etrue/req/bodies/in/traffic")
		reqBodyOutPacketsMeter    = metrics.NewMeter("etrue/req/bodies/out/packets")
		reqBodyOutTrafficMeter    = metrics.NewMeter("etrue/req/bodies/out/traffic")
		reqStateInPacketsMeter    = metrics.NewMeter("etrue/req/states/in/packets")
		reqStateInTrafficMeter    = metrics.NewMeter("etrue/req/states/in/traffic")
		reqStateOutPacketsMeter   = metrics.NewMeter("etrue/req/states/out/packets")
		reqStateOutTrafficMeter   = metrics.NewMeter("etrue/req/states/out/traffic")
		reqReceiptInPacketsMeter  = metrics.NewMeter("etrue/req/receipts/in/packets")
		reqReceiptInTrafficMeter  = metrics.NewMeter("etrue/req/receipts/in/traffic")
		reqReceiptOutPacketsMeter = metrics.NewMeter("etrue/req/receipts/out/packets")
		reqReceiptOutTrafficMeter = metrics.NewMeter("etrue/req/receipts/out/traffic")*/
	miscInPacketsMeter  = metrics.NewRegisteredMeter("les/misc/in/packets", nil)
	miscInTrafficMeter  = metrics.NewRegisteredMeter("les/misc/in/traffic", nil)
	miscOutPacketsMeter = metrics.NewRegisteredMeter("les/misc/out/packets", nil)
	miscOutTrafficMeter = metrics.NewRegisteredMeter("les/misc/out/traffic", nil)
)

// meteredMsgReadWriter is a wrapper around a p2p.MsgReadWriter, capable of
// accumulating the above defined metrics based on the data stream contents.
type meteredMsgReadWriter struct {
	p2p.MsgReadWriter     // Wrapped message stream to meter
	version           int // Protocol version to select correct meters
}

// newMeteredMsgWriter wraps a p2p MsgReadWriter with metering support. If the
// metrics system is disabled, this function returns the original object.
func newMeteredMsgWriter(rw p2p.MsgReadWriter) p2p.MsgReadWriter {
	if !metrics.Enabled {
		return rw
	}
	return &meteredMsgReadWriter{MsgReadWriter: rw}
}

// Init sets the protocol version used by the stream to know which meters to
// increment in case of overlapping message ids between protocol versions.
func (rw *meteredMsgReadWriter) Init(version int) {
	rw.version = version
}

func (rw *meteredMsgReadWriter) ReadMsg() (p2p.Msg, error) {
	// Read the message and short circuit in case of an error
	msg, err := rw.MsgReadWriter.ReadMsg()
	if err != nil {
		return msg, err
	}
	// Account for the data traffic
	packets, traffic := miscInPacketsMeter, miscInTrafficMeter
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	return msg, err
}

func (rw *meteredMsgReadWriter) WriteMsg(msg p2p.Msg) error {
	// Account for the data traffic
	packets, traffic := miscOutPacketsMeter, miscOutTrafficMeter
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	// Send the packet to the p2p layer
	return rw.MsgReadWriter.WriteMsg(msg)
}
