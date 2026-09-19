package learning

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
TaskLearner is a Value closure that forecasts or updates a task-head RLS model.
No structs, pure Value closure.
*/
type TaskLearner types.Value[Sample, algo.RLSPosterior]

func newRLSTaskLearner(dim int, lambda float64) TaskLearner {
	state := algo.RLSState{
		Beta:         make([]float64, dim),
		Design:       make([]float64, dim),
		Root:         make([][]float64, dim),
		NoiseShape:   0.001,
		NoiseScale:   0.001,
		Observations: 0,
	}
	for i := range state.Root {
		state.Root[i] = make([]float64, dim)
		state.Root[i][i] = 100.0 // Identity scaled by ridge
	}

	predict := algo.NewRLSPrediction()
	update := algo.NewRLSUpdate()

	return func(sample Sample) algo.RLSPosterior {
		// Prepare state design
		if len(state.Design) == len(sample.Features)+1 {
			copy(state.Design, sample.Features)
			state.Design[len(sample.Features)] = 1.0 // Bias
		}
		if len(state.Design) != len(sample.Features)+1 {
			copy(state.Design, sample.Features)
		}

		forecast := predict(state)

		if !sample.Observed {
			return algo.RLSPosterior{RLSForecast: forecast}
		}

		obs := algo.RLSObservation{
			RLSForecast: forecast,
			Lambda:      lambda,
			Target:      sample.Target,
		}

		posterior := update(obs)
		state = posterior.RLSForecast.RLSState
		return posterior
	}
}
