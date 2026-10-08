package transport

import (
	"errors"
	"fmt"
	"iter"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

const cacheLineBytes = 64

/*
Ring is a bounded single-producer, single-consumer queue presented as a
Primitive. Construction requires a power-of-two capacity and allocates the
backing storage once; pushes and pops are lock-free SPSC operations with
cached tails and never allocate or lock.

On the Primitive wire Next is the queue discipline: each arrival is
published in order, then everything poppable is yielded in order. A value
that cannot be published because the ring is full is never silently
dropped; it records a shape error and ends the run.
*/
type Ring struct {
	err    error
	buffer []unsafe.Pointer
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
NewRing instantiates a Ring Primitive whose capacity must be a power of two
greater than one. Any other capacity records a shape error.
*/
func NewRing(capacity uint64) core.Primitive {
	op := &Ring{}

	if capacity < 2 || capacity&(capacity-1) != 0 {
		op.Error(fmt.Errorf(
			"%w: ring capacity %d must be a power of two greater than one",
			core.ErrShape,
			capacity,
		))
		return op
	}

	op.buffer = make([]unsafe.Pointer, capacity)
	op.mask = capacity - 1
	return op
}

func (op *Ring) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.Error() != nil {
			return
		}

		for arriving := range in {
			if !op.push(arriving) {
				op.Error(fmt.Errorf("%w: ring at capacity", core.ErrShape))
				return
			}
		}

		for {
			value, found := op.pop()

			if !found {
				return
			}

			if !yield(value) {
				return
			}
		}
	}
}

func (op *Ring) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
push publishes one value when capacity is available.
*/
func (op *Ring) push(value unsafe.Pointer) bool {
	head := op.head.Load()
	nextHead := head + 1
	capacity := uint64(len(op.buffer))

	if nextHead-op.producerTailCache > capacity {
		op.producerTailCache = op.tail.Load()

		if nextHead-op.producerTailCache > capacity {
			return false
		}
	}

	op.buffer[head&op.mask] = value
	op.head.Store(nextHead)

	return true
}

/*
pop consumes one value when the ring is not empty.
*/
func (op *Ring) pop() (unsafe.Pointer, bool) {
	var zero unsafe.Pointer

	tail := op.tail.Load()

	if tail == op.consumerHeadCache {
		op.consumerHeadCache = op.head.Load()

		if tail == op.consumerHeadCache {
			return zero, false
		}
	}

	index := tail & op.mask
	value := op.buffer[index]
	op.buffer[index] = zero
	op.tail.Store(tail + 1)

	return value, true
}
