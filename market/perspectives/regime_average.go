package perspectives

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
)

const (
	RegimeAxisVolatility = "volatility"
	RegimeAxisTrend      = "trend"
	RegimeAxisBullish    = "bullish"
	RegimeAxisBearish    = "bearish"
	RegimeAxisChoppiness = "choppiness"
)

var regimeRadarAxes = []string{
	RegimeAxisVolatility,
	RegimeAxisTrend,
	RegimeAxisBullish,
	RegimeAxisBearish,
	RegimeAxisChoppiness,
}

/*
AverageRadarAxes averages each regime radar axis across symbols. Zero values are
skipped so quiet or unclassified symbols do not pull the market shape toward the
origin.
*/
func AverageRadarAxes(perSymbol map[string]map[string]float64) map[string]float64 {
	averaged := make(map[string]float64, len(regimeRadarAxes))

	for _, axis := range regimeRadarAxes {
		sum := 0.0
		count := 0

		for _, axes := range perSymbol {
			value := axes[axis]

			if value == 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				continue
			}

			sum += value
			count++
		}

		if count > 0 {
			averaged[axis] = sum / float64(count)
		}
	}

	return averaged
}

/*
MajorityRegime picks the most common classified regime across symbols. RegimeNone
votes are ignored so sparse symbols do not dominate before they have enough data.
*/
func MajorityRegime(perSymbol map[string]RegimeFeatures) types.Regime {
	counts := make(map[types.Regime]int)

	for _, features := range perSymbol {
		if features.Regime == types.RegimeNone {
			continue
		}

		counts[features.Regime]++
	}

	best := types.RegimeNone
	bestCount := 0

	for regime, count := range counts {
		if count > bestCount {
			best = regime
			bestCount = count
		}
	}

	return best
}
