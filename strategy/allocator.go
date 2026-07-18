package strategy

import (
	"context"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
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
	if allocator == nil || allocator.price == nil || thesis == nil {
		return map[string]float64{}
	}

	fees := make(map[string]float64, len(thesis.Forecasts))

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
		holding, ok := value.(*types.Holding)

		if !ok {
			return true
		}

		if !allocator.size(holding) {
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
	allocator.resetSurface(holding.Symbol)

	pair, ok := allocator.pairLookup(holding.Symbol)

	if !ok {
		return false
	}

	ask, ok := allocator.marketAsk(holding.Symbol)

	if !ok {
		return false
	}

	quote, ok := allocator.quoteCapital()

	if !ok {
		return false
	}

	quantity, ok := allocator.lotWithinBudget(pair, ask, quote)

	if !ok {
		return false
	}

	cost, ok := allocator.takerCost(pair, quantity)

	if !ok {
		return false
	}

	if !allocator.passesMinimums(pair, cost) {
		return false
	}

	if !allocator.balanceAvailable(cost) {
		return false
	}

	return allocator.persistSized(holding, quantity, cost)
}

func (allocator *Allocator) resetSurface(symbol string) {
	allocator.Symbol = symbol
	allocator.Quantity = nil
	allocator.Cost = nil
	allocator.Action = "nothing"
	allocator.Reason = ""
}

func (allocator *Allocator) pairLookup(symbol string) (*kraken.InstrumentPair, bool) {
	pair, err := allocator.instrument.Pair(symbol)

	if err != nil {
		allocator.Reason = "instrument pair unavailable"
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"allocator: instrument pair not found",
			err,
		))

		return nil, false
	}

	return pair, true
}

func (allocator *Allocator) marketAsk(symbol string) (*decimal.Decimal, bool) {
	ticker, err := allocator.price.Get(symbol)

	if err != nil || ticker.Ask == nil || ticker.Ask.Sign() <= 0 {
		// Demote the enter lot; a missing ask is subscription lag, not a fault.
		allocator.Reason = "ask unavailable"

		return nil, false
	}

	ask := ticker.Ask.Copy()

	if ask == nil || ask.Sign() <= 0 {
		allocator.Reason = "ask unavailable"

		return nil, false
	}

	return ask, true
}

func (allocator *Allocator) quoteCapital() (float64, bool) {
	quote, err := allocator.balance.AvailableQuote()

	if err != nil || quote <= 0 {
		allocator.Reason = "quote capital unavailable"

		return 0, false
	}

	return quote, true
}

func (allocator *Allocator) lotWithinBudget(
	pair *kraken.InstrumentPair,
	ask *decimal.Decimal,
	quote float64,
) (*decimal.Decimal, bool) {
	// Div scales the divisor down to the dividend scale; a scale-0 budget
	// would truncate ask 0.05 → 0 and panic. Keep ask precision in the budget.
	// Mul across mismatched decimal scales can truncate large quote budgets to
	// zero; derive the cap in float64 space then lift it into ask-scaled decimal.
	budget := decimal.NewFromFloat64(
		quote * allocator.maxFraction.Float64(),
	).SetScale(
		ask.GetScale() + 8,
	)
	quantity := allocator.quantizeQuantity(budget.Div(ask), pair)
	minLot := decimal.NewFromFloat64(pair.QtyMin)

	if quantity.Cmp(minLot) >= 0 {
		return quantity, true
	}

	minCost, err := allocator.price.Taker(pair, minLot)

	if err != nil || minCost == nil {
		allocator.Reason = "taker cost unavailable"
		errnie.Error(errnie.Err(
			errnie.Internal,
			"allocator: min lot cost unavailable for "+pair.Symbol,
			err,
		))

		return nil, false
	}

	if !allocator.balance.Available(minCost) {
		allocator.Reason = "below qty_min"

		return nil, false
	}

	if pair.CostMin != nil && minCost.Cmp(pair.CostMin) < 0 {
		allocator.Reason = "below cost_min"

		return nil, false
	}

	return allocator.quantizeQuantity(minLot, pair), true
}

func (allocator *Allocator) quantizeQuantity(
	quantity *decimal.Decimal,
	pair *kraken.InstrumentPair,
) *decimal.Decimal {
	if quantity == nil {
		return nil
	}

	value := quantity.Float64()

	if pair.QtyIncrement > 0 {
		increment := pair.QtyIncrement
		value = math.Floor(value/increment) * increment
	}

	if pair.QtyPrecision >= 0 {
		scale := math.Pow(10, float64(pair.QtyPrecision))
		value = math.Floor(value*scale) / scale
	}

	return decimal.NewFromFloat64(value)
}

func (allocator *Allocator) takerCost(
	pair *kraken.InstrumentPair,
	quantity *decimal.Decimal,
) (*decimal.Decimal, bool) {
	cost, err := allocator.price.Taker(pair, quantity)

	if err != nil || cost == nil {
		allocator.Reason = "taker cost unavailable"
		errnie.Error(errnie.Err(
			errnie.Internal,
			"allocator: taker cost unavailable for "+pair.Symbol,
			err,
		))

		return nil, false
	}

	return cost, true
}

func (allocator *Allocator) passesMinimums(
	pair *kraken.InstrumentPair,
	cost *decimal.Decimal,
) bool {
	zero := decimal.NewFromInt64(0)

	if pair.CostMin != nil && pair.CostMin.Cmp(zero) > 0 &&
		cost.Cmp(pair.CostMin) < 0 {
		allocator.Reason = "below cost_min"

		return false
	}

	return true
}

func (allocator *Allocator) balanceAvailable(cost *decimal.Decimal) bool {
	if !allocator.balance.Available(cost) {
		allocator.Reason = "insufficient balance"

		return false
	}

	return true
}

func (allocator *Allocator) persistSized(
	holding *types.Holding,
	quantity, cost *decimal.Decimal,
) bool {
	holding.Qty = quantity

	allocator.Quantity = quantity
	allocator.Cost = cost
	allocator.Action = "enter"
	allocator.Reason = "sized from ask and max_fraction"

	return true
}

func (allocator *Allocator) sync(thesis *types.Thesis, holding *types.Holding) {
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

	thesis.Positions.Delete(symbol)
	thesis.Holdings.Delete(symbol)
}
