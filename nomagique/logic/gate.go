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
func Gate(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
	condition, hasCondition := input.Get(SymbolCondition)
	value, hasValue := input.Get(SymbolValue)

	if !hasCondition || !hasValue || !utils.IsFinite(condition) || !utils.IsFinite(value) {
		return state, types.Frame{}, fmt.Errorf("logic: gate requires finite condition and value")
	}

	result := 0.0

	if condition != 0 {
		result = value
	}

	output := input
	output.Put(SymbolResult, result)

	return state, output, nil
}
