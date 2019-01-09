package help

import "github.com/ethereum/go-ethereum/log"

func CheckAndPrintError(err error) {
	if err != nil {
		log.Error("CheckAndPrintError", "error", err.Error())
	}
}
