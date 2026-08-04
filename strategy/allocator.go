package strategy

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
allocationScale is the working precision for sizing money. Quote currencies
carry at most eight decimals, so this holds a fraction of a balance without
losing anything the venue could execute.
*/
const allocationScale = 8

type Allocator struct {
	ctx             context.Context
	balance         *broker.Balance
	instrument      *broker.Instrument
	price           *broker.Price
	maxFraction     *decimal.Decimal
	maxLossFraction *decimal.Decimal
	multiples       types.RiskMultiples
}

func NewAllocator(
	ctx context.Context,
	balance *broker.Balance,
	instrument *broker.Instrument,
	price *broker.Price,
) *Allocator {
	defaults := types.DefaultRiskMultiples()

	viper.SetDefault("trading.risk.max_loss_fraction", 0.01)
	viper.SetDefault("trading.risk.noise_multiple", defaults.Risk)
	viper.SetDefault("trading.risk.trail_multiple", defaults.Trail)
	viper.SetDefault("trading.risk.arm_multiple", defaults.Arm)
	viper.SetDefault("trading.risk.lock_multiple", defaults.Lock)
	viper.SetDefault("trading.risk.min_edge_multiple", defaults.MinEdge)
	viper.SetDefault("trading.risk.min_ticks", defaults.MinTicks)
	viper.SetDefault("trading.risk.confirm_marks", defaults.ConfirmMarks)

	return &Allocator{
		ctx:        ctx,
		balance:    balance,
		instrument: instrument,
		price:      price,
		maxFraction: decimal.NewFromFloat64(
			viper.GetFloat64("trading.allocation.max_fraction"),
		),
		maxLossFraction: decimal.NewFromFloat64(
			viper.GetFloat64("trading.risk.max_loss_fraction"),
		),
		multiples: types.RiskMultiples{
			Risk:         viper.GetFloat64("trading.risk.noise_multiple"),
			Trail:        viper.GetFloat64("trading.risk.trail_multiple"),
			Arm:          viper.GetFloat64("trading.risk.arm_multiple"),
			Lock:         viper.GetFloat64("trading.risk.lock_multiple"),
			MinEdge:      viper.GetFloat64("trading.risk.min_edge_multiple"),
			MinTicks:     viper.GetInt64("trading.risk.min_ticks"),
			ConfirmMarks: viper.GetInt("trading.risk.confirm_marks"),
		},
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

	/*
		The loss budget is measured against the wallet as it stood when the pass
		began, not against what is left after each entry is funded. Every
		decision in this round is judged the same way, and a lot sized late in
		the loop is not allowed a smaller risk boundary purely because earlier
		entries had already spent the cash.
	*/
	riskBudget := budget.Copy()

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

		/*
			Stop distance and position size are one decision, so the geometry is
			derived here rather than left to the regulator to invent after the
			order has already been placed at whatever size the cash allowed.

			Without a derivable boundary the entry is refused outright. Sizing
			against no risk distance is what produces the position that is too
			large for any stop that could survive ordinary noise, and the
			regulator cannot repair that after the fill.
		*/
		plan := allocator.plan(pair, *decision, riskBudget)

		if !plan.Present {
			allocator.reject(decision, "risk geometry unavailable")
			continue
		}

		// Quantize quantity against instrument venue rules
		qty, err := allocator.price.Quantity(pair.Symbol, decision.ProposedNotional)

		if err != nil || qty == nil || qty.Sign() <= 0 {
			allocator.reject(decision, "quantity sizing failed")
			continue
		}

		/*
			Widening a stop to survive noise is only affordable if the size
			behind it comes down to match. Uncapped, the two changes cancel:
			the boundary moves further away and every trade that reaches it
			loses proportionally more.
		*/
		if capacity := plan.MaxQuantity(); capacity != nil && qty.Cmp(capacity) > 0 {
			funded := allocator.price.WithFriction(pair.Symbol, broker.BUY, capacity)

			if funded == nil || funded.Sign() <= 0 {
				allocator.reject(decision, "risk-capped quantity could not be priced")
				continue
			}

			qty, err = allocator.price.Quantity(pair.Symbol, funded)

			if err != nil || qty == nil || qty.Sign() <= 0 {
				allocator.reject(decision, "risk-capped quantity sizing failed")
				continue
			}
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
		decision.Risk = plan
		decision.Reason = "sized from transaction budget and risk distance"

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

/*
plan derives the stop geometry this entry would be sized under.

Everything it reads is already on the decision or the instrument: the spread
and impact the candidate was priced with, the forecast's own dispersion, and
the venue's tick size and taker rate. Nothing new is measured here, because the
numbers that describe how far this symbol ordinarily moves are the same ones
that decided the trade was worth taking.
*/
func (allocator *Allocator) plan(
	pair kraken.InstrumentPair,
	decision types.Decision,
	riskBudget *decimal.Decimal,
) types.RiskPlan {
	exitFeeRate, err := allocator.price.Fee(pair.Symbol)

	if err != nil {
		return types.RiskPlan{}
	}

	reference := decision.ReferencePrice

	if reference == nil || reference.Sign() <= 0 {
		reference = allocator.price.Mark(pair.Symbol, broker.BUY)
	}

	maxLoss := riskBudget

	if allocator.maxLossFraction != nil && allocator.maxLossFraction.Sign() > 0 {
		maxLoss = riskBudget.SetScale(allocationScale).Mul(allocator.maxLossFraction)
	}

	return types.NewRiskPlan(types.RiskInputs{
		ReferencePrice: reference,
		Spread:         decision.ExpectedSpread,
		Impact:         decision.ExpectedImpact,
		Uncertainty:    decision.Uncertainty,
		TickSize:       tickSize(pair),
		ExitFeeRate:    exitFeeRate,
		MaxLoss:        maxLoss,
		Multiples:      allocator.multiples,
	})
}

/*
tickSize reads the venue's price granularity, preferring the explicit tick over
the price increment because a pair can quote one without the other.
*/
func tickSize(pair kraken.InstrumentPair) *decimal.Decimal {
	if pair.TickSize.Sign() > 0 {
		return pair.TickSize.Copy()
	}

	if pair.PriceIncrement.Sign() > 0 {
		return pair.PriceIncrement.Copy()
	}

	return nil
}

func (allocator *Allocator) reject(decision *types.Decision, reason string) {
	decision.Action = types.ActionNothing
	decision.Reason = reason
	decision.ProposedQuantity = nil
	decision.ProposedNotional = nil
	decision.ReservationID = ""
	decision.Risk = types.RiskPlan{}
}

func (allocator *Allocator) Close() {}
