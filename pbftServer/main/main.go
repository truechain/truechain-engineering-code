package main

import (
	pb "github.com/truechain/truechain-engineering-code/pbftServer"
	"fmt"
	"os"
	"os/signal"
	"golang.org/x/sys/unix"
)

func main (){
	//cfg.LoadPbftSimConfig()
	pb.StartPbftServers()

	fmt.Println("服务已启动")

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh)
	for s := range sigCh {
		switch s {
		case unix.SIGTERM:
			fallthrough
		case unix.SIGINT:
			return
		default:
			continue
		}
	}

}
