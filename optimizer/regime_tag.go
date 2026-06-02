package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
StructuralRegime is the dominant causal environment on a replay tick.
*/
type StructuralRegime int

const (
	StructuralRegimeUnclassified StructuralRegime = iota
	StructuralRegimeNormalFlow
	StructuralRegimeSystemicBeta
	StructuralRegimeLiquidityPanic
	StructuralRegimeCausalNoise
)

var structuralRegimeCategories = []struct {
	regime   StructuralRegime
	category perspectives.CategoryType
}{
	{StructuralRegimeNormalFlow, perspectives.CategoryEndogenousAlpha},
	{StructuralRegimeSystemicBeta, perspectives.CategorySystemicBeta},
	{StructuralRegimeLiquidityPanic, perspectives.CategoryLiquidityShock},
	{StructuralRegimeCausalNoise, perspectives.CategoryCausalNoise},
}

func categoryStructuralRegime(
	category perspectives.CategoryType,
) StructuralRegime {
	for _, mapping := range structuralRegimeCategories {
		if mapping.category == category {
			return mapping.regime
		}
	}

	return StructuralRegimeUnclassified
}

/*
DominantStructuralRegime picks the highest-SNR causal category on a tick snapshot.
*/
func DominantStructuralRegime(
	snapshots []perspectives.Measurement,
) StructuralRegime {
	bestRegime := StructuralRegimeUnclassified
	bestSNR := 0.0

	for _, measurement := range snapshots {
		regime := categoryStructuralRegime(measurement.Category)

		if regime == StructuralRegimeUnclassified {
			continue
		}

		if measurement.SNR > bestSNR {
			bestSNR = measurement.SNR
			bestRegime = regime
		}
	}

	return bestRegime
}

/*
TagRowRegimes assigns each measurement row its dominant structural regime.
*/
func TagRowRegimes(rows []perspectives.Measurement) []StructuralRegime {
	tape := PrecompileTape(rows)
	tags := make([]StructuralRegime, len(rows))

	for index, tick := range tape.Ticks {
		tags[index] = DominantStructuralRegime(tick.Snapshots)
	}

	return tags
}

func filterRowsByRegime(
	rows []perspectives.Measurement,
	tags []StructuralRegime,
	regime StructuralRegime,
) []perspectives.Measurement {
	if regime == StructuralRegimeUnclassified {
		return rows
	}

	filtered := make([]perspectives.Measurement, 0)

	for index, row := range rows {
		if tags[index] == regime {
			filtered = append(filtered, row)
		}
	}

	return filtered
}

func regimeSetInRange(
	tags []StructuralRegime,
	start int,
	end int,
) map[StructuralRegime]struct{} {
	regimes := make(map[StructuralRegime]struct{})

	for index := start; index < end && index < len(tags); index++ {
		regime := tags[index]

		if regime == StructuralRegimeUnclassified {
			continue
		}

		regimes[regime] = struct{}{}
	}

	return regimes
}

func testSliceHasUnprecedentedRegime(
	trainRegimes map[StructuralRegime]struct{},
	testTags []StructuralRegime,
	testStart int,
	testEnd int,
) bool {
	for index := testStart; index < testEnd && index < len(testTags); index++ {
		regime := testTags[index]

		if regime == StructuralRegimeUnclassified {
			continue
		}

		if _, seen := trainRegimes[regime]; !seen {
			return true
		}
	}

	return false
}
