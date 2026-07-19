package strategy

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
manage projects stop evidence and continuation for every open holding before
entry scoring. Stoploss owns full exits; continuation may only hold or reduce.
*/
func (planner *Planner) manage(thesis *types.Thesis) {
	if thesis == nil || planner.price == nil {
		return
	}

	forecasts := make(map[string]types.Forecasts, len(thesis.Forecasts))

	for _, forecast := range thesis.Forecasts {
		forecasts[forecast.Symbol] = forecast
	}

	thesis.Holdings.Range(func(_, value any) bool {
		holding, ok := value.(*types.Holding)

		if !ok || holding == nil || holding.Status == types.CLOSED {
			return true
		}

		if holding.Stoploss != nil {
			holding.Stoploss.Regulate(thesis, *holding, Project(thesis, *holding))
		}

		if exiting(thesis, holding.Symbol) {
			return true
		}

		forecast, found := forecasts[holding.Symbol]

		if !found || holding.Mark == nil || holding.Qty == nil {
			return true
		}

		if !forecast.Eligible() {
			return true
		}

		fee := 0.0

		if fraction, err := planner.price.Fraction(holding.Symbol); err == nil {
			fee = fraction.Float64()
		}

		decision := planner.continuation(forecast, fee, holding)
		decision.Cause = planner.cause(thesis, forecast, decision.Action)
		thesis.Decisions = append(thesis.Decisions, decision)

		return true
	})
}

func exiting(thesis *types.Thesis, symbol string) bool {
	phase, found := thesis.Lifecycle.Load(symbol)

	return found &&
		(phase == types.LifecycleExitSelected ||
			phase == types.LifecycleExitSubmitted)
}

/*
continuation compares keep-score against liquidity-forced reduction only. Full
exits belong to Stoploss. Keep-score is expected return net of uncertainty —
the same holdUtility rotate uses — never raw predicted return.
*/
func (planner *Planner) continuation(
	forecast types.Forecasts,
	fee float64,
	holding *types.Holding,
) types.Decision {
	hold := planner.holdUtility(forecast)
	exit := -planner.exitCost(forecast, fee)
	action := types.ActionHold
	utility := hold
	reason := "stoploss owns full exit; continuation holds"
	alternatives := map[string]float64{"hold": hold}
	quantity := 0.0
	mark := holding.Mark.Float64()
	qty := holding.Qty.Float64()
	notional := mark * qty

	if notional > forecast.SellCapacity && notional > 0 && forecast.SellCapacity > 0 {
		fraction := forecast.SellCapacity / notional
		reduce := fraction*exit + (1-fraction)*hold
		alternatives["reduce"] = reduce

		if reduce > utility {
			action = types.ActionReduce
			utility = reduce
			quantity = qty * fraction
			reason = "visible bid capacity supports reduction but not complete exit"
		}
	}

	proposedNotional := 0.0

	if action == types.ActionReduce {
		proposedNotional = mark * quantity
	}

	return types.Decision{
		Action:            action,
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      alternatives,
		ProposedNotional:  decimal.NewFromFloat64(proposedNotional),
		ProposedQuantity:  decimal.NewFromFloat64(quantity),
		ExpectedReturn:    decimal.NewFromFloat64(forecast.ExpectedReturn),
		ExpectedFees:      decimal.NewFromFloat64(fee),
		ExpectedSpread:    decimal.NewFromFloat64(forecast.ExpectedSpread / 2),
		ExpectedImpact:    decimal.NewFromFloat64(forecast.ExpectedImpact),
		AdverseSelection:  forecast.ExpectedAdverseSelection,
		Uncertainty:       forecast.Uncertainty,
		Confidence:        forecast.Confidence,
		ReferencePrice:    decimal.NewFromFloat64(forecast.ReferencePrice),
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		ForecastEpoch:     forecast.SourceEpoch,
		Cause:             "continuation",
		Reason:            reason,
	}
}

/*
cause identifies the evidence boundary behind a management action. A ready
negative causal outcome is opposing-thesis formation; an elapsed entry forecast
is invalidation; a negative current forecast without either is weakening.
*/
func (planner *Planner) cause(
	thesis *types.Thesis,
	forecast types.Forecasts,
	action types.Action,
) string {
	if action == types.ActionHold {
		return "continuation"
	}

	if action == types.ActionReduce {
		return "liquidity_deterioration"
	}

	for index := len(thesis.Hypotheses) - 1; index >= 0; index-- {
		hypothesis := thesis.Hypotheses[index]

		if hypothesis.Symbol == forecast.Symbol && hypothesis.Ready &&
			hypothesis.Outcome == forecast.Target && hypothesis.DoExpectation < 0 &&
			hypothesis.Uplift < 0 {
			return "opposing_thesis"
		}
	}

	for index := len(thesis.Decisions) - 1; index >= 0; index-- {
		decision := thesis.Decisions[index]

		if decision.Symbol == forecast.Symbol &&
			decision.Action == types.ActionEnter &&
			forecast.SourceEpoch >= decision.ValidThroughEpoch {
			return "thesis_invalidation"
		}
	}

	return "thesis_weakening"
}
