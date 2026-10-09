package hindsight

import (
	"context"
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
	queue *lf.Queue[*data.Measurement]
}

func NewStoreTee(ctx context.Context, label string) *StoreTee {
	tee := &StoreTee{
		queue: lf.NewQueue[*data.Measurement](),
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	return tee
}

func (tee *StoreTee) Push(measurement *data.Measurement) {
	if tee.Status() != runtime.READY {
		errnie.Warn(
			"[storeTee] pushing to a non-ready system may have unintended consequences",
		)

		return
	}

	if measurement == nil {
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

func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length())
}
