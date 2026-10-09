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

func (tee *StoreTee) Pop() *data.Measurement {
	if tee.Status() != runtime.READY {
		errnie.Warn(
			"[storeTee] pulling from a non-ready system may have unintended consequences",
		)

		return nil
	}

	measurement, ok := tee.queue.Dequeue()

	if !ok {
		return nil
	}

	return measurement
}

func (tee *StoreTee) Next() unsafe.Pointer {
	measurement := tee.Pop()

	if measurement == nil {
		return nil
	}

	return unsafe.Pointer(&measurement)
}

/*
Dropped is the number of measurements Push refused because the tee was not
READY.
*/
func (tee *StoreTee) Dropped() int64 {
	return tee.dropped.Load()
}

func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length())
}

/*
Take dequeues every measurement pending at the time of the call. It is the
one-shot drain for batch producers (symm detect) that commit what they queued
as a single snapshot instead of running the continuous Drain loop.
*/
func (tee *StoreTee) Take() []*data.Measurement {
	pending := tee.Pending()
	taken := make([]*data.Measurement, 0, pending)

	for remaining := pending; remaining > 0; remaining-- {
		measurement := tee.Pop()

		if measurement == nil {
			continue
		}

		taken = append(taken, measurement)
	}

	return taken
}
