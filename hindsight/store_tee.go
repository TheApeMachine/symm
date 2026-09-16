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
StoreTee queues measurements for the catalog drain. It implements runtime.Tee
and remains idle until startup explicitly transitions it to READY.
*/
type StoreTee struct {
	*runtime.System
	queue *wf.RingBuffer[*data.Measurement[float64]]
}

/*
NewStoreTee creates an idle storage off-ramp.
*/
func NewStoreTee(label string, capacity int) *StoreTee {
	if capacity < 1 {
		capacity = 1
	}

	tee := &StoreTee{
		queue: wf.NewRingBuffer[*data.Measurement[float64]](capacity),
	}

	tee.System = runtime.NewSystem(context.Background(), label, tee)
	return tee
}

/*
Push receives measurements from the workspace after startup opens the tee.
*/
func (tee *StoreTee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return
	}

	if measurement == nil {
		return
	}

	if !tee.queue.Put(measurement.Clone()) {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"store tee: measurement queue is full",
			nil,
		))
	}
}

/*
Next returns a measurement pointer, or nil while idle or when the queue is empty.
*/
func (tee *StoreTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn(tee.Name() + ": Next called before READY; dropping event")
		return nil
	}

	measurement, ok := tee.queue.Get()

	if !ok {
		return nil
	}

	return unsafe.Pointer(measurement)
}
