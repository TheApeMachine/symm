package transport

import (
	"fmt"
	"sync/atomic"
)

const cacheLineBytes = 64

/*
RingBuffer is a bounded single-producer, single-consumer queue. Construction
allocates its backing storage once; Push and Pop do not allocate or lock.
*/
type RingBuffer[Value any] struct {
	buffer []Value
	mask   uint64

	head atomic.Uint64
	_    [cacheLineBytes - 8]byte
	tail atomic.Uint64
	_    [cacheLineBytes - 8]byte

	producerTailCache uint64
	_                 [cacheLineBytes - 8]byte
	consumerHeadCache uint64
	_                 [cacheLineBytes - 8]byte
}

/*
NewRingBuffer creates an SPSC queue whose capacity must be a power of two.
*/
func NewRingBuffer[Value any](capacity uint64) (*RingBuffer[Value], error) {
	if capacity < 2 || capacity&(capacity-1) != 0 {
		return nil, fmt.Errorf(
			"transport: ring capacity %d must be a power of two greater than one",
			capacity,
		)
	}

	return &RingBuffer[Value]{
		buffer: make([]Value, capacity),
		mask:   capacity - 1,
	}, nil
}

/*
MustNewRingBuffer creates an SPSC queue or panics for an invalid capacity.
*/
func MustNewRingBuffer[Value any](capacity uint64) *RingBuffer[Value] {
	ring, err := NewRingBuffer[Value](capacity)

	if err != nil {
		panic(err)
	}

	return ring
}

/*
Push publishes one value when capacity is available.
*/
func (ring *RingBuffer[Value]) Push(value Value) bool {
	if ring == nil {
		return false
	}

	head := ring.head.Load()
	nextHead := head + 1
	capacity := uint64(len(ring.buffer))

	if nextHead-ring.producerTailCache > capacity {
		ring.producerTailCache = ring.tail.Load()

		if nextHead-ring.producerTailCache > capacity {
			return false
		}
	}

	ring.buffer[head&ring.mask] = value
	ring.head.Store(nextHead)

	return true
}

/*
Pop consumes one value when the ring is not empty.
*/
func (ring *RingBuffer[Value]) Pop() (Value, bool) {
	var zero Value

	if ring == nil {
		return zero, false
	}

	tail := ring.tail.Load()

	if tail == ring.consumerHeadCache {
		ring.consumerHeadCache = ring.head.Load()

		if tail == ring.consumerHeadCache {
			return zero, false
		}
	}

	index := tail & ring.mask
	value := ring.buffer[index]
	ring.buffer[index] = zero
	ring.tail.Store(tail + 1)

	return value, true
}

/*
Capacity returns the fixed number of queue positions.
*/
func (ring *RingBuffer[Value]) Capacity() uint64 {
	if ring == nil {
		return 0
	}

	return uint64(len(ring.buffer))
}

/*
Len returns an instantaneous queue depth.
*/
func (ring *RingBuffer[Value]) Len() uint64 {
	if ring == nil {
		return 0
	}

	return ring.head.Load() - ring.tail.Load()
}
