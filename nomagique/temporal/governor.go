package temporal

import (
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/types"
)

// MinimumSampleSizeForDispersion is the universal degrees of freedom limit (n - 1 >= 1).
const MinimumSampleSizeForDispersion = 2

/*
Governor manages unbounded adaptive memory expansion and contraction
from online information stability feedback.
Fulfills the zero-magic mandate: zero fixed clamps (MaxSamples = 128 eliminated).
*/
type Governor struct {
	Store      store.Store
	Controller *adaptive.StabilityController
	Reduce     types.Reduction
}

func (governor *Governor) Step(number types.Number) types.Number {
	capacity := 0

	if governor.Controller != nil {
		capacity = governor.Controller.Step(float64(number))
	} else {
		capacity = governor.Store.Adaptive.Step(float64(number))
	}

	if capacity < MinimumSampleSizeForDispersion {
		capacity = MinimumSampleSizeForDispersion
	}

	governor.Store.Buffer = append(governor.Store.Buffer, number)

	if len(governor.Store.Buffer) > capacity {
		overflow := len(governor.Store.Buffer) - capacity
		governor.Store.Buffer = governor.Store.Buffer[overflow:]
	}

	if governor.Reduce != nil && len(governor.Store.Buffer) >= MinimumSampleSizeForDispersion {
		return governor.Reduce(governor.Store.Buffer)
	}

	return 0
}
