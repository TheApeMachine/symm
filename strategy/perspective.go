package strategy

import (
	"fmt"
	"slices"
	"strings"

	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

/*
decisionPerspective combines only estimates with the same meaning: signed log
return over the graph's forward horizon. Native log returns are converted to a
per-tick rate before projection onto that horizon. Resonance contributes size
only after it has already made a direction call; the call itself is not a
return. Signal categories and cognition remain graph evidence because treating
their scores as returns would mix incompatible units.
*/
func decisionPerspective(
	graph *logicgraph.Graph,
	positiveProbability float64,
) (float64, float64, []types.DecisionPerspectiveSource, error) {
	if graph == nil || graph.Forecast == nil || !graph.Forecast.Ready {
		return 0, 0, nil, fmt.Errorf("planner: ready direction forecast required")
	}

	forecastConfidence := max(
		positiveProbability,
		1-positiveProbability,
	)
	sources := []types.DecisionPerspectiveSource{{
		Source: "resonance", LogReturn: graph.Forecast.Value,
		Horizon: graph.ForecastHorizon, Confidence: forecastConfidence,
	}}

	for nodeID, node := range graph.Nodes {
		if node == nil || node.Confidence <= 0 {
			continue
		}

		if node.Kind == logicgraph.KindCausal &&
			strings.HasSuffix(nodeID, ":doExpectation") {
			horizon, err := perspectiveHorizon(node)

			if err != nil {
				return 0, 0, nil, err
			}

			sources = append(sources, types.DecisionPerspectiveSource{
				Source:     "causal",
				LogReturn:  node.Value * float64(graph.ForecastHorizon) / float64(horizon),
				Horizon:    graph.ForecastHorizon,
				Confidence: node.Confidence,
			})
		}

		if node.Kind == logicgraph.KindManifold &&
			strings.HasSuffix(nodeID, ":phase_alignment") {
			horizon, err := perspectiveHorizon(node)

			if err != nil {
				return 0, 0, nil, err
			}

			sources = append(sources, types.DecisionPerspectiveSource{
				Source:     "manifold",
				LogReturn:  node.Value * float64(graph.ForecastHorizon) / float64(horizon),
				Horizon:    graph.ForecastHorizon,
				Confidence: node.Confidence,
			})
		}
	}

	slices.SortFunc(sources, func(left, right types.DecisionPerspectiveSource) int {
		return strings.Compare(left.Source, right.Source)
	})
	weightedReturn := 0.0
	confidenceMass := 0.0

	for _, source := range sources {
		weightedReturn += source.LogReturn * source.Confidence
		confidenceMass += source.Confidence
	}

	if confidenceMass <= 0 {
		return 0, 0, nil, fmt.Errorf("planner: positive return-evidence confidence required")
	}

	return weightedReturn / confidenceMass,
		confidenceMass / float64(len(sources)), sources, nil
}

func perspectiveHorizon(node *logicgraph.Node) (int, error) {
	horizonValue, found := node.Metadata["horizon"]
	horizon, valid := horizonValue.(int)

	if !found || !valid || horizon < 1 {
		return 0, fmt.Errorf(
			"planner: positive native horizon required for %s", node.ID,
		)
	}

	return horizon, nil
}
