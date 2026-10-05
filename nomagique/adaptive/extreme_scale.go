package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
ExtremeScale owns sqrt(2 log n), the coefficient of the Gaussian/EVT envelope
identity.
*/
type ExtremeScale struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewExtremeScale() core.Primitive {
	output := data.NewOutputMap()
	output.Values["scale"] = 0

	return &ExtremeScale{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("count", "count"),
		output:         output,
	}
}

func (op *ExtremeScale) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			count, ok := values.Values["count"]

			if !ok || count <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			op.output.Values["scale"] = math.Sqrt(2.0 * math.Log(count))

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
