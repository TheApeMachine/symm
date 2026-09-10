package learning

import (
	"fmt"
	"iter"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/rls"
)

/*
SumQuery predicts a sum of future feature rows from a retained posterior.
Shared coefficient covariance is evaluated on the summed design; independent
observation noise is counted once per row. The posterior is never trained.
*/
type SumQuery struct {
	State rls.State
	Rows  [][]float64
}

/*
Sum owns that query.
*/
type Sum struct {
	core.Base[SumQuery, rls.Forecast]
	prediction *rls.Prediction
}

func NewRLSSum() *Sum {
	return &Sum{prediction: rls.NewPrediction()}
}

func (op *Sum) Next(
	in iter.Seq[core.Primitive[SumQuery, SumQuery]],
) iter.Seq[core.Primitive[rls.Forecast, rls.Forecast]] {
	return func(yield func(core.Primitive[rls.Forecast, rls.Forecast]) bool) {
		for arriving := range in {
			forecast, err := op.Project(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(forecast)) {
				return
			}
		}
	}
}

func (op *Sum) Project(query SumQuery) (rls.Forecast, error) {
	if len(query.Rows) == 0 {
		return rls.Forecast{}, fmt.Errorf("%w: RLS sum requires at least one row", core.ErrShape)
	}

	width := len(query.Rows[0])
	design := make([]float64, width+1)
	design[0] = float64(len(query.Rows))

	for _, row := range query.Rows {
		if len(row) != width {
			return rls.Forecast{}, fmt.Errorf("%w: RLS sum rows are ragged", core.ErrShape)
		}

		for index, value := range row {
			design[index+1] += value
		}
	}

	state := query.State
	state.Design = design
	state.Observations = float64(len(query.Rows))
	return op.prediction.Project(state)
}
