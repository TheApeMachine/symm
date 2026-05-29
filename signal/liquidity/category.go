package liquidity

import (
	"math"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/numeric"
)

/*
liquidityCategory maps cross-section quote volume onto the scarcity perspective.
*/
func liquidityCategory(quoteVol float64, peers []float64, peakScore float64) engine.Category {
	if peakScore > 0 {
		return engine.CatExtremeScarcity
	}

	if len(peers) == 0 || quoteVol <= 0 {
		return engine.CatMedianDepth
	}

	sorted := numeric.CopySorted(peers)
	median := numeric.PercentileSorted(sorted, 0.5)

	if median <= 0 {
		return engine.CatMedianDepth
	}

	ratio := quoteVol / median

	if ratio >= 1.25 {
		return engine.CatRobustLiquidity
	}

	if ratio >= 0.75 {
		return engine.CatMedianDepth
	}

	return engine.CatExtremeScarcity
}

/*
liquidityConfidence scores how decisively the symbol sits in its scarcity category.
*/
func liquidityConfidence(quoteVol float64, peers []float64, peakScore float64) float64 {
	if peakScore > 0 {
		return engine.ConfidenceFromScore(peakScore)
	}

	if len(peers) == 0 || quoteVol <= 0 {
		return 0
	}

	median := numeric.PercentileSorted(numeric.CopySorted(peers), 0.5)

	if median <= 0 {
		return 0
	}

	return engine.ConfidenceFromScore(math.Abs(quoteVol-median) / median)
}
