package probability

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Argmax preserves a winning value's ordinal and value through comparison.
*/
type Argmax struct {
	*core.PrimitiveError
	bestValue    float64
	bestIndex    float64
	currentIndex float64
	seen         bool
	input        data.Map[string]
	output       data.Map[float64]
}

func NewArgmax() *Argmax {
	output := data.NewOutputMap()
	output.Values["winner_index"] = 0
	output.Values["winner_value"] = 0

	return &Argmax{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("value", "value"),
		output:         output,
	}
}

func (op *Argmax) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			val, ok := values.Values["value"]

			if !ok {
				op.Error(core.ErrNotHeld)
				return
			}

			if !op.seen || val > op.bestValue {
				op.bestValue = val
				op.bestIndex = op.currentIndex
				op.seen = true
			}

			op.currentIndex++
			op.output.Values["winner_index"] = op.bestIndex
			op.output.Values["winner_value"] = op.bestValue

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
