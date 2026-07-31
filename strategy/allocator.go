package strategy

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

type Allocator struct {
	ctx         context.Context
	balance     *broker.Balance
	instrument  *broker.Instrument
	price       *broker.Price
	maxFraction *decimal.Decimal
}

func NewAllocator(
	ctx context.Context,
	balance *broker.Balance,
	instrument *broker.Instrument,
	price *broker.Price,
) *Allocator {
	return &Allocator{
		ctx:        ctx,
		balance:    balance,
		instrument: instrument,
		price:      price,
		maxFraction: decimal.NewFromFloat64(
			viper.GetFloat64("trading.allocation.max_fraction"),
		),
	}
}

// Allocate sizes each accepted 'enter' decision against free wallet cash.
func (allocator *Allocator) Allocate(thesis *types.Thesis) error {
	budget, err := allocator.balance.Cash()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"ledger cash unavailable",
			err,
		))
	}

	for i := range thesis.Decisions {
		decision := &thesis.Decisions[i]

		if decision.Action != types.ActionEnter {
			continue
		}

		pair, err := allocator.instrument.Pair(decision.Symbol)

		if err != nil {
			allocator.reject(decision, "instrument pair unavailable")
			continue
		}

		if pair.QtyMin == nil || pair.QtyMin.Sign() <= 0 {
			allocator.reject(decision, "instrument qty_min unavailable")
			continue
		}

		// Calculate Max Fraction Slice or reuse rotation freed capital
		if decision.ProposedNotional == nil || decision.ProposedNotional.Sign() <= 0 {
			decision.ProposedNotional = budget.Mul(allocator.maxFraction)
		}

		// Quantize quantity against instrument venue rules
		qty, err := allocator.price.Quantity(pair.Symbol, decision.ProposedNotional)

		if err != nil || qty == nil {
			allocator.reject(decision, "quantity sizing failed")
			continue
		}

		cost := allocator.price.WithFriction(pair.Symbol, broker.BUY, qty)

		if err != nil || cost == nil || cost.Cmp(budget) > 0 {
			allocator.reject(decision, "taker cost exceeds available budget")
			continue
		}

		if err := allocator.balance.Reserve(cost); err != nil {
			allocator.reject(decision, "ledger reservation failed")
			continue
		}

		ask := allocator.price.WithFriction(pair.Symbol, broker.BUY, qty)

		decision.ProposedQuantity = qty
		decision.ProposedNotional = cost
		decision.ReferencePrice = ask.Copy()
		decision.Reason = "sized from transaction budget"

		if decision.Cause != "rotation" {
			budget = budget.Sub(cost)
		}
	}

	return nil
}

func (a *Allocator) reject(decision *types.Decision, reason string) {
	decision.Action = types.ActionNothing
	decision.Reason = reason
	decision.ProposedQuantity = nil
	decision.ProposedNotional = nil
	decision.ReservationID = ""
}

func (a *Allocator) Close() {}
