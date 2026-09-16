package algo

import (
	"fmt"
	"iter"
	"math"
	"slices"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Query is one design, optional target, and the forgetting used if the target is
observed.
*/
type Query struct {
	Design   []float64
	Target   float64
	Observed bool
	Lambda   float64
}

/*
Reading is the prior forecast, and the committed posterior when a target was
observed.
*/
type Reading struct {
	RLSForecast
	Innovation float64
	Observed   bool
}

/*
SquareRootRLS owns the four posterior fields and predicts before it trains.
*/
type SquareRootRLS struct {
	*core.PrimitiveError

	state    RLSState
	ready    bool
	variance float64
	out      Reading
}

func NewSquareRootRLS(variance float64) *SquareRootRLS {
	return &SquareRootRLS{PrimitiveError: core.NewPrimitiveError(), variance: variance}
}

func (squareRootRLS *SquareRootRLS) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			query := (*Query)(arriving)
			reading, err := squareRootRLS.step(*query)

			if err != nil {
				squareRootRLS.Error(err)
				return
			}

			squareRootRLS.out = reading

			if !yield(unsafe.Pointer(&squareRootRLS.out)) {
				return
			}
		}
	}
}

/*
step prepares one prior forecast and commits a validated posterior when labeled.
*/
func (squareRootRLS *SquareRootRLS) step(query Query) (Reading, error) {
	if len(query.Design) == 0 {
		return Reading{}, fmt.Errorf("%w: RLS design is empty", core.ErrShape)
	}

	if !(query.Lambda > 0 && query.Lambda <= 1) {
		return Reading{}, fmt.Errorf("%w: RLS forgetting factor must be in (0,1]", core.ErrDomain)
	}

	if !squareRootRLS.ready {
		state, err := squareRootRLS.prior(query.Design)

		if err != nil {
			return Reading{}, err
		}

		squareRootRLS.state = state
		squareRootRLS.ready = true
	}

	if len(query.Design) != len(squareRootRLS.state.Beta) {
		return Reading{}, fmt.Errorf("%w: RLS design dimension differs from the posterior", core.ErrShape)
	}

	squareRootRLS.state.Design = slices.Clone(query.Design)
	squareRootRLS.state.Observations = 1

	factor := make([]float64, len(squareRootRLS.state.Design))
	value := 0.0

	for row, feature := range squareRootRLS.state.Design {
		if len(squareRootRLS.state.Root[row]) != len(squareRootRLS.state.Design) {
			return Reading{}, fmt.Errorf("%w: RLS root must be square", core.ErrShape)
		}

		value += squareRootRLS.state.Beta[row] * feature

		for column, coefficient := range squareRootRLS.state.Root[row] {
			factor[column] += coefficient * feature
		}
	}

	forecast := RLSForecast{
		RLSState:   squareRootRLS.state,
		Prediction: value,
		Factor:     factor,
	}

	if squareRootRLS.state.NoiseShape > 0 && squareRootRLS.state.NoiseScale > 0 {
		energy := 0.0

		for _, member := range factor {
			energy += member * member
		}

		variance := (squareRootRLS.state.NoiseScale / squareRootRLS.state.NoiseShape) * (squareRootRLS.state.Observations + energy)

		if !(variance > 0) {
			return Reading{}, fmt.Errorf("%w: RLS predictive variance %g", core.ErrDomain, variance)
		}

		forecast.PredictiveVariance = variance
		forecast.Scale = math.Sqrt(variance)
		forecast.DegreesOfFreedom = 2 * squareRootRLS.state.NoiseShape
		forecast.Ready = true
	}

	reading := Reading{RLSForecast: forecast}

	if !query.Observed {
		return reading, nil
	}

	beta := forecast.Beta
	root := forecast.Root
	lambda := query.Lambda
	innovation := query.Target - forecast.Prediction

	energy := 0.0

	for _, v := range factor {
		energy += v * v
	}

	alpha := lambda + energy

	if !(alpha > 0) {
		return Reading{}, fmt.Errorf("%w: invalid RLS information", core.ErrDomain)
	}

	rootLambda := math.Sqrt(lambda)
	denominator := alpha + rootLambda*math.Sqrt(alpha)
	gain := make([]float64, len(beta))
	coefficients := make([]float64, len(beta))
	posterior := make([][]float64, len(root))
	storage := make([]float64, len(root)*len(root))

	for row := range root {
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

	noise := lambda*forecast.NoiseScale + 0.5*innovation*innovation/alpha
	squareRootRLS.state.Beta = coefficients
	squareRootRLS.state.Root = posterior
	squareRootRLS.state.NoiseShape = lambda*forecast.NoiseShape + 0.5
	squareRootRLS.state.NoiseScale = noise

	forecast.Beta = coefficients
	forecast.Root = posterior
	forecast.NoiseShape = squareRootRLS.state.NoiseShape
	forecast.NoiseScale = noise

	reading.RLSForecast = forecast
	reading.Innovation = innovation
	reading.Observed = true
	return reading, nil
}

func (squareRootRLS *SquareRootRLS) prior(design []float64) (RLSState, error) {
	if !(squareRootRLS.variance > 0) {
		return RLSState{}, fmt.Errorf("%w: RLS prior variance must be positive", core.ErrDomain)
	}

	size := len(design)
	root := make([][]float64, size)
	storage := make([]float64, size*size)

	for index := range root {
		root[index] = storage[index*size : (index+1)*size]
		root[index][index] = math.Sqrt(squareRootRLS.variance)
	}

	return RLSState{
		Beta:         make([]float64, size),
		Root:         root,
		Design:       slices.Clone(design),
		Observations: 1,
	}, nil
}
