package ui

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
UITee is a concrete off-ramp that accepts *data.Measurement[float64]
and yields encoded FlatBuffer []byte frames for the dashboard, as well as
streaming fluid manifold frames over WebRTC.
It satisfies runtime.Tee[*data.Measurement[float64], []byte].
*/
type UITee struct {
	*runtime.System
	queue    *lf.Queue[*data.Measurement[float64]]
	bound    uint64
	snapshot func() *types.ManifoldState
}

/*
NewUITee creates a new wait-free UITee off-ramp.
*/
func NewUITee(label string, capacity int) *UITee {
	if capacity < 1 {
		capacity = 1
	}

	tee := &UITee{
		queue: lf.NewQueue[*data.Measurement[float64]](),
		bound: uint64(capacity),
	}

	tee.System = runtime.NewSystem(context.Background(), label, tee)
	return tee
}

/*
Push receives measurements from the workspace. Raw venue feeds stay off the
dashboard websocket; the page route still selects which analytical sources
are on the wire.
*/
func (tee *UITee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	if measurement == nil {
		return
	}

	if !types.AllowsRoute(measurement) {
		return
	}

	if tee.queue.Length() >= tee.bound {
		return
	}

	tee.queue.Enqueue(measurement)
}

/*
Next drains available measurements from the queue, batches them,
and returns an encoded FlatBuffer frame ([]byte).
*/
func (tee *UITee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return nil
	}

	const batchCapacity = 32
	batch := make([]*data.Measurement[float64], 0, batchCapacity)

	for len(batch) < batchCapacity {
		measurement, ok := tee.queue.Dequeue()

		if !ok {
			break
		}

		if measurement != nil {
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

	return unsafe.Pointer(&payload)
}
