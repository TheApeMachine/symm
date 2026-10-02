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
		errnie.Warn(
			"[storeTee] pushing to a non-ready system may have unintended consequences",
		)

		return
	}

	if pub.Measurement == nil {
		return
	}

	pub.Retain()
	tee.queue.Enqueue(pub)
}

func (tee *StoreTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn(
			"[storeTee] pulling from a non-ready system may have unintended consequences",
		)

		return nil
	}

	pub, ok := tee.queue.Dequeue()

	if !ok {
		return nil
	}

	return unsafe.Pointer(&pub)
}

func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length())
}
