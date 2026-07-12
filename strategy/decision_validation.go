package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
Validate proves that a decision is a complete, internally consistent strategy
artifact. Broker must receive the chosen action exactly as evaluated, so the
journal cannot repair missing provenance or infer a selected utility.
*/
func (decision Decision) Validate() error {
	if decision.Symbol == "" || decision.At.IsZero() || decision.Reason == "" {
		return errnie.Err(
			errnie.Validation,
			"strategy decision: symbol, evaluation time, and reason required",
			nil,
		)
	}

	if !decision.Forecast.Eligible() {
		return errnie.Err(
			errnie.Validation,
			"strategy decision: complete eligible forecast required",
			nil,
		)
	}

	if decision.Symbol != decision.Forecast.Symbol ||
		decision.At.Before(decision.Forecast.At) ||
		!finiteUtility(decision.Utility) {
		return errnie.Err(
			errnie.Validation,
			"strategy decision: invalid evaluation time or selected utility",
			nil,
		)
	}

	return decision.validateAlternatives()
}

func (decision Decision) validateAlternatives() error {
	if len(decision.Alternatives) == 0 {
		return errnie.Err(
			errnie.Validation,
			"strategy decision: action alternatives required",
			nil,
		)
	}

	actions := make(map[Action]struct{}, len(decision.Alternatives))
	selected := false
	hold := false

	for _, alternative := range decision.Alternatives {
		if !validAction(alternative.Action) || !finiteUtility(alternative.Utility) {
			return errnie.Err(
				errnie.Validation,
				"strategy decision: invalid action alternative",
				nil,
			)
		}

		if _, duplicate := actions[alternative.Action]; duplicate {
			return errnie.Err(
				errnie.Validation,
				"strategy decision: duplicate action alternative",
				nil,
			)
		}

		actions[alternative.Action] = struct{}{}
		hold = hold || alternative.Action == ActionHold
		selected = selected ||
			(alternative.Action == decision.Action && alternative.Utility == decision.Utility)
	}

	if !hold || !selected {
		return errnie.Err(
			errnie.Validation,
			"strategy decision: hold and selected alternatives required",
			nil,
		)
	}

	return nil
}

func validAction(action Action) bool {
	switch action {
	case ActionHold, ActionBuy, ActionSell:
		return true
	}

	return false
}

func finiteUtility(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
