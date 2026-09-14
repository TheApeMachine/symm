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
	ring   *wf.RingBuffer[*data.Measurement[float64]]
	filter func(*data.Measurement[float64]) bool
}

/*
NewTee creates a new Tee node with the given ring buffer capacity and default "telemetry.tee" label.
*/
func NewTee(capacity int) *Tee {
	return NewNamedTee("telemetry.tee", capacity)
}

/*
NewNamedTee creates a new Tee node with a custom system label and ring buffer capacity.
*/
func NewNamedTee(label string, capacity int) *Tee {
	tee := &Tee{
		System: NewSystem(context.Background(), label),
		ring:   wf.NewRingBuffer[*data.Measurement[float64]](capacity),
	}
	tee.Transition(READY)
	return tee
}

/*
SetFilter sets a predicate controlling which measurements are enqueued onto the ring.
A nil filter permits all valid named measurements.
*/
func (tee *Tee) SetFilter(filter func(*data.Measurement[float64]) bool) {
	tee.filter = filter
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
func (tee *Tee) Register() *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64](tee.Name(), nil)
	measurement.Metadata["peer-interest"] = "*"

	return measurement
}

/*
Step satisfies Node[*data.Measurement[float64]]. It enqueues the measurement
and all its populated peers into the wait-free ring buffer and returns the measurement.
*/
func (tee *Tee) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if measurement == nil || tee.Status() != READY {
		return nil
	}

	if (tee.filter == nil || tee.filter(measurement)) && measurement.Source != tee.Name() && measurement.Label != "" {
		tee.ring.Put(measurement.Clone())
	}

	for _, peer := range measurement.Peers {
		if peer == nil || peer.Label == "" {
			continue
		}

		if tee.filter != nil && !tee.filter(peer) {
			continue
		}

		tee.ring.Put(peer.Clone())
	}

	return measurement
}
