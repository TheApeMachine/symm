package budget

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/replay"
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
DominantStructuralRegime classifies a tick snapshot using causal categories
and condition-number/contagion proxies carried on causal measurements.
*/
func DominantStructuralRegime(
	snapshots []perspectives.Measurement,
) StructuralRegime {
	if causalRegime, ok := causalStructuralRegime(snapshots); ok {
		return causalRegime
	}

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

func causalStructuralRegime(
	snapshots []perspectives.Measurement,
) (StructuralRegime, bool) {
	conditionSwitch := viper.GetViper().GetFloat64("signals.causal.condition_switch")
	panicStrength := 0.0
	hasCausal := false
	hasLiquidityShock := false

	for _, measurement := range snapshots {
		if measurement.Source != perspectives.SourceCausal {
			continue
		}

		hasCausal = true

		if measurement.Category == perspectives.CategoryLiquidityShock {
			hasLiquidityShock = true

			if measurement.Strength > panicStrength {
				panicStrength = measurement.Strength
			}
		}
	}

	if !hasCausal {
		return StructuralRegimeUnclassified, false
	}

	if hasLiquidityShock {
		if conditionSwitch > 0 && panicStrength >= conditionSwitch {
			return StructuralRegimeLiquidityPanic, true
		}

		if conditionSwitch <= 0 || panicStrength > 0 {
			return StructuralRegimeLiquidityPanic, true
		}
	}

	return StructuralRegimeUnclassified, false
}

/*
TagRowRegimes assigns each measurement row its dominant structural regime.
*/
func TagRowRegimes(rows []perspectives.Measurement) []StructuralRegime {
	tape := replay.PrecompileTape(rows)
	tags := make([]StructuralRegime, len(rows))

	for index, tick := range tape.Ticks {
		tags[index] = DominantStructuralRegime(tick.Snapshots)
	}

	return tags
}

func FilterRowsByRegime(
	rows []perspectives.Measurement,
	tags []StructuralRegime,
	regime StructuralRegime,
) []perspectives.Measurement {
	return filterRowsByRegime(rows, tags, regime)
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

func RegimeSetInRange(
	tags []StructuralRegime,
	start int,
	end int,
) map[StructuralRegime]struct{} {
	return regimeSetInRange(tags, start, end)
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

func TestSliceHasUnprecedentedRegime(
	trainRegimes map[StructuralRegime]struct{},
	testTags []StructuralRegime,
	testStart int,
	testEnd int,
) bool {
	return testSliceHasUnprecedentedRegime(trainRegimes, testTags, testStart, testEnd)
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

func RegimePairMinRows(regime StructuralRegime, rowCount int) int {
	return regimePairMinRows(regime, rowCount)
}

func regimePairMinRows(regime StructuralRegime, rowCount int) int {
	if rowCount <= 0 {
		return 2
	}

	minRows := int(math.Ceil(math.Sqrt(float64(rowCount))))

	if minRows < 2 {
		return 2
	}

	if regime == StructuralRegimeLiquidityPanic && minRows > 4 {
		return 4
	}

	return minRows
}
