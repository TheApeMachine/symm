package causal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Backdoor standardizes predictions over observed evidence after replacing the
treatment coordinate with the interventional level.
*/
type Backdoor struct {
	*core.PrimitiveError
	tolerance float64
	input     data.Map[string]
	output    data.Map[float64]
}

func NewBackdoor(tolerance float64) *Backdoor {
	output := data.NewOutputMap()
	output.Values["expectation"] = 0
	output.Values["defined"] = 0

	return &Backdoor{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
		input: data.NewMap(
			"level", "level",
			"baseline", "baseline",
			"effect", "effect",
		),
		output: output,
	}
}

func (op *Backdoor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			level, levelOK := values.Values["level"]
			baseline, baselineOK := values.Values["baseline"]
			effect, effectOK := values.Values["effect"]

			if !levelOK || !baselineOK || !effectOK {
				op.Error(core.ErrNotHeld)
				return
			}

			op.output.Values["expectation"] = baseline + effect*level
			op.output.Values["defined"] = 1.0

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
