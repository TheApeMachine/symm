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

var (
	instrumentRulesMu sync.Mutex
	sharedRules       *InstrumentRulesCache
)

/*
EnsureInstrumentRulesCache returns the process-wide instrument rules cache.
*/
func EnsureInstrumentRulesCache(ctx context.Context, pool *qpool.Q) *InstrumentRulesCache {
	instrumentRulesMu.Lock()
	defer instrumentRulesMu.Unlock()

	if sharedRules != nil && sharedRules.ctx.Err() == nil {
		return sharedRules
	}

	if sharedRules != nil {
		sharedRules.cancel()
	}

	sharedRules = NewInstrumentRulesCache(ctx)
	sharedRules.Start(pool)

	return sharedRules
}

/*
ResetInstrumentRulesCacheForTest tears down the shared cache between isolated runs.
*/
func ResetInstrumentRulesCacheForTest() {
	instrumentRulesMu.Lock()
	defer instrumentRulesMu.Unlock()

	if sharedRules != nil {
		sharedRules.cancel()
		sharedRules = nil
	}
}

func NewInstrumentRulesCache(ctx context.Context) *InstrumentRulesCache {
	ctx, cancel := context.WithCancel(ctx)

	return &InstrumentRulesCache{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (cache *InstrumentRulesCache) Start(pool *qpool.Q) {
	if pool == nil || !cache.started.CompareAndSwap(false, true) {
		return
	}

	raw := pool.CreateBroadcastGroup("raw", 0)
	subscriber := raw.Subscribe("broker:instrument-rules", 1024)

	go func() {
		for {
			select {
			case <-cache.ctx.Done():
				return
			case message, ok := <-subscriber.Incoming:
				if !ok || message == nil {
					return
				}

				cache.ingest(message.Value)
			}
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

	if math.Abs(value-aligned) <= increment*1e-6 {
		return true
	}

	next := quantityFromSteps(steps+1, increment)

	return math.Abs(value-next) <= increment*1e-6
}

func roundDownToIncrement(value, increment float64) float64 {
	if increment <= 0 {
		return value
	}

	return quantityFromSteps(orderStepsDown(value, increment), increment)
}

func orderStepsDown(value, increment float64) float64 {
	return math.Floor(value/increment + 1e-12)
}

func quantityFromSteps(steps, increment float64) float64 {
	return steps * increment
}
