package learning

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	arithmetic "github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
LinearFit composes the table's affine design into ordinary least squares.
*/
type LinearFit struct {
	*core.PrimitiveError

	ols       core.Primitive
	tolerance float64
	out       algo.Fit
}

func NewLinearFit(tolerance float64) *LinearFit {
	return &LinearFit{PrimitiveError: core.NewPrimitiveError(), ols: algo.NewOLS(tolerance),
		tolerance: tolerance,
	}
}

func (linearFit *LinearFit) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*Query)(arriving)

			if !validFeatures(*query) {
				linearFit.Error(core.ErrShape)
				return
			}

			designNode := arithmetic.NewDesign(query.Features...)
			x := make([][]float64, 0, len(query.Rows))
			y := make([]float64, 0, len(query.Rows))

			for _, row := range query.Rows {
				var designRow []float64

				for out := range designNode.Next(sequence.NewValues(row).Next(nil)) {
					copied := make([]float64, len(*(*[]float64)(out)))
					copy(copied, *(*[]float64)(out))
					designRow = copied
				}

				if err := designNode.Error(); err != nil {
					linearFit.Error(err)
					return
				}

				if query.Target < 0 || query.Target >= len(row) {
					linearFit.Error(core.ErrShape)
					return
				}

				x = append(x, designRow)
				y = append(y, row[query.Target])
			}

			design := algo.Design{X: x, Y: y}

			for out := range linearFit.ols.Next(sequence.NewValues(design).Next(nil)) {
				linearFit.out = *(*algo.Fit)(out)
			}

			if err := linearFit.ols.Error(); err != nil {
				linearFit.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&linearFit.out)) {
				return
			}
		}
	}
}
