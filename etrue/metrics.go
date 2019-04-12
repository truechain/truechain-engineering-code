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
	propTxnInTxsMeter         = metrics.NewRegisteredMeter("etrue/prop/txns/in/txs", nil)
	propTxnInTrafficMeter     = metrics.NewRegisteredMeter("etrue/prop/txns/in/traffic", nil)
	propTxnOutPacketsMeter    = metrics.NewRegisteredMeter("etrue/prop/txns/out/packets", nil)
	propTxnOutTrafficMeter    = metrics.NewRegisteredMeter("etrue/prop/txns/out/traffic", nil)
	propFtnInPacketsMeter     = metrics.NewRegisteredMeter("etrue/prop/ftns/in/packets", nil)
	propFtnInTrafficMeter     = metrics.NewRegisteredMeter("etrue/prop/ftns/in/traffic", nil)
	propFtnOutPacketsMeter    = metrics.NewRegisteredMeter("etrue/prop/ftns/out/packets", nil)
	propFtnOutTrafficMeter    = metrics.NewRegisteredMeter("etrue/prop/ftns/out/traffic", nil)
	propFHashInPacketsMeter   = metrics.NewRegisteredMeter("etrue/prop/fhashes/in/packets", nil)
	propFHashInTrafficMeter   = metrics.NewRegisteredMeter("etrue/prop/fhashes/in/traffic", nil)
	propFHashOutPacketsMeter  = metrics.NewRegisteredMeter("etrue/prop/fhashes/out/packets", nil)
	propFHashOutTrafficMeter  = metrics.NewRegisteredMeter("etrue/prop/fhashes/out/traffic", nil)
	propSHashInPacketsMeter   = metrics.NewRegisteredMeter("etrue/prop/shashes/in/packets", nil)
	propSHashInTrafficMeter   = metrics.NewRegisteredMeter("etrue/prop/shashes/in/traffic", nil)
	propSHashOutPacketsMeter  = metrics.NewRegisteredMeter("etrue/prop/shashes/out/packets", nil)
	propSHashOutTrafficMeter  = metrics.NewRegisteredMeter("etrue/prop/shashes/out/traffic", nil)
	propFBlockInPacketsMeter  = metrics.NewRegisteredMeter("etrue/prop/fblocks/in/packets", nil)
	propFBlockInTrafficMeter  = metrics.NewRegisteredMeter("etrue/prop/fblocks/in/traffic", nil)
	propFBlockOutPacketsMeter = metrics.NewRegisteredMeter("etrue/prop/fblocks/out/packets", nil)
	propFBlockOutTrafficMeter = metrics.NewRegisteredMeter("etrue/prop/fblocks/out/traffic", nil)
	propSBlockInPacketsMeter  = metrics.NewRegisteredMeter("etrue/prop/sblocks/in/packets", nil)
	propSBlockInTrafficMeter  = metrics.NewRegisteredMeter("etrue/prop/sblocks/in/traffic", nil)
	propSBlockOutPacketsMeter = metrics.NewRegisteredMeter("etrue/prop/sblocks/out/packets", nil)
	propSBlockOutTrafficMeter = metrics.NewRegisteredMeter("etrue/prop/sblocks/out/traffic", nil)

	propNodeInfoPacketsMeter    = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/in/packets", nil)
	propNodeInfoInTrafficMeter  = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/in/traffic", nil)
	propNodeInfoOutPacketsMeter = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/out/packets", nil)
	propNodeInfoOutTrafficMeter = metrics.NewRegisteredMeter("etrue/prop/nodeinfo/out/traffic", nil)

	reqFHeaderInPacketsMeter  = metrics.NewRegisteredMeter("etrue/req/headers/in/packets", nil)
	reqFHeaderInTrafficMeter  = metrics.NewRegisteredMeter("etrue/req/headers/in/traffic", nil)
	reqFHeaderOutPacketsMeter = metrics.NewRegisteredMeter("etrue/req/headers/out/packets", nil)
	reqFHeaderOutTrafficMeter = metrics.NewRegisteredMeter("etrue/req/headers/out/traffic", nil)
	reqSHeaderInPacketsMeter  = metrics.NewRegisteredMeter("etrue/req/sheaders/in/packets", nil)
	reqSHeaderInTrafficMeter  = metrics.NewRegisteredMeter("etrue/req/sheaders/in/traffic", nil)
	reqSHeaderOutPacketsMeter = metrics.NewRegisteredMeter("etrue/req/sheaders/out/packets", nil)
	reqSHeaderOutTrafficMeter = metrics.NewRegisteredMeter("etrue/req/sheaders/out/traffic", nil)

	reqFBodyInPacketsMeter  = metrics.NewRegisteredMeter("etrue/req/fbodies/in/packets", nil)
	reqFBodyInTrafficMeter  = metrics.NewRegisteredMeter("etrue/req/fbodies/in/traffic", nil)
	reqFBodyOutPacketsMeter = metrics.NewRegisteredMeter("etrue/req/fbodies/out/packets", nil)
	reqFBodyOutTrafficMeter = metrics.NewRegisteredMeter("etrue/req/fbodies/out/traffic", nil)
	reqSBodyInPacketsMeter  = metrics.NewRegisteredMeter("etrue/req/sbodies/in/packets", nil)
	reqSBodyInTrafficMeter  = metrics.NewRegisteredMeter("etrue/req/sbodies/in/traffic", nil)
	reqSBodyOutPacketsMeter = metrics.NewRegisteredMeter("etrue/req/sbodies/out/packets", nil)
	reqSBodyOutTrafficMeter = metrics.NewRegisteredMeter("etrue/req/sbodies/out/traffic", nil)

	reqStateInPacketsMeter    = metrics.NewRegisteredMeter("etrue/req/states/in/packets", nil)
	reqStateInTrafficMeter    = metrics.NewRegisteredMeter("etrue/req/states/in/traffic", nil)
	reqStateOutPacketsMeter   = metrics.NewRegisteredMeter("etrue/req/states/out/packets", nil)
	reqStateOutTrafficMeter   = metrics.NewRegisteredMeter("etrue/req/states/out/traffic", nil)
	reqReceiptInPacketsMeter  = metrics.NewRegisteredMeter("etrue/req/receipts/in/packets", nil)
	reqReceiptInTrafficMeter  = metrics.NewRegisteredMeter("etrue/req/receipts/in/traffic", nil)
	reqReceiptOutPacketsMeter = metrics.NewRegisteredMeter("etrue/req/receipts/out/packets", nil)
	reqReceiptOutTrafficMeter = metrics.NewRegisteredMeter("etrue/req/receipts/out/traffic", nil)

	getHeadInPacketsMeter  = metrics.NewRegisteredMeter("etrue/get/head/in/packets", nil)
	getHeadInTrafficMeter  = metrics.NewRegisteredMeter("etrue/get/head/in/traffic", nil)
	getHeadOutPacketsMeter = metrics.NewRegisteredMeter("etrue/get/head/out/packets", nil)
	getHeadOutTrafficMeter = metrics.NewRegisteredMeter("etrue/get/head/out/traffic", nil)
	getBodyInPacketsMeter  = metrics.NewRegisteredMeter("etrue/get/bodies/in/packets", nil)
	getBodyInTrafficMeter  = metrics.NewRegisteredMeter("etrue/get/bodies/in/traffic", nil)
	getBodyOutPacketsMeter = metrics.NewRegisteredMeter("etrue/get/bodies/out/packets", nil)
	getBodyOutTrafficMeter = metrics.NewRegisteredMeter("etrue/get/bodies/out/traffic", nil)

	miscInPacketsMeter  = metrics.NewRegisteredMeter("etrue/misc/in/packets", nil)
	miscInTrafficMeter  = metrics.NewRegisteredMeter("etrue/misc/in/traffic", nil)
	miscOutPacketsMeter = metrics.NewRegisteredMeter("etrue/misc/out/packets", nil)
	miscOutTrafficMeter = metrics.NewRegisteredMeter("etrue/misc/out/traffic", nil)
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
		packets, traffic = reqFHeaderInPacketsMeter, reqFHeaderInTrafficMeter
	case msg.Code == SnailBlockHeadersMsg:
		packets, traffic = reqSHeaderInPacketsMeter, reqSHeaderInTrafficMeter
	case msg.Code == FastBlockBodiesMsg:
		packets, traffic = reqFBodyInPacketsMeter, reqFBodyInTrafficMeter
	case msg.Code == SnailBlockBodiesMsg:
		packets, traffic = reqSBodyInPacketsMeter, reqSBodyInTrafficMeter

	case msg.Code == NodeDataMsg:
		packets, traffic = reqStateInPacketsMeter, reqStateInTrafficMeter
	case msg.Code == ReceiptsMsg:
		packets, traffic = reqReceiptInPacketsMeter, reqReceiptInTrafficMeter

	case msg.Code == NewFastBlockHashesMsg:
		packets, traffic = propFHashInPacketsMeter, propFHashInTrafficMeter
	case msg.Code == NewSnailBlockHashesMsg:
		packets, traffic = propSHashInPacketsMeter, propSHashInTrafficMeter
	case msg.Code == NewFastBlockMsg:
		packets, traffic = propFBlockInPacketsMeter, propFBlockInTrafficMeter
	case msg.Code == NewSnailBlockMsg:
		packets, traffic = propSBlockInPacketsMeter, propSBlockInTrafficMeter
	case msg.Code == TxMsg:
		packets, traffic = propTxnInPacketsMeter, propTxnInTrafficMeter
	case msg.Code == NewFruitMsg:
		packets, traffic = propFtnInPacketsMeter, propFtnInTrafficMeter
	case msg.Code == TbftNodeInfoMsg:
		packets, traffic = propNodeInfoPacketsMeter, propNodeInfoInTrafficMeter
	case msg.Code == GetFastBlockHeadersMsg:
		packets, traffic = getHeadInPacketsMeter, getHeadInTrafficMeter
	case msg.Code == GetFastBlockBodiesMsg:
		packets, traffic = getHeadInPacketsMeter, getHeadInTrafficMeter
	case msg.Code == GetSnailBlockHeadersMsg:
		packets, traffic = getBodyInPacketsMeter, getBodyInTrafficMeter
	case msg.Code == GetSnailBlockBodiesMsg:
		packets, traffic = getBodyInPacketsMeter, getBodyInTrafficMeter
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
		packets, traffic = reqFHeaderOutPacketsMeter, reqFHeaderOutTrafficMeter
	case msg.Code == SnailBlockHeadersMsg:
		packets, traffic = reqSHeaderOutPacketsMeter, reqSHeaderOutTrafficMeter
	case msg.Code == FastBlockBodiesMsg:
		packets, traffic = reqFBodyOutPacketsMeter, reqFBodyOutTrafficMeter
	case msg.Code == SnailBlockBodiesMsg:
		packets, traffic = reqSBodyOutPacketsMeter, reqSBodyOutTrafficMeter

	case msg.Code == NodeDataMsg:
		packets, traffic = reqStateOutPacketsMeter, reqStateOutTrafficMeter
	case msg.Code == ReceiptsMsg:
		packets, traffic = reqReceiptOutPacketsMeter, reqReceiptOutTrafficMeter

	case msg.Code == NewFastBlockHashesMsg:
		packets, traffic = propFHashOutPacketsMeter, propFHashOutTrafficMeter
	case msg.Code == NewSnailBlockHashesMsg:
		packets, traffic = propSHashOutPacketsMeter, propSHashOutTrafficMeter
	case msg.Code == NewFastBlockMsg:
		packets, traffic = propFBlockOutPacketsMeter, propFBlockOutTrafficMeter
	case msg.Code == NewSnailBlockMsg:
		packets, traffic = propSBlockOutPacketsMeter, propSBlockOutTrafficMeter
	case msg.Code == TxMsg:
		packets, traffic = propTxnOutPacketsMeter, propTxnOutTrafficMeter
	case msg.Code == NewFruitMsg:
		packets, traffic = propFtnOutPacketsMeter, propFtnOutTrafficMeter
	case msg.Code == TbftNodeInfoMsg:
		packets, traffic = propNodeInfoOutPacketsMeter, propNodeInfoOutTrafficMeter
	case msg.Code == GetFastBlockHeadersMsg:
		packets, traffic = getHeadOutPacketsMeter, getHeadOutTrafficMeter
	case msg.Code == GetFastBlockBodiesMsg:
		packets, traffic = getHeadInPacketsMeter, getHeadOutTrafficMeter
	case msg.Code == GetSnailBlockHeadersMsg:
		packets, traffic = getBodyOutPacketsMeter, getBodyOutTrafficMeter
	case msg.Code == GetSnailBlockBodiesMsg:
		packets, traffic = getBodyOutPacketsMeter, getBodyOutTrafficMeter
	}
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	// Send the packet to the p2p layer
	return rw.MsgReadWriter.WriteMsg(msg)
}
