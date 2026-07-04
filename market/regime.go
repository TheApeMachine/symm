package market

import (
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/statistic"
)

type RegimeType string

const (
	RegimeTypeNone     RegimeType = ""
	RegimeTypeDead     RegimeType = "dead"
	RegimeTypeChoppy   RegimeType = "choppy"
	RegimeTypeTrending RegimeType = "trending"
	RegimeTypeBullish  RegimeType = "bullish"
	RegimeTypeBearish  RegimeType = "bearish"
)

func (crossSection *CrossSection) RegimeArtifact() *datura.Artifact {
	observed := crossSection.observedSymbols
	breadth := crossSection.Breadth()
	changes := crossSection.absoluteChanges("", 0)
	trend := regimeTrend(changes)
	volatility := crossSection.regimeVolatility(changes)
	confidence := regimeConfidence(observed, crossSection.MinBarsRequired())

	frame := datura.Acquire("regime", datura.APPJSON).
		WithRole("regime").
		WithScope("regime").
		WithPayload(datura.Map[any]{
			"volatility": volatility,
			"trend":      trend,
			"bullish":    breadth,
			"bearish":    regimeBearish(observed, breadth),
			"choppiness": clamp01(1 - trend),
			"observed":   observed,
			"output": datura.Map[any]{
				"status":     "measured",
				"confidence": confidence,
				"strength":   trend,
			},
		}.Marshal())

	frame.SetTimestamp(time.Now().UTC().UnixNano())
	return frame
}

func regimeTrend(changes []float64) float64 {
	sum := 0.0
	leader := 0.0

	for _, change := range changes {
		if change < 0 || math.IsNaN(change) || math.IsInf(change, 0) {
			continue
		}

		sum += change
		leader = math.Max(leader, change)
	}

	if sum <= 0 {
		return 0
	}

	return clamp01(leader / sum)
}

func (crossSection *CrossSection) regimeVolatility(changes []float64) float64 {
	if len(changes) == 0 {
		return 0
	}

	median, _ := statistic.MedianOf(changes)
	threshold := crossSection.leadershipThreshold(changes)
	if threshold <= 0 {
		return 0
	}

	return clamp01(median / threshold)
}

func regimeBearish(observed int, breadth float64) float64 {
	if observed == 0 {
		return 0
	}

	return clamp01(1 - breadth)
}

func regimeConfidence(observed int, required int) float64 {
	if observed <= 0 || required <= 0 {
		return 0
	}

	return clamp01(float64(observed) / float64(required))
}

func clamp01(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}

	if value >= 1 {
		return 1
	}

	return value
}
