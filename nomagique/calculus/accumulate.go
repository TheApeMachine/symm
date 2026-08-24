package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Accumulate adds the explicit input delta to the total retained in state.
*/
func Accumulate(input types.Frame) types.Frame {
	delta, found := input.Get(SymbolDelta)

	if !found || !utils.IsFinite(delta) {
		input.Err = fmt.Errorf("calculus: accumulate requires a finite delta")

		return input
	}

	total, hasTotal := input.Get(SymbolTotal)

	if hasTotal && !utils.IsFinite(total) {
		input.Err = fmt.Errorf("calculus: accumulate state must be finite")

		return input
	}

	result := total + delta

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: accumulate overflowed")

		return input
	}

	input.Put(SymbolTotal, result)
	input.Put(PortResult, result)

	return input
}
