package dist

import (
	"fmt"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/statutil"
)

/*
Share names one category's evidence mass before normalisation.
*/
type Share struct {
	Key      string
	Category logic.CategoryType
	Mass     float64
}

/*
Write normalises shares to a distribution on the measurement artifact, stamps
each category by wire key and global index, and returns confidence as the peak
mass (how concentrated the distribution is).
*/
func Write(measurement *datura.Artifact, shares []Share) float64 {
	masses := make([]float64, len(shares))

	for index := range shares {
		masses[index] = shares[index].Mass
	}

	statutil.NormalizeMasses(masses)

	for index := range shares {
		mass := masses[index]
		measurement.MergeOutput(shares[index].Key, mass)
		measurement.MergeOutput(
			fmt.Sprintf("category.%d", logic.CategoryIndex(shares[index].Category)),
			mass,
		)
	}

	confidence := statutil.MaxMass(masses)
	measurement.MergeOutput("confidence", confidence)
	measurement.MergeOutput("strength", confidence)

	return confidence
}
