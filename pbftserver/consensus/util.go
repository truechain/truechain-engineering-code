package consensus

import (
	"math/big"
	"crypto/sha256"
	"encoding/hex"
)

const (
	Agree 			int = iota
	Against 	
	ActionFecth 	
	ActionBroadcast 
)

type ActionIn struct {
	AC 		int
	ID 		*big.Int
	Height	*big.Int
}
var ActionChan chan *ActionIn = make(chan *ActionIn)

func Hash(content []byte) string {
	h := sha256.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

