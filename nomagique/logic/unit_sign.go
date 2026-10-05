package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/* UnitSign requires one numeric coordinate to be exactly -1 or +1. */
type UnitSign struct {
	*core.PrimitiveError
	input data.Map[string]
}

func NewUnitSign() core.Primitive {
	return &UnitSign{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
	}
}

func (op *UnitSign) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			adapter := *(**data.Adapter)(arriving)
			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			value, ok := values.Values["value"]

			if !ok || (value != -1 && value != 1) {
				op.Error(core.ErrDomain)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
