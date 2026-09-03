package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
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
		excess := len(governor.Store.Buffer) - capacity
		governor.Store.Buffer = governor.Store.Buffer[excess:]
	}

	if governor.Reduce != nil {
		return governor.Reduce(governor.Store.Buffer)
	}

	return 0
}

var (
	symbolStability         = nmtypes.MustIntern("stability")
	SymbolPreviousStability = nmtypes.MustIntern("governor/previous_stability")
)

/*
GovernorPrimitive controls a Window's next capacity from universal stability feedback in Frame pipelines.
*/
func GovernorPrimitive(input *types.Frame) {
	stability, hasStability := input.Get(symbolStability)
	count, hasCount := input.Get(nmtypes.SampleCount)

	if !hasStability || !hasCount || count < 0 || count != math.Trunc(count) {
		input.Err = fmt.Errorf(
			"temporal: governor requires stability and an integer sample count",
		)

		return
	}

	capacity, found := input.Get(SymbolCapacity)

	if !found || capacity < count {
		capacity = count
	}

	target := capacity
	previous, hasPrevious := input.Get(SymbolPreviousStability)

	if count < MinimumSampleSizeForDispersion {
		target = MinimumSampleSizeForDispersion
	}

	if count >= MinimumSampleSizeForDispersion && hasPrevious && stability < previous {
		target = capacity + capacity
	}

	if count >= MinimumSampleSizeForDispersion && stability >= 1 {
		target = math.Max(MinimumSampleSizeForDispersion, count)
	}

	input.Put(SymbolPreviousStability, stability)
	input.Put(SymbolCapacity, target)
	input.Put(nmtypes.Span, target)
}
