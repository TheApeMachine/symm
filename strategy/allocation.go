package strategy

import (
	"context"
	"slices"

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

	slices.SortFunc(decisions, func(left, right *types.Decision) int {
		if left.Utility == right.Utility {
			return 0
		}

		if left.Utility > right.Utility {
			return -1
		}

		return 1
	})

	normalSlots := allocation.desk.OpenSlots(false)
	reserveSlots := allocation.desk.OpenSlots(true) - normalSlots

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
			decision.Forecast.Value,
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
			decision.Forecast.Value,
		)

		if err != nil {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: entry is not executable: " + err.Error()

			continue
		}

		decision.ExpectedReturn = economics.ExpectedReturn
		decision.ExpectedFees = economics.ExpectedFees
		decision.ExpectedSpread = economics.ExpectedSpread
		decision.ExpectedImpact = economics.ExpectedImpact
		decision.OpportunityMargin = economics.NetReturn.Float64()

		if economics.NetReturn.Sign() <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: forecast does not clear current spread and taker fees"

			continue
		}

		stoploss, err := types.NewStoploss(
			allocation.ctx,
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

			continue
		}

		availableSlots := normalSlots

		if decision.Opportunity {
			availableSlots += reserveSlots
		}

		if availableSlots <= 0 {
			decision.Action = types.ActionNothing
			decision.Reason = "planner: no position slot available for allocation"

			continue
		}

		if normalSlots > 0 {
			normalSlots--
			decision.Stoploss = stoploss

			continue
		}

		reserveSlots--
		decision.Stoploss = stoploss
	}

	return nil
}
