package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Parallel streams transparent throughput across multiple nomagique Primitives.
When stepped without input, it drains each configured primitive in order.
When stepped with an input stream, it broadcasts arriving payloads across
all primitives and yields their outputs.
*/
type Parallel struct {
	*core.PrimitiveError
	primitives []core.Primitive
}

/*
NewParallel takes a slice of Primitives and returns a new Parallel Primitive.
*/
func NewParallel(primitives ...core.Primitive) *Parallel {
	return &Parallel{
		PrimitiveError: core.NewPrimitiveError(),
		primitives:     primitives,
	}
}

/*
Next provides transparent throughput across the configured primitives.
*/
func (parallel *Parallel) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {
			for _, primitive := range parallel.primitives {
				if err := primitive.Error(); err != nil {
					parallel.Error(err)
				}
			}
		}()

		if in == nil {
			for _, primitive := range parallel.primitives {
				stream := primitive.Next(nil)
				if stream != nil {
					for item := range stream {
						if !yield(item) {
							return
						}
					}
				}
			}
			return
		}

		for arriving := range in {
			one := func(yieldOne func(unsafe.Pointer) bool) {
				yieldOne(arriving)
			}

			for _, primitive := range parallel.primitives {
				stream := primitive.Next(one)
				if stream != nil {
					for item := range stream {
						if !yield(item) {
							return
						}
					}
				}
			}
		}
	}
}
