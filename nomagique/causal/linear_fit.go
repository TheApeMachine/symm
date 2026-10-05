package causal

import (
	"iter"
	"unsafe"

	"gonum.org/v1/gonum/mat"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
LinearFit estimates ordinary least squares parameters for arriving observations.
*/
type LinearFit struct {
	*core.PrimitiveError
	tolerance float64
	input     data.Map[string]
	output    data.Map[float64]
}

func NewLinearFit(tolerance float64) *LinearFit {
	output := data.NewOutputMap()
	output.Values["intercept"] = 0
	output.Values["coefficient"] = 0
	output.Values["defined"] = 0

	return &LinearFit{
		PrimitiveError: core.NewPrimitiveError(),
		tolerance:      tolerance,
		input: data.NewMap(
			"target", "target",
			"feature", "feature",
		),
		output: output,
	}
}

func (op *LinearFit) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
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

			target, targetOK := values.Values["target"]
			feature, featureOK := values.Values["feature"]

			if !targetOK || !featureOK {
				op.Error(core.ErrNotHeld)
				return
			}

			design := mat.NewDense(1, 2, []float64{1.0, feature})
			outcome := mat.NewDense(1, 1, []float64{target})

			var solved mat.Dense
			err := solved.Solve(design, outcome)

			if err != nil {
				op.output.Values["defined"] = 0
			}

			if err == nil {
				op.output.Values["intercept"] = solved.At(0, 0)
				op.output.Values["coefficient"] = solved.At(1, 0)
				op.output.Values["defined"] = 1.0
			}

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
