package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Mux selects A when condition is non-zero, otherwise B.
*/
func Mux(input types.Frame) types.Frame {
	condition, hasCondition := input.Get(SymbolCondition)
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasCondition || !hasA || !hasB || !utils.IsFinite(condition) ||
		!utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("logic: mux requires finite condition, a, and b")

		return input
	}

	result := b

	if condition != 0 {
		result = a
	}

	input.Put(SymbolResult, result)

	return input
}
