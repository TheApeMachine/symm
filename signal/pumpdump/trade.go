package pumpdump

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/types"
)

type tradeState struct {
	spanStore           store.Store
	rateResidual        equation.CausalResidual
	accumulatedQty      float64
	accumulatedNotional float64
	barTradeCount       float64
	barStart            time.Time
	hasBarStart         bool
	prevTradeTime       time.Time
	hasPrevTradeTime    bool
	barOrdinal          float64
	barOpenMidpoint     float64
	hasBarOpenMidpoint  bool
	prevLogReturn       float64
	hasPrevLogReturn    bool
}

/*
Trade owns the volume-clock activity pipeline.
It accumulates trades into volume bars sized adaptively by median transaction size,
measuring throughput rates and response price dynamics without Frame or Wire blocks.
*/
type Trade struct {
	states map[string]*tradeState
	quote  func(symbol string) (bid, ask *decimal.Decimal)
	mu     sync.RWMutex
}

func NewTrade() *Trade {
	return &Trade{
		states: make(map[string]*tradeState),
	}
}

func (trade *Trade) Close() error {
	return nil
}

func (trade *Trade) SetQuote(quote func(symbol string) (bid, ask *decimal.Decimal)) {
	trade.mu.Lock()
	defer trade.mu.Unlock()

	trade.quote = quote
}

func (trade *Trade) Step(tick kraken.TradeData) *data.Measurement[float64] {
	price := tick.Price.Float64()
	qty := tick.Qty

	if price <= 0 || qty <= 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: non-positive trade (price=%f, qty=%f)", price, qty)}
	}

	trade.mu.Lock()
	defer trade.mu.Unlock()

	state, found := trade.states[tick.Symbol]

	if !found {
		state = &tradeState{
			spanStore: store.Store{
				Type:     store.DynamicRing,
				Adaptive: adaptive.Window{Type: adaptive.ADWIN},
				Reduce:   statistic.MedianReduction,
			},
		}
		trade.states[tick.Symbol] = state
	}

	notional := price * qty
	targetQty := float64(state.spanStore.Step(types.Scalar(qty)))

	if targetQty <= 0 {
		targetQty = qty
	}

	if !state.hasBarStart {
		state.barStart = tick.Timestamp
		state.hasBarStart = true
	}

	state.accumulatedQty += qty
	state.accumulatedNotional += notional
	state.barTradeCount++

	var intervalSeconds float64
	hasInterval := state.hasPrevTradeTime

	if hasInterval {
		intervalSeconds = tick.Timestamp.Sub(state.prevTradeTime).Seconds()
	}

	state.prevTradeTime = tick.Timestamp
	state.hasPrevTradeTime = true

	duration := tick.Timestamp.Sub(state.barStart).Seconds()
	id := fmt.Sprintf("pumpdump:%s:%d", tick.Symbol, tick.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, tick.Symbol, "pumpdump", tick.Timestamp, tick.Timestamp)
	measurement.Metadata = make(map[string]float64)

	putPumpDumpMetric(measurement, "trade_price", price, data.UnitRate)
	putPumpDumpMetric(measurement, "trade_quantity", qty, data.UnitCount)
	putPumpDumpMetric(measurement, "trade_notional", notional, data.UnitCount)
	putPumpDumpMetric(measurement, "volume_bar_target_quantity", targetQty, data.UnitCount)
	putPumpDumpMetric(measurement, "volume_bar_quantity", state.accumulatedQty, data.UnitCount)
	putPumpDumpMetric(measurement, "volume_bar_notional", state.accumulatedNotional, data.UnitCount)
	putPumpDumpMetric(measurement, "volume_bar_trade_count", state.barTradeCount, data.UnitCount)
	putPumpDumpMetric(measurement, "volume_bar_duration", duration, data.UnitSecond)

	if hasInterval {
		putPumpDumpMetric(measurement, "trade_interval_seconds", intervalSeconds, data.UnitSecond)
	}

	putPumpDumpMetric(measurement, "completed_volume_bar_ordinal", state.barOrdinal, data.UnitCount)

	// Midpoint quote resolution
	var currentMidpoint float64
	var hasMidpoint bool

	if trade.quote != nil {
		bid, ask := trade.quote(tick.Symbol)

		if bid != nil && ask != nil {
			bidVal := bid.Float64()
			askVal := ask.Float64()

			if bidVal > 0 && askVal > bidVal {
				currentMidpoint = (bidVal + askVal) / 2.0
				hasMidpoint = true
			}
		}
	}

	if hasMidpoint && !state.hasBarOpenMidpoint {
		state.barOpenMidpoint = currentMidpoint
		state.hasBarOpenMidpoint = true
	}

	// Closure: accumulated >= targetQty && duration > 0
	if state.accumulatedQty >= targetQty && duration > 0 {
		state.barOrdinal++
		putPumpDumpMetric(measurement, "completed_volume_bar_ordinal", state.barOrdinal, data.UnitCount)

		volumeRate := state.accumulatedQty / duration
		notionalRate := state.accumulatedNotional / duration
		tradeRate := state.barTradeCount / duration

		putPumpDumpMetric(measurement, "volume_rate", volumeRate, data.UnitPerSecond)
		putPumpDumpMetric(measurement, "notional_rate", notionalRate, data.UnitPerSecond)
		putPumpDumpMetric(measurement, "trade_rate", tradeRate, data.UnitPerSecond)

		priorRateCount := state.rateResidual.Count()
		priorRateMean := float64(state.rateResidual.Mean())

		state.rateResidual.Step(types.Scalar(notionalRate))

		if priorRateCount == 0 {
			putPumpDumpMetric(measurement, "notional_rate_baseline", notionalRate, data.UnitPerSecond)
			putPumpDumpMetric(measurement, "notional_rate_ratio", 1.0, data.UnitDimensionless)
		} else {
			putPumpDumpMetric(measurement, "notional_rate_baseline", priorRateMean, data.UnitPerSecond)
			putPumpDumpMetric(measurement, "notional_rate_ratio", notionalRate/priorRateMean, data.UnitDimensionless)
		}

		if hasMidpoint {
			putPumpDumpMetric(measurement, "midpoint", currentMidpoint, data.UnitRate)
			putPumpDumpMetric(measurement, "midpoint:at", currentMidpoint, data.UnitRate)

			if state.hasBarOpenMidpoint && state.barOpenMidpoint > 0 {
				putPumpDumpMetric(measurement, "midpoint:from", state.barOpenMidpoint, data.UnitRate)
				logReturn := math.Log(currentMidpoint / state.barOpenMidpoint)
				returnRate := logReturn / duration

				putPumpDumpMetric(measurement, "midpoint_log_return", logReturn, data.UnitDimensionless)
				putPumpDumpMetric(measurement, "midpoint_return_rate", returnRate, data.UnitPerSecond)

				posReturn := 0.0
				negReturn := 0.0

				if logReturn > 0 {
					posReturn = logReturn
				} else if logReturn < 0 {
					negReturn = -logReturn
				}

				putPumpDumpMetric(measurement, "positive_midpoint_return", posReturn, data.UnitDimensionless)
				putPumpDumpMetric(measurement, "negative_midpoint_return", negReturn, data.UnitDimensionless)

				if state.hasPrevLogReturn {
					velocity := logReturn - state.prevLogReturn
					putPumpDumpMetric(measurement, "midpoint_return_velocity", velocity, data.UnitPerSecond)
				}

				state.prevLogReturn = logReturn
				state.hasPrevLogReturn = true
			}

			state.barOpenMidpoint = currentMidpoint
		}

		// Reset bar state
		state.accumulatedQty = 0
		state.accumulatedNotional = 0
		state.barTradeCount = 0
		state.barStart = tick.Timestamp
	}

	return measurement
}
