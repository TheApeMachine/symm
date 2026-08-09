package strategy

import (
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const allocationClassNormal = "normal"

/*
size adds execution quantity and forecast-derived protection to an entry.
*/
func (planner *Planner) size(
	decision *types.Decision,
) (*types.Decision, error) {
	if decision == nil || decision.Action != types.ActionEnter {
		return decision, nil
	}

	if planner.desk == nil || planner.maxFraction <= 0 || planner.maxFraction > 1 {
		return decision, errnie.Err(
			errnie.Validation,
			"planner: executable desk and allocation required",
			nil,
		)
	}

	if planner.desk.OpenSlots(decision.Opportunity) <= 0 {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: no position slot available for allocation"

		return decision, nil
	}

	cash := planner.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return decision, errnie.Err(
			errnie.Validation,
			"planner: positive quote cash required",
			nil,
		)
	}

	notional := decimal.ExactMul(
		cash,
		decimal.NewFromFloat64(planner.maxFraction),
	)
	price := planner.desk.Price()
	tick := price.Tick(decision.Symbol)

	if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 ||
		tick.Bid == nil || tick.Bid.Sign() <= 0 {
		return decision, errnie.Err(
			errnie.Validation,
			"planner: executable bid and ask required",
			nil,
		)
	}

	quantity := price.Quantity(decision.Symbol, notional)

	if quantity == nil || quantity.Sign() <= 0 {
		return decision, errnie.Err(
			errnie.Validation,
			"planner: allocation produced no executable quantity",
			nil,
		)
	}

	pair := planner.desk.Instrument().Pair(decision.Symbol)

	if pair.Symbol == "" || pair.TickSize.Sign() <= 0 {
		return decision, errnie.Err(
			errnie.Validation,
			"planner: instrument tick size required",
			nil,
		)
	}

	fee := price.Fee(decision.Symbol)

	if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
		return decision, errnie.Err(
			errnie.Validation,
			"planner: taker fee required",
			nil,
		)
	}

	feeRate := decimal.ExactDiv(fee.Fee, decimal.NewFromInt64(100))
	decision.AvailableCapital = cash
	decision.ProposedNotional = notional
	decision.ProposedQuantity = quantity
	decision.ReferencePrice = tick.Ask
	decision.EntryPrice = tick.Ask
	decision.Mark = tick.Bid

	economics, err := price.EntryEconomics(
		decision.Symbol,
		quantity,
		decision.Forecast.ExpectedReturn,
	)

	if err != nil {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: entry is not executable: " + err.Error()

		return decision, nil
	}

	decision.ExpectedReturn = economics.ExpectedReturn
	decision.ExpectedFees = economics.ExpectedFees
	decision.ExpectedSpread = economics.ExpectedSpread
	decision.ExpectedImpact = economics.ExpectedImpact
	decision.OpportunityMargin = economics.NetReturn.Float64()

	if economics.NetReturn.Sign() <= 0 {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: forecast does not clear current spread and taker fees"

		return decision, nil
	}

	stoploss, err := types.NewStoploss(
		planner.ctx,
		decision.Symbol,
		tick.Ask,
		tick.Bid,
		decision.Forecast,
		&pair.TickSize,
		feeRate,
		feeRate,
	)

	if err != nil {
		decision.Action = types.ActionNothing
		decision.Reason = "planner: forecast cannot construct an executable stop: " + err.Error()

		return decision, nil
	}

	decision.Stoploss = stoploss

	return decision, nil
}
