package hindsight

import (
	"context"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

/*
StoreTee queues measurements for the catalog drain.
Training tape must not silently drop: a full queue applies backpressure, and
if the consumer cannot catch up before context cancellation the push fails
explicitly so the tape never contains invisible gaps.
*/
const storeTeeQueueCap = 8192

type StoreTee struct {
	*runtime.System
	queue *lf.Queue[*data.Measurement[float64]]
}

func NewStoreTee(ctx context.Context, label string) *StoreTee {
	tee := &StoreTee{
		queue: lf.NewQueue[*data.Measurement[float64]](),
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	return tee
}

func (tee *StoreTee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
		tee.Error(errnie.Error(errnie.Err(
			errnie.Conflict,
			"storeTee: push refused — system not READY (would create a silent tape gap)",
			nil,
		)))
		return
	}

	if measurement == nil {
		return
	}

	for {
		if tee.queue.Length() < storeTeeQueueCap {
			tee.queue.Enqueue(measurement)
			return
		}

		// Backpressure: wait for drain rather than drop. Fail explicitly if
		// the run ends while still full so callers see the gap.
		select {
		case <-tee.Context().Done():
			tee.Error(errnie.Error(errnie.Err(
				errnie.Timeout,
				"storeTee: queue full at shutdown — tape push failed (no silent drop)",
				tee.Context().Err(),
			)))
			return
		case <-time.After(time.Millisecond):
			if tee.Status() != runtime.READY {
				tee.Error(errnie.Error(errnie.Err(
					errnie.Conflict,
					"storeTee: queue full and system left READY — tape push failed",
					nil,
				)))
				return
			}
		}
	}
}

func (tee *StoreTee) Next() unsafe.Pointer {
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

	return unsafe.Pointer(measurement)
}

func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length())
}
