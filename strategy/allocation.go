package strategy

import (
	"context"
	"slices"
	"strings"
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

	// allocationLookahead is the bounded number of fully-priced candidates the
	// admission walk may inspect beyond the open slots: one slot's allocation
	// must not price the whole universe, but a temporarily infeasible strongest
	// candidate must not block the next candidate from being considered.
	allocationLookahead = 4
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
	confidence := 0.95

	config := system.Cfg.Snapshot()

	if config != nil && config.Regulator != nil &&
		config.Regulator.OptimizationConfidence > 0 &&
		config.Regulator.OptimizationConfidence < 1 {
		confidence = config.Regulator.OptimizationConfidence
	}

	excursion, ready := allocation.desk.PassageAdverseQuantile(confidence)

	return applyAdverseExcursion(multiples, excursion, ready)
}

/*
Calculate turns economically selected candidates into current executable
orders. It observes only present book depth, fees, capital, and risk geometry.
Whether a candidate deserves capital was decided by the causal MCTS economic
outcome; allocation enforces real constraints only and never re-ranks or
vetoes using semantic evidence.
*/
func (allocation *Allocation) Calculate(decisions []*types.Decision) error {
	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: planner configuration required",
			nil,
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

	normalSlots := allocation.desk.OpenSlots(false)
	reserveSlots := allocation.desk.OpenSlots(true) - normalSlots

	/*
		Arbitrate from the front of the economically ordered entry candidates,
		not by pricing the whole universe first. A candidate is fully priced
		(execution geometry, EntryCost, RiskPlan, stoploss) only while it is
		actually within reach of a free slot; candidates beyond the open slots
		plus a bounded infeasibility lookahead are marked unallocated without
		being priced. A temporarily infeasible strongest candidate does not
		block the next candidate: it is marked and the walk continues.
	*/
	entering := make([]*types.Decision, 0, len(decisions))
	occupied := occupiedSymbols(allocation.desk)

	for _, decision := range decisions {
		if decision == nil || decision.Action != types.ActionEnter {
			continue
		}

		// Only candidates whose causal entry advantage is strictly positive
		// may consume capital; a zero or negative marginal value over waiting
		// is not an entry, and the planner never forwards it as one.
		if decision.Alternatives["economic:enter_advantage"] <= 0 {
			decision.Action = types.ActionNothing
			decision.AllocationClass = "none"
			decision.Stoploss = nil
			decision.Reason = "planner: entry advantage is not positive"
			continue
		}

		if occupied[decision.Symbol] {
			decision.Action = types.ActionNothing
			decision.AllocationClass = "none"
			decision.Stoploss = nil
			decision.Reason = "planner: symbol already occupies a slot"
			continue
		}

		entering = append(entering, decision)
	}

	slices.SortFunc(entering, economicOrder)

	totalSlots := normalSlots + reserveSlots
	admitted := 0
	priced := 0
	eligible := make([]*types.Decision, 0, totalSlots)

	for _, decision := range entering {
		if admitted >= totalSlots {
			decision.Action = types.ActionNothing
			decision.AllocationClass = "none"
			decision.Stoploss = nil
			decision.Reason = "planner: no position slot available for allocation"
			continue
		}

		if priced >= totalSlots+allocationLookahead {
			decision.Action = types.ActionNothing
			decision.AllocationClass = "none"
			decision.Stoploss = nil
			decision.Reason = "planner: allocation lookahead exhausted; candidate not priced this pass"
			continue
		}

		priced++

		decision.OpenPositions = allocation.desk.OpenPositions()
		decision.SlotCapacity = allocation.desk.MaxPositions() + allocation.desk.MaxReserved()
		decision.AllocationClass = "unallocated"

		cash := allocation.desk.Balance().Cash()

		if cash == nil || cash.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: positive quote cash required"
			continue
		}

		notionalBudget := decimal.NewFromInt64(0).Add(cash).Mul(
			decimal.NewFromFloat64(config.Planner.MaxAllocationFraction),
		)
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

		// The candidate is executable now: consume the next slot in economic
		// rank order. Normal slots fill first, then reserve.
		if normalSlots > 0 {
			normalSlots--
			decision.AllocationClass = "normal"
		} else if reserveSlots > 0 {
			reserveSlots--
			decision.AllocationClass = "reserve"
		} else {
			decision.Action = types.ActionNothing
			decision.AllocationClass = "none"
			decision.Stoploss = nil
			decision.Reason = "planner: no position slot available for allocation"
			continue
		}

		admitted++
		eligible = append(eligible, decision)
	}

	return nil
}

/*
economicOrder ranks candidates by their incremental economic advantage over
waiting (Enter mean less Wait mean under the causal model), then by search
visits, then by symbol identity for replay determinism. Ranking by the selected
branch's raw expected outcome would let a symbol whose absolute reward is
inflated beat genuinely better alternatives; the advantage isolates the value
the candidate actually adds. No semantic score participates.
*/
func economicOrder(left, right *types.Decision) int {
	leftOutcome := alternativesOf(left)["economic:enter_advantage"]
	rightOutcome := alternativesOf(right)["economic:enter_advantage"]

	if leftOutcome != rightOutcome {
		if leftOutcome > rightOutcome {
			return -1
		}

		return 1
	}

	leftVisits := alternativesOf(left)["economic:visits"]
	rightVisits := alternativesOf(right)["economic:visits"]

	if leftVisits != rightVisits {
		if leftVisits > rightVisits {
			return -1
		}

		return 1
	}

	return strings.Compare(left.Symbol, right.Symbol)
}

/*
admitBest keeps the highest-expected-economic-outcome candidates that still
fit in open slots. Already-held symbols do not consume a slot another pair
could fill. Scarce capacity is allocated by expected economic outcome, never
by semantic score rank.
*/
func admitBest(
	decisions []*types.Decision,
	normalSlots int,
	reserveSlots int,
	occupied map[string]bool,
) {
	if occupied == nil {
		occupied = make(map[string]bool)
	}

	eligible := make([]*types.Decision, 0, len(decisions))

	for _, decision := range decisions {
		if decision == nil || decision.Action != types.ActionEnter {
			continue
		}

		if occupied[decision.Symbol] {
			decision.Action = types.ActionNothing
			decision.AllocationClass = "none"
			decision.Stoploss = nil
			decision.Reason = "planner: symbol already occupies a slot"
			continue
		}

		eligible = append(eligible, decision)
	}

	slices.SortFunc(eligible, economicOrder)

	for _, decision := range eligible {
		if occupied[decision.Symbol] {
			decision.Action = types.ActionNothing
			decision.AllocationClass = "none"
			decision.Stoploss = nil
			decision.Reason = "planner: symbol already occupies a slot"
			continue
		}

		if normalSlots > 0 {
			normalSlots--
			decision.AllocationClass = "normal"
			occupied[decision.Symbol] = true
			continue
		}

		if reserveSlots > 0 {
			reserveSlots--
			decision.AllocationClass = "reserve"
			occupied[decision.Symbol] = true
			continue
		}

		decision.Action = types.ActionNothing
		decision.AllocationClass = "none"
		decision.Stoploss = nil
		decision.Reason = "planner: no position slot available for allocation"
	}
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
