package strategy

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

/*
Planner is a component that generates a plan for the trading agent.
It evaluates utility for different actions based on forecasts and evidence.
*/
type Planner struct {
	tree *dmt.Tree
}

/*
NewPlanner creates a new Planner.
*/
func NewPlanner(tree *dmt.Tree) *Planner {
	return &Planner{tree: tree}
}

/*
Update evaluates the thesis for all symbols and returns intended actions.
*/
func (planner *Planner) Update(thesis *Thesis) []*Intent {
	intents := make([]*Intent, 0)

	for _, symbol := range thesis.Symbols() {
		intent := planner.evaluate(symbol, thesis)

		if intent != nil {
			intents = append(intents, intent)
		}
	}

	return intents
}

func (planner *Planner) evaluate(symbol string, thesis *Thesis) *Intent {
	snapshot, ok := thesis.Evidence(symbol, "manifold_forecasts")
	if !ok {
		return &Intent{
			Symbol:     symbol,
			Action:     ActionHold,
			Edge:       *decimal.NewFromFloat64(0.0),
			Confidence: 0.0,
			Thesis:     thesis,
		}
	}

	forecasts, ok := snapshot.(types.Forecasts)
	if !ok {
		return &Intent{
			Symbol:     symbol,
			Action:     ActionHold,
			Edge:       *decimal.NewFromFloat64(0.0),
			Confidence: 0.0,
			Thesis:     thesis,
		}
	}

	// Calculate utility based on Phase 5 specs:
	// U(action) = E[R_executable] - E[fees] - E[spread] - E[impact] - E[adverse selection]
	// Forecasts provides ExecutableReturn (which is midMove - impactEstimate)
	// We use a simplified utility calculation here as a starting point.
	utilityBuy := forecasts.ExecutableReturn
	utilitySell := -forecasts.ExecutableReturn
	utilityHold := 0.0

	alternatives := map[Action]float64{
		ActionBuy:  utilityBuy,
		ActionSell: utilitySell,
		ActionHold: utilityHold,
	}

	var bestAction Action = ActionHold
	var bestUtility float64 = utilityHold

	if utilityBuy > bestUtility && forecasts.Uncertainty < 0.8 {
		bestAction = ActionBuy
		bestUtility = utilityBuy
	}

	if utilitySell > bestUtility && forecasts.Uncertainty < 0.8 {
		bestAction = ActionSell
		bestUtility = utilitySell
	}

	decision := Decision{
		At:           time.Now(),
		Symbol:       symbol,
		Action:       bestAction,
		Utility:      bestUtility,
		Alternatives: alternatives,
		Reason:       "utility maximization based on manifold forecasts",
	}

	thesis.AddEvidence(symbol, "decision", decision)

	if bestAction == ActionHold {
		return &Intent{
			Symbol:     symbol,
			Action:     bestAction,
			Edge:       *decimal.NewFromFloat64(bestUtility),
			Confidence: 1.0 - forecasts.Uncertainty,
			Thesis:     thesis,
		}
	}

	return &Intent{
		Symbol:     symbol,
		Action:     bestAction,
		Edge:       *decimal.NewFromFloat64(bestUtility),
		Confidence: 1.0 - forecasts.Uncertainty,
		Thesis:     thesis,
	}
}
