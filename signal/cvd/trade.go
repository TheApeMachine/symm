package cvd

import (
	"errors"
	"math"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/types"
)

type quotePair struct {
	bid *decimal.Decimal
	ask *decimal.Decimal
}

type symbolState struct {
	firstTimestamp    time.Time
	lastTimestamp     time.Time
	tradeCount        float64
	buyCount          float64
	sellCount         float64
	buyQty            float64
	sellQty           float64
	buyNotional       float64
	sellNotional      float64
	priorMidpoint     float64
	hasPriorMidpoint  bool
	netFractionEstimator equation.CausalResidual
}

/*
Trade is the executed-flow signal processor. It maintains online cumulative
volume delta, execution rates, causal directional baselines, and quote response
metrics with zero wire transport or reflection.
*/
type Trade struct {
	mu         sync.Mutex
	states     map[string]*symbolState
	quoteMu    sync.Mutex
	quote      func(string) (*decimal.Decimal, *decimal.Decimal)
	priorQuote map[string]quotePair
}

/*
NewTrade constructs an empty Trade signal entity.
*/
func NewTrade() *Trade {
	return &Trade{
		states:     make(map[string]*symbolState),
		priorQuote: make(map[string]quotePair),
	}
}

/*
SetQuote installs the shared top-of-book quote source.
*/
func (trade *Trade) SetQuote(quote func(symbol string) (bid, ask *decimal.Decimal)) {
	trade.quoteMu.Lock()
	defer trade.quoteMu.Unlock()
	trade.quote = quote
}

/*
Step receives one trade observation, advances online flow accounting and
causal baselines, and returns a fully populated Measurement.
*/
func (trade *Trade) Step(observation kraken.TradeData) *data.Measurement[float64] {
	trade.mu.Lock()
	defer trade.mu.Unlock()

	state, found := trade.states[observation.Symbol]
	if !found {
		state = &symbolState{
			firstTimestamp: observation.Timestamp,
			lastTimestamp:  observation.Timestamp,
		}
		trade.states[observation.Symbol] = state
	}

	from := state.firstTimestamp
	id := observation.Symbol + ":cvd:" + observation.Timestamp.Format(time.RFC3339Nano)
	measurement := data.NewMeasurement[float64](
		id,
		observation.Symbol,
		"cvd",
		observation.Timestamp,
		from,
	)

	price := observation.Price.Float64()
	qty := observation.Qty

	if price <= 0 || qty <= 0 || math.IsNaN(price) || math.IsInf(price, 0) || math.IsNaN(qty) || math.IsInf(qty, 0) {
		measurement.Err = errnie.Error(errnie.Err(
			errnie.Validation,
			"cvd: non-positive or non-finite price/quantity",
			errors.New("invalid trade coordinates"),
		))
		return measurement
	}

	if observation.Timestamp.Before(state.lastTimestamp) {
		return nil
	}
	state.lastTimestamp = observation.Timestamp

	notional := price * qty
	isBuy := observation.Side == "buy"

	if isBuy {
		state.buyCount++
		state.buyQty += qty
		state.buyNotional += notional
	} else {
		state.sellCount++
		state.sellQty += qty
		state.sellNotional += notional
	}
	state.tradeCount++

	grossQty := state.buyQty + state.sellQty
	netQty := state.buyQty - state.sellQty
	grossNotional := state.buyNotional + state.sellNotional
	netNotional := state.buyNotional - state.sellNotional
	signedCountFraction := (state.buyCount - state.sellCount) / state.tradeCount
	signedNetFraction := netNotional / grossNotional
	meanTradeNotional := grossNotional / state.tradeCount

	putMetric(measurement, "trade_count", state.tradeCount, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "trade_count:buy", state.buyCount, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "trade_count:sell", state.sellCount, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "signed_count_fraction", signedCountFraction, data.UnitDimensionless, data.TimescaleInstantaneous)

	putMetric(measurement, "executed_quantity:buy", state.buyQty, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "executed_quantity:sell", state.sellQty, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "gross_executed_quantity", grossQty, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "net_executed_quantity", netQty, data.UnitCount, data.TimescaleInstantaneous)

	putMetric(measurement, "aggressive_notional:buy", state.buyNotional, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "aggressive_notional:sell", state.sellNotional, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "gross_notional", grossNotional, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "net_notional", netNotional, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "signed_net_fraction", signedNetFraction, data.UnitDimensionless, data.TimescaleInstantaneous)
	putMetric(measurement, "mean_trade_notional", meanTradeNotional, data.UnitRate, data.TimescaleInstantaneous)

	putMetric(measurement, "cumulative_volume_delta", netQty, data.UnitCount, data.TimescaleInstantaneous)
	putMetric(measurement, "cumulative_notional_delta", netNotional, data.UnitRate, data.TimescaleInstantaneous)
	putMetric(measurement, "cvd_epoch_from", float64(state.firstTimestamp.Unix()), data.UnitSecond, data.TimescaleInstantaneous)

	elapsedSec := observation.Timestamp.Sub(state.firstTimestamp).Seconds()
	if elapsedSec > 0 {
		putMetric(measurement, "trade_rate", state.tradeCount/elapsedSec, data.UnitPerSecond, data.TimescalePerSecond)
		putMetric(measurement, "gross_notional_rate", grossNotional/elapsedSec, data.UnitPerSecond, data.TimescalePerSecond)
		putMetric(measurement, "net_notional_rate", netNotional/elapsedSec, data.UnitPerSecond, data.TimescalePerSecond)
		putMetric(measurement, "buy_notional_rate", state.buyNotional/elapsedSec, data.UnitPerSecond, data.TimescalePerSecond)
		putMetric(measurement, "sell_notional_rate", state.sellNotional/elapsedSec, data.UnitPerSecond, data.TimescalePerSecond)
	}

	state.netFractionEstimator.Step(types.Scalar(signedNetFraction))

	if state.netFractionEstimator.HasPrior() {
		baseline := float64(state.netFractionEstimator.Baseline())
		divergence := float64(state.netFractionEstimator.Residual())
		zscore := float64(state.netFractionEstimator.ZScore())

		putMetric(measurement, "signed_net_fraction_baseline", baseline, data.UnitDimensionless, data.TimescaleInstantaneous)
		putMetric(measurement, "signed_net_fraction_divergence", divergence, data.UnitDimensionless, data.TimescaleInstantaneous)
		putMetric(measurement, "signed_net_fraction_zscore", zscore, data.UnitDimensionless, data.TimescaleInstantaneous)

		dispersion := float64(state.netFractionEstimator.Dispersion())
		if dispersion > 0 {
			measurement.SNR = (divergence * divergence) / (dispersion * dispersion)
			measurement.SNRDefined = true
		}
	}

	measurement.Maturity = float64(state.netFractionEstimator.Maturity())

	trade.calculateResponsePrice(observation.Symbol, state, measurement)

	return measurement
}

func putMetric(m *data.Measurement[float64], label string, raw float64, unit data.Unit, timescale data.Timescale) {
	m.PutMetric(data.Metric[float64]{
		Label:     label,
		Raw:       raw,
		Unit:      unit,
		Timescale: timescale,
	})
}

func (trade *Trade) calculateResponsePrice(
	symbol string,
	state *symbolState,
	measurement *data.Measurement[float64],
) {
	trade.quoteMu.Lock()
	quoteFunc := trade.quote
	prior, hasPrior := trade.priorQuote[symbol]
	trade.quoteMu.Unlock()

	if quoteFunc == nil {
		return
	}

	bid, ask := quoteFunc(symbol)
	if bid == nil || ask == nil {
		return
	}

	bidVal := bid.Float64()
	askVal := ask.Float64()

	if bidVal <= 0 || askVal <= bidVal || math.IsNaN(bidVal) || math.IsInf(bidVal, 0) || math.IsNaN(askVal) || math.IsInf(askVal, 0) {
		return
	}

	trade.quoteMu.Lock()
	trade.priorQuote[symbol] = quotePair{bid: bid, ask: ask}
	trade.quoteMu.Unlock()

	midpoint := (bidVal + askVal) / 2.0

	if !hasPrior || prior.bid == nil || prior.ask == nil {
		state.priorMidpoint = midpoint
		state.hasPriorMidpoint = true
		return
	}

	priorBidVal := prior.bid.Float64()
	priorAskVal := prior.ask.Float64()
	priorMid := (priorBidVal + priorAskVal) / 2.0

	if priorMid > 0 && midpoint > 0 {
		midpointLogReturn := math.Log(midpoint / priorMid)
		putMetric(measurement, "midpoint_log_return", midpointLogReturn, data.UnitDimensionless, data.TimescaleInstantaneous)
	}

	state.priorMidpoint = midpoint
	state.hasPriorMidpoint = true
}

/*
Close releases resources held by the Trade processor.
*/
func (trade *Trade) Close() error {
	return nil
}
