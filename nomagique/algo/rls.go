package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RLSPrediction projects a square-root RLS posterior through a design before any
model update.

Each arrival is *[][]float64{design, {observations}, beta, {noiseShape,
noiseScale}, root row 0, root row 1, ...}; rows [2:] are the posterior layout
shared by every RLS Primitive. Observations counts the independent noise draws
the design carries. It yields *[]float64:

	[0] prediction  [1] scale  [2] degrees of freedom
	[3] predictive variance    [4] ready (1 or 0)
	[5:] factor = root · design

Scale, degrees of freedom and predictive variance stay 0 with ready=0 until
the noise posterior is identified.
*/
type RLSPrediction struct {
	*core.PrimitiveError
	out []float64
}

func NewRLSPrediction() core.Primitive {
	return &RLSPrediction{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (op *RLSPrediction) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			rows := *(*[][]float64)(arriving)

			if len(rows) < 4 || len(rows[1]) != 1 || len(rows[3]) != 2 {
				op.Error(fmt.Errorf("%w: RLS prediction requires {design, {observations}, beta, {shape, scale}, root...}", core.ErrShape))
				return
			}

			design, beta, root := rows[0], rows[2], rows[4:]
			observations, noiseShape, noiseScale := rows[1][0], rows[3][0], rows[3][1]

			if len(beta) != len(design) || len(root) != len(design) {
				op.Error(fmt.Errorf("%w: RLS prediction coefficient, root and design dimensions differ", core.ErrShape))
				return
			}

			op.out = append(op.out[:0], 0, 0, 0, 0, 0)

			for range design {
				op.out = append(op.out, 0)
			}

			factor := op.out[5:]
			value := 0.0

			for row, feature := range design {
				if len(root[row]) != len(design) {
					op.Error(fmt.Errorf("%w: RLS root must be square", core.ErrShape))
					return
				}

				value += beta[row] * feature

				for column, coefficient := range root[row] {
					factor[column] += coefficient * feature
				}
			}

			op.out[0] = value

			if noiseShape > 0 && noiseScale > 0 {
				energy := 0.0

				for _, member := range factor {
					energy += member * member
				}

				variance := (noiseScale / noiseShape) * (observations + energy)

				if !(variance > 0) {
					op.Error(fmt.Errorf("%w: RLS predictive variance %g", core.ErrDomain, variance))
					return
				}

				op.out[1] = math.Sqrt(variance)
				op.out[2] = 2 * noiseShape
				op.out[3] = variance
				op.out[4] = 1
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
RLSUpdate owns the symmetric square-root rank-one posterior update.

Each arrival is *[][]float64{{lambda, target, prediction}, factor, beta,
{noiseShape, noiseScale}, root row 0, ...}, where factor and prediction come
from RLSPrediction on the same posterior. It yields *[][]float64{{alpha,
innovation, rootLambda, gammaDenominator}, gain, beta, {noiseShape,
noiseScale}, root row 0, ...}; rows [2:] are the updated posterior in the
arriving layout. The yielded rows are owned by this Primitive and are reused
by the next arrival.
*/
type RLSUpdate struct {
	*core.PrimitiveError
	header  []float64
	gain    []float64
	beta    []float64
	noise   []float64
	storage []float64
	out     [][]float64
}

func NewRLSUpdate() core.Primitive {
	return &RLSUpdate{
		PrimitiveError: core.NewPrimitiveError(),
		header:         make([]float64, 4),
		noise:          make([]float64, 2),
	}
}

func (op *RLSUpdate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			rows := *(*[][]float64)(arriving)

			if len(rows) < 4 || len(rows[0]) != 3 || len(rows[3]) != 2 {
				op.Error(fmt.Errorf("%w: RLS update requires {{lambda, target, prediction}, factor, beta, {shape, scale}, root...}", core.ErrShape))
				return
			}

			lambda, target, prediction := rows[0][0], rows[0][1], rows[0][2]
			factor, beta, root := rows[1], rows[2], rows[4:]
			size := len(beta)

			if len(root) != size || len(factor) != size {
				op.Error(fmt.Errorf("%w: RLS update dimensions differ", core.ErrShape))
				return
			}

			energy := 0.0

			for _, value := range factor {
				energy += value * value
			}

			alpha := lambda + energy

			if !(lambda > 0) || !(alpha > 0) {
				op.Error(fmt.Errorf("%w: invalid RLS information", core.ErrDomain))
				return
			}

			innovation := target - prediction
			rootLambda := math.Sqrt(lambda)
			denominator := alpha + rootLambda*math.Sqrt(alpha)

			if len(op.gain) != size {
				op.gain = make([]float64, size)
				op.beta = make([]float64, size)
				op.storage = make([]float64, size*size)
				op.out = make([][]float64, 4+size)

				for row := range size {
					op.out[4+row] = op.storage[row*size : (row+1)*size]
				}
			}

			clear(op.gain)

			for row := range root {
				if len(root[row]) != size {
					op.Error(fmt.Errorf("%w: RLS root must be square", core.ErrShape))
					return
				}

				for column, coefficient := range root[row] {
					op.gain[row] += coefficient * factor[column]
				}

				op.gain[row] /= alpha
				op.beta[row] = beta[row] + op.gain[row]*innovation
				posterior := op.out[4+row]

				for column, coefficient := range root[row] {
					posterior[column] = (coefficient - op.gain[row]*(alpha/denominator)*factor[column]) / rootLambda
				}
			}

			op.header[0], op.header[1], op.header[2], op.header[3] = alpha, innovation, rootLambda, denominator
			op.noise[0] = lambda*rows[3][0] + 0.5
			op.noise[1] = lambda*rows[3][1] + 0.5*innovation*innovation/alpha
			op.out[0], op.out[1], op.out[2], op.out[3] = op.header, op.gain, op.beta, op.noise

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
