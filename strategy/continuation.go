package strategy

import (
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Continuity scores keep/reduce decisions for open wallet lots before entry
scoring. Full exits belong to Stoploss on Position via ticker updates.
*/
type Continuity struct {
	price   *broker.Price
	balance *broker.Balance
	rotate  Rotate
}

/*
NewContinuity wires price fees, wallet qty, and rotate keep-scores.
*/
func NewContinuity(
	price *broker.Price,
	balance *broker.Balance,
	rotate Rotate,
) Continuity {
	return Continuity{
		price:   price,
		balance: balance,
		rotate:  rotate,
	}
}

/*
Manage scores continuation for every open Balance lot.
*/
func (continuity Continuity) Manage(thesis *types.Thesis) {
	if err := continuity.validate(map[string]any{"thesis": thesis}); err != nil {
		return
	}

	forecasts := selectForecasts(thesis.Forecasts)

	for holding := range continuity.balance.Holdings() {
		lot := holding

		if lot.Status != types.OPEN {
			continue
		}

		if continuity.exiting(thesis, lot.Symbol) {
			continue
		}

		if lot.Mark == nil || lot.Qty == nil {
			continue
		}

		forecast, found := forecasts[lot.Symbol]

		if !found || !forecast.Eligible() {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action: types.ActionHold,
				Symbol: lot.Symbol,
				Cause:  "continuation",
				Reason: "awaiting eligible forecast for continuation scoring",
			})

			continue
		}

		fraction, err := continuity.price.Fraction(lot.Symbol)

		if err != nil {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action: types.ActionHold,
				Symbol: lot.Symbol,
				Cause:  "continuation",
				Reason: "fee schedule unavailable; refuse continuation score",
			})

			continue
		}

		decision := continuity.Score(forecast, fraction.Float64(), &lot)
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
	quantity := decimal.NewFromInt64(0)
	notional := continuity.notional(holding.Mark, holding.Qty)

	if forecast.SellCapacity != nil && forecast.SellCapacity.Sign() > 0 &&
		notional.Sign() > 0 && notional.Cmp(forecast.SellCapacity) > 0 {
		quantity = decimal.ExactDivFloor(
			forecast.SellCapacity,
			holding.Mark,
			holding.Qty.GetScale(),
		)
		scale := max(
			int64(decimal.DefaultScale),
			quantity.GetScale(),
			holding.Qty.GetScale(),
		)
		fraction := quantity.SetScale(scale).
			Div(holding.Qty.SetScale(scale)).
			Float64()
		reduce := fraction*exit + (1-fraction)*hold
		alternatives["reduce"] = reduce

		if reduce > utility {
			action = types.ActionReduce
			utility = reduce
			reason = "visible bid capacity supports reduction but not complete exit"
		}
	}

	proposedNotional := decimal.NewFromInt64(0)

	if action == types.ActionReduce {
		proposedNotional = continuity.notional(holding.Mark, quantity)
	}

	return types.Decision{
		Action:            action,
		Symbol:            forecast.Symbol,
		At:                forecast.At,
		Utility:           utility,
		Alternatives:      alternatives,
		ProposedNotional:  proposedNotional,
		ProposedQuantity:  quantity,
		ExpectedReturn:    decimal.NewFromFloat64(forecast.ExpectedReturn),
		ExpectedFees:      decimal.NewFromFloat64(fee),
		ExpectedSpread:    decimal.NewFromFloat64(forecast.ExpectedSpread / 2),
		ExpectedImpact:    decimal.NewFromFloat64(forecast.ExpectedImpact),
		AdverseSelection:  forecast.ExpectedAdverseSelection,
		Uncertainty:       forecast.Uncertainty,
		Confidence:        forecast.Confidence,
		ReferencePrice:    forecast.ReferencePrice.Copy(),
		ValidThroughEpoch: forecast.ExpiresEpoch,
		ForecastSource:    forecast.Source,
		ForecastEpoch:     forecast.SourceEpoch,
		Cause:             "continuation",
		Reason:            reason,
	}
}

/*
notional multiplies a finite fixed-point price and quantity at their combined
scale so fine quantity precision is not rounded to the price's coarser scale.
Integer products retain one fractional place to avoid the SDK's incorrect
scale-zero banker rounding.
*/
func (continuity Continuity) notional(
	price *decimal.Decimal,
	quantity *decimal.Decimal,
) *decimal.Decimal {
	return decimal.ExactMul(price, quantity)
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
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
