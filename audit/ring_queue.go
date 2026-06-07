package audit

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

const defaultRingCapacity = 4096

// The consumer's wait strategy, in the go-disruptor mold: yield the scheduler a
// few times for low-latency wakeup under flow, then sleep when the tape is
// actually idle. No condvar — the previous "lock-free" ring took a mutex on
// EVERY Push to signal one, and the consumer held that same mutex across its
// whole drain loop, serialising every producer in the system through a single
// lock while the doc comment claimed otherwise.
const (
	consumerSpinYields = 64
	consumerIdleSleep  = time.Millisecond
)

/*
ringQueue is a multi-producer, single-consumer bounded queue. Producers reserve
slots with CAS and write with an atomic store — no locks, no signaling; the lone
consumer polls with a yield-then-sleep backoff. Push returns an error when the
ring is full so the hot path never blocks.
*/
type ringQueue struct {
	capacity  uint64
	mask      uint64
	published atomic.Uint64
	consumed  atomic.Uint64
	slots     []atomic.Pointer[map[string]any]
	closed    atomic.Bool
}

func newRingQueue() *ringQueue {
	capacity := uint64(defaultRingCapacity)

	return &ringQueue{
		capacity: capacity,
		mask:     capacity - 1,
		slots:    make([]atomic.Pointer[map[string]any], capacity),
	}
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

		queue.slots[head&queue.mask].Store(&frame)

		return nil
	}
}

func (queue *ringQueue) Pop() (map[string]any, bool) {
	for spins := 0; ; spins++ {
		if frame, ok := queue.tryPop(); ok {
			return frame, true
		}

		if queue.closed.Load() {
			// One final attempt catches a producer that claimed its slot
			// before Close but stored just after our last tryPop.
			if frame, ok := queue.tryPop(); ok {
				return frame, true
			}

			return nil, false
		}

		if spins < consumerSpinYields {
			runtime.Gosched()

			continue
		}

		time.Sleep(consumerIdleSleep)
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

	// A producer has claimed this slot but not yet stored into it; treat the
	// queue as momentarily empty and let the backoff retry preserve order.
	if framePtr == nil {
		return nil, false
	}

	queue.slots[slot].Store(nil)
	queue.consumed.Store(tail + 1)

	return *framePtr, true
}

func (queue *ringQueue) Close() {
	queue.closed.Store(true)
}
