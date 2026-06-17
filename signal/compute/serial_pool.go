package compute

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

/*
SerialPool runs mutations on one worker goroutine. Hot-path callers use
Enqueue for fire-and-forget work; Call and Run block until the task finishes.
*/
type SerialPool struct {
	ctx          context.Context
	cancel       context.CancelFunc
	tasks        chan func()
	wg           sync.WaitGroup
	droppedTasks atomic.Int64
	running      atomic.Bool
}

func NewSerialPool(
	ctx context.Context,
	buffer int,
	flushInterval time.Duration,
) *SerialPool {
	if buffer < 1 {
		buffer = 1
	}

	if flushInterval <= 0 {
		flushInterval = 50 * time.Millisecond
	}

	workerCtx, cancel := context.WithCancel(ctx)
	serial := &SerialPool{
		ctx:    workerCtx,
		cancel: cancel,
		tasks:  make(chan func(), buffer),
	}

	serial.wg.Add(1)

	go func() {
		defer serial.wg.Done()
		serial.run(flushInterval)
	}()

	return serial
}

func (serial *SerialPool) Enqueue(task func()) bool {
	if serial == nil || task == nil {
		return false
	}

	select {
	case serial.tasks <- task:
		return true
	case <-serial.ctx.Done():
		return false
	default:
		select {
		case <-serial.tasks:
			serial.droppedTasks.Add(1)
		default:
		}

		select {
		case serial.tasks <- task:
			return true
		case <-serial.ctx.Done():
			return false
		default:
			return false
		}
	}
}

func (serial *SerialPool) enqueueBlocking(task func()) bool {
	if serial == nil || task == nil {
		return false
	}

	select {
	case serial.tasks <- task:
		return true
	case <-serial.ctx.Done():
		return false
	}
}

func (serial *SerialPool) Call(task func()) {
	if serial == nil || task == nil {
		return
	}

	if serial.running.Load() {
		task()
		return
	}

	done := make(chan struct{}, 1)

	if !serial.enqueueBlocking(func() {
		task()
		close(done)
	}) {
		return
	}

	<-done
}

func Run[T any](serial *SerialPool, work func() T) (zero T) {
	if work == nil {
		return zero
	}

	if serial == nil {
		return work()
	}

	if serial.running.Load() {
		return work()
	}

	reply := make(chan T, 1)

	if !serial.enqueueBlocking(func() {
		reply <- work()
	}) {
		return zero
	}

	return <-reply
}

func (serial *SerialPool) Running() bool {
	if serial == nil {
		return false
	}

	return serial.running.Load()
}

func (serial *SerialPool) DroppedTasks() int64 {
	if serial == nil {
		return 0
	}

	return serial.droppedTasks.Load()
}

func (serial *SerialPool) Close() {
	if serial == nil {
		return
	}

	serial.cancel()
	serial.wg.Wait()
}

func (serial *SerialPool) run(flushInterval time.Duration) {
	pending := make([]func(), 0, 256)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}

		serial.running.Store(true)

		for _, task := range pending {
			task()
		}

		serial.running.Store(false)
		pending = pending[:0]
	}

	for {
		select {
		case <-serial.ctx.Done():
			serial.drainPending(&pending)
			flush()
			return
		case task := <-serial.tasks:
			pending = append(pending, task)
			serial.drainPending(&pending)
		case <-ticker.C:
			flush()
		}
	}
}

func (serial *SerialPool) drainPending(pending *[]func()) {
	for len(*pending) < cap(*pending) {
		select {
		case task := <-serial.tasks:
			*pending = append(*pending, task)
		default:
			return
		}
	}
}
