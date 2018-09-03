package lock

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/log"
)

const IfPrint = true

func PSLog(a ...interface{}) {
	if IfPrint {
		fmt.Println("[PSLog]", a)
		return
	}
	log.Debug("[PSLog]", a)
}
