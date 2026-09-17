package strategy

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/statistic"
)

type GridObservationAdapter struct {
	*core.PrimitiveError
	priors map[*geometry.Coordinate]float64
	out    statistic.Observation[*geometry.Coordinate]
}

func NewGridObservationAdapter() *GridObservationAdapter {
	return &GridObservationAdapter{
		PrimitiveError: core.NewPrimitiveError(),
		priors:         make(map[*geometry.Coordinate]float64),
	}
}

func (adapter *GridObservationAdapter) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			input := (*core.Input[*geometry.Coordinate, string, float64])(arriving)
			if input == nil || input.Origin == nil || input.Value == nil {
				continue
			}

			coord := input.Origin.Identity()
			val := *input.Value
			if math.IsNaN(val) || math.IsInf(val, 0) {
				continue
			}

			prior, hasPrior := adapter.priors[coord]
			adapter.priors[coord] = val

			movement := 0.0
			if hasPrior && prior != 0 {
				movement = (val - prior) / math.Abs(prior)
			} else if hasPrior {
				movement = val - prior
			}

			// Pack into the format statistic.Sympathy expects
			adapter.out = statistic.Observation[*geometry.Coordinate]{
				Address:   coord,
				Movement:  movement,
				Authority: 1.0, // Can be calibrated by SNR if available
				Weight:    1.0,
			}

			if !yield(unsafe.Pointer(&adapter.out)) {
				return
			}
		}
	}
}
