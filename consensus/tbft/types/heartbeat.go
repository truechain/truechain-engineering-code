package types

import (
	"fmt"
	"github.com/truechain/truechain-engineering-code/consensus/tbft/help"
)

// Heartbeat is a simple vote-like structure so validators can
// alert others that they are alive and waiting for transactions.
// Note: We aren't adding ",omitempty" to Heartbeat's
// json field tags because we always want the JSON
// representation to be in its canonical form.
type Heartbeat struct {
	ValidatorAddress help.Address `json:"validator_address"`
	ValidatorIndex   uint         `json:"validator_index"`
	Height           uint64       `json:"height"`
	Round            uint         `json:"round"`
	Sequence         uint         `json:"sequence"`
	Signature        []byte       `json:"signature"`
}

// SignBytes returns the Heartbeat bytes for signing.
// It panics if the Heartbeat is nil.
func (heartbeat *Heartbeat) SignBytes(chainID string) []byte {
	bz, err := cdc.MarshalJSON(CanonicalJSONHeartbeat{
		ChainID:          chainID,
		Type:             "heartbeat",
		Height:           heartbeat.Height,
		Round:            heartbeat.Round,
		Sequence:         heartbeat.Sequence,
		ValidatorAddress: heartbeat.ValidatorAddress,
		ValidatorIndex:   heartbeat.ValidatorIndex,
	})
	if err != nil {
		panic(err)
	}
	return bz
}

// Copy makes a copy of the Heartbeat.
func (heartbeat *Heartbeat) Copy() *Heartbeat {
	if heartbeat == nil {
		return nil
	}
	heartbeatCopy := *heartbeat
	return &heartbeatCopy
}

// String returns a string representation of the Heartbeat.
func (heartbeat *Heartbeat) String() string {
	if heartbeat == nil {
		return "nil-heartbeat"
	}

	return fmt.Sprintf("Heartbeat{%v:%X %v/%02d (%v) %v}",
		heartbeat.ValidatorIndex, help.Fingerprint(heartbeat.ValidatorAddress),
		heartbeat.Height, heartbeat.Round, heartbeat.Sequence,
		fmt.Sprintf("/%X.../", help.Fingerprint(heartbeat.Signature[:])))
}
