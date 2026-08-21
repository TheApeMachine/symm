package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var (
	symbolStability         = nomagique.MustIntern("stability")
	SymbolPreviousStability = nomagique.MustIntern("governor/previous_stability")
)

/*
Governor controls a Window's next capacity from universal stability feedback.
It expands after a stability decline, contracts to used evidence after perfect
stability, and otherwise holds the current capacity.
*/
func Governor(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	stability, hasStability := input.Get(symbolStability)
	count, hasCount := input.Get(nomagique.SampleCount)

	if !hasStability || !hasCount || count < 0 || count != math.Trunc(count) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: governor requires stability and an integer sample count",
		)
	}

	capacity, found := state.Get(SymbolCapacity)

	if !found || capacity < count {
		capacity = count
	}

	target := capacity
	previous, hasPrevious := state.Get(SymbolPreviousStability)

	if count < minimumGovernorSamples {
		target = minimumGovernorSamples
	}

	if count >= minimumGovernorSamples && hasPrevious && stability < previous {
		target = math.Min(nomagique.MaxSamples, capacity+capacity)
	}

	if count >= minimumGovernorSamples && stability >= 1 {
		target = math.Max(minimumGovernorSamples, count)
	}

	nextState := state
	nextState.Put(SymbolPreviousStability, stability)
	nextState.Put(SymbolCapacity, target)
	nextState.Put(nmtypes.Span, target)

	output := input
	output.Put(nmtypes.Span, target)

	return nextState, output, nil
}

const minimumGovernorSamples = 2
