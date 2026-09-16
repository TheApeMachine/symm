package hindsight

import (
	"context"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
StoreTee queues measurements for the catalog drain. It implements runtime.Tee
and remains idle until startup explicitly transitions it to READY. Its bounded
lock-free queue accepts concurrent workspace consumers without waiting for the
catalog drain.
*/
type StoreTee struct {
	*runtime.System
	queue    *lf.Queue[*data.Measurement[float64]]
	pending  atomic.Int64
	capacity int64
}

/*
NewStoreTee creates an idle storage off-ramp.
*/
func NewStoreTee(ctx context.Context, label string, capacity int) *StoreTee {
	if capacity < 1 {
		capacity = 1
	}

	tee := &StoreTee{
		queue:    lf.NewQueue[*data.Measurement[float64]](),
		capacity: int64(capacity),
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
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

	if tee.pending.Add(1) > tee.capacity {
		tee.pending.Add(-1)
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"store tee: measurement queue is full",
			nil,
		))
		return
	}

	tee.queue.Enqueue(measurement.Clone())
}

/*
Next returns a measurement pointer, or nil while idle or when the queue is empty.
*/
func (tee *StoreTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn(tee.Name() + ": Next called before READY; dropping event")
		return nil
	}

	measurement, ok := tee.queue.Dequeue()

	if !ok {
		return nil
	}

	tee.pending.Add(-1)
	return unsafe.Pointer(measurement)
}

// Pending reports accepted observations waiting for the catalog drain.
func (tee *StoreTee) Pending() int {
	return int(tee.pending.Load())
}
