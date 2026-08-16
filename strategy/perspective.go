package strategy

import (
	"fmt"

	logicgraph "github.com/theapemachine/symm/logic/graph"
)

/*
decisionPerspective is the planner-facing interpretation of the graph's
explicit long-opportunity proposition. It is dimensionless structural evidence:
no category, causal rung, phase analogue, or predictive direction is converted
into a price return.
*/
type decisionPerspective struct {
	Hypothesis    string
	Support       float64
	Contradiction float64
	Conditions    float64
	Balance       float64
	Confidence    float64
	Score         float64
	Direction     float64
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

	return decisionPerspective{
		Hypothesis:    summary.Hypothesis,
		Support:       summary.Support,
		Contradiction: summary.Contradiction,
		Conditions:    summary.Conditions,
		Balance:       summary.Balance,
		Confidence:    summary.Confidence,
		Score:         summary.Score,
		Direction:     summary.Direction,
	}, nil
}
