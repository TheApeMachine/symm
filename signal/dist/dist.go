package dist

import (
	"fmt"
	"math"

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
each category by wire key and global index, and returns confidence as the
evidence-weighted peak share.
*/
func Write(measurement *datura.Artifact, shares []Share) float64 {
	output, confidence := Fields(shares)
	measurement.MergeOutputs(output)

	return confidence
}

/*
Fields returns the normalised distribution fields without mutating an artifact.
Callers that already have signal-specific output can merge the returned map into
their own batch and write once.
*/
func Fields(shares []Share) (map[string]any, float64) {
	if len(shares) == 0 {
		return nil, 0
	}

	masses := make([]float64, len(shares))
	categoryMasses := map[logic.CategoryType]float64{}
	rawTotal := 0.0

	for index := range shares {
		mass := shares[index].Mass

		if math.IsNaN(mass) || math.IsInf(mass, 0) || mass < 0 {
			mass = 0
		}

		masses[index] = mass
		rawTotal += mass
	}

	statutil.NormalizeMasses(masses)

	for index := range shares {
		mass := masses[index]
		categoryMasses[shares[index].Category] += mass
	}

	output := map[string]any{}

	for index := range shares {
		output[shares[index].Key] = masses[index]
	}

	for category, mass := range categoryMasses {
		output[fmt.Sprintf("category.%d", logic.CategoryIndex(category))] = mass
	}

	categoryValues := make([]float64, 0, len(categoryMasses))

	for _, mass := range categoryMasses {
		categoryValues = append(categoryValues, mass)
	}

	peak := statutil.MaxMass(categoryValues)
	strength := 0.0

	if rawTotal > 0 {
		strength = rawTotal / (1 + rawTotal)
	}

	confidence := peak * strength
	output["confidence"] = confidence
	output["strength"] = strength
	output["evidence"] = rawTotal

	bestCategory := shares[0].Category

	for category, mass := range categoryMasses {
		if mass > categoryMasses[bestCategory] {
			bestCategory = category
		}
	}

	globalIndex := logic.CategoryIndex(bestCategory)
	output["value"] = float64(globalIndex)
	output["category"] = float64(globalIndex)

	return output, confidence
}
