package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* Positive requires one numeric coordinate to be strictly positive. */
type Positive struct {
	*core.PrimitiveError
	input data.Map[string]
}

func NewPositive() core.Primitive {
	return &Positive{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
	}
}

func (op *Positive) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			value, ok := values.Values["value"]

			if !ok || value <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
