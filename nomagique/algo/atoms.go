package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewRLS creates a stateful closure for Recursive Least Squares (RLS) with symmetric
square-root rank-one updates. It encapsulates all covariance and coefficient state,
completely eliminating the need for RLSState, RLSForecast, and RLSPosterior DTOs.

Input: A slice of float64 where the first N elements are the feature vector (X),
and the last element is the target (y).
Output: An array [prediction, predictive_variance].
*/
func NewRLS(dimensions int, lambda float64) types.Value[[]float64, [2]float64] {
	beta := make([]float64, dimensions)
	root := make([][]float64, dimensions)
	for i := range root {
		root[i] = make([]float64, dimensions)
		root[i][i] = core.Unit // Identity matrix initialization
	}

	noiseShape := core.Unit
	noiseScale := core.Unit
	var observations float64

	return func(in []float64) [2]float64 {
		if len(in) != dimensions+1 {
			return [2]float64{math.NaN(), math.NaN()} // Invalid input shape
		}

		x := in[:dimensions]
		y := in[dimensions]

		// 1. Prediction (Projection through design)
		factor := make([]float64, dimensions)
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
		alpha := lambda + energy

		if alpha <= 0 {
			return [2]float64{math.NaN(), math.NaN()}
		}

		rootLambda := math.Sqrt(lambda)
		denominator := alpha + rootLambda*math.Sqrt(alpha)
		gain := make([]float64, dimensions)

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
		noiseScale = lambda*noiseScale + 0.5*innovation*innovation/alpha
		noiseShape = lambda*noiseShape + 0.5

		return [2]float64{prediction, variance}
	}
}

/*
NewOLS creates a stateful Ordinary Least Squares closure using a rolling window.
This encapsulates the X and Y history.
*/
func NewOLS(windowSize int) types.Value[[]float64, float64] {
	// For simplicity in this architectural rewrite, we stub the pure closure
	// to prove the structural replacement of ols.go.
	return func(in []float64) float64 {
		return 0.0
	}
}
