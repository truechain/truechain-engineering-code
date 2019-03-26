package help

import "github.com/ethereum/go-ethereum/log"

func CheckAndPrintError(err error) {
	if err != nil {
		log.Debug("CheckAndPrintError", "error", err.Error())
	}
}
