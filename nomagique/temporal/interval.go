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
func Interval(input *types.Frame) {
	timestamp, found := input.Get(SymbolTimestamp)

	if !found || !utils.IsFinite(timestamp) {
		input.Err = fmt.Errorf(
			"temporal: interval requires a finite timestamp",
		)

		return
	}

	previous, hasPrevious := input.Get(SymbolPrevious)
	hasSeen, _ := input.Get(SymbolHasSeen)
	delta := 0.0

	if hasPrevious && hasSeen != 0 {
		delta = timestamp - previous

		if delta < 0 {
			input.Err = fmt.Errorf(
				"temporal: interval timestamp cannot move backwards",
			)

			return
		}
	}

	input.Put(SymbolPrevious, timestamp)
	input.Put(SymbolHasSeen, 1)
	input.Put(SymbolDelta, delta)
}
