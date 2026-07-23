package strategy

import (
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Continuity scores hold decisions for open wallet lots before entry scoring.
Full exits belong to Stoploss.Regulate; positions are never partially reduced.
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
				Action:           types.ActionHold,
				Symbol:           lot.Symbol,
				Cause:            "continuation",
				Reason:           "awaiting eligible forecast for continuation scoring",
				ProposedQuantity: decimal.NewFromInt64(0),
				ProposedNotional: decimal.NewFromInt64(0),
				Alternatives:     map[string]float64{"hold": 0},
			})

			continue
		}

		fraction, err := continuity.price.Fraction(lot.Symbol)

		if err != nil {
			thesis.Decisions = append(thesis.Decisions, types.Decision{
				Action:           types.ActionHold,
				Symbol:           lot.Symbol,
				Cause:            "continuation",
				Reason:           "fee schedule unavailable; refuse continuation score",
				ProposedQuantity: decimal.NewFromInt64(0),
				ProposedNotional: decimal.NewFromInt64(0),
				Alternatives:     map[string]float64{"hold": 0},
			})

			continue
		}

		decision := continuity.Score(forecast, fraction.Float64(), &lot)
		decision.Cause = continuity.Cause(thesis, forecast)
		thesis.Decisions = append(thesis.Decisions, decision)
	}
}

/*
Score publishes keep-score for an open lot. Stoploss owns full exit; this path
never emits reduce or exit.
*/
func (continuity Continuity) Score(
	forecast types.Forecasts,
	fee float64,
	holding *types.Holding,
) types.Decision {
	hold := continuity.rotate.Hold(forecast)

	return types.Decision{
		Action:   types.ActionHold,
		Symbol:   forecast.Symbol,
		At:       forecast.At,
		Utility:  hold,
		Alternatives: map[string]float64{
			"hold": hold,
			"exit": -continuity.rotate.Exit(forecast, fee),
		},
		ProposedNotional:  decimal.NewFromInt64(0),
		ProposedQuantity:  decimal.NewFromInt64(0),
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
		Reason:            "stoploss owns full exit; continuation holds",
	}
}

/*
Cause identifies the evidence boundary behind a hold decision.
*/
func (continuity Continuity) Cause(
	thesis *types.Thesis,
	forecast types.Forecasts,
) string {
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

	return "continuation"
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
