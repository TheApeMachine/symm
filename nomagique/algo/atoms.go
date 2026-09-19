package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

type RLS types.Value[[]float64, [2]float64]
/*
NewRLS creates a stateful closure for Recursive Least Squares (RLS) with symmetric
square-root rank-one updates. It encapsulates all covariance and coefficient state,
completely eliminating the need for RLSState, RLSForecast, and RLSPosterior DTOs.

Input: A slice of float64 where the first N elements are the feature vector (X),
and the last element is the target (y).
Output: An array [prediction, predictive_variance].
*/
func NewRLS(dimensions types.Integer, lambda types.Float) RLS {
	dim := 1
	if dimensions != nil {
		if d := dimensions(nil); d > 0 {
			dim = d
		}
	}
	lam := 0.99
	if lambda != nil {
		if l := lambda(nil); l > 0 {
			lam = l
		}
	}
	beta := make([]float64, dim)
	root := make([][]float64, dim)
	for i := range root {
		root[i] = make([]float64, dim)
		root[i][i] = core.Unit // Identity matrix initialization
	}

	noiseShape := core.Unit
	noiseScale := core.Unit
	var observations float64

	return func(in []float64) [2]float64 {
		if len(in) != dim+1 {
			return [2]float64{math.NaN(), math.NaN()} // Invalid input shape
		}

		x := in[:dim]
		y := in[dim]

		// 1. Prediction (Projection through design)
		factor := make([]float64, dim)
		prediction := 0.0
		for row, feature := range x {
			prediction += beta[row] * feature
			for col, coeff := range root[row] {
				factor[col] += coeff * feature
			}
		}

		energy := 0.0
		for _, val := range factor {
			energy += val * val
		}

		variance := (noiseScale / noiseShape) * (observations + energy)

		// 2. Symmetric Square-Root Rank-One Update
		innovation := y - prediction
		alpha := lam + energy

		if alpha <= 0 {
			return [2]float64{math.NaN(), math.NaN()}
		}

		rootLambda := math.Sqrt(lam)
		denominator := alpha + rootLambda*math.Sqrt(alpha)
		gain := make([]float64, dim)

		for row := range root {
			for col, coeff := range root[row] {
				gain[row] += coeff * factor[col]
			}
			gain[row] /= alpha
			beta[row] += gain[row] * innovation

			for col, coeff := range root[row] {
				root[row][col] = (coeff - gain[row]*(alpha/denominator)*factor[col]) / rootLambda
			}
		}

		observations++
		noiseScale = lam*noiseScale + 0.5*innovation*innovation/alpha
		noiseShape = lam*noiseShape + 0.5

		return [2]float64{prediction, variance}
	}
}


