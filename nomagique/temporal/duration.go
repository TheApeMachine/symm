package temporal

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/utils"
)

var (
	SymbolCurrentSec   = nomagique.MustIntern("current_sec")
	SymbolCurrentNsec  = nomagique.MustIntern("current_nsec")
	SymbolPreviousSec  = nomagique.MustIntern("previous_sec")
	SymbolPreviousNsec = nomagique.MustIntern("previous_nsec")
	SymbolDelta        = nomagique.MustIntern("delta")
)

/*
Duration computes elapsed seconds from separate second and nanosecond coordinates.
*/
func Duration(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	currentSec, hasCurrentSec := input.Get(SymbolCurrentSec)
	currentNsec, hasCurrentNsec := input.Get(SymbolCurrentNsec)
	previousSec, hasPreviousSec := input.Get(SymbolPreviousSec)
	previousNsec, hasPreviousNsec := input.Get(SymbolPreviousNsec)

	if !hasCurrentSec || !hasCurrentNsec || !hasPreviousSec || !hasPreviousNsec {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: duration requires current and previous timestamp coordinates",
		)
	}

	if !utils.IsFinite(currentSec, currentNsec, previousSec, previousNsec) ||
		currentNsec < 0 || currentNsec >= 1e9 || previousNsec < 0 || previousNsec >= 1e9 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"temporal: duration coordinates must be finite and normalized",
		)
	}

	delta := currentSec - previousSec + (currentNsec-previousNsec)*1e-9
	output := input
	output.Put(SymbolDelta, delta)

	return state, output, nil
}
