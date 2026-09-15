package hindsight

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/wf"
)

/*
StoreTee is a concrete off-ramp that accepts *data.Measurement[float64]
and yields encoded FlatBuffer []byte frames for the dashboard, as well as
streaming fluid manifold frames over WebRTC.
It satisfies runtime.Tee[*data.Measurement[float64], []byte].
*/
type StoreTee struct {
	*runtime.System
	queue *wf.RingBuffer[*data.Measurement[float64]]
	bound uint64
}

/*
NewUITee creates a new wait-free UITee off-ramp.
*/
func NewStoreTee(label string, capacity int) *StoreTee {
	if capacity < 1 {
		capacity = 1
	}

	tee := &StoreTee{
		queue: wf.NewRingBuffer[*data.Measurement[float64]](capacity),
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
func (tee *StoreTee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	if measurement == nil {
		return
	}

	tee.queue.Put(measurement)
}

/*
Next drains available measurements from the queue, batches them,
and returns an encoded FlatBuffer frame ([]byte).
*/
func (tee *StoreTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return nil
	}

	measurement, ok := tee.queue.Get()

	if !ok {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[storetee] bad measurement retrieved from tie ring",
			nil,
		))
		return nil
	}

	return unsafe.Pointer(measurement)
}
