package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Xor emits one when exactly one finite operand is non-zero.
*/
func Xor(input *types.Frame) {
	a, hasA := input.Get(calculus.PortA)
	b, hasB := input.Get(calculus.PortB)

	if !hasA || !hasB || !utils.IsFinite(a) || !utils.IsFinite(b) {
		input.Err = fmt.Errorf("logic: xor requires finite a and b")

		return
	}

	condition := 0.0

	if (a != 0) != (b != 0) {
		condition = 1
	}

	input.Put(SymbolCondition, condition)
	input.Put(SymbolResult, condition)
}
