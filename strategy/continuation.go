package strategy

import (
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Continuity projects stop evidence and keep/reduce decisions for open wallet
lots before entry scoring. Stoploss lives on Position; Continuity may only
hold or reduce.
*/
type Continuity struct {
	price    *broker.Price
	balance  *broker.Balance
	desk     *broker.Desk
	rotate   Rotate
	evidence Evidence
}

/*
NewContinuity wires price fees, wallet qty, Desk positions, rotate keep-scores,
and stop evidence.
*/
func NewContinuity(
	price *broker.Price,
	balance *broker.Balance,
	desk *broker.Desk,
	rotate Rotate,
	evidence Evidence,
) Continuity {
	return Continuity{
		price:    price,
		balance:  balance,
		desk:     desk,
		rotate:   rotate,
		evidence: evidence,
	}
}

/*
Manage projects stop evidence and continuation for every open Balance lot.
*/
func (continuity Continuity) Manage(thesis *types.Thesis) {
	if err := continuity.validate(map[string]any{"thesis": thesis}); err != nil {
		return
	}

	forecasts := make(map[string]types.Forecasts, len(thesis.Forecasts))

	for _, forecast := range thesis.Forecasts {
		forecasts[forecast.Symbol] = forecast
	}

	for holding := range continuity.balance.Holdings() {
		lot := holding

		if lot.Status == types.CLOSED {
			continue
		}

		if position, ok := continuity.desk.Position(lot.Symbol); ok {
			if stop := position.Stop(); stop != nil {
				stop.Regulate(
					thesis, lot, continuity.evidence.Project(thesis, lot),
				)
			}
		}

		if continuity.exiting(thesis, lot.Symbol) {
			continue
		}

		forecast, found := forecasts[lot.Symbol]

		if !found || lot.Mark == nil || lot.Qty == nil {
			continue
		}

		if !forecast.Eligible() {
			continue
		}

		fee := 0.0

		if fraction, err := continuity.price.Fraction(lot.Symbol); err == nil {
			fee = fraction.Float64()
		}

		decision := continuity.Score(forecast, fee, &lot)
		decision.Cause = continuity.Cause(thesis, forecast, decision.Action)
		thesis.Decisions = append(thesis.Decisions, decision)
	}
}

/*
Score compares keep-score against liquidity-forced reduction only. Full exits
belong to Stoploss. Keep-score is expected return net of uncertainty — the same
Hold rotate uses — never raw predicted return.
*/
func (continuity Continuity) Score(
	forecast types.Forecasts,
	fee float64,
	holding *types.Holding,
) types.Decision {
	hold := continuity.rotate.Hold(forecast)
	exit := -continuity.rotate.Exit(forecast, fee)
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
Cause identifies the evidence boundary behind a management action.
*/
func (continuity Continuity) Cause(
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

func (continuity Continuity) exiting(thesis *types.Thesis, symbol string) bool {
	phase, found := thesis.Lifecycle.Load(symbol)

	return found &&
		(phase == types.LifecycleExitSelected ||
			phase == types.LifecycleExitSubmitted)
}

func (continuity Continuity) validate(mandatory map[string]any) error {
	check := map[string]any{
		"price":   continuity.price,
		"balance": continuity.balance,
		"desk":    continuity.desk,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
