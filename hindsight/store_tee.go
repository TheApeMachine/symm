package hindsight

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"golang.design/x/lockfree/lf"
)

/*
StoreTee queues measurements for the catalog drain.
*/
type StoreTee struct {
	*runtime.System
	queue   *lf.Queue[*data.Measurement]
	dropped atomic.Int64
	client  *store.Client
}

func NewStoreTee(ctx context.Context, label string) *StoreTee {
	tee := &StoreTee{
		queue:  lf.NewQueue[*data.Measurement](),
		client: store.NewClient(ctx, "symm"),
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

/*
Next returns the next measurement from the queue.
*/
func (tee *StoreTee) Next() unsafe.Pointer {
	measurement, ok := tee.queue.Dequeue()

	if !ok || measurement == nil {
		return nil
	}

	return unsafe.Pointer(&measurement)
}

/*
Run consumes the queue and writes to S3.
*/
func (tee *StoreTee) Run() {
	go func() {
		for {
			select {
			case <-tee.Context().Done():
				for {
					measurement, ok := tee.queue.Dequeue()

					if !ok || measurement == nil {
						return
					}

					if err := tee.client.Put(measurement); err != nil {
						errnie.Error(err)
					}
				}
			default:
			}

			measurement, ok := tee.queue.Dequeue()

			if !ok || measurement == nil {
				time.Sleep(5 * time.Millisecond)
				continue
			}

			if err := tee.client.Put(measurement); err != nil {
				errnie.Error(err)
			}
		}
	}()
}

func (tee *StoreTee) Pending() int {
	return int(tee.queue.Length())
}
