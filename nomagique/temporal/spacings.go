package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Spacings owns consecutive timestamp differences within one delivery run.
*/
type Spacings struct {
	*core.PrimitiveError
	previous float64
	seen     bool
	input    data.Map[string]
	output   data.Map[float64]
}

func NewSpacings() *Spacings {
	output := data.NewOutputMap()
	output.Values["spacing"] = 0

	return &Spacings{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("at", "at"),
		output:         output,
	}
}

func (op *Spacings) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			at, ok := values.Values["at"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			if !op.seen {
				op.seen = true
				op.previous = at
				op.output.Values["spacing"] = 0

				for range adapter.Next(data.NewValue(op.output)) {
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				if !yield(arriving) {
					return
				}

				continue
			}

			op.output.Values["spacing"] = at - op.previous
			op.previous = at

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
