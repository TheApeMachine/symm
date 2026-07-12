package strategy

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Alternative records the utility assigned to one available action.
The complete set is retained so a postmortem can compare the chosen action with
the alternatives that were available at evaluation time.
*/
type Alternative struct {
	Action  Action
	Utility float64
}

/*
Decision records one completed evaluation and its selected action.
The forecast provenance and alternatives are copied into the decision journal
so later evidence updates cannot rewrite the reasoning that produced it. At is
the deterministic logical evaluation time; the current synchronous entry path
evaluates at the forecast epoch, while broker records later wall-clock handling.
*/
type Decision struct {
	At           time.Time
	Symbol       string
	Action       Action
	Utility      float64
	Alternatives []Alternative
	Forecast     types.Forecasts
	Reason       string
}

func newDecision(forecasts types.Forecasts) (Decision, error) {
	utilityBuy := forecasts.ExecutableReturn()

	if !finiteUtility(utilityBuy) {
		return Decision{}, errnie.Err(
			errnie.Validation,
			"strategy decision: executable utility is not finite",
			nil,
		)
	}

	decision := Decision{
		At:      forecasts.At,
		Symbol:  forecasts.Symbol,
		Action:  ActionHold,
		Utility: 0.0,
		Alternatives: []Alternative{
			{Action: ActionBuy, Utility: utilityBuy},
			{Action: ActionHold, Utility: 0.0},
		},
		Forecast: forecasts,
		Reason:   "highest executable return after reported execution friction",
	}

	if utilityBuy > decision.Utility {
		decision.Action = ActionBuy
		decision.Utility = utilityBuy
	}

	return decision, nil
}

func (decision Decision) clone() Decision {
	decision.Alternatives = append([]Alternative(nil), decision.Alternatives...)

	return decision
}
