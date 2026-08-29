package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Exponential computes e raised to negative finite progress.
*/
func Exponential(input *types.Frame) {
	progress, found := input.Get(SymbolProgress)

	if !found || !utils.IsFinite(progress) {
		input.Err = fmt.Errorf("calculus: exponential requires finite progress")

		return
	}

	result := math.Exp(-progress)

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: exponential overflowed")

		return
	}

	input.Put(PortResult, result)
}
