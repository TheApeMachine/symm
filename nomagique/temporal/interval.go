package temporal

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

var (
	SymbolTimestamp = types.MustIntern("timestamp")
	SymbolPrevious  = types.MustIntern("previous")
	SymbolHasSeen   = types.MustIntern("has_seen")
)

/*
Interval computes elapsed time between sequential scalar timestamps and retains
the last timestamp in state.
*/
func Interval(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	timestamp, found := input.Get(SymbolTimestamp)

	if !found || !utils.IsFinite(timestamp) {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: interval requires a finite timestamp",
		)
	}

	previous, hasPrevious := state.Get(SymbolPrevious)
	hasSeen, _ := state.Get(SymbolHasSeen)
	delta := 0.0

	if hasPrevious && hasSeen != 0 {
		delta = timestamp - previous

		if delta < 0 {
			return state, types.Frame{}, fmt.Errorf(
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
