package sentiment

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
sentimentCategory maps breadth and leadership onto the bullish-breadth perspective.
*/
func sentimentCategory(breadth, change, topChange float64) perspectives.CategoryType {
	if breadth >= minBreadth {
		return perspectives.CategoryRiskOnSurge
	}

	leaderShare := 0.0

	if topChange > 0 {
		leaderShare = math.Abs(change) / topChange
	}

	if leaderShare >= 0.5 && change != 0 {
		return perspectives.CategoryDivergentMove
	}

	return perspectives.CategorySystemicSlump
}
