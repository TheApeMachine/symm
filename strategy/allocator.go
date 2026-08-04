package strategy

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
allocationScale is the working precision for sizing money. Quote currencies
carry at most eight decimals, so this holds a fraction of a balance without
losing anything the venue could execute.
*/
const allocationScale = 8

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

	if budget == nil || budget.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"allocator budget unavailable or non-positive",
			nil,
		))
	}

	/*
		Multiplication coerces the operand to the receiver's scale, so a
		balance the venue reported without decimals would round the allocation
		fraction to zero and size every order at nothing. Widening the budget
		first keeps the fraction intact whatever precision the balance arrived
		with.
	*/
	budget = budget.SetScale(allocationScale)

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
			if allocator.maxFraction == nil || allocator.maxFraction.Sign() <= 0 {
				decision.ProposedNotional = budget
			} else {
				decision.ProposedNotional = budget.Mul(allocator.maxFraction)
			}
		}

		// Quantize quantity against instrument venue rules
		qty, err := allocator.price.Quantity(pair.Symbol, decision.ProposedNotional)

		if err != nil || qty == nil || qty.Sign() <= 0 {
			allocator.reject(decision, "quantity sizing failed")
			continue
		}

		if pair.QtyMin != nil && qty.Cmp(pair.QtyMin) < 0 {
			allocator.reject(decision, "sized quantity below minimum pair order size")
			continue
		}

		cost := allocator.price.WithFriction(pair.Symbol, broker.BUY, qty)

		if cost == nil || cost.Cmp(budget) > 0 {
			allocator.reject(decision, "taker cost exceeds available budget")
			continue
		}

		referencePrice := allocator.price.Mark(pair.Symbol, broker.BUY)

		if referencePrice == nil {
			allocator.reject(decision, "reference price unavailable")
			continue
		}

		decision.ProposedQuantity = qty
		decision.ProposedNotional = cost
		decision.ReferencePrice = referencePrice.Copy()
		decision.Reason = "sized from transaction budget"

		if decision.Cause != "rotation" {
			budget = budget.Sub(cost)
		}
	}

	/*
		The pass ran against a real wallet balance, which is what the stage
		records. Every entry that survived it carries a size the venue could
		execute, and every one that did not was rejected in place with the
		reason it failed on.
	*/
	thesis.Readiness.Allocation = true

	return nil
}

func (allocator *Allocator) reject(decision *types.Decision, reason string) {
	decision.Action = types.ActionNothing
	decision.Reason = reason
	decision.ProposedQuantity = nil
	decision.ProposedNotional = nil
	decision.ReservationID = ""
}

func (allocator *Allocator) Close() {}
