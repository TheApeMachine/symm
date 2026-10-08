package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
SquareRootRLS owns one square-root RLS posterior and predicts before it
trains, composing RLSPrediction and RLSUpdate. The zero-mean diagonal prior
with the configured coefficient variance is created on the first arrival, whose
design fixes the dimension.

Each arrival is *[2][]float64{design, {lambda}} for a query that never trains,
or {design, {lambda, target}} for a labeled row; lambda is the forgetting
factor in (0,1]. It yields *[][]float64:

	row [0] {prediction, scale, degrees of freedom, predictive variance,
	         ready (1 or 0), innovation, observed (1 or 0)}
	rows [1:] {beta, {noiseShape, noiseScale}, root row 0, ...}

Row [0] is the prior forecast; rows [1:] are the committed posterior, in the
layout RLSPrediction and RLSUpdate share. The yielded rows are owned by this
Primitive and are reused by the next arrival.
*/
type SquareRootRLS struct {
	*core.PrimitiveError
	prediction core.Primitive
	update     core.Primitive
	variance   float64
	posterior  [][]float64
	unit       []float64
	control    []float64
	predict    [][]float64
	train      [][]float64
	header     []float64
	out        [][]float64
}

func NewSquareRootRLS(variance float64) core.Primitive {
	return &SquareRootRLS{
		PrimitiveError: core.NewPrimitiveError(),
		prediction:     NewRLSPrediction(),
		update:         NewRLSUpdate(),
		variance:       variance,
		unit:           []float64{1},
		control:        make([]float64, 3),
		header:         make([]float64, 7),
	}
}

func (op *SquareRootRLS) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			query := (*[2][]float64)(arriving)
			design, control := query[0], query[1]

			if len(design) == 0 || len(control) < 1 || len(control) > 2 {
				op.Error(fmt.Errorf("%w: RLS requires {design, {lambda}} or {design, {lambda, target}}", core.ErrShape))
				return
			}

			lambda := control[0]
			observed := len(control) == 2

			if !(lambda > 0 && lambda <= 1) {
				op.Error(fmt.Errorf("%w: RLS forgetting factor must be in (0,1]", core.ErrDomain))
				return
			}

			if op.posterior == nil {
				if !(op.variance > 0) {
					op.Error(fmt.Errorf("%w: RLS prior variance must be positive", core.ErrDomain))
					return
				}

				size := len(design)
				storage := make([]float64, size*size)
				op.posterior = make([][]float64, 2+size)
				op.posterior[0] = make([]float64, size)
				op.posterior[1] = make([]float64, 2)

				for row := range size {
					op.posterior[2+row] = storage[row*size : (row+1)*size]
					op.posterior[2+row][row] = math.Sqrt(op.variance)
				}
			}

			if len(design) != len(op.posterior[0]) {
				op.Error(fmt.Errorf("%w: RLS design dimension differs from the posterior", core.ErrShape))
				return
			}

			op.predict = append(append(op.predict[:0], design, op.unit), op.posterior...)
			var forecast []float64

			for pointer := range op.prediction.Next(data.NewValue(op.predict).Next(nil)) {
				forecast = *(*[]float64)(pointer)
			}

			if err := op.prediction.Error(); err != nil {
				op.Error(err)
				return
			}

			if len(forecast) < 5 {
				op.Error(fmt.Errorf("%w: RLS prediction yielded no forecast", core.ErrShape))
				return
			}

			copy(op.header, forecast[:5])
			op.header[5], op.header[6] = 0, 0

			if observed {
				op.control[0], op.control[1], op.control[2] = lambda, control[1], forecast[0]
				op.train = append(append(op.train[:0], op.control, forecast[5:]), op.posterior...)
				var posterior [][]float64

				for pointer := range op.update.Next(data.NewValue(op.train).Next(nil)) {
					posterior = *(*[][]float64)(pointer)
				}

				if err := op.update.Error(); err != nil {
					op.Error(err)
					return
				}

				if len(posterior) != 2+len(op.posterior) {
					op.Error(fmt.Errorf("%w: RLS update yielded no posterior", core.ErrShape))
					return
				}

				for row := range op.posterior {
					copy(op.posterior[row], posterior[2+row])
				}

				op.header[5], op.header[6] = posterior[0][1], 1
			}

			op.out = append(append(op.out[:0], op.header), op.posterior...)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
