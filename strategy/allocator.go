package strategy

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
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
	ctx                   context.Context
	balance               *broker.Balance
	instrument            *broker.Instrument
	price                 *broker.Price
	desk                  *broker.Desk
	maxFraction           *decimal.Decimal
	maxLossFraction       *decimal.Decimal
	portfolioLossFraction *decimal.Decimal
	multiples             types.RiskMultiples
	/*
		riskValid is false when the configured geometry would not protect
		anything. Sizing then refuses every entry rather than falling back to a
		permissive default, because each of the ways this block degenerates
		produces a position that looks sized and is not.
	*/
	riskValid bool
}

func NewAllocator(
	ctx context.Context,
	balance *broker.Balance,
	instrument *broker.Instrument,
	price *broker.Price,
	desk *broker.Desk,
) *Allocator {
	defaults := types.DefaultRiskMultiples()

	viper.SetDefault("trading.risk.max_loss_fraction", 0.01)
	viper.SetDefault("trading.risk.portfolio_loss_fraction", 0.03)
	viper.SetDefault("trading.risk.noise_multiple", defaults.Risk)
	viper.SetDefault("trading.risk.trail_multiple", defaults.Trail)
	viper.SetDefault("trading.risk.arm_multiple", defaults.Arm)
	viper.SetDefault("trading.risk.lock_multiple", defaults.Lock)
	viper.SetDefault("trading.risk.min_edge_multiple", defaults.MinEdge)
	viper.SetDefault("trading.risk.min_ticks", defaults.MinTicks)
	viper.SetDefault("trading.risk.confirm_marks", defaults.ConfirmMarks)

	risk := config.RiskConfig{
		MaxLossFraction:       viper.GetFloat64("trading.risk.max_loss_fraction"),
		PortfolioLossFraction: viper.GetFloat64("trading.risk.portfolio_loss_fraction"),
		NoiseMultiple:         viper.GetFloat64("trading.risk.noise_multiple"),
		TrailMultiple:         viper.GetFloat64("trading.risk.trail_multiple"),
		ArmMultiple:           viper.GetFloat64("trading.risk.arm_multiple"),
		LockMultiple:          viper.GetFloat64("trading.risk.lock_multiple"),
		MinEdgeMultiple:       viper.GetFloat64("trading.risk.min_edge_multiple"),
		MinTicks:              viper.GetInt("trading.risk.min_ticks"),
		ConfirmMarks:          viper.GetInt("trading.risk.confirm_marks"),
	}

	valid := true

	if err := risk.Validate(); err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "allocator: risk", err))
		valid = false
	}

	return &Allocator{
		ctx:        ctx,
		balance:    balance,
		instrument: instrument,
		price:      price,
		desk:       desk,
		riskValid:  valid,
		maxFraction: decimal.NewFromFloat64(
			viper.GetFloat64("trading.allocation.max_fraction"),
		),
		maxLossFraction:       decimal.NewFromFloat64(risk.MaxLossFraction),
		portfolioLossFraction: decimal.NewFromFloat64(risk.PortfolioLossFraction),
		multiples: types.RiskMultiples{
			Risk:         risk.NoiseMultiple,
			Trail:        risk.TrailMultiple,
			Arm:          risk.ArmMultiple,
			Lock:         risk.LockMultiple,
			MinEdge:      risk.MinEdgeMultiple,
			MinTicks:     int64(risk.MinTicks),
			ConfirmMarks: risk.ConfirmMarks,
		},
	}
}

/*
committedRisk is what the open book already stands to lose if every position
runs to its hard floor.

It is read from the regulators rather than from the decisions that opened them,
because the geometry a lot is actually being defended at is the only number that
describes the account's exposure now.
*/
func (allocator *Allocator) committedRisk() *decimal.Decimal {
	committed := decimal.NewFromInt64(0).SetScale(allocationScale)

	if allocator.desk == nil {
		return committed
	}

	for position := range allocator.desk.Positions() {
		if position.Status == types.CLOSED || position.Holding == nil {
			continue
		}

		snapshot := position.StopSnapshot()

		if !snapshot.Present || snapshot.RiskDistance == nil ||
			position.Holding.SellableQty == nil {
			continue
		}

		committed = committed.Add(
			snapshot.RiskDistance.SetScale(allocationScale).
				Mul(position.Holding.SellableQty),
		)
	}

	return committed
}

// Allocate sizes each accepted 'enter' decision against free wallet cash.
func (allocator *Allocator) Allocate(thesis *types.Thesis) error {
	/*
		A risk block that does not describe a protectable position stops trading
		rather than degrading to a permissive default. Every degenerate value it
		can hold produces an entry that looks sized and is not, and a refused
		round costs nothing that a mis-sized one does not cost more of.
	*/
	if !allocator.riskValid {
		thesis.Readiness.Allocation = true

		for index := range thesis.Decisions {
			if thesis.Decisions[index].Action == types.ActionEnter {
				allocator.reject(&thesis.Decisions[index], "risk configuration invalid")
			}
		}

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"allocator: risk configuration will not protect a position",
			nil,
		))
	}

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

	/*
		riskPool is what the whole account may still lose, after what the open
		book already stands to lose is taken out of it.

		The per-position limit alone does not describe account risk. Four
		entries at one percent each in a single pass are a four percent account
		exposure, and nothing in a per-position cap notices — so the pool is
		decremented as each entry is sized, and an entry that would overdraw it
		is refused rather than shrunk to nothing.
	*/
	riskPool := budget.Mul(allocator.portfolioLossFraction).
		Sub(allocator.committedRisk())

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

		// Calculate the maximum wallet slice this entry may consume.
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
		if riskPool.Sign() <= 0 {
			allocator.reject(decision, "portfolio risk budget exhausted")
			continue
		}

		plan := allocator.plan(pair, *decision, riskBudget, riskPool)

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
		capacity := plan.MaxQuantity(allocator.price.Mark(pair.Symbol, broker.BUY))

		if capacity != nil && qty.Cmp(capacity) > 0 {
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

		// What this lot can now lose comes out of what the account has left to
		// lose, so the next entry in this pass is sized against the remainder.
		if committed := plan.LossPerUnit(referencePrice); committed != nil {
			riskPool = riskPool.Sub(committed.SetScale(allocationScale).Mul(qty))
		}

		budget = budget.Sub(cost)
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
	riskPool *decimal.Decimal,
) types.RiskPlan {
	exitFeeRate, err := allocator.price.Fee(pair.Symbol)

	if err != nil {
		return types.RiskPlan{}
	}

	reference := decision.ReferencePrice

	if reference == nil || reference.Sign() <= 0 {
		reference = allocator.price.Mark(pair.Symbol, broker.BUY)
	}

	/*
		The loss budget for this lot is the smaller of what one position may lose
		and what the account has left to lose. Late entries in a busy pass are
		therefore sized against a pool the earlier ones have already drawn on,
		which is what stops several individually-compliant positions from adding
		up to an account risk nobody chose.
	*/
	maxLoss := riskBudget.SetScale(allocationScale).Mul(allocator.maxLossFraction)

	if riskPool != nil && riskPool.Cmp(maxLoss) < 0 {
		maxLoss = riskPool.SetScale(allocationScale)
	}

	/*
		The forecast's Uncertainty is deliberately not passed as the return-risk
		fraction. It is the resonance kernel's reconstruction residual, which is
		a model-error magnitude and not a price move, and feeding it in as one
		puts the hard floor wherever that residual happens to sit.

		The boundary is therefore derived from what is genuinely measured here —
		crossing cost and venue granularity — until a calibrated forward-return
		error or adverse-excursion quantile exists to fill this in.
	*/
	return types.NewRiskPlan(types.RiskInputs{
		ReferencePrice: reference,
		Spread:         decision.ExpectedSpread,
		Impact:         decision.ExpectedImpact,
		TickSize:       tickSize(pair),
		ExitFeeRate:    exitFeeRate,
		EntryFeeRate:   exitFeeRate,
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
