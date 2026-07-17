package strategy

import (
	"math"

	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
Opportunity is the decomposed admit state for one forecast. Margin is economic
edge after uncertainty; Lead is how far cognition has committed ahead of the
manifold basin. Reserved overflow requires both positive — anticipatory edge,
not planner horizon outrunning a world model.
*/
type Opportunity struct {
	Margin     float64
	Lead       float64
	Cognitive  float64
	Basin      float64
	BasinReady bool
}

/*
Reserved reports whether this reading may consume overflow slots. Positive
margin alone is a normal-lane SNR clear; reserved needs cognition ahead of the
settling manifold as well.
*/
func (opportunity Opportunity) Reserved() bool {
	return opportunity.Margin > 0 && opportunity.Lead > 0
}

/*
measureOpportunity builds OpportunityMargin and CognitiveLead from independent
estimators: forecast return/uncertainty, cognition winner confidence, and
manifold coherence as basin settling.
*/
func measureOpportunity(
	forecast types.Forecasts,
	cognition types.Cognition,
	thesis *types.Thesis,
) Opportunity {
	reading := Opportunity{
		Margin:    forecast.ExpectedReturn - forecast.Uncertainty,
		Cognitive: cognition.Confidence,
	}

	basin, ready := basinConfidence(thesis, forecast.Symbol)
	reading.Basin = basin
	reading.BasinReady = ready

	if ready {
		reading.Lead = cognition.Confidence - basin
	}

	return reading
}

/*
basinConfidence returns manifold coherence as the physical basin settling
measure. Missing or invalid manifold leaves BasinReady false so Lead stays
neutral rather than inventing dynamics.
*/
func basinConfidence(thesis *types.Thesis, symbol string) (float64, bool) {
	if thesis == nil || thesis.Manifold == nil {
		return 0, false
	}

	value, found := thesis.Manifold.Load(symbol)

	if !found {
		return 0, false
	}

	state, ok := value.(manifold.State)

	if !ok || !state.GasReady() {
		return 0, false
	}

	coherence := state.Reading.CoherenceMag2

	if math.IsNaN(coherence) || math.IsInf(coherence, 0) || coherence < 0 {
		return 0, false
	}

	return coherence, true
}
