package manifold

import (
	"time"
)

/*
ObservationMetadata identifies the SDK book observations accumulated into one
field advance without retaining mutable book data.
*/
type ObservationMetadata struct {
	At        time.Time
	FrameType string
	Checksum  uint32
	Count     int
}

/*
pendingObservation is the latest valid market boundary for the population that
has not yet advanced the field solver.
*/
type pendingObservation struct {
	metadata        ObservationMetadata
	bestBid         float64
	bestAsk         float64
	bestBidQuantity float64
	bestAskQuantity float64
	midPrice        float64
}

/*
invalidate clears pending field work and exposes the reason as a failed state.
*/
func (slot *Slot) invalidate(
	observation ObservationMetadata,
	reason InvalidReason,
) ProcessResult {
	slot.pending = pendingObservation{}
	slot.advanceReady = false
	slot.population.invalidate(reason)

	return slot.failedResult(observation, reason)
}
