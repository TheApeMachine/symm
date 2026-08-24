package runtime

import (
	"runtime"
	"sync/atomic"
)

/*
CacheLinePad is the padding width used to isolate cursor fields onto distinct
cache lines and eliminate false sharing between producer and consumer cores.
*/
const CacheLinePad = 64

/*
DisruptorRing is a bounded, cache-line padded multi-producer/single-consumer
ring buffer. Producers reserve slots with a compare-and-swap on the head
cursor, write the value, and then publish the slot's sequence; the consumer
polls in FIFO order and only observes a value after its sequence is published.
Offer fails (and counts a drop) when the ring is full, so the caller may apply
backpressure instead of overwriting the oldest value.
*/
type DisruptorRing[T any] struct {
	_        [CacheLinePad]byte
	head     atomic.Uint64
	_        [CacheLinePad - 8]byte
	tail     atomic.Uint64
	_        [CacheLinePad - 8]byte
	capacity uint64
	mask     uint64
	slots    []T
	seqs     []atomic.Uint64
	dropped  atomic.Uint64
}

/*
NewDisruptorRing constructs a ring whose capacity is rounded up to a power of
two (minimum two slots).
*/
func NewDisruptorRing[T any](capacity uint64) *DisruptorRing[T] {
	capPow2 := nextPowerOfTwo(capacity)

	if capPow2 < 2 {
		capPow2 = 2
	}

	return &DisruptorRing[T]{
		capacity: capPow2,
		mask:     capPow2 - 1,
		slots:    make([]T, capPow2),
		seqs:     make([]atomic.Uint64, capPow2),
	}
}

/*
Offer publishes one value when capacity is available, otherwise it counts a
drop and returns false.
*/
func (ring *DisruptorRing[T]) Offer(item T) bool {
	for {
		head := ring.head.Load()
		tail := ring.tail.Load()

		if head-tail >= ring.capacity {
			ring.dropped.Add(1)

			return false
		}

		if ring.head.CompareAndSwap(head, head+1) {
			index := head & ring.mask
			ring.slots[index] = item
			ring.seqs[index].Store(head + 1)

			return true
		}

		runtime.Gosched()
	}
}

/*
Poll consumes one value in FIFO order when available.
*/
func (ring *DisruptorRing[T]) Poll() (T, bool) {
	var zero T

	for {
		tail := ring.tail.Load()
		head := ring.head.Load()

		if tail >= head {
			return zero, false
		}

		index := tail & ring.mask
		seq := ring.seqs[index].Load()

		if seq != tail+1 {
			if seq > tail+1 {
				ring.tail.Store(seq - 1)

				continue
			}

			return zero, false
		}

		value := ring.slots[index]
		ring.slots[index] = zero
		ring.tail.Store(tail + 1)

		return value, true
	}
}

/*
Dropped returns the number of values rejected because the ring was full.
*/
func (ring *DisruptorRing[T]) Dropped() uint64 {
	return ring.dropped.Load()
}

/*
Length returns an instantaneous queue depth.
*/
func (ring *DisruptorRing[T]) Length() uint64 {
	if ring == nil {
		return 0
	}

	return ring.head.Load() - ring.tail.Load()
}
