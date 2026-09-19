package learning

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/types"
)

type rlsPrimitive struct {
	predict types.Value[algo.RLSState, algo.RLSForecast]
	update  types.Value[algo.RLSObservation, algo.RLSPosterior]
	state   algo.RLSState
	err     error
}

func newRLSPrimitive(dim int, lambda float64) *rlsPrimitive {
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
	return &rlsPrimitive{
		predict: types.Value[algo.RLSState, algo.RLSForecast](algo.NewRLSPrediction()),
		update:  types.Value[algo.RLSObservation, algo.RLSPosterior](algo.NewRLSUpdate()),
		state:   state,
	}
}

func (r *rlsPrimitive) Next(seq iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for v := range seq {
			if v == nil {
				continue
			}

			samplePtr := (*Sample)(v)
			if samplePtr == nil {
				r.err = fmt.Errorf("expected Sample")
				break
			}

			// Prepare state design
			if len(r.state.Design) == len(samplePtr.Features)+1 {
				copy(r.state.Design, samplePtr.Features)
				r.state.Design[len(samplePtr.Features)] = 1.0 // Bias
			} else {
				copy(r.state.Design, samplePtr.Features)
			}

			forecast := r.predict(r.state)

			if !samplePtr.Observed {
				posterior := algo.RLSPosterior{RLSForecast: forecast}
				if !yield(unsafe.Pointer(&posterior)) {
					return
				}
				continue
			}

			obs := algo.RLSObservation{
				RLSForecast: forecast,
				Lambda:      0.99, // default lambda for now
				Target:      samplePtr.Target,
			}

			posterior := r.update(obs)
			r.state = posterior.RLSForecast.RLSState
			if !yield(unsafe.Pointer(&posterior)) {
				return
			}
		}
	}
}

func (r *rlsPrimitive) Error(errs ...error) error {
	if len(errs) > 0 {
		r.err = errs[0]
	}
	return r.err
}
