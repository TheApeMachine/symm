package runtime

import (
	"sync/atomic"
)

/*
Ring is a bounded many-producer/single-consumer buffer with overwrite-oldest
semantics. Producers never block: when the ring is full the oldest retained
value is discarded and counted, so the consumer always sees the freshest
window of the stream and the pipeline never queues behind the market.

Each slot is sequence-guarded: the producer reserves the next slot with a
compare-and-swap on the head cursor, writes the value, and only then publishes
the sequence (release ordering). The single consumer pops in FIFO order and
skips any region the producer has lapped, so it never observes a partially
written value. T must be a pointer-sized item — every hot stream in the
pipeline carries pointers.
*/
type Ring[T any] struct {
	slots   []T
	seqs    []atomic.Uint64
	mask    uint64
	head    atomic.Uint64
	tail    atomic.Uint64
	dropped atomic.Uint64
}

/*
NewRing constructs a bounded ring with the given capacity rounded up to a
power of two (minimum two slots).
*/
func NewRing[T any](capacity uint64) *Ring[T] {
	capacity = nextPowerOfTwo(capacity)
	if capacity < 2 {
		capacity = 2
	}

	return &Ring[T]{
		slots: make([]T, capacity),
		seqs:  make([]atomic.Uint64, capacity),
		mask:  capacity - 1,
	}
}

/*
Push reserves the next slot for value and publishes it. It never blocks and
never allocates. When the consumer is more than capacity behind, the oldest
unread value is overwritten and counted as dropped; the consumer detects the
gap through the sequence numbers and skips straight to the freshest retained
value.
*/
func (ring *Ring[T]) Push(value T) {
	for {
		head := ring.head.Load()
		tail := ring.tail.Load()

		if head-tail >= uint64(len(ring.slots)) {
			ring.dropped.Add(1)
		}

		index := head & ring.mask

		if !ring.head.CompareAndSwap(head, head+1) {
			continue
		}

		ring.slots[index] = value
		ring.seqs[index].Store(head + 1)

		return
	}
}

/*
Pop returns the oldest retained value in FIFO order. It returns ok=false when
the ring is empty or the next slot is not yet published. Lapped regions (the
producer overwrote them) are skipped via the sequence gap.
*/
func (ring *Ring[T]) Pop() (value T, ok bool) {
	for {
		tail := ring.tail.Load()
		head := ring.head.Load()

		if tail >= head {
			return value, false
		}

		if head-tail > uint64(len(ring.slots)) {
			ring.tail.Store(head - uint64(len(ring.slots)))
			continue
		}

		index := tail & ring.mask
		seq := ring.seqs[index].Load()

		if seq != tail+1 {
			if seq > tail+1 {
				ring.tail.Store(seq - 1)
				continue
			}

			return value, false
		}

		value = ring.slots[index]
		ring.tail.Store(tail + 1)

		return value, true
	}
}

/*
Length reports the number of retained values, including any overwritten
region the consumer has not yet skipped. It is an instantaneous pressure
read for diagnostics.
*/
func (ring *Ring[T]) Length() uint64 {
	head := ring.head.Load()
	tail := ring.tail.Load()

	if head < tail {
		return 0
	}

	return head - tail
}

/*
Dropped reports the lifetime count of values discarded by the overwrite
bound, the pressure signal that shows a reader is falling behind.
*/
func (ring *Ring[T]) Dropped() uint64 {
	return ring.dropped.Load()
}

/*
Capacity reports the fixed slot count of the ring.
*/
func (ring *Ring[T]) Capacity() uint64 {
	return uint64(len(ring.slots))
}
