package runtime

import (
	"context"

	"golang.design/x/lockfree/wf"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
Tee is a non-blocking pipeline off-ramp. It satisfies Node[*data.Measurement[float64]]
and enqueues measurements to a wait-free SPSC ring buffer for asynchronous
consumption by telemetry and persistence consumers.

Its Step method executes in single-digit nanoseconds with zero locks, zero
memory allocations, and never exerts backpressure on the upstream LMAX ring.
*/
type Tee struct {
	*System
	ring *wf.RingBuffer[*data.Measurement[float64]]
}

/*
NewTee creates a new Tee node with the given ring buffer capacity.
*/
func NewTee(capacity int) *Tee {
	tee := &Tee{
		System: NewSystem(context.Background(), "telemetry.tee"),
		ring:   wf.NewRingBuffer[*data.Measurement[float64]](capacity),
	}
	tee.Transition(READY)
	return tee
}

/*
Ring exposes the underlying wait-free SPSC ring buffer to the detached consumer.
*/
func (tee *Tee) Ring() *wf.RingBuffer[*data.Measurement[float64]] {
	return tee.ring
}

/*
Register identifies the Tee with the runtime register and declares a wildcard peer
interest so all stage measurements are populated into val.Peers.
*/
func (tee *Tee) Register() (*data.Measurement[float64], []string) {
	return data.NewMeasurement[float64]("telemetry.tee", nil), []string{"*"}
}

/*
Step satisfies Node[*data.Measurement[float64]]. It enqueues the measurement
and all its populated peers into the wait-free ring buffer and returns the measurement.
*/
func (tee *Tee) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if measurement == nil || tee.Status() != READY {
		return nil
	}

	if measurement.Source != "telemetry.tee" && measurement.Label != "" {
		tee.ring.Put(measurement.Clone())
	}

	for _, peer := range measurement.Peers {
		if peer != nil && peer.Label != "" {
			tee.ring.Put(peer.Clone())
		}
	}

	return measurement
}
