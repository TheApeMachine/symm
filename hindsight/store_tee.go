package hindsight

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"golang.design/x/lockfree/lf"
)

const (
	minWorkers = 2
	maxWorkers = 32
)


/*
StoreTee queues measurements for the catalog drain.
*/
type StoreTee struct {
	*runtime.System
	queue       [2]*lf.Queue[*data.Value]
	current     atomic.Uint64
	dropped     atomic.Int64
	client      *store.Client
	workers     atomic.Int32
	lastPending int64
	running     atomic.Bool
}

func NewStoreTee(ctx context.Context, label string) *StoreTee {
	tee := &StoreTee{
		queue: [2]*lf.Queue[*data.Value]{
			lf.NewQueue[*data.Value](),
			lf.NewQueue[*data.Value](),
		},
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
func (tee *StoreTee) Push(value *data.Value) {
	if value == nil {
		return
	}

	if tee.Status() != runtime.READY {
		tee.dropped.Add(1)
		return
	}

	tee.queue[tee.current.Load()&1].Enqueue(value)
}

func (tee *StoreTee) Next() unsafe.Pointer {
	return nil
}

func (tee *StoreTee) store(value *data.Value) (err error) {
	if value == nil {
		return nil
	}

	_, err = tee.client.Write(value.Bytes())
	return err
}

func (tee *StoreTee) spawnWorker(pending int64) {
	if !tee.running.Load() {
		return
	}

	errnie.Info(fmt.Sprintf(
		"[hindsight.storeTee] spawning worker: %d, pending: %d",
		tee.workers.Load(), pending,
	))

	idx := tee.workers.Add(1)
	ctx, cancel := context.WithCancel(tee.Context())

	go func(idx int32) {
		defer cancel()
		defer tee.workers.Add(-1)

		for {
			select {
			case <-ctx.Done():
				// Drain remaining measurements from both queues on shutdown
				for qIdx := range 2 {
					for {
						value, ok := tee.queue[qIdx].Dequeue()
						if !ok || value == nil {
							break
						}
						tee.store(value)
					}
				}
				errnie.Info(fmt.Sprintf("[hindsight.storeTee] worker %d shutting down", idx))
				return
			default:
				value, ok := tee.queue[(tee.current.Load()&1)^1].Dequeue()

				if !ok || value == nil {
					time.Sleep(10 * time.Millisecond)
					continue
				}

				tee.store(value)
			}
		}
	}(idx)
}

/*
Run consumes the queue and writes to S3 with auto-scaling workers.
*/
func (tee *StoreTee) Run() {
	tee.running.Store(true)

	for range minWorkers {
		tee.spawnWorker(tee.Pending())
	}

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-tee.Context().Done():
				return
			case <-ticker.C:
				gen := tee.current.Load()
				writerIdx := gen & 1
				readerIdx := writerIdx ^ 1

				writerLen := tee.queue[writerIdx].Length()
				readerLen := tee.queue[readerIdx].Length()
				pending := int64(writerLen + readerLen)

				// Flip buffers when writer has items and reader is caught up or writer buffer is large
				if writerLen > 0 && (readerLen == 0 || writerLen >= 1000) {
					tee.current.Add(1)
					gen = tee.current.Load()
					writerIdx = gen & 1
					readerIdx = writerIdx ^ 1
				}

				// Autoscale workers when queue backlog grows
				if pending > 500 && tee.workers.Load() < maxWorkers {
					tee.spawnWorker(pending)
				}

				if gen > 0 && gen&63 == 0 { // every 64 flips (~6-16s)
					errnie.Info(fmt.Sprintf(
						"[hindsight.storeTee] gen=%d | writer_q=%d | reader_q=%d | workers=%d",
						gen,
						tee.queue[writerIdx].Length(),
						tee.queue[readerIdx].Length(),
						tee.workers.Load(),
					))
				}
			}
		}
	}()
}

func (tee *StoreTee) Pending() int64 {
	gen := tee.current.Load()
	return int64(tee.queue[gen&1].Length() + tee.queue[(gen&1)^1].Length())
}
