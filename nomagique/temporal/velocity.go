package temporal

import (
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Velocity owns the previous observation and computes the finite difference rate.
The first observation and non-advancing time have zero rate.
*/
type Velocity struct {
	*core.PrimitiveError
	seen      bool
	prevValue float64
	prevAt    float64
	input     data.Map[string]
	output    data.Map[float64]
}

func NewVelocity() *Velocity {
	output := data.NewOutputMap()
	output.Values["rate"] = 0
	output.Values["defined"] = 0

	return &Velocity{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"value", "value",
			"at", "at",
		),
		output: output,
	}
}

func (op *Velocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			value, valueOK := values.Values["value"]
			at, atOK := values.Values["at"]

			if !valueOK || !atOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if !op.seen {
				op.seen = true
				op.prevValue = value
				op.prevAt = at
				op.output.Values["rate"] = 0
				op.output.Values["defined"] = 0

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

			rate := 0.0
			defined := 0.0
			diffTime := (at - op.prevAt) / float64(time.Second)

			if diffTime > 0 {
				rate = (value - op.prevValue) / diffTime
				defined = 1.0
			}

			op.prevValue = value
			op.prevAt = at
			op.output.Values["rate"] = rate
			op.output.Values["defined"] = defined

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
