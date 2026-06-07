package audit

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const defaultRingCapacity = 4096

/*
ringQueue is a multi-producer, single-consumer bounded queue. Producers reserve
slots with atomic fetch-and-add and write without mutexes; the lone consumer
drains slots in order. Push returns an error when the ring is full so the hot
path never blocks on a contended lock or a parked scheduler.
*/
type ringQueue struct {
	capacity  uint64
	mask      uint64
	published atomic.Uint64
	consumed  atomic.Uint64
	slots     []atomic.Pointer[map[string]any]
	closed    atomic.Bool
	idleMu    sync.Mutex
	ready     *sync.Cond
}

func newRingQueue() *ringQueue {
	capacity := uint64(defaultRingCapacity)
	queue := &ringQueue{
		capacity: capacity,
		mask:     capacity - 1,
		slots:    make([]atomic.Pointer[map[string]any], capacity),
	}
	queue.idleMu.Lock()
	queue.ready = sync.NewCond(&queue.idleMu)
	queue.idleMu.Unlock()

	return queue
}

func (queue *ringQueue) Push(frame map[string]any) error {
	if queue.closed.Load() {
		return fmt.Errorf("audit: writer closed")
	}

	for {
		head := queue.published.Load()
		tail := queue.consumed.Load()

		if head-tail >= queue.capacity {
			return fmt.Errorf("audit: queue full")
		}

		if !queue.published.CompareAndSwap(head, head+1) {
			continue
		}

		slot := head & queue.mask
		queue.slots[slot].Store(&frame)

		queue.idleMu.Lock()
		queue.ready.Signal()
		queue.idleMu.Unlock()

		return nil
	}
}

func (queue *ringQueue) Pop() (map[string]any, bool) {
	queue.idleMu.Lock()
	defer queue.idleMu.Unlock()

	for {
		if frame, ok := queue.tryPop(); ok {
			queue.ready.Signal()

			return frame, true
		}

		if queue.closed.Load() {
			return nil, false
		}

		queue.ready.Wait()
	}
}

func (queue *ringQueue) tryPop() (map[string]any, bool) {
	tail := queue.consumed.Load()
	head := queue.published.Load()

	if tail >= head {
		return nil, false
	}

	slot := tail & queue.mask
	framePtr := queue.slots[slot].Load()

	if framePtr == nil {
		return nil, false
	}

	queue.slots[slot].Store(nil)
	queue.consumed.Store(tail + 1)

	return *framePtr, true
}

func (queue *ringQueue) Close() {
	queue.closed.Store(true)

	queue.idleMu.Lock()
	queue.ready.Broadcast()
	queue.idleMu.Unlock()
}
