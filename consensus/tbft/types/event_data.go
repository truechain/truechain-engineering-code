package types

// import (
// 	"fmt"
// )


// NOTE: This goes into the replay WAL
type EventDataRoundState struct {
	Height int64  				`json:"height"`
	Round  int    				`json:"round"`
	Step   string 				`json:"step"`

	// private, not exposed to websockets
	RoundState interface{} 		`json:"-"`
}

type EventDataVote struct {
	Vote *Vote
}