package pumpdump

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

type tradeState struct {
	quantityTarget      core.Primitive
	rateReading         map[string]core.Primitive
	rateResidual        core.Primitive
	returnResidual      core.Primitive
	returnRateResidual  core.Primitive
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
	prevNotionalRate    float64
	hasPrevNotionalRate bool
}

/*
Trade owns the volume-clock activity pipeline.
It accumulates trades into volume bars sized adaptively by median transaction size,
measuring throughput rates and response price dynamics without Frame or Wire blocks.
*/
type Trade struct {
	api    *websocket.API
	states map[string]*tradeState
}

func NewTrade(api *websocket.API) *Trade {
	return &Trade{
		api:    api,
		states: make(map[string]*tradeState),
	}
}

func (trade *Trade) Close() error {
	return nil
}

func (trade *Trade) Step(tick kraken.TradeData) *data.Measurement[float64] {
	price := tick.Price.Float64()
	qty := tick.Qty

	if price <= 0 || qty <= 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf(
			"pumpdump: non-positive trade (price=%f, qty=%f)", price, qty,
		)}
	}

	state, found := trade.states[tick.Symbol]

	if !found {
		state = &tradeState{
			quantityTarget:     newQuantityTarget(),
			rateResidual:       equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())),
			returnResidual:     equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())),
			returnRateResidual: equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())),
			rateReading:        core.To[map[string]core.Primitive](core.Record(map[string]any{"count": 0.0, "has_prior": false, "variance_defined": false})),
		}
		trade.states[tick.Symbol] = state
	}

	if state.hasPrevTradeTime && tick.Timestamp.Before(state.prevTradeTime) {
		return nil
	}
	notional := price * qty
	targetQty, err := transport.Evaluate[float64](state.quantityTarget, core.From(qty))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	if targetQty <= 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: invalid observed quantity target %g", targetQty)}
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

	trade.api.Book(tick.Symbol, func(spotbook *book.Book) {
  if spotbook == nil || spotbook.BestBid() == nil || spotbook.BestAsk() == nil { return }

		currentMidpoint = spotbook.Midpoint().Float64()
		hasMidpoint = true
	})

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

		state.rateReading, err = transport.Evaluate[map[string]core.Primitive](state.rateResidual, core.From(notionalRate))
		if err != nil {
			measurement.Err = err
			return measurement
		}
		baseline, err := core.Field[float64](state.rateReading, "baseline")
		if err != nil {
			measurement.Err = err
			return measurement
		}
		putPumpDumpMetric(measurement, "notional_rate_baseline", baseline, data.UnitPerSecond)
		putPumpDumpMetric(measurement, "notional_rate_ratio", notionalRate/baseline, data.UnitDimensionless)

		// notional_rate_zscore is this entity's headline throughput reading and
		// the evidence VerticalIgnition reads. The ratio above says how many
		// times the baseline this bar ran at; the z-score says whether that
		// distance is large against the estimator's own noise.
		putResidualReadings(
			measurement, "notional_rate", state.rateReading, data.UnitPerSecond,
		)

		// notional_rate says how fast capital is moving through this bar;
		// notional_rate_velocity says whether that is accelerating. Momentum
		// and ProfitRun both read the acceleration, not the level.
		if state.hasPrevNotionalRate {
			putPumpDumpMetric(
				measurement, "notional_rate_velocity",
				notionalRate-state.prevNotionalRate, data.UnitPerSecond,
			)
		}

		state.prevNotionalRate = notionalRate
		state.hasPrevNotionalRate = true

		if hasMidpoint {
			putPumpDumpMetric(measurement, "midpoint", currentMidpoint, data.UnitRate)
			putPumpDumpMetric(measurement, "midpoint:at", currentMidpoint, data.UnitRate)

			if state.hasBarOpenMidpoint && state.barOpenMidpoint > 0 {
				putPumpDumpMetric(measurement, "midpoint:from", state.barOpenMidpoint, data.UnitRate)
				logReturn := math.Log(currentMidpoint / state.barOpenMidpoint)
				returnRate := logReturn / duration

				putPumpDumpMetric(measurement, "midpoint_log_return", logReturn, data.UnitDimensionless)
				putPumpDumpMetric(measurement, "midpoint_return_rate", returnRate, data.UnitPerSecond)

				// OrganicTrend and FadedExhaustion read these two standardized
				// forms: a bar's move, and that move per second, each against
				// the run of bars this symbol has already produced.
				returns, err := transport.Evaluate[map[string]core.Primitive](state.returnResidual, core.From(logReturn))
				if err != nil {
					measurement.Err = err
					return measurement
				}
				returnRates, err := transport.Evaluate[map[string]core.Primitive](state.returnRateResidual, core.From(returnRate))
				if err != nil {
					measurement.Err = err
					return measurement
				}
				baseline, err := core.Field[float64](returns, "baseline")
				if err != nil {
					measurement.Err = err
					return measurement
				}
				putPumpDumpMetric(measurement, "midpoint_return_baseline", baseline, data.UnitDimensionless)
				putResidualReadings(measurement, "midpoint_return", returns, data.UnitDimensionless)
				putResidualReadings(measurement, "midpoint_return_rate", returnRates, data.UnitPerSecond)

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

	// Quality is derived by Finalize from the measurement's own facts. This
	// entity always carries a throughput estimator, so it always declares its
	// support: an absent support slot would tell Finalize this is a stateless
	// direct reading and mark it whole, when in truth it may still be immature.
	rate := core.NewDecoder(state.rateReading)
	measurement.Metadata[data.MetadataSupport] = core.Decode[float64](rate, "count")
	if core.Decode[bool](rate, "has_prior") && core.Decode[bool](rate, "variance_defined") {
		noiseVariance := core.Decode[float64](rate, "variance")
		if noiseVariance > 0 {
			measurement.Metadata[data.MetadataDivergence] = core.Decode[float64](rate, "residual")
			measurement.Metadata[data.MetadataNoiseVariance] = noiseVariance
		}
	}
	measurement.Err = errors.Join(measurement.Err, rate.Error())

	measurement.Finalize()

	return measurement
}

/*
putResidualReadings publishes the standardized forms of one metric: its
divergence from the estimator's prior mean, and that divergence in units of
the estimator's own dispersion. Both are undefined until a prior exists, so
nothing is emitted on the first observation rather than a fabricated zero.

The estimator must already have observed this sample; the caller drains its delivery run, so
that a metric whose raw form is emitted conditionally cannot silently skip the
update and leave the baseline behind the tape.
*/
func putResidualReadings(measurement *data.Measurement[float64], name string, fields map[string]core.Primitive, unit data.Unit) {
	decoder := core.NewDecoder(fields)
	if core.Decode[bool](decoder, "has_prior") {
		putPumpDumpMetric(measurement, name+"_divergence", core.Decode[float64](decoder, "residual"), unit)
		putPumpDumpMetric(measurement, name+"_zscore", core.Decode[float64](decoder, "zscore"), data.UnitDimensionless)
	}
	measurement.Err = errors.Join(measurement.Err, decoder.Error())
}
