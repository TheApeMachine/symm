package ui

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/wf"
)

/*
UITee is a concrete off-ramp that accepts *data.Measurement[float64]
and yields encoded FlatBuffer []byte frames for the dashboard.
It satisfies runtime.Tee[*data.Measurement[float64], []byte].
*/
type UITee struct {
	*runtime.System
	ring *wf.RingBuffer[*data.Measurement[float64]]
}

/*
NewUITee creates a new wait-free UITee off-ramp.
*/
func NewUITee(label string, capacity int) *UITee {
	tee := &UITee{
		ring: wf.NewRingBuffer[*data.Measurement[float64]](capacity),
	}

	tee.System = runtime.NewSystem(context.Background(), label, tee)
	tee.Transition(runtime.READY)

	return tee
}

/*
Push enqueues a measurement onto the wait-free ring buffer for asynchronous
FlatBuffers encoding. Executes in single-digit nanoseconds with zero allocations.
*/
func (tee *UITee) Push(measurement *data.Measurement[float64]) {
	if measurement == nil {
		return
	}

	if !tee.ring.Put(measurement) {
		tee.Transition(runtime.ERROR)
	}
}

/*
Next drains available measurements from the ring buffer, batches them,
and returns an encoded FlatBuffer frame ([]byte).
*/
func (tee *UITee) Next() []byte {
	if tee.Status() != runtime.READY {
		return nil
	}

	const batchCapacity = 128
	batch := make([]*data.Measurement[float64], 0, batchCapacity)

	for len(batch) < batchCapacity {
		measurement, ok := tee.ring.Get()

		if !ok || measurement == nil {
			continue
		}

		if types.AllowsRoute(measurement) {
			batch = append(batch, measurement)
		}
	}

	if len(batch) == 0 {
		return nil
	}

	var payload []byte
	err := types.EncodeMeasurementsFrameWith(batch, func(frame []byte) error {
		payload = make([]byte, len(frame))
		copy(payload, frame)
		return nil
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"ui tee: failed to encode measurements frame",
			err,
		))
		return nil
	}

	return payload
}
