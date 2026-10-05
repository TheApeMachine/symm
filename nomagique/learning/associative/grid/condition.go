package grid

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Condition encodes a quantity's identity and the ternary directions of its level
and change into a 64-bit token.
*/
type Condition struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewCondition() *Condition {
	output := data.NewOutputMap()
	output.Values["token"] = 0

	return &Condition{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"quantity", "quantity",
			"level", "level",
			"change", "change",
		),
		output: output,
	}
}

func (op *Condition) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			quantityVal, quantityOK := values.Values["quantity"]
			level, levelOK := values.Values["level"]
			change, changeOK := values.Values["change"]

			if !quantityOK || !levelOK || !changeOK {
				op.Error(core.ErrNotHeld)
				return
			}

			quantity := uint64(quantityVal)

			if quantity == 0 || quantity >= 1<<48 {
				op.Error(core.ErrDomain)
				return
			}

			state := uint64(0)

			if level > 0 {
				state |= 1
			}

			if level < 0 {
				state |= 2
			}

			if change > 0 {
				state |= 1 << 2
			}

			if change < 0 {
				state |= 2 << 2
			}

			out := uint64(1<<52 | quantity<<4 | state)
			op.output.Values["token"] = float64(out)

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
