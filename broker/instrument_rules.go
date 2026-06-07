package broker

import (
	"math/big"
	"strconv"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

/*
InstrumentRulesCache ingests Kraken instrument snapshots from the raw bus.
*/
type InstrumentRulesCache struct {
	ctx     context.Context
	cancel  context.CancelFunc
	pairs   sync.Map
	started atomic.Bool
}

/*
EnsureInstrumentRulesCache constructs a fresh instrument-rules cache. Prefer
runtime.Runtime at the process root so live components share one instance.
*/
func EnsureInstrumentRulesCache(ctx context.Context, pool *qpool.Q[any]) *InstrumentRulesCache {
	cache := NewInstrumentRulesCache(ctx)
	cache.Start(pool)

	return cache
}

func NewInstrumentRulesCache(ctx context.Context) *InstrumentRulesCache {
	ctx, cancel := context.WithCancel(ctx)

	return &InstrumentRulesCache{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (cache *InstrumentRulesCache) Start(pool *qpool.Q[any]) {
	if pool == nil || !cache.started.CompareAndSwap(false, true) {
		return
	}

	// Pool registry only — qpool.NewBroadcastGroup creates a detached group that
	// never receives the bus traffic (2026-06-07 incident, commit e26ef63b).
	raw := pool.CreateBroadcastGroup("raw", 0)

	if raw == nil {
		errnie.Error(errors.New("broker/instrument_rules: raw broadcast group unavailable — rules cache will never ingest"), "broker/instrument_rules")
		return
	}

	consumer := raw.Subscribe("broker:instrument-rules", 1024)

	if consumer == nil {
		errnie.Error(errors.New("broker/instrument_rules: raw subscription failed — rules cache will never ingest"), "broker/instrument_rules")
		return
	}

	go func() {
		for {
			message, err := consumer.Wait(cache.ctx)

			if err != nil {
				return
			}

			if message == nil {
				return
			}

			cache.ingest(message.Value)
		}
	}()
}

func (cache *InstrumentRulesCache) ingest(value any) {
	frame, ok := value.(map[string]any)

	if !ok {
		return
	}

	if frame["channel"] != public.InstrumentsChannel {
		return
	}

	rawData, ok := frame["data"].(json.RawMessage)

	if !ok {
		return
	}

	var update market.InstrumentUpdate

	if err := sonic.Unmarshal(rawData, &update); err != nil {
		// A malformed instrument frame is a data-layer incident, not noise:
		// silently skipping it is how a desk ends up with no rules for a pair
		// and rejects every order at the worst possible moment.
		errnie.Error(fmt.Errorf("instrument rules: undecodable frame: %w", err))

		return
	}

	for _, pair := range update.Pairs {
		if pair.Symbol == "" {
			errnie.Warn("instrument rules: pair without symbol in instrument frame — skipped")

			continue
		}

		cache.pairs.Store(pair.Symbol, pair)
	}
}

/*
InstallPair seeds one pair without a live instrument feed.
*/
func (cache *InstrumentRulesCache) InstallPair(pair market.InstrumentPair) {
	cache.pairs.Store(pair.Symbol, pair)
}

/*
InstallPairForTest is an alias for InstallPair kept for tests.
*/
func (cache *InstrumentRulesCache) InstallPairForTest(pair market.InstrumentPair) {
	cache.InstallPair(pair)
}

/*
PriceTickSize returns the exchange price increment for symbol.
*/
func (cache *InstrumentRulesCache) PriceTickSize(symbol string) (float64, error) {
	if cache == nil {
		return 0, fmt.Errorf("broker: instrument rules cache is required")
	}

	pair, ok := cache.pair(symbol)

	if !ok {
		return 0, fmt.Errorf("broker: unknown pair %s", symbol)
	}

	if pair.PriceIncrement <= 0 {
		return 0, fmt.Errorf("broker: missing price increment for %s", symbol)
	}

	return pair.PriceIncrement, nil
}

func (cache *InstrumentRulesCache) pair(symbol string) (market.InstrumentPair, bool) {
	value, ok := cache.pairs.Load(symbol)

	if !ok {
		return market.InstrumentPair{}, false
	}

	pair, ok := value.(market.InstrumentPair)

	return pair, ok
}

/*
PrepareOrder rounds quantity and limit price to exchange increments, then validates
minimums on the aligned values. Submit only the returned quantity and price.
*/
func (cache *InstrumentRulesCache) PrepareOrder(
	symbol string,
	quantity float64,
	price float64,
	orderType trading.OrderType,
) (float64, float64, error) {
	alignedQty, alignedPrice := cache.AlignOrder(symbol, quantity, price, orderType)

	if alignedQty <= 0 {
		return alignedQty, alignedPrice, fmt.Errorf(
			"preflight: quantity rounds to zero below increment for %s",
			symbol,
		)
	}

	if err := cache.ValidateOrder(symbol, alignedQty, alignedPrice, orderType); err != nil {
		return alignedQty, alignedPrice, err
	}

	return alignedQty, alignedPrice, nil
}

/*
PrepareEntryOrder aligns an entry and raises it to the exchange minimum quantity
or minimum cost when the requested exposure is smaller than Kraken will accept.
*/
func (cache *InstrumentRulesCache) PrepareEntryOrder(
	symbol string,
	quantity float64,
	price float64,
	orderType trading.OrderType,
) (float64, float64, error) {
	alignedQty, alignedPrice := cache.AlignOrder(symbol, quantity, price, orderType)
	minimumQty, ok := cache.MinimumOrderQuantity(symbol, alignedPrice)

	if ok && minimumQty > alignedQty {
		alignedQty = minimumQty
	}

	if alignedQty <= 0 {
		return alignedQty, alignedPrice, fmt.Errorf(
			"preflight: quantity rounds to zero below increment for %s",
			symbol,
		)
	}

	if err := cache.ValidateOrder(symbol, alignedQty, alignedPrice, orderType); err != nil {
		return alignedQty, alignedPrice, err
	}

	return alignedQty, alignedPrice, nil
}

/*
MinimumOrderQuantity returns the smallest aligned quantity Kraken will accept for
symbol at price, considering both quantity and cost minimums when they are known.
*/
func (cache *InstrumentRulesCache) MinimumOrderQuantity(
	symbol string,
	price float64,
) (float64, bool) {
	pair, ok := cache.pair(symbol)

	if !ok {
		return 0, false
	}

	minimumQty := pair.QtyMin

	if pair.CostMin > 0 && price > 0 {
		costQty := pair.CostMin / price

		if costQty > minimumQty {
			minimumQty = costQty
		}
	}

	if minimumQty <= 0 {
		return 0, true
	}

	if pair.QtyIncrement > 0 {
		minimumQty = roundUpToIncrement(minimumQty, pair.QtyIncrement)
	}

	return minimumQty, true
}

// minimumRuleTolerance forgives sub-ppm float artifacts from position
// bookkeeping when checking venue minimums: a stop_loss on a 20000-minimum
// position held as 19999.99999000 (5e-10 relative, YALA/EUR 2026-06-07) was
// blocked from ever closing. Genuine undersizing is still rejected, and the
// venue keeps its own final say.
const minimumRuleTolerance = 1e-6

/*
ValidateOrder rejects quantities and prices that violate Kraken instrument rules.
*/
func (cache *InstrumentRulesCache) ValidateOrder(
	symbol string,
	quantity float64,
	price float64,
	orderType trading.OrderType,
) error {
	pair, ok := cache.pair(symbol)

	if !ok {
		return fmt.Errorf("preflight: missing instrument rules for %s", symbol)
	}

	if quantity <= 0 {
		return fmt.Errorf("preflight: quantity must be positive")
	}

	if pair.QtyMin > 0 && quantity < pair.QtyMin*(1-minimumRuleTolerance) {
		return fmt.Errorf(
			"preflight: quantity %.8f below minimum %.8f for %s",
			quantity,
			pair.QtyMin,
			symbol,
		)
	}

	if pair.QtyIncrement > 0 && !isAligned(quantity, pair.QtyIncrement) {
		return fmt.Errorf(
			"preflight: quantity %.8f is not aligned to increment %.8f for %s",
			quantity,
			pair.QtyIncrement,
			symbol,
		)
	}

	if orderType != trading.Market && price > 0 {
		if pair.PriceIncrement > 0 && !isAligned(price, pair.PriceIncrement) {
			return fmt.Errorf(
				"preflight: price %.8f is not aligned to increment %.8f for %s",
				price,
				pair.PriceIncrement,
				symbol,
			)
		}
	}

	if pair.CostMin > 0 && price > 0 {
		cost := quantity * price

		if cost < pair.CostMin*(1-minimumRuleTolerance) {
			return fmt.Errorf(
				"preflight: order cost %.8f below minimum %.8f for %s",
				cost,
				pair.CostMin,
				symbol,
			)
		}
	}

	return nil
}

/*
AlignOrder rounds quantity and limit price to exchange increments.
*/
func (cache *InstrumentRulesCache) AlignOrder(
	symbol string,
	quantity float64,
	price float64,
	orderType trading.OrderType,
) (float64, float64) {
	pair, ok := cache.pair(symbol)

	if !ok {
		return quantity, price
	}

	alignedQty := quantity

	if pair.QtyIncrement > 0 {
		alignedQty = roundDownToIncrement(quantity, pair.QtyIncrement)
	}

	alignedPrice := price

	if orderType != trading.Market && pair.PriceIncrement > 0 && price > 0 {
		alignedPrice = roundDownToIncrement(price, pair.PriceIncrement)
	}

	return alignedQty, alignedPrice
}

/*
Grid arithmetic is exact, on math/big rationals built from each float's
shortest round-trip DECIMAL — the number the exchange actually sent ("0.00001"),
not its binary approximation. The previous float stepping —
floor(value/increment + 1e-12) — broke at large step counts: 20000/0.00001
computes as 1999999999.9999998, the absolute nudge is six orders of magnitude
too small at 2e9 steps, and the entry left the desk one full increment short of
the venue minimum (the unsellable YALA/EUR position of 2026-06-07). With exact
decimal ratios, on-grid values are integers BY IDENTITY and alignment needs no
tolerance at all.
*/
func decimalRat(value float64) *big.Rat {
	rat, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'f', -1, 64))

	if !ok {
		return new(big.Rat).SetFloat64(value)
	}

	return rat
}

// gridRatio returns the exact rational value/increment.
func gridRatio(value, increment float64) *big.Rat {
	return new(big.Rat).Quo(decimalRat(value), decimalRat(increment))
}

func isAligned(value, increment float64) bool {
	if increment <= 0 {
		return true
	}

	return gridRatio(value, increment).IsInt()
}

func roundDownToIncrement(value, increment float64) float64 {
	if increment <= 0 {
		return value
	}

	return quantityFromSteps(orderStepsDown(value, increment), increment)
}

func roundUpToIncrement(value, increment float64) float64 {
	if increment <= 0 {
		return value
	}

	return quantityFromSteps(orderStepsUp(value, increment), increment)
}

// orderStepsDown floors the exact ratio: how many whole increments fit.
func orderStepsDown(value, increment float64) float64 {
	ratio := gridRatio(value, increment)
	steps := new(big.Int).Quo(ratio.Num(), ratio.Denom())

	// Quo truncates toward zero; flooring a negative non-integer steps down.
	if ratio.Sign() < 0 && !ratio.IsInt() {
		steps.Sub(steps, big.NewInt(1))
	}

	result, _ := new(big.Float).SetInt(steps).Float64()

	return result
}

func orderStepsUp(value, increment float64) float64 {
	ratio := gridRatio(value, increment)
	steps := new(big.Int).Quo(ratio.Num(), ratio.Denom())

	if ratio.Sign() > 0 && !ratio.IsInt() {
		steps.Add(steps, big.NewInt(1))
	}

	result, _ := new(big.Float).SetInt(steps).Float64()

	return result
}

// quantityFromSteps returns the float closest to the EXACT decimal grid point
// steps × increment, so a prepared order is never a hair below the grid value
// it claims to sit on.
func quantityFromSteps(steps, increment float64) float64 {
	exact := new(big.Rat).Mul(decimalRat(steps), decimalRat(increment))
	result, _ := exact.Float64()

	return result
}
