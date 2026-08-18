package strategy

import (
	"context"
	"slices"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
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
the evidence has actually observed winners survive. Excursions are stated in
the risk distances the finishing lots carried, so the derived multiple is the
excursion times the multiple those lots were entered under. An absent or
degenerate estimate leaves the default geometry untouched.
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
riskMultiples returns the stop geometry multiples for this entry: the
configured defaults, tightened to the calibrated winners' adverse-excursion
quantile once the desk's first-passage model has enough finished winners to
speak.
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
Calculate turns structurally admitted candidates into current executable orders.
It observes only present book depth, fees, capital, and risk geometry. Whether a
candidate deserves capital was decided by the evidence graph; allocation does
not invent a future midpoint to veto that conclusion.
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
	eligible := make([]*types.Decision, 0, len(decisions))

	for _, decision := range decisions {
		if decision == nil || decision.Action != types.ActionEnter {
			continue
		}

		decision.OpenPositions = allocation.desk.OpenPositions()
		decision.SlotCapacity = allocation.desk.MaxPositions() + allocation.desk.MaxReserved()
		decision.AllocationClass = "unallocated"

		if decision.Direction <= 0 || decision.ThesisScore <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: current structural thesis does not authorize a long entry"
			continue
		}

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

		executable, err := price.ExecutableQuantity(decision.Symbol, quantity)

		if err != nil || executable == nil || executable.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: current visible asks cannot fill an entry"

			if err != nil {
				decision.Reason += ": " + err.Error()
			}

			continue
		}

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

		// The venue refuses orders below its own stated minimum quantity and
		// cost. Refusing them here, after risk capping has settled the final
		// size, replaces an exchange rejection at execution time with an
		// observable decision reason.
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

		// Legacy forecast fields are deliberately cleared. The current execution
		// observation is carried by EntryCost and the structural case by Thesis*.
		decision.ExpectedReturn = nil
		decision.ExpectedFees = nil
		decision.ExpectedSpread = nil
		decision.ExpectedImpact = nil
		decision.PerspectiveReturn = 0
		decision.PerspectiveSources = nil
		decision.Utility = 0
		decision.OpportunityMargin = 0
		eligible = append(eligible, decision)
	}

	admitBest(eligible, normalSlots, reserveSlots, occupiedSymbols(allocation.desk))
	return nil
}

/*
admissionOrder ranks current candidates by structural thesis first and the
causal MCTS evidence path second. Equal evidence compares symbol identity so a
replay of the same state makes the same slot decision.
*/
func admissionOrder(left, right *types.Decision) int {
	if left.ThesisScore != right.ThesisScore {
		if left.ThesisScore > right.ThesisScore {
			return -1
		}

		return 1
	}

	if left.GraphScore != right.GraphScore {
		if left.GraphScore > right.GraphScore {
			return -1
		}

		return 1
	}

	if left.ForecastHorizon != right.ForecastHorizon && left.ForecastHorizon > 0 && right.ForecastHorizon > 0 {
		if left.ForecastHorizon < right.ForecastHorizon {
			return -1
		}

		return 1
	}

	return strings.Compare(left.Symbol, right.Symbol)
}

/*
admitBest keeps the highest-ranked candidates that still fit in open slots.
Already-held symbols do not consume a slot another pair could fill.
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

	slices.SortFunc(eligible, admissionOrder)

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

		if decision.ReserveEligible && decision.Opportunity && reserveSlots > 0 {
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
venueMinimumReason states why the venue would refuse the order as sized, or
returns an empty string when the venue's stated minimums admit it. Kraken
quotes a minimum quantity and a minimum total cost per pair; an order below
either is rejected by the exchange at execution time, so it is refused at
allocation with an observable reason instead.
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
