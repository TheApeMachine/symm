package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
)

// Min returns the minimum value in a slice of Numbers.
var Min types.Reduction = func(values []types.Number) types.Number {
	if len(values) == 0 {
		return 0
	}

	minimum := values[0]

	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}

	return minimum
}
