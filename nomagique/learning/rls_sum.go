package learning

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
SumQuery predicts a sum of future feature rows from a retained posterior.
Shared coefficient covariance is evaluated on the summed design; independent
observation noise is counted once per row. The posterior is never trained.
*/
type SumQuery struct {
	State algo.RLSState
	Rows  [][]float64
}

/*
Sum owns that query.
*/
type Sum struct {
	*core.PrimitiveError

	prediction core.Primitive
	out        algo.RLSForecast
}

func NewRLSSum() *Sum {
	return &Sum{PrimitiveError: core.NewPrimitiveError(), prediction: algo.NewRLSPrediction()}
}

func (sum *Sum) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*SumQuery)(arriving)

			if len(query.Rows) == 0 {
				sum.Error(fmt.Errorf("%w: RLS sum requires at least one row", core.ErrShape))
				return
			}

			width := len(query.Rows[0])
			design := make([]float64, width+1)
			design[0] = float64(len(query.Rows))

			ragged := false

			for _, row := range query.Rows {
				if len(row) != width {
					ragged = true
					break
				}

				for index, value := range row {
					design[index+1] += value
				}
			}

			if ragged {
				sum.Error(fmt.Errorf("%w: RLS sum rows are ragged", core.ErrShape))
				return
			}

			state := query.State
			state.Design = design
			state.Observations = float64(len(query.Rows))

			for out := range sum.prediction.Next(sequence.NewValues(state).Next(nil)) {
				sum.out = *(*algo.RLSForecast)(out)
			}

			if err := sum.prediction.Error(); err != nil {
				sum.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&sum.out)) {
				return
			}
		}
	}
}
