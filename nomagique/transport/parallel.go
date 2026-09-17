package transport

import (
	"iter"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime/disruptor"
)

/*
Parallel combines multiple nomagique Primitives that reside in different
processes. Each of the Primitives is given a goroutine, and is run in
isolation, with the exception of the top-most and bottom-most Primitive.
*/
type Parallel struct {
	*core.PrimitiveError
	primitives []core.Primitive
}

/*
New takes a slice of Primitives and returns a new Parallel Primitive.
*/
func NewParallel(primitives ...core.Primitive) *Parallel {
	return &Parallel{
		PrimitiveError: core.NewPrimitiveError(),
		primitives:     primitives,
	}
}

/*
Next takes the input and proxies it through the input Primitive, then takes
the result of the input Primitive's Next method, and passes that as the input
to the output Primitive.
*/
func (parallel *Parallel) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		capacity := uint32(1024)
		ringBuffer := make([]unsafe.Pointer, capacity)

		handler := &outHandler{
			ringBuffer: ringBuffer,
			yield:      yield,
			capacity:   int64(capacity),
		}

		channel, err := disruptor.New(
			disruptor.Options.BufferCapacity(capacity),
			disruptor.Options.WriterCount(uint8(len(parallel.primitives))),
			disruptor.Options.NewHandlerGroup(handler),
		)
		if err != nil {
			parallel.Error(err)
			return
		}

		handler.channel = channel

		var wg sync.WaitGroup
		for _, primitive := range parallel.primitives {
			wg.Add(1)
			go func(prim core.Primitive) {
				defer wg.Done()
				stream := prim.Next(in)
				if stream != nil {
					for item := range stream {
						seq := channel.Reserve(1)
						ringBuffer[seq&(int64(capacity)-1)] = item
						channel.Commit(seq, seq)
					}
				}
			}(primitive)
		}

		go func() {
			wg.Wait()
			channel.Close()
		}()

		// Listen runs on the main goroutine so that yield is called safely.
		channel.Listen()
	}
}

type outHandler struct {
	ringBuffer []unsafe.Pointer
	yield      func(unsafe.Pointer) bool
	capacity   int64
	channel    disruptor.Disruptor
	aborted    bool
}

func (h *outHandler) Next(seq iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if h.aborted {
		return nil
	}
	for ptr := range seq {
		bounds := *(*[]int64)(ptr)
		lower, upper := bounds[0], bounds[1]
		for i := lower; i <= upper; i++ {
			item := h.ringBuffer[i&(h.capacity-1)]
			if !h.yield(item) {
				h.aborted = true
				h.channel.Close()
				return nil
			}
		}
	}
	return nil
}
