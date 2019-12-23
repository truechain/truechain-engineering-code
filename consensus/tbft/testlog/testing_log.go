package testlog

import "github.com/truechain/truechain-engineering-code/log"

var msg string = "P2P"

func AddLog(ctx ...interface{}) {
	log.Info(msg, ctx...)
}
