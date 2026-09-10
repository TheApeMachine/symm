package causal

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LinearFit composes the table's affine design into ordinary least squares.
*/
type LinearFit struct {
	core.Base[Query, algo.Fit]
	ols    *algo.OLS
	design *equation.Design[float64]
}

func NewLinearFit(tolerance float64) *LinearFit {
	return &LinearFit{ols: algo.NewOLS(tolerance)}
}

func (op *LinearFit) Next(
	in iter.Seq[core.Primitive[Query, Query]],
) iter.Seq[core.Primitive[algo.Fit, algo.Fit]] {
	return func(yield func(core.Primitive[algo.Fit, algo.Fit]) bool) {
		for arriving := range in {
			fit, err := op.Fit(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(fit)) {
				return
			}
		}
	}
}

func (op *LinearFit) Fit(query Query) (algo.Fit, error) {
	if err := shape(query); err != nil {
		return algo.Fit{}, err
	}

	op.design = equation.NewDesign[float64](query.Features)
	x := make([][]float64, 0, len(query.Rows))
	y := make([]float64, 0, len(query.Rows))
	at := collection.NewAt[float64](query.Target)

	for _, row := range query.Rows {
		design, err := transport.Evaluate(op.design, transport.Values(row))

		if err != nil {
			return algo.Fit{}, err
		}

		outcome, err := transport.Evaluate(at, transport.Values(row))

		if err != nil {
			return algo.Fit{}, err
		}

		x = append(x, design)
		y = append(y, outcome)
	}

	return op.ols.Fit(algo.Design{X: x, Y: y})
}
