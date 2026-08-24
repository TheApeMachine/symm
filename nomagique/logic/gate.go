package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

var (
	SymbolCondition = types.MustIntern("condition")
	SymbolValue     = types.MustIntern("value")
	SymbolResult    = types.MustIntern("result")
)

// Gate emits value when condition is non-zero and zero otherwise.
func Gate(input types.Frame) types.Frame {
	condition, hasCondition := input.Get(SymbolCondition)
	value, hasValue := input.Get(SymbolValue)

	if !hasCondition || !hasValue || !utils.IsFinite(condition) || !utils.IsFinite(value) {
		input.Err = fmt.Errorf("logic: gate requires finite condition and value")

		return input
	}

	result := 0.0

	if condition != 0 {
		result = value
	}

	input.Put(SymbolResult, result)

	return input
}
