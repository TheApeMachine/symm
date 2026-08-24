package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
LogRatio computes log(current/previous) for positive finite observations.
*/
func LogRatio(input types.Frame) types.Frame {
	current, hasCurrent := input.Get(SymbolCurrent)
	previous, hasPrevious := input.Get(SymbolPrevious)

	if !hasCurrent || !hasPrevious || !utils.IsFinite(current) ||
		!utils.IsFinite(previous) {
		input.Err = fmt.Errorf("calculus: log ratio requires finite current and previous")

		return input
	}

	if current <= 0 || previous <= 0 {
		input.Err = fmt.Errorf("calculus: log ratio requires positive operands")

		return input
	}

	result := math.Log(current / previous)

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: log ratio produced a non-finite result")

		return input
	}

	input.Put(PortResult, result)

	return input
}
