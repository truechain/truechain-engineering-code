package tbft

import "github.com/ethereum/go-ethereum/log"

var msg string = "P2P"

func AddLog(ctx ...interface{}) {
	log.Info(msg, ctx)
}
