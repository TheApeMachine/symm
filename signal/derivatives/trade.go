package derivatives

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

type tradeState struct {
	liqBuyTotal      float64
	liqSellTotal     float64
	grossTradeTotal  float64
	startTime        time.Time
	tradeCount       float64
	prevLiqShare     float64
	hasPrevLiqShare  bool
	lastAdvancedTime time.Time
}

/*
Trade is the liquidation-notional accounting entity.
It measures gross and net liquidation flow, liquidation share of aggregate volume,
and throughput rates across a causal timeline without Frame or Wire blocks.
*/
type Trade struct {
	states map[string]*tradeState
	clock  causalClock
	mu     sync.RWMutex
}

func NewTrade() *Trade {
	return &Trade{
		states: make(map[string]*tradeState),
		clock:  newCausalClock(),
	}
}

func (trade *Trade) Close() error {
	return nil
}

func (trade *Trade) Step(point kraken.FuturesTradeData) *data.Measurement[float64] {
	trade.mu.Lock()
	defer trade.mu.Unlock()

	stamped, advanced := trade.clock.stamp(
		point.Symbol, point.Timestamp, point.SyntheticTimestamp,
	)
	point.Timestamp = stamped

	price := point.Price.Float64()
	qty := point.Qty
	notional := price * qty

	state, found := trade.states[point.Symbol]

	if !found {
		state = &tradeState{startTime: stamped}
		trade.states[point.Symbol] = state
	}

	state.grossTradeTotal += notional

	if point.Type == "liquidation" {
		switch point.Side {
		case "buy":
			state.liqBuyTotal += notional
		case "sell":
			state.liqSellTotal += notional
		}
	}

	// Totals include every received trade, including historical reconnect data.
	// Their interval is the earliest through latest accounted event time.
	if stamped.Before(state.startTime) {
		state.startTime = stamped
	}

	if advanced {
		state.tradeCount++
		state.lastAdvancedTime = stamped
	}

	grossLiq := state.liqBuyTotal + state.liqSellTotal
	netLiq := state.liqBuyTotal - state.liqSellTotal

	id := fmt.Sprintf("derivatives:%s:%d", point.Symbol, point.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](
		id, point.Symbol, "derivatives", state.lastAdvancedTime, state.startTime,
	)
	measurement.Metadata = make(map[string]float64)

	putDerivMetric(measurement, "liquidation_notional:buy", state.liqBuyTotal, data.UnitRate)
	putDerivMetric(measurement, "liquidation_notional:sell", state.liqSellTotal, data.UnitRate)
	putDerivMetric(measurement, "gross_liquidation_notional", grossLiq, data.UnitRate)
	putDerivMetric(measurement, "net_liquidation_notional", netLiq, data.UnitRate)
	putDerivMetric(measurement, "gross_derivative_trade_notional", state.grossTradeTotal, data.UnitRate)

	if grossLiq > 0 {
		signedFraction := netLiq / grossLiq
		putDerivMetric(measurement, "liquidation_signed_fraction", signedFraction, data.UnitDimensionless)
	}

	var currentShare float64

	if state.grossTradeTotal > 0 {
		currentShare = grossLiq / state.grossTradeTotal
		putDerivMetric(measurement, "liquidation_share", currentShare, data.UnitDimensionless)
	}

	// Late trades revise totals and the earliest boundary, but do not emit a
	// rate or share change until the live event-time clock advances again.
	if advanced {
		duration := state.lastAdvancedTime.Sub(state.startTime).Seconds()

		if duration > 0 {
			putDerivMetric(measurement, "liquidation_notional_rate", grossLiq/duration, data.UnitPerSecond)
		}

		if state.hasPrevLiqShare {
			putDerivMetric(measurement, "liquidation_share_velocity", currentShare-state.prevLiqShare, data.UnitPerSecond)
		}

		state.prevLiqShare = currentShare
		state.hasPrevLiqShare = true
	}

	measurement.Metadata[data.MetadataSupport] = state.tradeCount
	measurement.Finalize()

	return measurement
}

func putDerivMetric(measurement *data.Measurement[float64], name string, value float64, unit data.Unit) {
	measurement.PutMetric(data.NewMetric(
		name, value, nil, nil, unit, data.TimescaleInstantaneous,
	))
}
