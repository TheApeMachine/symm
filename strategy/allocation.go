package strategy

import (
	"math"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

const (
	allocationClassNormal   = "normal"
	allocationClassReserved = "reserved"
)

/*
classifyAllocation reserves overflow capacity for return-supported entries
whose cognitive basin is ahead of the physical field. The forecast's worst
intermediate drawdown is the return-denominated uncertainty term; model-space
residuals and confidence probabilities are not mixed with returns.
*/
func (planner *Planner) classifyAllocation(
	thesis *types.Thesis,
	decision *types.Decision,
) error {
	if thesis == nil || decision == nil || decision.Forecast == nil {
		return errnie.Err(
			errnie.Validation,
			"planner: thesis, decision, and forecast required for allocation",
			nil,
		)
	}

	uncertainty, err := decision.Forecast.WorstIntermediateDrawdown()

	if err != nil {
		return errnie.Err(
			errnie.Validation,
			"planner: could not derive allocation uncertainty",
			err,
		)
	}

	stored, found := thesis.Cognition.Load(decision.Symbol)
	cognition, valid := stored.(types.Cognition)

	if !found || !valid || !cognition.Ready {
		return errnie.Err(
			errnie.Validation,
			"planner: ready cognition required for "+decision.Symbol+" allocation",
			nil,
		)
	}

	coherence := thesis.Manifold.CoherenceMag2

	if math.IsNaN(cognition.Confidence) || math.IsInf(cognition.Confidence, 0) ||
		math.IsNaN(coherence) || math.IsInf(coherence, 0) {
		return errnie.Err(
			errnie.Validation,
			"planner: finite cognition and coherence required for allocation",
			nil,
		)
	}

	decision.Uncertainty = uncertainty
	decision.OpportunityMargin = decision.Forecast.ExpectedReturn - uncertainty
	decision.CognitiveLead = cognition.Confidence - coherence
	decision.BasinConfidence = cognition.Confidence
	decision.AllocationClass = allocationClassNormal
	decision.Opportunity = false

	if decision.OpportunityMargin <= 0 || decision.CognitiveLead <= 0 {
		return nil
	}

	decision.AllocationClass = allocationClassReserved
	decision.Opportunity = true

	return nil
}

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
		return decision, errnie.Err(
			errnie.Validation,
			"planner: could not construct strategy stoploss",
			err,
		)
	}

	decision.AvailableCapital = cash
	decision.ProposedNotional = notional
	decision.ProposedQuantity = quantity
	decision.ReferencePrice = tick.Ask
	decision.EntryPrice = tick.Ask
	decision.Mark = tick.Bid
	decision.Stoploss = stoploss

	return decision, nil
}
