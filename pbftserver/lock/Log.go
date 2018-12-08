package lock

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
)

const (
	IfPrint = false
	//Less    = true
)

func PSLog(a ...interface{}) {
	if IfPrint {
		fmt.Println("[PSLog]", a)
		return
	}

	log.Debug("[PbftServer]", "[PSLog]", a)
}

func PSLogInfo(a ...interface{}) {
	if IfPrint {
		fmt.Println("[PSLog]", a)
		return
	}

	log.Info("[PbftServer]", "[PSLog]", a)
}
