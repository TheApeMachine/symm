package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RLSState is the posterior and the query design used to forecast.
*/
type RLSState struct {
	Beta         []float64
	Design       []float64
	Root         [][]float64
	NoiseShape   float64
	NoiseScale   float64
	Observations float64
}

/*
RLSForecast is the projection of a posterior through a design.
*/
type RLSForecast struct {
	RLSState
	Prediction         float64
	Factor             []float64
	Scale              float64
	DegreesOfFreedom   float64
	PredictiveVariance float64
	Ready              bool
}

/*
RLSObservation is a forecast plus the target and forgetting that close the update.
*/
type RLSObservation struct {
	RLSForecast
	Lambda float64
	Target float64
}

/*
RLSPosterior is the symmetric square-root rank-one update.
*/
type RLSPosterior struct {
	RLSForecast
	Alpha            float64
	Innovation       float64
	RootLambda       float64
	GammaDenominator float64
	Gain             []float64
}

/*
RLSPrediction forecasts from the supplied posterior before any model update.
*/
type RLSPrediction struct {
	err error
	out RLSForecast
}

func NewRLSPrediction() core.Primitive {
	return &RLSPrediction{}
}

func (op *RLSPrediction) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			state := (*RLSState)(arriving)

			if len(state.Beta) != len(state.Design) || len(state.Root) != len(state.Design) {
				op.err = fmt.Errorf("%w: RLS prediction coefficient, root and design dimensions differ", core.ErrShape)
				return
			}

			factor := make([]float64, len(state.Design))
			value := 0.0

			for row, feature := range state.Design {
				if len(state.Root[row]) != len(state.Design) {
					op.err = fmt.Errorf("%w: RLS root must be square", core.ErrShape)
					return
				}

				value += state.Beta[row] * feature

				for column, coefficient := range state.Root[row] {
					factor[column] += coefficient * feature
				}
			}

			forecast := RLSForecast{
				RLSState:   *state,
				Prediction: value,
				Factor:     factor,
			}

			if state.NoiseShape > 0 && state.NoiseScale > 0 {
				energy := 0.0

				for _, member := range factor {
					energy += member * member
				}

				variance := (state.NoiseScale / state.NoiseShape) * (state.Observations + energy)

				if !(variance > 0) {
					op.err = fmt.Errorf("%w: RLS predictive variance %g", core.ErrDomain, variance)
					return
				}

				forecast.PredictiveVariance = variance
				forecast.Scale = math.Sqrt(variance)
				forecast.DegreesOfFreedom = 2 * state.NoiseShape
				forecast.Ready = true
			}

			op.out = forecast

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *RLSPrediction) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}

/*
RLSUpdate owns symmetric square-root rank-one update delivery.
*/
type RLSUpdate struct {
	err error
	out RLSPosterior
}

func NewRLSUpdate() core.Primitive {
	return &RLSUpdate{}
}

func (op *RLSUpdate) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			obs := (*RLSObservation)(arriving)
			beta := obs.Beta
			root := obs.Root
			factor := obs.Factor
			lambda := obs.Lambda
			innovation := obs.Target - obs.Prediction

			if len(root) != len(beta) || len(factor) != len(beta) {
				op.err = fmt.Errorf("%w: RLS update dimensions differ", core.ErrShape)
				return
			}

			energy := 0.0

			for _, value := range factor {
				energy += value * value
			}

			alpha := lambda + energy

			if !(alpha > 0) {
				op.err = fmt.Errorf("%w: invalid RLS information", core.ErrDomain)
				return
			}

			rootLambda := math.Sqrt(lambda)
			denominator := alpha + rootLambda*math.Sqrt(alpha)
			gain := make([]float64, len(beta))
			coefficients := make([]float64, len(beta))
			posterior := make([][]float64, len(root))
			storage := make([]float64, len(root)*len(root))

			for row := range root {
				if len(root[row]) != len(beta) {
					op.err = fmt.Errorf("%w: RLS root must be square", core.ErrShape)
					return
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

			noise := lambda*obs.NoiseScale + 0.5*innovation*innovation/alpha
			result := obs.RLSForecast
			result.Beta = coefficients
			result.Root = posterior
			result.NoiseShape = lambda*obs.NoiseShape + 0.5
			result.NoiseScale = noise

			op.out = RLSPosterior{
				RLSForecast:      result,
				Alpha:            alpha,
				Innovation:       innovation,
				RootLambda:       rootLambda,
				GammaDenominator: denominator,
				Gain:             gain,
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *RLSUpdate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
