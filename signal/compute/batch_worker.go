package compute

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

/*
BatchWorker drains ingest closures off the hot path and applies them on a
dedicated goroutine at a fixed cadence so heavy field math cannot block the
next market tick.

Submit drops the oldest queued task when the buffer is full, then enqueues the
new task. Each drop increments DroppedTasks.
*/
type BatchWorker struct {
	ctx          context.Context
	cancel       context.CancelFunc
	tasks        chan func()
	wg           sync.WaitGroup
	droppedTasks atomic.Int64
}

func NewBatchWorker(
	ctx context.Context,
	buffer int,
	interval time.Duration,
) *BatchWorker {
	if buffer < 1 {
		buffer = 1
	}

	if interval <= 0 {
		interval = 50 * time.Millisecond
	}

	workerCtx, cancel := context.WithCancel(ctx)
	worker := &BatchWorker{
		ctx:    workerCtx,
		cancel: cancel,
		tasks:  make(chan func(), buffer),
	}

	worker.wg.Add(1)

	go worker.run(interval)

	return worker
}

func (worker *BatchWorker) Submit(task func()) bool {
	if worker == nil || task == nil {
		return false
	}

	select {
	case worker.tasks <- task:
		return true
	case <-worker.ctx.Done():
		return false
	default:
		select {
		case <-worker.tasks:
			worker.droppedTasks.Add(1)
		default:
		}

		select {
		case worker.tasks <- task:
			return true
		case <-worker.ctx.Done():
			return false
		default:
			return false
		}
	}
}

func (worker *BatchWorker) DroppedTasks() int64 {
	if worker == nil {
		return 0
	}

	return worker.droppedTasks.Load()
}

func (worker *BatchWorker) Close() {
	if worker == nil {
		return
	}

	worker.cancel()
	worker.wg.Wait()
}

func (worker *BatchWorker) run(interval time.Duration) {
	defer worker.wg.Done()

	pending := make([]func(), 0, 256)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}

		for _, task := range pending {
			task()
		}

		pending = pending[:0]
	}

	for {
		select {
		case <-worker.ctx.Done():
			worker.drainPending(&pending)
			flush()
			return
		case task := <-worker.tasks:
			pending = append(pending, task)
			worker.drainPending(&pending)
		case <-ticker.C:
			flush()
		}
	}
}

func (worker *BatchWorker) drainPending(pending *[]func()) {
	for len(*pending) < cap(*pending) {
		select {
		case task := <-worker.tasks:
			*pending = append(*pending, task)
		default:
			return
		}
	}
}
