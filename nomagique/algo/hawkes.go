package algo

import (
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Hawkes is a composite Primitive assembled from the shared atomic units:

	intensity transition (HawkesIntensity)
	-> branching matrix and spectral radius (Branching)
	-> log-likelihood differentials (Likelihood)
*/
func Hawkes() types.Primitive {
	return types.Pipe(
		statistic.HawkesIntensity,
		statistic.Branching,
		statistic.Likelihood,
	)
}
