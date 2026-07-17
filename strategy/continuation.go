package strategy

import "github.com/theapemachine/symm/types"

/*
continuation compares hold against liquidity-forced reduction only. Full exits
belong to Stoploss (stop / take_profit); Decide must not compete on expected-
return color once inventory is live.
*/
func (planner *Planner) continuation(
	forecast types.Forecasts,
	fee float64,
	holding types.Holding,
) types.Decision {
	hold := forecast.ExpectedReturn
	exit := -(fee + forecast.ExpectedSpread/2)
	action := "hold"
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
			action = "reduce"
			utility = reduce
			quantity = qty * fraction
			reason = "visible bid capacity supports reduction but not complete exit"
		}
	}

	return types.Decision{
		Action:            action,
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      alternatives,
		ProposedQuantity:  quantity,
		ExpectedFees:      fee,
		ExpectedSpread:    forecast.ExpectedSpread / 2,
		ReferencePrice:    forecast.ReferencePrice,
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
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
	action string,
) string {
	if action == "hold" {
		return "continuation"
	}

	if action == "reduce" {
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

		if decision.Symbol == forecast.Symbol && decision.Action == "enter" &&
			forecast.SourceEpoch >= decision.ValidThroughEpoch {
			return "thesis_invalidation"
		}
	}

	return "thesis_weakening"
}
