package strategy

import (
	"context"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

const (
	executionCoverageKey = "execution:visible_coverage"
	executionFrictionKey = "execution:friction_fraction"
	executionSpreadKey   = "execution:spread_fraction"
	executionImpactKey   = "execution:impact_fraction"
)

type Allocation struct {
	ctx    context.Context
	cancel context.CancelFunc
	desk   *broker.Desk
}

func NewAllocation(
	ctx context.Context,
	desk *broker.Desk,
) *Allocation {
	ctx, cancel := context.WithCancel(ctx)

	return &Allocation{
		ctx:    ctx,
		cancel: cancel,
		desk:   desk,
	}
}

/*
applyAdverseExcursion tightens the assumed risk multiple toward the excursion
the evidence has actually observed winners survive. An absent or degenerate
estimate leaves the default geometry untouched.
*/
func applyAdverseExcursion(
	multiples types.RiskMultiples,
	excursion float64,
	ready bool,
) types.RiskMultiples {
	if !ready || !(excursion > 0) || multiples.Risk <= 0 {
		return multiples
	}

	multiples.Risk = excursion * multiples.Risk

	return multiples
}

/*
riskMultiples returns the stop geometry multiples for this entry.
*/
func (allocation *Allocation) riskMultiples() types.RiskMultiples {
	multiples := types.DefaultRiskMultiples()
	confidence, err := system.Cfg.OptimizationConfidence()

	if err != nil || confidence <= 0 || confidence >= 1 {
		return multiples
	}

	excursion, ready := allocation.desk.PassageAdverseQuantile(confidence)

	return applyAdverseExcursion(multiples, excursion, ready)
}

/*
Calculate independently prices every admitted entry against current cash,
visible depth, fees, venue limits, and risk geometry. It preserves stream order
and never selects a winner or imposes a position-slot limit.
*/
func (allocation *Allocation) Calculate(decisions []*types.Decision) error {
	config, err := system.Cfg.PlannerPolicy()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: planner configuration required",
			err,
		))
	}

	multiples := allocation.riskMultiples()

	hasEntry := false

	for _, decision := range decisions {
		if decision != nil && decision.Action == types.ActionEnter {
			hasEntry = true
			break
		}
	}

	if !hasEntry {
		return nil
	}

	if allocation.desk == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: broker desk required for entry allocation",
			nil,
		))
	}

	if config.MaxAllocationFraction <= 0 || config.MaxAllocationFraction > 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: allocation fraction must be in (0, 1]",
			nil,
		))
	}

	cash := allocation.desk.Balance().Cash()

	if cash == nil || cash.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"planner: positive quote cash required",
			nil,
		))
	}

	entryLimit := decimal.NewFromInt64(0).Add(cash).Mul(
		decimal.NewFromFloat64(config.MaxAllocationFraction),
	)
	remainingCash := decimal.NewFromInt64(0).Add(cash)
	occupied := occupiedSymbols(allocation.desk)

	for _, decision := range decisions {
		if decision == nil || decision.Action != types.ActionEnter {
			continue
		}

		decision.OpenPositions = allocation.desk.OpenPositions()
		decision.SlotCapacity = 0
		decision.AllocationClass = "none"

		if occupied[decision.Symbol] {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: symbol already has an active position"
			continue
		}

		if remainingCash.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: no quote cash remains for entry"
			continue
		}

		notionalBudget := decimal.NewFromInt64(0).Add(entryLimit)

		if remainingCash.Cmp(notionalBudget) < 0 {
			notionalBudget = decimal.NewFromInt64(0).Add(remainingCash)
		}

		price := allocation.desk.Price()
		tick := price.Tick(decision.Symbol)

		if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 ||
			tick.Bid == nil || tick.Bid.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: executable bid and ask required"
			continue
		}

		quantity := price.Quantity(decision.Symbol, notionalBudget)

		if quantity == nil || quantity.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: allocation produced no executable quantity"
			continue
		}

		requestedQuantity := decimal.NewFromInt64(0).Add(quantity)
		executable, err := price.ExecutableQuantity(decision.Symbol, quantity)

		if err != nil || executable == nil || executable.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: current visible asks cannot fill an entry"

			if err != nil {
				decision.Reason += ": " + err.Error()
			}

			continue
		}

		coverage := decimal.NewFromInt64(0).Add(executable).Div(requestedQuantity).Float64()
		alternativesOf(decision)[executionCoverageKey] = min(1, max(0, coverage))
		quantity = executable
		pair := allocation.desk.Instrument().Pair(decision.Symbol)

		if pair.Symbol == "" || pair.TickSize.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: instrument tick size required"
			continue
		}

		fee := price.Fee(decision.Symbol)

		if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
			fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: valid taker fee required"
			continue
		}

		feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(
			decimal.NewFromInt64(100),
		)
		cost, err := price.EntryCost(decision.Symbol, quantity)

		if err != nil {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: current entry cannot be priced: " + err.Error()
			continue
		}

		recordExecutionFriction(decision, cost)
		riskPlan := types.NewRiskPlan(types.RiskInputs{
			ReferencePrice: cost.EntryPrice,
			Spread:         cost.Spread,
			Impact:         cost.Impact,
			TickSize:       &pair.TickSize,
			ExitFeeRate:    feeRate,
			EntryFeeRate:   feeRate,
			MaxLoss:        notionalBudget,
			Multiples:      multiples,
		})

		if !riskPlan.Present {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: current execution geometry cannot support a risk plan"
			continue
		}

		if riskQuantity := riskPlan.MaxQuantity(cost.EntryPrice); riskQuantity != nil &&
			riskQuantity.Sign() > 0 && riskQuantity.Cmp(quantity) < 0 {
			quantity = riskQuantity
			cost, err = price.EntryCost(decision.Symbol, quantity)

			if err != nil {
				decision.Action = types.ActionNothing
				decision.Reason = "planner: risk-capped entry cannot be priced: " + err.Error()
				continue
			}

			recordExecutionFriction(decision, cost)
			riskPlan = types.NewRiskPlan(types.RiskInputs{
				ReferencePrice: cost.EntryPrice,
				Spread:         cost.Spread,
				Impact:         cost.Impact,
				TickSize:       &pair.TickSize,
				ExitFeeRate:    feeRate,
				EntryFeeRate:   feeRate,
				MaxLoss:        notionalBudget,
				Multiples:      multiples,
			})

			if !riskPlan.Present {
				decision.Action = types.ActionNothing
				decision.Reason = "planner: risk-capped execution geometry is invalid"
				continue
			}
		}

		if reason := venueMinimumReason(pair, quantity, cost.GrossNotional); reason != "" {
			decision.Action = types.ActionNothing
			decision.Reason = reason
			continue
		}

		horizon := max(0, decision.ForecastHorizon)
		stoploss, err := types.NewStoplossWithPlan(
			allocation.ctx,
			decision.Symbol,
			cost.EntryPrice,
			cost.BestBid,
			decision.Forecast,
			horizon,
			&pair.TickSize,
			feeRate,
			feeRate,
			&riskPlan,
			time.Now(),
		)

		if err != nil {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: current risk plan cannot construct a stop: " + err.Error()
			continue
		}

		decision.AvailableCapital = decimal.NewFromInt64(0).Add(cash)
		decision.ProposedQuantity = decimal.NewFromInt64(0).Add(quantity)
		decision.ProposedNotional = decimal.NewFromInt64(0).Add(cost.GrossNotional).Add(
			cost.EntryFee,
		)
		decision.ReferencePrice = decimal.NewFromInt64(0).Add(cost.BestAsk)
		decision.EntryPrice = decimal.NewFromInt64(0).Add(cost.EntryPrice)
		decision.Mark = decimal.NewFromInt64(0).Add(cost.BestBid)
		decision.EntryCost = cost
		decision.Risk = riskPlan
		decision.Stoploss = stoploss

		decision.AllocationClass = "capital"
		occupied[decision.Symbol] = true
		remainingCash = decimal.NewFromInt64(0).Add(remainingCash).Sub(
			decision.ProposedNotional,
		)
	}

	return nil
}

/*
recordExecutionFriction expresses current spread and depth impact as fractions
of the executable entry price. These are unitless and directly comparable
across symbols; no future price or arbitrary liquidity multiplier is invented.
*/
func recordExecutionFriction(decision *types.Decision, cost *types.EntryCost) {
	if decision == nil || cost == nil || cost.EntryPrice == nil ||
		cost.EntryPrice.Sign() <= 0 || cost.Spread == nil || cost.Impact == nil {
		return
	}

	entry := cost.EntryPrice.Float64()
	spread := max(0, cost.Spread.Float64()/entry)
	impact := max(0, cost.Impact.Float64()/entry)
	alternatives := alternativesOf(decision)
	alternatives[executionSpreadKey] = spread
	alternatives[executionImpactKey] = impact
	alternatives[executionFrictionKey] = spread + impact
}

/*
venueMinimumReason states why the venue would refuse the order as sized, or
returns an empty string when the venue's stated minimums admit it.
*/
func venueMinimumReason(
	pair kraken.InstrumentPair,
	quantity *decimal.Decimal,
	grossNotional *decimal.Decimal,
) string {
	if pair.QtyMin != nil && pair.QtyMin.Sign() > 0 &&
		quantity.Cmp(pair.QtyMin) < 0 {
		return "planner: quantity is below the venue minimum order size"
	}

	if pair.CostMin != nil && pair.CostMin.Sign() > 0 &&
		grossNotional.Cmp(pair.CostMin) < 0 {
		return "planner: notional is below the venue minimum order cost"
	}

	return ""
}

func occupiedSymbols(desk *broker.Desk) map[string]bool {
	occupied := make(map[string]bool)

	if desk == nil {
		return occupied
	}

	for position := range desk.Positions() {
		if position == nil || position.Decision.Symbol == "" {
			continue
		}

		occupied[position.Decision.Symbol] = true
	}

	return occupied
}

/*
alternativesOf returns the decision's alternatives map, creating it when
absent. A nil decision has no alternatives map and returns nil.
*/
func alternativesOf(decision *types.Decision) map[string]float64 {
	if decision == nil {
		return nil
	}

	if decision.Alternatives == nil {
		decision.Alternatives = make(map[string]float64)
	}

	return decision.Alternatives
}
