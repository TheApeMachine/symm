package causal

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

/*
LinearFit composes the table's affine design into ordinary least squares.
*/
type LinearFit struct {
	err       error
	ols       core.Primitive
	tolerance float64
	out       algo.Fit
}

func NewLinearFit(tolerance float64) core.Primitive {
	return &LinearFit{
		ols:       algo.NewOLS(tolerance),
		tolerance: tolerance,
	}
}

func (op *LinearFit) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*Query)(arriving)

			if !validFeatures(*query) {
				op.Error(core.ErrShape)
				return
			}

			designNode := vector.NewDesign(query.Features...)
			x := make([][]float64, 0, len(query.Rows))
			y := make([]float64, 0, len(query.Rows))

			for _, row := range query.Rows {
				var designRow []float64

				for out := range designNode.Next(transport.NewValues(row).Next(nil)) {
					copied := make([]float64, len(*(*[]float64)(out)))
					copy(copied, *(*[]float64)(out))
					designRow = copied
				}

				if err := designNode.Error(); err != nil {
					op.Error(err)
					return
				}

				if query.Target < 0 || query.Target >= len(row) {
					op.Error(core.ErrShape)
					return
				}

				x = append(x, designRow)
				y = append(y, row[query.Target])
			}

			design := algo.Design{X: x, Y: y}

			for out := range op.ols.Next(transport.NewValues(design).Next(nil)) {
				op.out = *(*algo.Fit)(out)
			}

			if err := op.ols.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *LinearFit) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
