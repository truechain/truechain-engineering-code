package true

package main

//"strings"

type CommitteeMember struct {
	Nodeid string
	Addr   string
	Port   int
}

type StandbyInfo struct {
	NodeId   string
	coinbase string
	addr     string
	port     int
	height   *big.Int
	comfire  bool
}
