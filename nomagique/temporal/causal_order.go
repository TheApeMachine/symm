package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
CausalOrder admits strictly advancing sequence identities on a non-regressing
numeric event-time coordinate.
*/
type CausalOrder struct {
	*core.PrimitiveError
	seen         bool
	lastSequence float64
	lastAt       float64
	input        data.Map[string]
}

func NewCausalOrder() core.Primitive {
	return &CausalOrder{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("sequence", "sequence", "at", "at"),
	}
}

func (op *CausalOrder) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			sequence, sequenceOK := values.Values["sequence"]
			at, atOK := values.Values["at"]

			if !sequenceOK || !atOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if op.seen && (sequence <= op.lastSequence || at < op.lastAt) {
				op.Error(core.ErrDomain)
				return
			}

			op.seen = true
			op.lastSequence = sequence
			op.lastAt = at

			if !yield(arriving) {
				return
			}
		}
	}
}
