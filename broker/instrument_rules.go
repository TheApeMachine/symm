package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
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
ResetInstrumentRulesCacheForTest is a no-op; caches are constructed per runtime.Runtime.
*/
func ResetInstrumentRulesCacheForTest() {}

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

	raw, err := qpool.NewBroadcastGroup(cache.ctx, "raw", 0)
	if err != nil {
		return
	}
	consumer := raw.Subscribe("broker:instrument-rules", 1024)

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
		return
	}

	for _, pair := range update.Pairs {
		if pair.Symbol == "" {
			continue
		}

		cache.pairs.Store(pair.Symbol, pair)
	}
}

/*
InstallPairForTest seeds one pair without a live instrument feed.
*/
func (cache *InstrumentRulesCache) InstallPairForTest(pair market.InstrumentPair) {
	cache.pairs.Store(pair.Symbol, pair)
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

	if pair.QtyMin > 0 && quantity < pair.QtyMin {
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

		if cost < pair.CostMin {
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

func isAligned(value, increment float64) bool {
	if increment <= 0 {
		return true
	}

	steps := orderStepsDown(value, increment)
	aligned := quantityFromSteps(steps, increment)
	tolerance := alignmentTolerance(value, increment)

	if math.Abs(value-aligned) <= tolerance {
		return true
	}

	next := quantityFromSteps(steps+1, increment)

	return math.Abs(value-next) <= tolerance
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

func orderStepsDown(value, increment float64) float64 {
	return math.Floor(value/increment + 1e-12)
}

func orderStepsUp(value, increment float64) float64 {
	return math.Ceil(value/increment - 1e-12)
}

func quantityFromSteps(steps, increment float64) float64 {
	return steps * increment
}

func alignmentTolerance(value, increment float64) float64 {
	tolerance := increment * 1e-6
	ulp := math.Nextafter(math.Abs(value), math.Inf(1)) - math.Abs(value)

	if floatingTolerance := ulp * 8; floatingTolerance > tolerance {
		return floatingTolerance
	}

	return tolerance
}
