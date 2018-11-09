package prints

import (
	"fmt"
	"runtime"
)

func PrintStack() {
	var buf [1024]byte
	n := runtime.Stack(buf[:], true)
	fmt.Println(string(buf[:]), n)
}
