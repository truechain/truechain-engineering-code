package lock

import (
	"fmt"
)

const IfPrint = true

func PSLog(a ...interface{}) {
	if IfPrint {
		fmt.Println("[PSLog]", a)
		return
	}
	log.Debug("[PbftServer]", "[PSLog]", a)
}
