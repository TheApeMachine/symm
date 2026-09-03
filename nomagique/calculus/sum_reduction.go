package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
)

// SumReduction calculates the unweighted summation of all values in the slice.
var SumReduction types.Reduction = func(values []types.Number) types.Number {
	var sum types.Number

	for _, value := range values {
		sum += value
	}

	return sum
}
