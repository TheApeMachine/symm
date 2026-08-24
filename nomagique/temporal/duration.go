package temporal

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

var (
	SymbolCurrentSec   = types.MustIntern("current_sec")
	SymbolCurrentNsec  = types.MustIntern("current_nsec")
	SymbolPreviousSec  = types.MustIntern("previous_sec")
	SymbolPreviousNsec = types.MustIntern("previous_nsec")
	SymbolDelta        = types.MustIntern("delta")
)

/*
Duration computes elapsed seconds from separate second and nanosecond coordinates.
*/
func Duration(input types.Frame) types.Frame {
	currentSec, hasCurrentSec := input.Get(SymbolCurrentSec)
	currentNsec, hasCurrentNsec := input.Get(SymbolCurrentNsec)
	previousSec, hasPreviousSec := input.Get(SymbolPreviousSec)
	previousNsec, hasPreviousNsec := input.Get(SymbolPreviousNsec)

	if !hasCurrentSec || !hasCurrentNsec || !hasPreviousSec || !hasPreviousNsec {
		input.Err = fmt.Errorf(
			"temporal: duration requires current and previous timestamp coordinates",
		)

		return input
	}

	if !utils.IsFinite(currentSec, currentNsec, previousSec, previousNsec) ||
		currentNsec < 0 || currentNsec >= 1e9 || previousNsec < 0 || previousNsec >= 1e9 {
		input.Err = fmt.Errorf(
			"temporal: duration coordinates must be finite and normalized",
		)

		return input
	}

	delta := currentSec - previousSec + (currentNsec-previousNsec)*1e-9
	input.Put(SymbolDelta, delta)

	return input
}
