package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
StateWriter captures the incoming stream and yields a Message(WRITE) 
containing the collected values. The first item in the stream must be 
the dynamic string key.
*/
type StateWriter struct {
	*core.PrimitiveError
	address string
	indices []int
}

func NewStateWriter(address string, indices ...int) core.Primitive {
	return &StateWriter{
		PrimitiveError: core.NewPrimitiveError(),
		address:        address,
		indices:        indices,
	}
}

func (op *StateWriter) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var cache []unsafe.Pointer
		for ptr := range in {
			cache = append(cache, ptr)
		}

		if len(cache) == 0 {
			return
		}

		// First item is the dynamic string key
		key := *(*string)(cache[0])
		
		// The rest are the values
		items := cache[1:]

		var selected []unsafe.Pointer
		if len(op.indices) > 0 {
			for _, idx := range op.indices {
				if idx >= 0 && idx < len(items) {
					selected = append(selected, items[idx])
				}
			}
		} else {
			selected = items
		}

		// Yield the write message at the end
		msg := NewMessage(WRITE, op.address, key, NewValue(selected...))
		if !yield(unsafe.Pointer(msg)) {
			return
		}
	}
}
