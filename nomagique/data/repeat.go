package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Repeat consumes an inbound sequence of arrivals and yields the entire run
count times downstream.
*/
type Repeat struct {
	*core.PrimitiveError
	count int
}

func NewRepeat(count int) core.Primitive {
	return &Repeat{
		PrimitiveError: core.NewPrimitiveError(),
		count:          count,
	}
}

func (op *Repeat) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if op.count <= 0 {
			return
		}

		var items []unsafe.Pointer

		for arriving := range in {
			items = append(items, arriving)
		}

		for range op.count {
			for _, item := range items {
				if !yield(item) {
					return
				}
			}
		}
	}
}
