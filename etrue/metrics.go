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

package etrue

import (
	"github.com/truechain/truechain-engineering-code/metrics"
	"github.com/truechain/truechain-engineering-code/p2p"
)

var (
	propTxnInPacketsMeter     = metrics.NewRegisteredMeter("etrue/prop/txns/in/packets", nil)
	propTxnInTrafficMeter     = metrics.NewRegisteredMeter("etrue/prop/txns/in/traffic", nil)
	propTxnOutPacketsMeter    = metrics.NewRegisteredMeter("etrue/prop/txns/out/packets", nil)
	propTxnOutTrafficMeter    = metrics.NewRegisteredMeter("etrue/prop/txns/out/traffic", nil)
	propFastHashInPacketsMeter    = metrics.NewRegisteredMeter("etrue/prop/fasthashes/in/packets", nil)
	propFastHashInTrafficMeter    = metrics.NewRegisteredMeter("etrue/prop/fasthashes/in/traffic", nil)
	propFastHashOutPacketsMeter   = metrics.NewRegisteredMeter("etrue/prop/fasthashes/out/packets", nil)
	propFastHashOutTrafficMeter   = metrics.NewRegisteredMeter("etrue/prop/fasthashes/out/traffic", nil)
	propFastBlockInPacketsMeter   = metrics.NewRegisteredMeter("etrue/prop/fastblocks/in/packets", nil)
	propFastBlockInTrafficMeter   = metrics.NewRegisteredMeter("etrue/prop/fastblocks/in/traffic", nil)
	propFastBlockOutPacketsMeter  = metrics.NewRegisteredMeter("etrue/prop/fastblocks/out/packets", nil)
	propFastBlockOutTrafficMeter  = metrics.NewRegisteredMeter("etrue/prop/fastblocks/out/traffic", nil)

	propBlockSignInPacketsMeter   = metrics.NewRegisteredMeter("etrue/prop/blocksign/in/packets", nil)
	propBlockSignInTrafficMeter   = metrics.NewRegisteredMeter("etrue/prop/blocksign/in/traffic", nil)
	propBlockSignOutPacketsMeter  = metrics.NewRegisteredMeter("etrue/prop/blocksign/out/packets", nil)
	propBlockSignOutTrafficMeter  = metrics.NewRegisteredMeter("etrue/prop/blocksign/out/traffic", nil)

	propNodeInfoPacketsMeter   	  = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/in/packets", nil)
	propNodeInfoInTrafficMeter    = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/in/traffic", nil)
	propNodeInfoOutPacketsMeter   = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/out/packets", nil)
	propNodeInfoOutTrafficMeter   = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/out/traffic", nil)

	reqHeaderInPacketsMeter   = metrics.NewRegisteredMeter("etrue/req/headers/in/packets", nil)
	reqHeaderInTrafficMeter   = metrics.NewRegisteredMeter("etrue/req/headers/in/traffic", nil)
	reqHeaderOutPacketsMeter  = metrics.NewRegisteredMeter("etrue/req/headers/out/packets", nil)
	reqHeaderOutTrafficMeter  = metrics.NewRegisteredMeter("etrue/req/headers/out/traffic", nil)
	reqBodyInPacketsMeter     = metrics.NewRegisteredMeter("etrue/req/bodies/in/packets", nil)
	reqBodyInTrafficMeter     = metrics.NewRegisteredMeter("etrue/req/bodies/in/traffic", nil)
	reqBodyOutPacketsMeter    = metrics.NewRegisteredMeter("etrue/req/bodies/out/packets", nil)
	reqBodyOutTrafficMeter    = metrics.NewRegisteredMeter("etrue/req/bodies/out/traffic", nil)
	reqStateInPacketsMeter    = metrics.NewRegisteredMeter("etrue/req/states/in/packets", nil)
	reqStateInTrafficMeter    = metrics.NewRegisteredMeter("etrue/req/states/in/traffic", nil)
	reqStateOutPacketsMeter   = metrics.NewRegisteredMeter("etrue/req/states/out/packets", nil)
	reqStateOutTrafficMeter   = metrics.NewRegisteredMeter("etrue/req/states/out/traffic", nil)
	reqReceiptInPacketsMeter  = metrics.NewRegisteredMeter("etrue/req/receipts/in/packets", nil)
	reqReceiptInTrafficMeter  = metrics.NewRegisteredMeter("etrue/req/receipts/in/traffic", nil)
	reqReceiptOutPacketsMeter = metrics.NewRegisteredMeter("etrue/req/receipts/out/packets", nil)
	reqReceiptOutTrafficMeter = metrics.NewRegisteredMeter("etrue/req/receipts/out/traffic", nil)
	miscInPacketsMeter        = metrics.NewRegisteredMeter("etrue/misc/in/packets", nil)
	miscInTrafficMeter        = metrics.NewRegisteredMeter("etrue/misc/in/traffic", nil)
	miscOutPacketsMeter       = metrics.NewRegisteredMeter("etrue/misc/out/packets", nil)
	miscOutTrafficMeter       = metrics.NewRegisteredMeter("etrue/misc/out/traffic", nil)
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
	switch {
	case msg.Code == FastBlockHeadersMsg:
		packets, traffic = reqHeaderInPacketsMeter, reqHeaderInTrafficMeter
	case msg.Code == FastBlockBodiesMsg:
		packets, traffic = reqBodyInPacketsMeter, reqBodyInTrafficMeter

	case rw.version >= eth63 && msg.Code == NodeDataMsg:
		packets, traffic = reqStateInPacketsMeter, reqStateInTrafficMeter
	case rw.version >= eth63 && msg.Code == ReceiptsMsg:
		packets, traffic = reqReceiptInPacketsMeter, reqReceiptInTrafficMeter

	case msg.Code == NewFastBlockHashesMsg:
		packets, traffic = propFastHashInPacketsMeter, propFastHashInTrafficMeter
	case msg.Code == NewFastBlockMsg:
		packets, traffic = propFastBlockInPacketsMeter, propFastBlockInTrafficMeter
	case msg.Code == TxMsg:
		packets, traffic = propTxnInPacketsMeter, propTxnInTrafficMeter
	case msg.Code == BlockSignMsg:
		packets, traffic = propBlockSignInPacketsMeter, propBlockSignInTrafficMeter
	case msg.Code == PbftNodeInfoMsg:
		packets, traffic = propNodeInfoPacketsMeter, propNodeInfoInTrafficMeter
	}
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	return msg, err
}

func (rw *meteredMsgReadWriter) WriteMsg(msg p2p.Msg) error {
	// Account for the data traffic
	packets, traffic := miscOutPacketsMeter, miscOutTrafficMeter
	switch {
	case msg.Code == FastBlockHeadersMsg:
		packets, traffic = reqHeaderOutPacketsMeter, reqHeaderOutTrafficMeter
	case msg.Code == FastBlockBodiesMsg:
		packets, traffic = reqBodyOutPacketsMeter, reqBodyOutTrafficMeter

	case rw.version >= eth63 && msg.Code == NodeDataMsg:
		packets, traffic = reqStateOutPacketsMeter, reqStateOutTrafficMeter
	case rw.version >= eth63 && msg.Code == ReceiptsMsg:
		packets, traffic = reqReceiptOutPacketsMeter, reqReceiptOutTrafficMeter

	case msg.Code == NewFastBlockHashesMsg:
		packets, traffic = propFastHashOutPacketsMeter, propFastHashOutTrafficMeter
	case msg.Code == NewFastBlockMsg:
		packets, traffic = propFastBlockOutPacketsMeter, propFastBlockOutTrafficMeter
	case msg.Code == TxMsg:
		packets, traffic = propTxnOutPacketsMeter, propTxnOutTrafficMeter
	case msg.Code == BlockSignMsg:
		packets, traffic = propBlockSignOutPacketsMeter, propBlockSignOutTrafficMeter
	case msg.Code == PbftNodeInfoMsg:
		packets, traffic = propNodeInfoOutPacketsMeter, propNodeInfoOutTrafficMeter
	}
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	// Send the packet to the p2p layer
	return rw.MsgReadWriter.WriteMsg(msg)
}
