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
	trueValue, hasA := input.Get(calculus.PortA)
	falseValue, hasB := input.Get(calculus.PortB)

	if !hasCondition || !hasA || !hasB || !utils.IsFinite(condition) ||
		!utils.IsFinite(trueValue) || !utils.IsFinite(falseValue) {
		input.Err = fmt.Errorf("logic: mux requires finite condition, true value, and false value")

		return input
	}

	result := falseValue

	if condition != 0 {
		result = trueValue
	}

	input.Put(SymbolResult, result)

	return input
}
