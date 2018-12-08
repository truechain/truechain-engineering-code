package prints

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"runtime"
)

func PrintStack() {
	var buf [1024]byte
	n := runtime.Stack(buf[:], true)
	log.Debug("PrintStack", "Stack", fmt.Sprintln(string(buf[:]), n))
}
