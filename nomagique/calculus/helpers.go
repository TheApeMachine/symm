package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
)

func number(
	frame *nomagique.Frame,
	symbol nomagique.Symbol,
	primitive string,
) (float64, error) {
	value, found := frame.Get(symbol)

	if !found {
		name, named := nomagique.SymbolName(symbol)

		if !named {
			name = fmt.Sprintf("symbol/%d", symbol)
		}

		return 0, fmt.Errorf("calculus: %s requires %s", primitive, name)
	}

	if !finite(value) {
		return 0, fmt.Errorf("calculus: %s requires finite operands", primitive)
	}

	return value, nil
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

func resultFrame(input nomagique.Frame, value float64) nomagique.Frame {
	output := input
	output.Put(SymbolResult, value)

	return output
}
