package strategy

import (
	"context"
	"fmt"
	"maps"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
Allocator sizes enter decisions from a transaction-local budget drawn from the
reservation ledger, honoring rotation notionals and validating haircuts.
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
Allocate sizes each enter decision against a shrinking transaction-local budget.
Per-decision failures reject that row and continue the batch.
*/
func (allocator *Allocator) Allocate(thesis *types.Thesis) error {
	if err := allocator.validate(map[string]any{"thesis": thesis}); err != nil {
		return err
	}

	cash, err := allocator.balance.FreeCash()

	if err != nil || cash == nil || cash.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"allocator: quote cash unavailable",
			err,
		))
	}

	budget := cash.Copy()

	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Action != types.ActionEnter {
			continue
		}

		allocator.size(thesis, decision, &budget)
	}

	return nil
}

func (allocator *Allocator) size(
	thesis *types.Thesis,
	decision *types.Decision,
	budget **decimal.Decimal,
) {
	pair, err := allocator.instrument.Pair(decision.Symbol)

	if err != nil {
		allocator.reject(thesis, decision, "instrument pair unavailable")
		return
	}

	if pair.QtyMin == nil || pair.QtyMin.Sign() <= 0 {
		allocator.reject(thesis, decision, "instrument qty_min unavailable")
		return
	}

	haircut, haircutErr := allocator.haircut(decision.AllocationHaircut)

	if haircutErr != nil {
		allocator.reject(thesis, decision, haircutErr.Error())
		return
	}

	slice := *budget

	if decision.Cause == "rotation" &&
		decision.ProposedNotional != nil &&
		decision.ProposedNotional.Sign() > 0 {
		slice = decision.ProposedNotional.Copy()
	}

	if decision.Cause != "rotation" {
		walletSlice := decimal.ExactMul(slice, allocator.maxFraction)

		if walletSlice == nil || walletSlice.Sign() <= 0 {
			allocator.reject(thesis, decision, "wallet slice unavailable")
			return
		}

		slice = walletSlice
	}

	riskAdjusted := slice.Copy().Sub(decimal.ExactMul(slice.Copy(), haircut))

	if riskAdjusted.Sign() <= 0 {
		allocator.reject(thesis, decision, "risk-adjusted budget below minimum")
		return
	}

	minCost, minErr := allocator.price.Taker(&pair, pair.QtyMin)

	if minErr != nil || minCost == nil {
		allocator.reject(thesis, decision, "minimum cost unavailable")
		return
	}

	if minCost.Cmp(riskAdjusted) > 0 {
		allocator.reject(thesis, decision, "minimum exceeds wallet slice")
		return
	}

	quantity, qtyErr := allocator.price.Quantity(&pair, riskAdjusted)

	if qtyErr != nil || quantity == nil {
		allocator.reject(thesis, decision, "quantity unavailable")
		return
	}

	cost, costErr := allocator.price.Taker(&pair, quantity)

	if costErr != nil || cost == nil || cost.Cmp(riskAdjusted) > 0 {
		allocator.reject(thesis, decision, "taker cost unavailable")
		return
	}

	if decision.Cause != "rotation" && cost.Cmp(*budget) > 0 {
		allocator.reject(thesis, decision, "insufficient transaction budget")
		return
	}

	if decision.Cause != "rotation" {
		intentID := fmt.Sprintf(
			"alloc:%s:%d", decision.Symbol, decision.At.UnixNano(),
		)

		if err := allocator.balance.Ledger().Reserve(
			intentID, decision.Symbol, cost, false,
		); err != nil {
			allocator.reject(thesis, decision, err.Error())
			return
		}

		decision.ReservationID = intentID

		if value, ok := thesis.Holdings.Load(decision.Symbol); ok {
			holding := value.(*types.Holding)
			holding.ReservationID = intentID
			thesis.Holdings.Store(decision.Symbol, holding)
		}

		*budget = (*budget).Sub(cost)
	}

	ask, askErr := allocator.price.ReferencePrice(&pair)

	if askErr != nil {
		if decision.ReservationID != "" {
			_ = allocator.balance.Ledger().Release(decision.ReservationID)
		}

		allocator.reject(thesis, decision, "reference price unavailable")
		return
	}

	decision.ProposedQuantity = quantity
	decision.ProposedNotional = cost
	decision.ReferencePrice = ask
	decision.Reason = "sized from transaction budget"

	if value, ok := thesis.Holdings.Load(decision.Symbol); ok {
		holding := value.(*types.Holding)
		holding.Qty = quantity
		thesis.Holdings.Store(decision.Symbol, holding)
	}
}

func (allocator *Allocator) haircut(value float64) (*decimal.Decimal, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"allocation haircut must be finite in [0,1]",
			nil,
		))
	}

	return decimal.NewFromFloat64(value), nil
}

func (allocator *Allocator) reject(
	thesis *types.Thesis,
	decision *types.Decision,
	reason string,
) {
	thesis.Holdings.Delete(decision.Symbol)
	decision.Reason = reason
	decision.ProposedQuantity = nil
	decision.ProposedNotional = nil
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
