package true

import (
	"math/big"
)

type CommitteeMember struct {
	Nodeid string
	Addr   string
	Port   int
}

type StandbyInfo struct {
	NodeId   string
	Coinbase string
	Addr     string
	Port     int
	Height   *big.Int
	Comfire  bool
}
