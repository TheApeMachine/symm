package calculus

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

func number(
	frame *types.Frame,
	symbol types.Symbol,
	primitive string,
) (float64, error) {
	value, found := frame.Get(symbol)

	if !found {
		name, named := types.SymbolName(symbol)

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

func resultFrame(input types.Frame, value float64) types.Frame {
	output := input
	output.Put(SymbolResult, value)

	return output
}
