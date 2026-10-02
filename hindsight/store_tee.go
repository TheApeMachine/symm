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
	queue *lf.Queue[data.Publication]
}

func NewStoreTee(ctx context.Context, label string) *StoreTee {
	tee := &StoreTee{
		queue: lf.NewQueue[data.Publication](),
	}

	tee.System = runtime.NewSystem(ctx, label, tee)
	return tee
}

func (tee *StoreTee) Push(pub data.Publication) {
	if tee.Status() != runtime.READY {
		tee.Error(errnie.Error(errnie.Err(
			errnie.Conflict,
			"storeTee: push refused — system not READY (would create a silent tape gap)",
			nil,
		)))
		return
	}

	if pub.Measurement == nil {
		return
	}

	pub.Retain()

	for {
		if tee.queue.Length() < storeTeeQueueCap {
			tee.queue.Enqueue(pub)
			return
		}

		// Backpressure: wait for drain rather than drop. Fail explicitly if
		// the run ends while still full so callers see the gap.
		select {
		case <-tee.Context().Done():
			pub.Release()
			tee.Error(errnie.Error(errnie.Err(
				errnie.Timeout,
				"storeTee: queue full at shutdown — tape push failed (no silent drop)",
				tee.Context().Err(),
			)))
			return
		case <-time.After(time.Millisecond):
			if tee.Status() != runtime.READY {
				pub.Release()
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

func (tee *StoreTee) Dequeue() (data.Publication, bool) {
	if tee.Status() != runtime.READY {
		errnie.Warn(
			"[storeTee] pulling from a non-ready system may have unintended consequences",
		)
		return data.Publication{}, false
	}

	return tee.queue.Dequeue()
}

func (tee *StoreTee) Next() unsafe.Pointer {
	pub, ok := tee.Dequeue()
	if !ok {
		return nil
	}
	return unsafe.Pointer(pub.Measurement)
}

func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length())
}
