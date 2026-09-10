package rls

import (
	"fmt"
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Observation is a forecast plus the target and forgetting that close the update.
*/
type Observation struct {
	Forecast
	Lambda float64
	Target float64
}

/*
Posterior is the symmetric square-root rank-one update.
*/
type Posterior struct {
	Forecast
	Alpha            float64
	Innovation       float64
	RootLambda       float64
	GammaDenominator float64
	Gain             []float64
}

/*
Update owns delivery of that posterior.
*/
type Update struct {
	core.Base[Observation, Posterior]
}

func NewUpdate() *Update {
	return &Update{}
}

func (op *Update) Next(
	in iter.Seq[core.Primitive[Observation, Observation]],
) iter.Seq[core.Primitive[Posterior, Posterior]] {
	return func(yield func(core.Primitive[Posterior, Posterior]) bool) {
		for arriving := range in {
			posterior, err := op.Apply(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(posterior)) {
				return
			}
		}
	}
}

/*
Apply computes (root - gain*factor^T*alpha/gamma)/sqrt(lambda) directly.
*/
func (op *Update) Apply(observation Observation) (Posterior, error) {
	beta := observation.Beta
	root := observation.Root
	factor := observation.Factor
	lambda := observation.Lambda
	innovation := observation.Target - observation.Prediction

	if len(root) != len(beta) || len(factor) != len(beta) {
		return Posterior{}, fmt.Errorf("%w: RLS update dimensions differ", core.ErrShape)
	}

	energy := 0.0

	for _, value := range factor {
		energy += value * value
	}

	alpha := lambda + energy

	if !(alpha > 0) {
		return Posterior{}, fmt.Errorf("%w: invalid RLS information", core.ErrDomain)
	}

	rootLambda := math.Sqrt(lambda)
	denominator := alpha + rootLambda*math.Sqrt(alpha)
	gain := make([]float64, len(beta))
	coefficients := make([]float64, len(beta))
	posterior := make([][]float64, len(root))
	storage := make([]float64, len(root)*len(root))

	for row := range root {
		if len(root[row]) != len(beta) {
			return Posterior{}, fmt.Errorf("%w: RLS root must be square", core.ErrShape)
		}

		for column, coefficient := range root[row] {
			gain[row] += coefficient * factor[column]
		}

		gain[row] /= alpha
		coefficients[row] = beta[row] + gain[row]*innovation
		posterior[row] = storage[row*len(root) : (row+1)*len(root)]

		for column, coefficient := range root[row] {
			posterior[row][column] = (coefficient - gain[row]*(alpha/denominator)*factor[column]) / rootLambda
		}
	}

	noise := lambda*observation.NoiseScale + 0.5*innovation*innovation/alpha
	result := observation.Forecast
	result.Beta = coefficients
	result.Root = posterior
	result.NoiseShape = lambda*observation.NoiseShape + 0.5
	result.NoiseScale = noise

	return Posterior{
		Forecast:         result,
		Alpha:            alpha,
		Innovation:       innovation,
		RootLambda:       rootLambda,
		GammaDenominator: denominator,
		Gain:             gain,
	}, nil
}
