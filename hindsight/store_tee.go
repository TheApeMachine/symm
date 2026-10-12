package hindsight

import (
	"context"
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
StoreTee queues measurements for the catalog drain.
*/
type StoreTee struct {
	*runtime.System
	queue   *lf.Queue[*data.Measurement]
	dropped atomic.Int64
}

func NewStoreTee(ctx context.Context, label string) *StoreTee {
	tee := &StoreTee{
		queue: lf.NewQueue[*data.Measurement](),
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	return tee
}

/*
Push queues a measurement for persistence. A tee that is not READY cannot
accept it: the measurement is dropped, the drop is logged as an error naming
the row, and Dropped counts it so batch callers can refuse a partial result.
*/
func (tee *StoreTee) Push(measurement *data.Measurement) {
	if measurement == nil {
		return
	}

	if status := tee.Status(); status != runtime.READY {
		tee.dropped.Add(1)
		errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf(
				"[storeTee] dropped %s/%s epoch=%d tick=%d: tee is %v, not ready",
				measurement.Source, measurement.Label, measurement.Epoch, measurement.Tick, status,
			),
			nil,
		))

		return
	}

	tee.queue.Enqueue(measurement)
}

func (tee *StoreTee) Next() unsafe.Pointer {
	measurement, ok := tee.queue.Dequeue()

	if !ok || measurement == nil {
		return nil
	}

	return unsafe.Pointer(&measurement)
}

func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length())
}
