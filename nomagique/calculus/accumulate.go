package calculus

import (
	"fmt"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Accumulate adds the explicit input delta to the total retained in state.
*/
func Accumulate(input *types.Frame) {
	delta, found := input.Get(SymbolDelta)

	if !found || !utils.IsFinite(delta) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: accumulate requires a finite delta",
			nil,
		))

		return
	}

	total, hasTotal := input.Get(SymbolTotal)

	if hasTotal && !utils.IsFinite(total) {
		input.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"calculus: accumulate state must be finite",
			nil,
		))

		return
	}

	result := total + delta

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: accumulate overflowed")

		return
	}

	input.Put(SymbolTotal, result)
	input.Put(PortResult, result)
}
