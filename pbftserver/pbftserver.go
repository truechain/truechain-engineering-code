package pbftserver

import (
	"os"
	"github.com/truechain/truechain-engineering-code/pbftserver/network"
)

func test() {
	nodeID := os.Args[1]
	server := network.NewServer(nodeID)

	server.Start()
}