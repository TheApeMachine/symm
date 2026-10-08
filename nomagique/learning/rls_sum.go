package learning

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
RLSSum predicts a sum of future feature rows from a retained posterior.
Shared coefficient covariance is evaluated on the summed design; independent
observation noise is counted once per row. The posterior is never trained.

Each arrival is *[2][][]float64. [0] is the retained posterior as rows
{beta, {noiseShape, noiseScale}, root row 0, root row 1, ...}; [1] is the
future feature rows. It yields algo.RLSPrediction's *[]float64 {prediction,
scale, degrees of freedom, predictive variance, ready, factor...} for the
summed design, with one noise draw counted per future row.
*/
type RLSSum struct {
	*core.PrimitiveError
	prediction core.Primitive
	state      [][]float64
	out        []float64
}

func NewRLSSum() core.Primitive {
	return &RLSSum{
		PrimitiveError: core.NewPrimitiveError(),
		prediction:     algo.NewRLSPrediction(),
	}
}

func (op *RLSSum) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			query := (*[2][][]float64)(arriving)
			posterior, rows := query[0], query[1]

			if len(posterior) < 2 || len(posterior[1]) != 2 {
				op.Error(fmt.Errorf("%w: RLS sum requires {beta, {shape, scale}, root...}", core.ErrShape))
				return
			}

			if len(rows) == 0 {
				op.Error(fmt.Errorf("%w: RLS sum requires at least one row", core.ErrShape))
				return
			}

			width := len(rows[0])
			design := make([]float64, width+1)
			design[0] = float64(len(rows))

			for _, row := range rows {
				if len(row) != width {
					op.Error(fmt.Errorf("%w: RLS sum rows are ragged", core.ErrShape))
					return
				}

				for index, value := range row {
					design[index+1] += value
				}
			}

			op.state = append(
				append(op.state[:0], design, []float64{float64(len(rows))}),
				posterior...,
			)

			for out := range op.prediction.Next(data.NewValue(op.state).Next(nil)) {
				op.out = *(*[]float64)(out)
			}

			if err := op.prediction.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
