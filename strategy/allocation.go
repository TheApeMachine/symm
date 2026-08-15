package strategy

import (
	"context"
	"slices"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
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
size adds execution quantity and forecast-derived protection to an entry.
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

	hasEntry := false

	for _, decision := range decisions {
		if decision.Action == types.ActionEnter {
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
		if decision.Action != types.ActionEnter {
			continue
		}

		cash := allocation.desk.Balance().Cash()

		if cash == nil || cash.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: positive quote cash required"

			errnie.Err(
				errnie.Validation,
				"planner: positive quote cash required",
				nil,
			)

			continue
		}

		notional := decimal.NewFromInt64(0).Add(cash).Mul(
			decimal.NewFromFloat64(config.Planner.MaxAllocationFraction),
		)
		price := allocation.desk.Price()
		tick := price.Tick(decision.Symbol)

		if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 ||
			tick.Bid == nil || tick.Bid.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: executable bid and ask required"

			errnie.Err(
				errnie.Validation,
				"planner: executable bid and ask required",
				nil,
			)

			continue
		}

		quantity := price.Quantity(decision.Symbol, notional)

		if quantity == nil || quantity.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: allocation produced no executable quantity"

			errnie.Err(
				errnie.Validation,
				"planner: allocation produced no executable quantity",
				nil,
			)

			continue
		}

		executableQuantity, err := price.ProfitableQuantity(
			decision.Symbol,
			quantity,
			decision.PerspectiveReturn,
			decision.AdmissionUtilityThreshold,
		)

		if err != nil {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: executable quantity unavailable: " + err.Error()
			continue
		}

		quantity = executableQuantity

		pair := allocation.desk.Instrument().Pair(decision.Symbol)

		if pair.Symbol == "" || pair.TickSize.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: instrument tick size required"

			errnie.Err(
				errnie.Validation,
				"planner: instrument tick size required",
				nil,
			)

			continue
		}

		fee := price.Fee(decision.Symbol)

		if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: taker fee required"

			errnie.Err(
				errnie.Validation,
				"planner: taker fee required",
				nil,
			)

			continue
		}

		feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(
			decimal.NewFromInt64(100),
		)
		decision.AvailableCapital = cash
		decision.ProposedNotional = price.Mark(decision.Symbol, broker.BUY).Mul(quantity)
		decision.ProposedQuantity = quantity
		decision.ReferencePrice = tick.Ask
		decision.EntryPrice = tick.Ask
		decision.Mark = tick.Bid

		economics, err := price.EntryEconomics(
			decision.Symbol,
			quantity,
			decision.PerspectiveReturn,
		)

		if err != nil {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: entry is not executable: " + err.Error()

			continue
		}

		riskMidpoint := decimal.NewFromInt64(0).Add(tick.Ask).Add(tick.Bid).Div(
			decimal.NewFromInt64(2),
		)

		if economics != nil {
			decision.ExpectedReturn = economics.ExpectedReturn
			decision.ExpectedFees = economics.ExpectedFees
			decision.ExpectedSpread = economics.ExpectedSpread
			decision.ExpectedImpact = economics.ExpectedImpact
			decision.OpportunityMargin = economics.NetReturn.Float64()
			decision.Utility = economics.NetReturn.Float64()

			if economics.Midpoint != nil && economics.Midpoint.Sign() > 0 {
				riskMidpoint = economics.Midpoint
			}
		}

		if decision.Utility <= decision.AdmissionUtilityThreshold {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: net forecast utility does not clear regulated entry threshold"

			continue
		}

		riskPlan := types.NewRiskPlan(types.RiskInputs{
			ReferencePrice: tick.Ask,
			// EntryEconomics reports these in midpoint-return units. RiskPlan
			// consumes price distances, so restore the common price unit here.
			Spread:       priceDistance(riskMidpoint, decision.ExpectedSpread),
			Impact:       priceDistance(riskMidpoint, decision.ExpectedImpact),
			TickSize:     &pair.TickSize,
			ExitFeeRate:  feeRate,
			EntryFeeRate: feeRate,
			MaxLoss:      notional,
			Multiples:    types.DefaultRiskMultiples(),
		})
		decision.Risk = riskPlan

		horizon := decision.ForecastHorizon

		if len(decision.ForwardCurve) > 0 {
			horizon = len(decision.ForwardCurve)
		}

		stoploss, err := types.NewStoplossWithPlan(
			allocation.ctx,
			decision.Symbol,
			tick.Ask,
			tick.Bid,
			decision.Forecast,
			horizon,
			&pair.TickSize,
			feeRate,
			feeRate,
			&riskPlan,
		)

		if err != nil {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: forecast cannot construct an executable stop: " + err.Error()

			continue
		}

		decision.Stoploss = stoploss
		eligible = append(eligible, decision)
	}

	admitBest(eligible, normalSlots, reserveSlots, occupiedSymbols(allocation.desk))

	return nil
}

/*
admissionOrder ranks fill candidates by the regulator-owned gates: net
utility first, graph score second. Equal scores compare symbol identity so
the same set always fills the same slots.
*/
func admissionOrder(left, right *types.Decision) int {
	if left.Utility != right.Utility {
		if left.Utility > right.Utility {
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
			decision.Stoploss = nil
			decision.Reason = "planner: symbol already occupies a slot"
			continue
		}

		if normalSlots > 0 {
			normalSlots--
			occupied[decision.Symbol] = true
			continue
		}

		if decision.Opportunity && reserveSlots > 0 {
			reserveSlots--
			occupied[decision.Symbol] = true
			continue
		}

		decision.Action = types.ActionNothing
		decision.Stoploss = nil
		decision.Reason = "planner: no position slot available for allocation"
	}
}

/*
priceDistance converts a dimensionless midpoint-return fraction back into the
price distance RiskPlan expects. Entry economics and risk geometry deliberately
keep their units explicit rather than relying on similarly sized decimals.
*/
func priceDistance(
	reference *decimal.Decimal,
	fraction *decimal.Decimal,
) *decimal.Decimal {
	if reference == nil || reference.Sign() <= 0 || fraction == nil || fraction.Sign() < 0 {
		return nil
	}

	return decimal.NewFromInt64(0).Add(reference).Mul(fraction)
}

/*
visibleAskQuantity keeps a cash-sized request inside the ticker's observable
ask. A deeper book walk already ran when return sources exist; this is the
ticker-only bound so a missing L3 book cannot admit more than it can fill.
*/
func visibleAskQuantity(askQty float64, requested *decimal.Decimal) *decimal.Decimal {
	if requested == nil || requested.Sign() <= 0 || askQty <= 0 {
		return requested
	}

	visible := decimal.NewFromFloat64(askQty)

	if requested.Cmp(visible) <= 0 {
		return requested
	}

	return visible
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
