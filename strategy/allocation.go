package strategy

import (
	"context"

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
func (allocation *Allocation) Calculate(thesis *types.Thesis) error {
	config := system.Cfg.Snapshot()

	if config == nil || config.Planner == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"planner: planner configuration required",
			nil,
		))
	}

	thesis.Symbols.Range(func(_, value any) bool {
		symbol := value.(*types.Symbol)

		symbol.Decisions.Range(func(_, value any) bool {
			decision := value.(*types.Decision)

			if decision.Action != types.ActionEnter {
				return true
			}

			if allocation.desk.OpenSlots(decision.Opportunity) <= 0 {
				decision.Action = types.ActionNothing
				decision.Reason = "planner: no position slot available for allocation"

				return true
			}

			cash := allocation.desk.Balance().Cash()

			if cash == nil || cash.Sign() <= 0 {
				errnie.Err(
					errnie.Validation,
					"planner: positive quote cash required",
					nil,
				)

				return true
			}

			notional := decimal.NewFromInt64(0).Add(cash).Mul(
				decimal.NewFromFloat64(config.Planner.MaxAllocationFraction),
			)
			price := allocation.desk.Price()
			tick := price.Tick(decision.Symbol)

			if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 ||
				tick.Bid == nil || tick.Bid.Sign() <= 0 {
				errnie.Err(
					errnie.Validation,
					"planner: executable bid and ask required",
					nil,
				)

				return true
			}

			quantity := price.Quantity(decision.Symbol, notional)

			if quantity == nil || quantity.Sign() <= 0 {
				errnie.Err(
					errnie.Validation,
					"planner: allocation produced no executable quantity",
					nil,
				)

				return true
			}

			pair := allocation.desk.Instrument().Pair(decision.Symbol)

			if pair.Symbol == "" || pair.TickSize.Sign() <= 0 {
				errnie.Err(
					errnie.Validation,
					"planner: instrument tick size required",
					nil,
				)

				return true
			}

			fee := price.Fee(decision.Symbol)

			if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 {
				errnie.Err(
					errnie.Validation,
					"planner: taker fee required",
					nil,
				)

				return true
			}

			feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(
				decimal.NewFromInt64(100),
			)
			decision.AvailableCapital = cash
			decision.ProposedNotional = notional
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

				return true
			}

			decision.ExpectedReturn = economics.ExpectedReturn
			decision.ExpectedFees = economics.ExpectedFees
			decision.ExpectedSpread = economics.ExpectedSpread
			decision.ExpectedImpact = economics.ExpectedImpact
			decision.OpportunityMargin = economics.NetReturn.Float64()

			if economics.NetReturn.Sign() <= 0 {
				decision.Action = types.ActionNothing
				decision.Reason = "planner: forecast does not clear current spread and taker fees"

				return true
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

				return true
			}

			decision.Stoploss = stoploss

			return true
		})

		return true
	})

	return nil
}
