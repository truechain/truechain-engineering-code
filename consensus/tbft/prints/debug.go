package prints

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/log"
	"runtime"
)

func PrintStack() {
	var buf [1024]byte
	n := runtime.Stack(buf[:], true)
	log.Debug("PrintStack", "Stack", fmt.Sprintln(string(buf[:]), n))
}
