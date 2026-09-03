package statistic

import (
	"github.com/theapemachine/symm/nomagique/types"
)

// Average calculates the arithmetic mean of a slice of Numbers.
var Average types.Reduction = func(values []types.Number) types.Number {
	count := len(values)

	if count == 0 {
		return 0
	}

	var sum types.Number

	for _, value := range values {
		sum += value
	}

	return sum / types.Number(count)
}
