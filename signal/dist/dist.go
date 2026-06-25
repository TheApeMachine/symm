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
	if len(shares) == 0 {
		return 0
	}

	masses := make([]float64, len(shares))
	categoryMasses := map[logic.CategoryType]float64{}

	for index := range shares {
		masses[index] = shares[index].Mass
	}

	statutil.NormalizeMasses(masses)

	for index := range shares {
		mass := masses[index]
		measurement.MergeOutput(shares[index].Key, mass)
		categoryMasses[shares[index].Category] += mass
	}

	for category, mass := range categoryMasses {
		measurement.MergeOutput(fmt.Sprintf("category.%d", logic.CategoryIndex(category)), mass)
	}

	categoryValues := make([]float64, 0, len(categoryMasses))

	for _, mass := range categoryMasses {
		categoryValues = append(categoryValues, mass)
	}

	confidence := statutil.MaxMass(categoryValues)
	measurement.MergeOutput("confidence", confidence)
	measurement.MergeOutput("strength", confidence)

	bestCategory := shares[0].Category

	for category, mass := range categoryMasses {
		if mass > categoryMasses[bestCategory] {
			bestCategory = category
		}
	}

	globalIndex := logic.CategoryIndex(bestCategory)
	measurement.MergeOutput("value", float64(globalIndex))
	measurement.MergeOutput("category", float64(globalIndex))

	return confidence
}
