package strategy

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/stat/distuv"

	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/nomagique/learning"
)

/*
decisionPerspective is the planner-facing interpretation of the graph's
explicit long-opportunity proposition. It is dimensionless structural evidence:
no category, causal rung, phase analogue, or predictive direction is converted
into a price return.

RelationConfidence is how strongly the graph believes its own category labels;
TradeConfidence is the separate, calibrated posterior mass that the forecast
agrees with the structural direction. The two are different probability
objects and must never be conflated: admission needs the latter, observability
keeps the former.
*/
type decisionPerspective struct {
	Hypothesis      string
	Support         float64
	Contradiction   float64
	Conditions      float64
	Balance         float64
	Confidence      float64
	TradeConfidence float64
	Score           float64
	Direction       float64
}

func graphPerspective(graph *logicgraph.Graph) (decisionPerspective, error) {
	if graph == nil {
		return decisionPerspective{}, fmt.Errorf("planner: evidence graph required")
	}

	summary := graph.OpportunitySummary()

	if !summary.Ready {
		return decisionPerspective{}, fmt.Errorf(
			"planner: directional evidence for explicit opportunity hypothesis required",
		)
	}

	tradeConfidence := summary.Confidence

	if graph.Forecast != nil && graph.Forecast.Ready && summary.Direction != 0 {
		// Once the calibrated predictive posterior exists, it owns admission:
		// the structural label confidence is a classification probability, not
		// a profitability probability. Confusing the two is what admitted the
		// historical false-positive "vertical ignition" entries.
		tradeConfidence = directionalPosteriorMass(*graph.Forecast, summary.Direction)
	}

	return decisionPerspective{
		Hypothesis:      summary.Hypothesis,
		Support:         summary.Support,
		Contradiction:   summary.Contradiction,
		Conditions:      summary.Conditions,
		Balance:         summary.Balance,
		Confidence:      summary.Confidence,
		TradeConfidence: tradeConfidence,
		Score:           summary.Score,
		Direction:       summary.Direction,
	}, nil
}

/*
directionalPosteriorMass returns P(sign(forecast) == direction) under the RLS
Student-t posterior: the calibrated probability the predictive coder agrees
with the structural direction call. It is the honest profitability-confidence
object the admission gate reads; a zero means the forecast is absent, not calm.
*/
func directionalPosteriorMass(
	forecast learning.RLSOutput,
	direction float64,
) float64 {
	if !forecast.Ready || forecast.Scale <= 0 || forecast.DegreesOfFreedom <= 0 ||
		math.IsNaN(forecast.Value) || math.IsInf(forecast.Value, 0) ||
		direction == 0 {
		return 0
	}

	distribution := distuv.StudentsT{
		Mu:    forecast.Value,
		Sigma: forecast.Scale,
		Nu:    forecast.DegreesOfFreedom,
	}

	if direction > 0 {
		return min(max(1-distribution.CDF(0), 0), 1)
	}

	return min(max(distribution.CDF(0), 0), 1)
}
