package temporal

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolTimestamp = nomagique.MustIntern("timestamp")
	SymbolPrevious  = nomagique.MustIntern("previous")
	SymbolHasSeen   = nomagique.MustIntern("has_seen")
)

/*
Interval computes elapsed time between sequential scalar timestamps and retains
the last timestamp in state.
*/
func Interval(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	timestamp, found := input.Get(SymbolTimestamp)

	if !found || !finite(timestamp) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: interval requires a finite timestamp",
		)
	}

	previous, hasPrevious := state.Get(SymbolPrevious)
	hasSeen, _ := state.Get(SymbolHasSeen)
	delta := 0.0

	if hasPrevious && hasSeen != 0 {
		delta = timestamp - previous

		if delta < 0 {
			return state, nomagique.Frame{}, fmt.Errorf(
				"temporal: interval timestamp cannot move backwards",
			)
		}
	}

	nextState := state
	nextState.Put(SymbolPrevious, timestamp)
	nextState.Put(SymbolHasSeen, 1)
	output := input
	output.Put(SymbolPrevious, timestamp)
	output.Put(SymbolHasSeen, 1)
	output.Put(SymbolDelta, delta)

	return nextState, output, nil
}
