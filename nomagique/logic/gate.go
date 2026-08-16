package logic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

var (
	SymbolCondition = nomagique.MustIntern("condition")
	SymbolValue     = nomagique.MustIntern("value")
	SymbolResult    = nomagique.MustIntern("result")
)

/*
Gate passes value to result when numeric condition is non-zero and otherwise
writes zero.
*/
func Gate(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	condition, hasCondition := input.Get(SymbolCondition)
	value, hasValue := input.Get(SymbolValue)

	if !hasCondition || !hasValue {
		return state, nomagique.Frame{}, fmt.Errorf(
			"logic: gate requires condition and value",
		)
	}

	if math.IsNaN(condition) || math.IsInf(condition, 0) ||
		math.IsNaN(value) || math.IsInf(value, 0) {
		return state, nomagique.Frame{}, fmt.Errorf(
			"logic: gate condition and value must be finite",
		)
	}

	result := 0.0

	if condition != 0 {
		result = value
	}

	output := input
	output.Put(SymbolResult, result)

	return state, output, nil
}
