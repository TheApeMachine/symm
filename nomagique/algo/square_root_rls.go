package algo

import (
	"fmt"
	"iter"
	"math"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation/rls"
)

/*
Query is one design, optional target, and the forgetting used if the target is
observed. A design-only query never trains.
*/
type Query struct {
	Design   []float64
	Target   float64
	Observed bool
	Lambda   float64
}

/*
Reading is the prior forecast, and the committed posterior when a target was
observed. Invalid input does not reset or partially train the model.
*/
type Reading struct {
	rls.Forecast
	Innovation float64
	Observed   bool
}

/*
SquareRootRLS owns the four posterior fields and predicts before it trains.
*/
type SquareRootRLS struct {
	core.Base[Query, Reading]
	prediction *rls.Prediction
	update     *rls.Update
	state      rls.State
	ready      bool
	variance   float64
}

func NewSquareRootRLS(variance float64) *SquareRootRLS {
	return &SquareRootRLS{
		prediction: rls.NewPrediction(),
		update:     rls.NewUpdate(),
		variance:   variance,
	}
}

func (op *SquareRootRLS) Next(
	in iter.Seq[core.Primitive[Query, Query]],
) iter.Seq[core.Primitive[Reading, Reading]] {
	return func(yield func(core.Primitive[Reading, Reading]) bool) {
		for arriving := range in {
			reading, err := op.Step(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

/*
Step prepares one prior forecast and commits a validated posterior when labeled.
*/
func (op *SquareRootRLS) Step(query Query) (Reading, error) {
	if len(query.Design) == 0 {
		return Reading{}, fmt.Errorf("%w: RLS design is empty", core.ErrShape)
	}

	if !(query.Lambda > 0 && query.Lambda <= 1) {
		return Reading{}, fmt.Errorf("%w: RLS forgetting factor must be in (0,1]", core.ErrDomain)
	}

	if !op.ready {
		state, err := op.prior(query.Design)

		if err != nil {
			return Reading{}, err
		}

		op.state = state
		op.ready = true
	}

	if len(query.Design) != len(op.state.Beta) {
		return Reading{}, fmt.Errorf("%w: RLS design dimension differs from the posterior", core.ErrShape)
	}

	op.state.Design = slices.Clone(query.Design)
	op.state.Observations = 1
	forecast, err := op.prediction.Project(op.state)

	if err != nil {
		return Reading{}, err
	}

	reading := Reading{Forecast: forecast}

	if !query.Observed {
		return reading, nil
	}

	posterior, err := op.update.Apply(rls.Observation{
		Forecast: forecast,
		Lambda:   query.Lambda,
		Target:   query.Target,
	})

	if err != nil {
		return Reading{}, err
	}

	op.state.Beta = posterior.Beta
	op.state.Root = posterior.Root
	op.state.NoiseShape = posterior.NoiseShape
	op.state.NoiseScale = posterior.NoiseScale
	reading.Forecast = posterior.Forecast
	reading.Innovation = posterior.Innovation
	reading.Observed = true
	return reading, nil
}

func (op *SquareRootRLS) prior(design []float64) (rls.State, error) {
	if !(op.variance > 0) {
		return rls.State{}, fmt.Errorf("%w: RLS prior variance must be positive", core.ErrDomain)
	}

	size := len(design)
	root := make([][]float64, size)
	storage := make([]float64, size*size)

	for index := range root {
		root[index] = storage[index*size : (index+1)*size]
		root[index][index] = math.Sqrt(op.variance)
	}

	return rls.State{
		Beta:         make([]float64, size),
		Root:         root,
		Design:       slices.Clone(design),
		Observations: 1,
	}, nil
}
