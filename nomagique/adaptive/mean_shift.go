package adaptive

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
MeanShift owns the support-dependent mean-shift bound.
*/
type MeanShift struct {
	*core.PrimitiveError
	input  data.Map[string]
	output data.Map[float64]
}

func NewMeanShift() core.Primitive {
	output := data.NewOutputMap()
	output.Values["bound"] = 0

	return &MeanShift{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"variance", "variance",
			"observations", "observations",
			"recent_count", "recent_count",
			"prior_count", "prior_count",
		),
		output: output,
	}
}

func (op *MeanShift) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			variance, varianceOK := values.Values["variance"]
			observations, observationsOK := values.Values["observations"]
			recentCount, recentOK := values.Values["recent_count"]
			priorCount, priorOK := values.Values["prior_count"]

			if !varianceOK || !observationsOK || !recentOK || !priorOK {
				op.Error(core.ErrNotHeld)
				return
			}

			bound := 0.0

			if recentCount > 0 && priorCount > 0 && observations > 0 && variance > 0 {
				bound = math.Sqrt(variance * (math.Log(4*observations*observations) * (0.5 * (1/recentCount + 1/priorCount))))
			}

			op.output.Values["bound"] = bound

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
