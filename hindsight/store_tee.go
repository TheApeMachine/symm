package hindsight

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"golang.design/x/lockfree/lf"
)

const (
	minWorkers = 2
	maxWorkers = 32
)

var payloadPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 4096)
	},
}

/*
StoreTee queues measurements for the catalog drain.
*/
type StoreTee struct {
	*runtime.System
	queue       *lf.Queue[*data.Measurement]
	dropped     atomic.Int64
	client      *store.Client
	workers     atomic.Int32
	lastPending int64
	running     atomic.Bool
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

	tee.queue.Enqueue(measurement)
}

/*
Next mus remain a noop.
*/
func (tee *StoreTee) Next() unsafe.Pointer {
	return nil
}

func (tee *StoreTee) store(measurement *data.Measurement) (err error) {
	if measurement == nil {
		return nil
	}

	payload := payloadPool.Get().([]byte)[:0]
	defer payloadPool.Put(payload)

	if payload, err = measurement.MarshalBinary(); err != nil {
		return tee.Error(errnie.Err(
			errnie.IO,
			"[hindsight.storeTee]: marshal failed for "+measurement.Prefix(),
			err,
		))
	}

	return tee.client.Put(measurement.Prefix(), payload)
}

func (tee *StoreTee) spawnWorker() {
	if !tee.running.Load() {
		return
	}

	tee.workers.Add(1)

	ctx, cancel := context.WithCancel(tee.Context())

	go func() {
		defer cancel()
		defer tee.workers.Add(-1)

		for {
			select {
			case <-ctx.Done():
				// Drain remaining measurements on shutdown
				for {
					measurement, ok := tee.queue.Dequeue()

					if !ok || measurement == nil {
						return
					}

					tee.store(measurement)
				}
			default:
				measurement, ok := tee.queue.Dequeue()

				if !ok || measurement == nil {
					cancel()
					continue
				}

				tee.store(measurement)
			}
		}
	}()
}

/*
Run consumes the queue and writes to S3 with auto-scaling workers.
*/
func (tee *StoreTee) Run() {
	tee.running.Store(true)

	for range minWorkers {
		tee.spawnWorker()
	}

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-tee.Context().Done():
				return
			case <-ticker.C:
				if tee.Pending() > tee.lastPending && tee.workers.Load() < maxWorkers {
					tee.spawnWorker()
				}
			}
		}
	}()
}

func (tee *StoreTee) Pending() int64 {
	return int64(tee.queue.Length())
}
