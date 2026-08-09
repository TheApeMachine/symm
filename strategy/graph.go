package strategy

import (
	"fmt"
	"math"

	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

const marketGraphKey = "market_graph"

/*
graphEvidence is the graph's bounded opinion about one forecast. Direct
supporting and contradictory masses retain only the strongest relation in each
direction, so repeated outputs from the same causal family cannot manufacture
evidence through graph degree.
*/
type graphEvidence struct {
	supports    float64
	contradicts float64
	relations   int
}

/*
forecastWithGraphEvidence resolves a forecast and the graph compiled for the
same Thesis cut. Graph evidence remains separate from forecast confidence:
without outcome calibration it is not a likelihood and cannot update a
posterior probability. Missing, stale, or mismatched evidence is an error.
*/
func forecastWithGraphEvidence(
	thesis *types.Thesis,
	symbol string,
) (*types.ResonanceForecast, graphEvidence, error) {
	evidence := graphEvidence{}

	if thesis == nil {
		return nil, evidence, fmt.Errorf("planner: thesis required")
	}

	readingRaw, found := thesis.Resonance.Load(symbol)

	if !found {
		return nil, evidence, fmt.Errorf("planner: resonance forecast required for %s", symbol)
	}

	reading, valid := readingRaw.(types.ResonanceReading)

	if !valid || reading.Symbol != symbol || !reading.At.Equal(thesis.At) ||
		reading.Forecast == nil {
		return nil, evidence, fmt.Errorf(
			"planner: current resonance forecast required for %s", symbol,
		)
	}

	if err := reading.Forecast.Validate(); err != nil {
		return nil, evidence, fmt.Errorf("planner: invalid resonance forecast for %s: %w", symbol, err)
	}

	storedGraph, found := thesis.Graphs.Load(marketGraphKey)

	if !found {
		return nil, evidence, fmt.Errorf("planner: market graph required for %s", symbol)
	}

	marketGraph, valid := storedGraph.(*logicgraph.Graph)

	if !valid || marketGraph == nil || !marketGraph.At.Equal(thesis.At) {
		return nil, evidence, fmt.Errorf("planner: current market graph required for %s", symbol)
	}

	graphForecast := marketGraph.Nodes["res:"+symbol+":forecast"]

	if graphForecast == nil || graphForecast.Value != reading.Forecast.ExpectedReturn ||
		graphForecast.Confidence != reading.Forecast.Confidence {
		return nil, evidence, fmt.Errorf("planner: graph forecast mismatch for %s", symbol)
	}

	evidence, err := newGraphEvidence(marketGraph, symbol)

	if err != nil {
		return nil, evidence, fmt.Errorf("planner: graph evidence for %s: %w", symbol, err)
	}

	return reading.Forecast, evidence, nil
}

/*
newGraphEvidence reads only current directional relations emitted directly
from this symbol's forecast.
*/
func newGraphEvidence(
	marketGraph *logicgraph.Graph,
	symbol string,
) (graphEvidence, error) {
	evidence := graphEvidence{}

	if marketGraph == nil || symbol == "" {
		return evidence, fmt.Errorf("strategy: graph and symbol required")
	}

	forecastID := "res:" + symbol + ":forecast"
	forecast := marketGraph.Nodes[forecastID]

	if forecast == nil || forecast.Kind != logicgraph.KindResonance ||
		forecast.Symbol != symbol || !forecast.At.Equal(marketGraph.At) {
		return evidence, fmt.Errorf("strategy: graph forecast node required for %s", symbol)
	}

	for _, edge := range marketGraph.Edges {
		if edge == nil {
			return evidence, fmt.Errorf("strategy: graph contains a nil edge")
		}

		if edge.From != forecastID ||
			(edge.Relation != logicgraph.RelationSupports &&
				edge.Relation != logicgraph.RelationContradicts) {
			continue
		}

		target := marketGraph.Nodes[edge.To]

		if target == nil || target.Symbol != symbol || !target.At.Equal(marketGraph.At) {
			return evidence, fmt.Errorf("strategy: graph relation target must be current for %s", symbol)
		}

		if math.IsNaN(edge.Weight) || math.IsInf(edge.Weight, 0) ||
			edge.Weight < 0 || edge.Weight > 1 ||
			math.IsNaN(edge.Confidence) || math.IsInf(edge.Confidence, 0) ||
			edge.Confidence < 0 || edge.Confidence > 1 ||
			!edge.At.Equal(marketGraph.At) {
			return evidence, fmt.Errorf("strategy: graph relation must be current with unit weight and confidence")
		}

		mass := edge.Weight * edge.Confidence

		if mass == 0 {
			continue
		}

		if edge.Relation == logicgraph.RelationSupports {
			evidence.supports = max(evidence.supports, mass)
			evidence.relations++
			continue
		}

		evidence.contradicts = max(evidence.contradicts, mass)
		evidence.relations++
	}

	return evidence, nil
}

/*
Reward converts signed graph evidence into the causal target's RMS scale.
*/
func (evidence graphEvidence) Reward(rows [][]float64, target int) (float64, error) {
	if err := evidence.validate(); err != nil {
		return 0, err
	}

	if len(rows) == 0 || target < 0 {
		return 0, fmt.Errorf("strategy: causal target history required")
	}

	squares := 0.0

	for _, row := range rows {
		if target >= len(row) || math.IsNaN(row[target]) || math.IsInf(row[target], 0) {
			return 0, fmt.Errorf("strategy: finite causal target history required")
		}

		squares += row[target] * row[target]
	}

	scale := math.Sqrt(squares / float64(len(rows)))

	return (evidence.supports - evidence.contradicts) * scale, nil
}

func (evidence graphEvidence) validate() error {
	if math.IsNaN(evidence.supports) || math.IsInf(evidence.supports, 0) ||
		math.IsNaN(evidence.contradicts) || math.IsInf(evidence.contradicts, 0) ||
		evidence.supports < 0 || evidence.supports > 1 ||
		evidence.contradicts < 0 || evidence.contradicts > 1 {
		return fmt.Errorf("strategy: graph evidence must be finite unit evidence")
	}

	return nil
}
