
// This file contains the implementation for interacting with the Trezor hardware
// wallets. The wire protocol spec can be found on the SatoshiLabs website:
// https://doc.satoshilabs.com/trezor-tech/api-protobuf.html

//go:generate protoc --go_out=import_path=trezor:. types.proto messages.proto

// Package trezor contains the wire protocol wrapper in Go.
package trezor

import (
	"reflect"

	"github.com/golang/protobuf/proto"
)

// Type returns the protocol buffer type number of a specific message. If the
// message is nil, this method panics!
func Type(msg proto.Message) uint16 {
	return uint16(MessageType_value["MessageType_"+reflect.TypeOf(msg).Elem().Name()])
}

// Name returns the friendly message type name of a specific protocol buffer
// type number.
func Name(kind uint16) string {
	name := MessageType_name[int32(kind)]
	if len(name) < 12 {
		return name
	}
	return name[12:]
}
