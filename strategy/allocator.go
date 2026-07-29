package strategy

import (
	"context"
	"fmt"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
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
func (a *Allocator) Allocate(thesis *types.Thesis) error {
	if thesis == nil {
		return nil
	}

	cash, err := a.balance.FreeCash()
	if err != nil || cash == nil || cash.Sign() <= 0 {
		return nil
	}

	budget := cash.Copy()

	for i := range thesis.Decisions {
		decision := &thesis.Decisions[i]
		if decision.Action != types.ActionEnter {
			continue
		}

		pair, err := a.instrument.Pair(decision.Symbol)
		if err != nil {
			a.reject(decision, "instrument pair unavailable")
			continue
		}

		if pair.QtyMin == nil || pair.QtyMin.Sign() <= 0 {
			a.reject(decision, "instrument qty_min unavailable")
			continue
		}

		// Calculate Max Fraction Slice or reuse rotation freed capital
		slice := decimal.ExactMul(budget, a.maxFraction)
		if decision.ProposedNotional != nil && decision.ProposedNotional.Sign() > 0 {
			slice = decision.ProposedNotional.Copy()
		}

		// Quantize quantity against instrument venue rules
		qty, err := a.price.Quantity(&pair, slice)
		if err != nil || qty == nil {
			a.reject(decision, "quantity sizing failed")
			continue
		}

		cost, err := a.price.Taker(&pair, qty)
		if err != nil || cost == nil || cost.Cmp(budget) > 0 {
			a.reject(decision, "taker cost exceeds available budget")
			continue
		}

		// Reserve capital on broker ledger
		intentID := fmt.Sprintf("alloc:%s:%d", decision.Symbol, decision.At.UnixNano())
		if err := a.balance.Reserve(intentID, decision.Symbol, cost, false); err != nil {
			a.reject(decision, "ledger reservation failed")
			continue
		}

		ask, err := a.price.ReferencePrice(&pair)
		if err != nil {
			_ = a.balance.Release(intentID)
			a.reject(decision, "reference price unavailable")
			continue
		}

		decision.ReservationID = intentID
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
