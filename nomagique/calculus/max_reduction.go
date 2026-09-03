package calculus

import (
	"github.com/theapemachine/symm/nomagique/types"
)

// Max returns the maximum value in a slice of Numbers.
var Max types.Reduction = func(values []types.Number) types.Number {
	if len(values) == 0 {
		return 0
	}

	maximum := values[0]

	for _, value := range values[1:] {
		if value > maximum {
			maximum = value
		}
	}

	return maximum
}
