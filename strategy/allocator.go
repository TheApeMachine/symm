package strategy

import (
	"context"
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
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
entries collects the pointers to this tick's entry candidates.

Sizing mutates the decisions it judges, and the map holds pointers, so writing
through them updates the thesis in place. They are gathered first because the
body below is a sequence of guarded rejections that reads as a loop and would
have to become a callback returning true to keep ranging.
*/
func (allocator *Allocator) entries(thesis *types.Thesis) []*types.Decision {
	entries := make([]*types.Decision, 0)

	thesis.Decisions.Range(func(key, value any) bool {
		decision, ok := value.(*types.Decision)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"allocator: decision map holds a value that is not a decision",
				nil,
			))

			return true
		}

		if decision.Action == types.ActionEnter {
			entries = append(entries, decision)
		}

		return true
	})

	return entries
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

		stopSnapshot := position.Holding.Stoploss
		committed = committed.Add(stopSnapshot.Floor)
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
		for _, decision := range allocator.entries(thesis) {
			allocator.reject(decision, "risk configuration invalid")
		}

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"allocator: risk configuration will not protect a position",
			nil,
		))
	}

	budget := allocator.balance.Cash()

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
		riskPool is what the whole account may still lose, after what the open
		book already stands to lose is taken out of it.

		The per-position limit alone does not describe account risk. Four
		entries at one percent each in a single pass are a four percent account
		exposure, and nothing in a per-position cap notices — so the pool is
		decremented as each entry is sized, and an entry that would overdraw it
		is refused rather than shrunk to nothing.
	*/
	riskPool := budget.Mul(allocator.portfolioLossFraction).Sub(
		allocator.committedRisk(),
	)

	for _, decision := range allocator.entries(thesis) {
		decision.AvailableCapital = budget

		pair := allocator.instrument.Pair(decision.Symbol)

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

		if math.IsNaN(decision.AllocationHaircut) ||
			math.IsInf(decision.AllocationHaircut, 0) ||
			decision.AllocationHaircut < 0 || decision.AllocationHaircut >= 1 {
			allocator.reject(decision, "allocation haircut invalid")
			continue
		}

		if decision.AllocationHaircut > 0 {
			notional := decision.ProposedNotional.SetScale(allocationScale)
			haircut := notional.Mul(
				decimal.NewFromFloat64(decision.AllocationHaircut),
			)
			decision.ProposedNotional = notional.Sub(haircut)

			if decision.ProposedNotional.Sign() <= 0 {
				allocator.reject(decision, "allocation haircut exhausted notional")
				continue
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

		plan := allocator.plan(pair, *decision, budget, riskPool)

		if !plan.Present {
			allocator.reject(decision, "risk geometry unavailable")
			continue
		}

		tick := allocator.price.Tick(pair.Symbol)

		if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 {
			allocator.reject(decision, "executable ask unavailable")
			continue
		}

		// Quantize quantity against instrument venue rules
		qty := allocator.price.Quantity(pair.Symbol, decision.ProposedNotional)

		if qty == nil || qty.Sign() <= 0 {
			allocator.reject(decision, "quantity sizing failed")
			continue
		}

		/*
			Widening a stop to survive noise is only affordable if the size
			behind it comes down to match. Uncapped, the two changes cancel:
			the boundary moves further away and every trade that reaches it
			loses proportionally more.
		*/
		capacity := plan.MaxQuantity(tick.Ask)

		if capacity != nil && qty.Cmp(capacity) > 0 {
			funded := allocator.price.WithFee(pair.Symbol, allocator.price.Tick(pair.Symbol).Bid, broker.BUY)

			if funded == nil || funded.Sign() <= 0 {
				allocator.reject(decision, "risk-capped quantity could not be priced")
				continue
			}

			qty = allocator.price.Quantity(pair.Symbol, funded)

			if qty == nil || qty.Sign() <= 0 {
				allocator.reject(decision, "risk-capped quantity sizing failed")
				continue
			}
		}

		if pair.QtyMin != nil && qty.Cmp(pair.QtyMin) < 0 {
			allocator.reject(decision, "sized quantity below minimum pair order size")
			continue
		}

		cost := allocator.price.WithFee(pair.Symbol, allocator.price.Tick(pair.Symbol).Bid, broker.BUY)

		if cost == nil || cost.Cmp(budget) > 0 {
			allocator.reject(decision, "taker cost exceeds available budget")
			continue
		}

		referencePrice := tick.Ask

		decision.ProposedQuantity = qty
		decision.ProposedNotional = cost
		decision.ReferencePrice = referencePrice
		decision.Risk = plan
		decision.Reason = "sized from transaction budget, flow haircut, and risk distance"

		// What this lot can now lose comes out of what the account has left to
		// lose, so the next entry in this pass is sized against the remainder.
		if committed := plan.LossPerUnit(referencePrice); committed != nil {
			riskPool = riskPool.Sub(committed.SetScale(allocationScale).Mul(qty))
		}

		budget = budget.Sub(cost)
	}

	thesis.Stamp(types.SourceAllocator)
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
		return &pair.TickSize
	}

	if pair.PriceIncrement.Sign() > 0 {
		return &pair.PriceIncrement
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
