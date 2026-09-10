package rls

import (
	"fmt"
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
State is the posterior and the query design used to forecast.
*/
type State struct {
	Beta         []float64
	Design       []float64
	Root         [][]float64
	NoiseShape   float64
	NoiseScale   float64
	Observations float64
}

/*
Forecast is the projection of a posterior through a design.
*/
type Forecast struct {
	State
	Prediction         float64
	Factor             []float64
	Scale              float64
	DegreesOfFreedom   float64
	PredictiveVariance float64
	Ready              bool
}

/*
Prediction forecasts from the supplied posterior before any model update.
*/
type Prediction struct {
	core.Base[State, Forecast]
}

func NewPrediction() *Prediction {
	return &Prediction{}
}

func (op *Prediction) Next(
	in iter.Seq[core.Primitive[State, State]],
) iter.Seq[core.Primitive[Forecast, Forecast]] {
	return func(yield func(core.Primitive[Forecast, Forecast]) bool) {
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

/*
Project multiplies the root's transpose implicitly, retaining only its factor.
*/
func (op *Prediction) Project(state State) (Forecast, error) {
	if len(state.Beta) != len(state.Design) || len(state.Root) != len(state.Design) {
		return Forecast{}, fmt.Errorf("%w: RLS prediction coefficient, root and design dimensions differ", core.ErrShape)
	}

	factor := make([]float64, len(state.Design))
	value := 0.0

	for row, feature := range state.Design {
		if len(state.Root[row]) != len(state.Design) {
			return Forecast{}, fmt.Errorf("%w: RLS root must be square", core.ErrShape)
		}

		value += state.Beta[row] * feature

		for column, coefficient := range state.Root[row] {
			factor[column] += coefficient * feature
		}
	}

	forecast := Forecast{
		State:      state,
		Prediction: value,
		Factor:     factor,
	}

	if !(state.NoiseShape > 0 && state.NoiseScale > 0) {
		return forecast, nil
	}

	energy := 0.0

	for _, member := range factor {
		energy += member * member
	}

	variance := (state.NoiseScale / state.NoiseShape) * (state.Observations + energy)

	if !(variance > 0) {
		return Forecast{}, fmt.Errorf("%w: RLS predictive variance %g", core.ErrDomain, variance)
	}

	forecast.PredictiveVariance = variance
	forecast.Scale = math.Sqrt(variance)
	forecast.DegreesOfFreedom = 2 * state.NoiseShape
	forecast.Ready = true
	return forecast, nil
}
