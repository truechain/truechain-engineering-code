package pex

import (
	"fmt"

	"github.com/truechain/truechain-engineering-code/consensus/tbft/tp2p"
)

type ErrAddrBookNonRoutable struct {
	Addr *tp2p.NetAddress
}

func (err ErrAddrBookNonRoutable) Error() string {
	return fmt.Sprintf("Cannot add non-routable address %v", err.Addr)
}

type ErrAddrBookSelf struct {
	Addr *tp2p.NetAddress
}

func (err ErrAddrBookSelf) Error() string {
	return fmt.Sprintf("Cannot add ourselves with address %v", err.Addr)
}

type ErrAddrBookPrivate struct {
	Addr *tp2p.NetAddress
}

func (err ErrAddrBookPrivate) Error() string {
	return fmt.Sprintf("Cannot add private peer with address %v", err.Addr)
}

type ErrAddrBookPrivateSrc struct {
	Src *tp2p.NetAddress
}

func (err ErrAddrBookPrivateSrc) Error() string {
	return fmt.Sprintf("Cannot add peer coming from private peer with address %v", err.Src)
}

type ErrAddrBookNilAddr struct {
	Addr *tp2p.NetAddress
	Src  *tp2p.NetAddress
}

func (err ErrAddrBookNilAddr) Error() string {
	return fmt.Sprintf("Cannot add a nil address. Got (addr, src) = (%v, %v)", err.Addr, err.Src)
}
