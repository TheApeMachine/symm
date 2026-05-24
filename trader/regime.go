package trader

import (
	"math"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
)

/*
MarketRegime is the cross-section state used to gate signal specialists.
*/
type MarketRegime int

const (
	RegimeTrending MarketRegime = iota
	RegimeChopping
	RegimeDead
)

/*
EnsembleContext carries regime and trust weights into decision scoring.
*/
type EnsembleContext struct {
	Regime MarketRegime
	Trust  *SourceTrustStore
}

/*
ClassifyMarketRegime infers trending, chopping, or dead from live snapshots.
*/
func ClassifyMarketRegime(
	market engine.MarketReader,
	symbols []string,
) MarketRegime {
	if market == nil || len(symbols) == 0 {
		return RegimeChopping
	}

	pressures := make([]float64, 0, len(symbols))
	volumes := make([]float64, 0, len(symbols))
	active := 0

	for _, symbol := range symbols {
		snapshot := market.Read(symbol)

		if snapshot.PressureOK {
			pressures = append(pressures, snapshot.BuyPressure)
		}

		if snapshot.BatchOK && snapshot.BatchVolume > 0 {
			volumes = append(volumes, snapshot.BatchVolume)
		}

		if snapshot.LastOK {
			active++
		}
	}

	if active < 2 || len(pressures) < 2 {
		return RegimeChopping
	}

	medianVolume := stats.Median(volumes)
	meanPressure := stats.Mean(pressures)
	sortedPressures := stats.CopySorted(pressures)
	pressureMedian := stats.PercentileSorted(sortedPressures, 0.5)
	churn := stats.MedianAbsoluteDeviation(sortedPressures, pressureMedian)

	if medianVolume <= 0 && churn <= 0.05 {
		return RegimeDead
	}

	if math.Abs(meanPressure) > math.Max(churn, 0.08) {
		return RegimeTrending
	}

	if medianVolume <= 0 && churn < 0.12 {
		return RegimeDead
	}

	return RegimeChopping
}

/*
RegimeWeight returns how much one source should contribute in the current regime.
*/
func RegimeWeight(regime MarketRegime, source string) float64 {
	weights, ok := regimeSourceWeights[regime]

	if !ok {
		return 1
	}

	weight, ok := weights[source]

	if !ok {
		return 0.5
	}

	return weight
}

var regimeSourceWeights = map[MarketRegime]map[string]float64{
	RegimeTrending: {
		"hawkes":   1.0,
		"fluid":    0.85,
		"pumpdump": 0.35,
		"causal":   0.75,
	},
	RegimeChopping: {
		"pumpdump": 1.0,
		"causal":   0.75,
		"hawkes":   0.3,
		"fluid":    0.55,
	},
	RegimeDead: {
		"pumpdump": 0.95,
		"fluid":    0.35,
		"hawkes":   0.15,
		"causal":   0.3,
	},
}

func regimeLabel(regime MarketRegime) string {
	switch regime {
	case RegimeTrending:
		return "trending"
	case RegimeDead:
		return "dead"
	default:
		return "chopping"
	}
}
