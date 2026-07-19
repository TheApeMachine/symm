package strategy

import (
	"context"
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Allocator sizes enter decisions as max_fraction of Available quote cash,
shrunk by the decision risk factor (0–1). Rotation challengers Book the
incumbent notional stamped by Arbiter instead of inventing a new slice.
Action demotion belongs to Planner after Allocate.
*/
type Allocator struct {
	ctx         context.Context
	cancel      context.CancelFunc
	balance     *broker.Balance
	instrument  *broker.Instrument
	price       *broker.Price
	maxFraction *decimal.Decimal
}

/*
NewAllocator wires Balance, Instrument, and Price for lot sizing.
*/
func NewAllocator(
	ctx context.Context,
	balance *broker.Balance,
	instrument *broker.Instrument,
	price *broker.Price,
) *Allocator {
	ctx, cancel := context.WithCancel(ctx)

	return &Allocator{
		ctx:        ctx,
		cancel:     cancel,
		balance:    balance,
		instrument: instrument,
		price:      price,
		maxFraction: decimal.NewFromFloat64(
			viper.GetFloat64("trading.allocation.max_fraction"),
		),
	}
}

/*
Close releases the allocator context.
*/
func (allocator *Allocator) Close() {
	if allocator.cancel != nil {
		allocator.cancel()
	}
}

/*
Allocate sizes each enter decision and Books a Reservation. Rotation reuses
ProposedNotional from Admit.Scale; free-slot enters take max_fraction.
*/
func (allocator *Allocator) Allocate(thesis *types.Thesis) error {
	if err := allocator.validate(map[string]any{"thesis": thesis}); err != nil {
		return err
	}

	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Action != types.ActionEnter {
			continue
		}

		pair, err := allocator.instrument.Pair(decision.Symbol)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"allocator: instrument pair unavailable",
				err,
			))
			decision.Reason = "instrument pair unavailable"
			continue
		}

		minCost, err := allocator.price.Taker(
			pair, decimal.NewFromFloat64(pair.QtyMin),
		)

		if err != nil || minCost == nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"allocator: minimum cost unavailable",
				err,
			))
		}

		claim, err := allocator.book(decision)

		if err != nil || claim == nil {
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "wallet slice unavailable"
			continue
		}

		slice := claim.Amount

		if minCost.Cmp(slice) > 0 {
			_ = allocator.balance.Release(claim.ID)
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "minimum exceeds wallet slice"
			continue
		}

		risk := decimal.NewFromFloat64(decision.Risk)
		budget := slice.Sub(allocator.price.Mul(slice, risk))

		if decision.Cause == "rotation" {
			budget = slice.Copy()
		}

		if budget.Sign() <= 0 || budget.Cmp(minCost) < 0 {
			_ = allocator.balance.Release(claim.ID)
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "risk-adjusted budget below minimum"
			continue
		}

		quantity, err := allocator.price.Quantity(pair, budget)

		if err != nil || quantity == nil {
			_ = allocator.balance.Release(claim.ID)
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "quantity unavailable"
			continue
		}

		cost, err := allocator.price.Taker(pair, quantity)

		if err != nil || cost == nil || cost.Cmp(budget) > 0 {
			_ = allocator.balance.Release(claim.ID)
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "taker cost unavailable"
			continue
		}

		ask, err := allocator.price.ReferencePrice(pair)

		if err != nil {
			_ = allocator.balance.Release(claim.ID)
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"allocator: reference price unavailable",
				err,
			))
		}

		if cost.Cmp(slice) < 0 {
			_ = allocator.balance.Release(claim.ID)

			claim, err = allocator.balance.Book(cost, nil)

			if err != nil || claim == nil {
				thesis.Holdings.Delete(decision.Symbol)
				decision.Reason = "wallet slice unavailable"
				continue
			}
		}

		decision.ProposedQuantity = quantity
		decision.ProposedNotional = cost
		decision.ReferencePrice = ask
		decision.ReservationID = claim.ID
		decision.Reason = "sized from wallet slice"

		if value, ok := thesis.Holdings.Load(decision.Symbol); ok {
			holding := value.(*types.Holding)
			holding.Qty = quantity
			thesis.Holdings.Store(decision.Symbol, holding)
		}
	}

	return nil
}

/*
book claims rotation notional when Arbiter stamped it, otherwise max_fraction.
*/
func (allocator *Allocator) book(
	decision *types.Decision,
) (*broker.Reservation, error) {
	if decision.Cause == "rotation" &&
		decision.ProposedNotional != nil &&
		decision.ProposedNotional.Sign() > 0 {
		return allocator.balance.Book(decision.ProposedNotional, nil)
	}

	return allocator.balance.Book(nil, allocator.maxFraction)
}

func (allocator *Allocator) validate(mandatory map[string]any) error {
	check := map[string]any{
		"balance":     allocator.balance,
		"instrument":  allocator.instrument,
		"price":       allocator.price,
		"maxFraction": allocator.maxFraction,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
