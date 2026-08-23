package derivatives

import (
	"math"

	"github.com/theapemachine/symm/nomagique/statistic"
)

func evaluateRegimes(
	priceVelocity float64,
	oiVelocity float64,
	aggressorImbalance float64,
	flowZScore float64,
	liqBuy float64,
	liqSell float64,
	liqIntensity float64,
	basis float64,
	tripartiteDiv float64,
) (float64, float64, float64, float64, float64) {
	posPrice := math.Max(0, statistic.StandardSeparation(priceVelocity*100))
	negPrice := math.Max(0, statistic.StandardSeparation(-priceVelocity*100))
	posOI := math.Max(0, statistic.StandardSeparation(oiVelocity*50))
	negOI := math.Max(0, statistic.StandardSeparation(-oiVelocity*50))
	posFlow := math.Max(0, statistic.StandardSeparation(aggressorImbalance*2))
	negFlow := math.Max(0, statistic.StandardSeparation(-aggressorImbalance*2))
	buyLiqProb := math.Max(0, statistic.StandardSeparation(liqIntensity*10))
	sellLiqProb := math.Max(0, statistic.StandardSeparation(liqIntensity*10))

	if liqBuy <= 0 {
		buyLiqProb = 0
	}

	if liqSell <= 0 {
		sellLiqProb = 0
	}

	divMagnitude := math.Abs(tripartiteDiv) + math.Abs(basis)
	divProb := math.Max(0, statistic.StandardSeparation(divMagnitude*100))

	ignition := math.Min(1, posPrice*posOI*(0.5+0.5*posFlow))
	squeeze := math.Min(1, posPrice*(0.5*negOI+0.5*buyLiqProb))
	buildup := math.Min(1, negPrice*posOI*(0.5+0.5*negFlow))
	deleveraging := math.Min(1, negPrice*(0.5*negOI+0.5*sellLiqProb))
	decoupling := divProb

	return ignition, squeeze, buildup, deleveraging, decoupling
}
