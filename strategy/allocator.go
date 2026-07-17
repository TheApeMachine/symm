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
Allocator sizes enter holdings from wallet cash and the live ask. Quantity is
chosen first (max_fraction of available quote ÷ ask, floored at qty_min); Taker
then prices that lot. Public fields expose the last sized surface for journals
and UI without a separate result type.
*/
type Allocator struct {
	ctx         context.Context
	cancel      context.CancelFunc
	balance     *broker.Balance
	instrument  *broker.Instrument
	price       *broker.Price
	maxFraction *decimal.Decimal

	Symbol   string           `json:"symbol,omitempty"`
	Quantity *decimal.Decimal `json:"quantity,omitempty"`
	Cost     *decimal.Decimal `json:"cost,omitempty"`
	Action   string           `json:"action,omitempty"`
	Reason   string           `json:"reason,omitempty"`
}

/*
NewAllocator wires broker surfaces used for friction and lot sizing.
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
Friction writes ExpectedFees from Price.Fraction and returns the per-symbol
fee map Decide needs. Forecasts without a cached tier stay FrictionReady=false.
*/
func (allocator *Allocator) Friction(thesis *types.Thesis) map[string]float64 {
	fees := make(map[string]float64, len(thesis.Forecasts))

	if allocator == nil || allocator.price == nil || thesis == nil {
		return fees
	}

	for index := range thesis.Forecasts {
		forecast := &thesis.Forecasts[index]
		fraction, err := allocator.price.Fraction(forecast.Symbol)

		if err != nil || fraction == nil || fraction.Sign() < 0 {
			continue
		}

		forecast.ExpectedFees = fraction.Float64()
		forecast.FrictionReady = true
		fees[forecast.Symbol] = fraction.Float64()
	}

	return fees
}

/*
Quote returns unreserved quote capital for Decide slot budgets.
*/
func (allocator *Allocator) Quote() (float64, error) {
	if allocator == nil || allocator.balance == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.NotFound,
			"allocator: balance unavailable",
			nil,
		))
	}

	return allocator.balance.AvailableQuote()
}

/*
Allocate sizes every Thesis holding that still carries an enter order. Failed
lots demote the matching enter Decision and leave Holdings.
*/
func (allocator *Allocator) Allocate(thesis *types.Thesis) {
	if allocator == nil || thesis == nil {
		return
	}

	thesis.Holdings.Range(func(key, value any) bool {
		holding, ok := holdingValue(value)

		if !ok || holding.Order == nil || holding.Order.Description == nil ||
			holding.Order.Description.Type != "enter" {
			return true
		}

		if !allocator.size(&holding) {
			allocator.demote(thesis, holding.Symbol)
			thesis.Holdings.Delete(key)

			return true
		}

		thesis.Holdings.Store(key, holding)
		allocator.sync(thesis, holding)

		return true
	})
}

/*
size chooses quantity from ask and max_fraction budget, then asks Price.Taker
for the all-in cost of that lot.
*/
func (allocator *Allocator) size(holding *types.Holding) bool {
	allocator.Symbol = holding.Symbol
	allocator.Quantity = nil
	allocator.Cost = nil
	allocator.Action = "nothing"
	allocator.Reason = ""

	pair, err := allocator.instrument.Pair(holding.Symbol)

	if err != nil {
		allocator.Reason = "instrument pair unavailable"
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"allocator: instrument pair not found",
			err,
		))

		return false
	}

	ticker, err := allocator.price.Get(holding.Symbol)

	if err != nil || ticker.Ask == nil || ticker.Ask.Sign() <= 0 {
		allocator.Reason = "ask unavailable"
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"allocator: ask unavailable for "+holding.Symbol,
			err,
		))

		return false
	}

	quote, err := allocator.balance.AvailableQuote()

	if err != nil || quote <= 0 {
		allocator.Reason = "quote capital unavailable"

		return false
	}

	budget := decimal.NewFromFloat64(quote).Mul(allocator.maxFraction)
	quantity := budget.Div(ticker.Ask)
	minLot := decimal.NewFromFloat64(pair.QtyMin)

	if quantity.Cmp(minLot) < 0 {
		quantity = minLot
	}

	cost, err := allocator.price.Taker(pair, quantity)

	if err != nil || cost == nil {
		allocator.Reason = "taker cost unavailable"
		errnie.Error(errnie.Err(
			errnie.Internal,
			"allocator: taker cost unavailable for "+holding.Symbol,
			err,
		))

		return false
	}

	zero := decimal.NewFromInt64(0)

	if pair.CostMin.Cmp(zero) > 0 && cost.Cmp(pair.CostMin) < 0 {
		allocator.Reason = "below cost_min"

		return false
	}

	if !allocator.balance.Available(cost) {
		allocator.Reason = "insufficient balance"

		return false
	}

	holding.Qty = quantity

	if holding.Order != nil {
		holding.Order.Volume = quantity
	}

	allocator.Quantity = quantity
	allocator.Cost = cost
	allocator.Action = "enter"
	allocator.Reason = "sized from ask and max_fraction"

	return true
}

func (allocator *Allocator) sync(thesis *types.Thesis, holding types.Holding) {
	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Symbol != holding.Symbol || decision.Action != "enter" {
			continue
		}

		decision.ProposedQuantity = holding.Qty.Float64()

		if allocator.Cost != nil {
			decision.ProposedNotional = allocator.Cost.Float64()
		}

		decision.Reason = allocator.Reason
	}
}

func (allocator *Allocator) demote(thesis *types.Thesis, symbol string) {
	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Symbol != symbol || decision.Action != "enter" {
			continue
		}

		decision.Action = "nothing"
		decision.Cause = "below_qty_min"
		decision.Reason = allocator.Reason
		decision.Utility = 0
	}
}

func holdingValue(value any) (types.Holding, bool) {
	switch holding := value.(type) {
	case types.Holding:
		return holding, true
	case *types.Holding:
		if holding == nil {
			return types.Holding{}, false
		}

		return *holding, true
	default:
		return types.Holding{}, false
	}
}
