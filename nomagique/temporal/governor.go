package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	symbolStability         = nmtypes.MustIntern("stability")
	SymbolPreviousStability = nmtypes.MustIntern("governor/previous_stability")
)

/*
Governor controls a Window's next capacity from universal stability feedback.
It expands after a stability decline, contracts to used evidence after perfect
stability, and otherwise holds the current capacity.
*/
func Governor(input *types.Frame) {
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

	if count < minimumGovernorSamples {
		target = minimumGovernorSamples
	}

	if count >= minimumGovernorSamples && hasPrevious && stability < previous {
		target = math.Min(nmtypes.MaxSamples, capacity+capacity)
	}

	if count >= minimumGovernorSamples && stability >= 1 {
		target = math.Max(minimumGovernorSamples, count)
	}

	input.Put(SymbolPreviousStability, stability)
	input.Put(SymbolCapacity, target)
	input.Put(nmtypes.Span, target)
}

const minimumGovernorSamples = 2
