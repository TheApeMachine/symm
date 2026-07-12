package strategy

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Planner is a component that generates a plan for the trading agent.
It evaluates utility for different actions based on forecasts and evidence.
*/
type Planner struct {
	status types.Status
	booter *system.Booter
	tree   *dmt.Tree
}

/*
NewPlanner creates a new Planner.
*/
func NewPlanner(booter *system.Booter, tree *dmt.Tree) *Planner {
	return &Planner{
		status: types.INITIALIZING,
		booter: booter,
		tree:   tree,
	}
}

func (planner *Planner) Status() types.Status {
	return planner.status
}

/*
Update evaluates the thesis for all symbols and returns intended actions.
*/
func (planner *Planner) Update(thesis *Thesis) []*Intent {
	if !planner.booter.Ready(system.StageReady) {
		return nil
	}

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
		return nil
	}

	forecasts, ok := snapshot.(types.Forecasts)
	if !ok || !forecasts.Eligible() || forecasts.Symbol != symbol {
		return &Intent{
			Symbol:     symbol,
			Action:     ActionHold,
			Edge:       *decimal.NewFromFloat64(0.0),
			Confidence: 0.0,
			Thesis:     thesis,
		}
	}

	utilityBuy := forecasts.ExecutableReturn()
	utilityHold := 0.0

	alternatives := map[Action]float64{
		ActionBuy:  utilityBuy,
		ActionHold: utilityHold,
	}

	bestAction := ActionHold
	bestUtility := utilityHold

	if utilityBuy > bestUtility {
		bestAction = ActionBuy
		bestUtility = utilityBuy
	}

	decision := Decision{
		At:           forecasts.At,
		Symbol:       symbol,
		Action:       bestAction,
		Utility:      bestUtility,
		Alternatives: alternatives,
		Reason:       "calibrated executable-return utility",
	}

	thesis.AddEvidence(symbol, "decision", decision)

	if bestAction == ActionHold {
		return &Intent{
			Symbol:     symbol,
			Action:     bestAction,
			Edge:       *decimal.NewFromFloat64(bestUtility),
			Confidence: forecasts.Confidence,
			Thesis:     thesis,
		}
	}

	return &Intent{
		Symbol:     symbol,
		Action:     bestAction,
		Edge:       *decimal.NewFromFloat64(bestUtility),
		Confidence: forecasts.Confidence,
		Thesis:     thesis,
	}
}
