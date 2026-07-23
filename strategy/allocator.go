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
Allocator sizes enter decisions as max_fraction of quote cash, then shrinks
that slice by the decision risk factor (0–1). Action demotion belongs to
Planner after Allocate.
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
Allocate sizes each enter decision from max_fraction of quote cash, scaled by
AllocationHaircut. Decisions that cannot clear the instrument minimum are
dropped from Thesis holdings.
*/
func (allocator *Allocator) Allocate(thesis *types.Thesis) error {
	if err := allocator.validate(map[string]any{"thesis": thesis}); err != nil {
		return err
	}

	cash, err := allocator.balance.AssetAvailable(
		viper.GetString("market.quote_currency"),
	)

	if err != nil || cash == nil || cash.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"allocator: quote cash unavailable",
			err,
		))
	}

	slice := decimal.ExactMul(cash, allocator.maxFraction)

	if slice == nil || slice.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"allocator: wallet slice unavailable",
			nil,
		))
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

		if pair.QtyMin == nil || pair.QtyMin.Sign() <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"allocator: instrument qty_min unavailable for "+decision.Symbol,
				nil,
			))
		}

		minCost, err := allocator.price.Taker(pair, pair.QtyMin)

		if err != nil || minCost == nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"allocator: minimum cost unavailable",
				err,
			))
		}

		if minCost.Cmp(slice) > 0 {
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "minimum exceeds wallet slice"
			continue
		}

		risk := decimal.NewFromFloat64(decision.AllocationHaircut)
		budget := slice.Copy().Sub(decimal.ExactMul(slice.Copy(), risk))

		if budget.Sign() <= 0 || budget.Cmp(minCost) < 0 {
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "risk-adjusted budget below minimum"
			continue
		}

		quantity, err := allocator.price.Quantity(pair, budget)

		if err != nil || quantity == nil {
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "quantity unavailable"
			continue
		}

		cost, err := allocator.price.Taker(pair, quantity)

		if err != nil || cost == nil || cost.Cmp(budget) > 0 {
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "taker cost unavailable"
			continue
		}

		if !allocator.balance.Available(cost) {
			thesis.Holdings.Delete(decision.Symbol)
			decision.Reason = "insufficient balance"
			continue
		}

		ask, err := allocator.price.ReferencePrice(pair)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"allocator: reference price unavailable",
				err,
			))
		}

		decision.ProposedQuantity = quantity
		decision.ProposedNotional = cost
		decision.ReferencePrice = ask
		decision.Reason = "sized from wallet slice"

		if value, ok := thesis.Holdings.Load(decision.Symbol); ok {
			holding := value.(*types.Holding)
			holding.Qty = quantity
			thesis.Holdings.Store(decision.Symbol, holding)
		}
	}

	return nil
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
