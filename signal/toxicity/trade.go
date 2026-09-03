package toxicity

import (
	"fmt"
	"math"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

type tradeState struct {
	bracketQty         float64
	matchedBidQty      float64
	matchedAskQty      float64
	lastSec            float64
	lastNsec           float64
	prevSec            float64
	prevNsec           float64
	hasTime            bool
	hasPrevTime        bool
	bidFractionStd     equation.Standardizer
	askFractionStd     equation.Standardizer
	bidFractionSamples int
	askFractionSamples int
}

/*
Trade matches incoming trades against the symbol's retained book touch.
Outputs are directly projected into data.Measurement without intermediate
Frame allocations or string intern table lookups.
*/
type Trade struct {
	states map[string]*tradeState
	mu     sync.RWMutex
}

/*
NewTrade constructs the Trade entity.
*/
func NewTrade() *Trade {
	return &Trade{
		states: make(map[string]*tradeState),
	}
}

func (trade *Trade) Close() error { return nil }

/*
Step matches one trade against the given touch and projects the fill attribution.
A zero touch (nothing observed yet for this symbol) yields no measurement.
*/
func (trade *Trade) Step(tick kraken.TradeData, bidPrice, askPrice, bidQty, askQty float64) *data.Measurement[float64] {
	if bidPrice == 0 || askPrice == 0 {
		return nil
	}

	sec := float64(tick.Timestamp.Unix())
	nsec := float64(tick.Timestamp.Nanosecond())

	trade.mu.Lock()
	state, found := trade.states[tick.Symbol]

	if !found {
		state = &tradeState{}
		trade.states[tick.Symbol] = state
	}

	if state.hasTime {
		if sec < state.lastSec || (sec == state.lastSec && nsec < state.lastNsec) {
			trade.mu.Unlock()

			return nil
		}
	}

	// Update trade clock
	if state.hasTime {
		state.prevSec = state.lastSec
		state.prevNsec = state.lastNsec
		state.hasPrevTime = true
	} else {
		state.prevSec = sec
		state.prevNsec = nsec
		state.hasPrevTime = false
	}

	state.lastSec = sec
	state.lastNsec = nsec
	state.hasTime = true

	tradePrice := tick.Price.Float64()
	tradeQty := tick.Qty

	state.bracketQty += tradeQty

	if tick.Side == "sell" && tradePrice == bidPrice {
		state.matchedBidQty += tradeQty
	}

	if tick.Side == "buy" && tradePrice == askPrice {
		state.matchedAskQty += tradeQty
	}

	bidFillQty := state.matchedBidQty
	askFillQty := state.matchedAskQty

	bidFillFraction := 0.0

	if bidQty > 0 {
		bidFillFraction = bidFillQty / bidQty
	}

	askFillFraction := 0.0

	if askQty > 0 {
		askFillFraction = askFillQty / askQty
	}

	id := fmt.Sprintf("toxicity:trade:%s:%d", tick.Symbol, tick.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, tick.Symbol, "toxicity", tick.Timestamp, tick.Timestamp)
	measurement.Metadata = make(map[string]float64)

	putTradeMetric(measurement, "bracket_trade_quantity", state.bracketQty, data.UnitCount)
	putTradeMetric(measurement, "matched_touch_trade_quantity:bid", state.matchedBidQty, data.UnitCount)
	putTradeMetric(measurement, "matched_touch_trade_quantity:ask", state.matchedAskQty, data.UnitCount)
	putTradeMetric(measurement, "touch_fill_quantity:bid", bidFillQty, data.UnitCount)
	putTradeMetric(measurement, "touch_fill_quantity:ask", askFillQty, data.UnitCount)
	putTradeMetric(measurement, "touch_fill_fraction:bid", bidFillFraction, data.UnitDimensionless)
	putTradeMetric(measurement, "touch_fill_fraction:ask", askFillFraction, data.UnitDimensionless)

	// Fill rates
	if state.hasPrevTime {
		deltaT := (sec - state.prevSec) + (nsec-state.prevNsec)*1e-9

		if deltaT > 0 {
			putTradeMetric(measurement, "touch_fill_rate:bid", bidFillQty/deltaT, data.UnitPerSecond)
			putTradeMetric(measurement, "touch_fill_rate:ask", askFillQty/deltaT, data.UnitPerSecond)
		}
	}

	// Bid standardizer / baseline
	if bidFillFraction > 0 {
		state.bidFractionSamples++
		bidZ := state.bidFractionStd.Step(nmtypes.Number(bidFillFraction))
		bidMean := state.bidFractionStd.Mean()
		bidDiv := bidFillFraction - bidMean

		putTradeMetric(measurement, "fill_fraction_baseline:bid", bidMean, data.UnitDimensionless)
		putTradeMetric(measurement, "fill_fraction_divergence:bid", bidDiv, data.UnitDimensionless)
		putTradeMetric(measurement, "fill_fraction_zscore:bid", float64(bidZ), data.UnitDimensionless)

		if state.bidFractionSamples >= 3 {
			measurement.SNRDefined = true
			measurement.SNR = math.Abs(float64(bidZ))
		}
	}

	// Ask standardizer / baseline
	if askFillFraction > 0 {
		state.askFractionSamples++
		askZ := state.askFractionStd.Step(nmtypes.Number(askFillFraction))
		askMean := state.askFractionStd.Mean()
		askDiv := askFillFraction - askMean

		putTradeMetric(measurement, "fill_fraction_baseline:ask", askMean, data.UnitDimensionless)
		putTradeMetric(measurement, "fill_fraction_divergence:ask", askDiv, data.UnitDimensionless)
		putTradeMetric(measurement, "fill_fraction_zscore:ask", float64(askZ), data.UnitDimensionless)

		if state.askFractionSamples >= 3 {
			measurement.SNRDefined = true
			measurement.SNR = math.Abs(float64(askZ))
		}
	}

	trade.mu.Unlock()
	measurement.Finalize()

	return measurement
}

func putTradeMetric(measurement *data.Measurement[float64], name string, value float64, unit data.Unit) {
	measurement.PutMetric(data.NewMetric(
		name, value, nil, nil, unit, data.TimescaleInstantaneous,
	))
}
