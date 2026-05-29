package sentiment

import (
	"math"

	"github.com/theapemachine/symm/engine"
)

/*
sentimentCategory maps breadth and leadership onto the bullish-breadth perspective.
*/
func sentimentCategory(breadth, change, topChange float64) engine.Category {
	if breadth >= minBreadth {
		return engine.CatRiskOnSurge
	}

	leaderShare := 0.0

	if topChange > 0 {
		leaderShare = math.Abs(change) / topChange
	}

	if leaderShare >= 0.5 && change != 0 {
		return engine.CatDivergentMove
	}

	return engine.CatSystemicSlump
}
