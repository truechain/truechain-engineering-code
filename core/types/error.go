package types

import "errors"

var (
	// ErrHeightNotYet When the height of the committee is higher than the local height, it is issued.
	ErrHeightNotYet = errors.New("pbft send block height not yet")
)
