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

type level3State struct {
	retainedBid    float64
	retainedAsk    float64
	retainedBidQty float64
	retainedAskQty float64
	hasRetainedBid bool
	hasRetainedAsk bool

	prevBid      float64
	prevAsk      float64
	prevBidQty   float64
	prevAskQty   float64
	hasPrevTouch bool
	prevSec      float64
	prevNsec     float64

	lastSec  float64
	lastNsec float64
	hasTime  bool

	withdrawalBidStd equation.Standardizer
	withdrawalAskStd equation.Standardizer
	retreatBidStd    equation.Standardizer
	retreatAskStd    equation.Standardizer

	withdrawalBidSamples int
	withdrawalAskSamples int
	retreatBidSamples    int
	retreatAskSamples    int
}

/*
Level3 is the book-touch market entity.
Outputs are directly projected into data.Measurement without intermediate
Frame allocations or string intern table lookups.
*/
type Level3 struct {
	states map[string]*level3State
	mu     sync.RWMutex
}

/*
NewLevel3 constructs the Level3 entity.
*/
func NewLevel3() *Level3 {
	return &Level3{
		states: make(map[string]*level3State),
	}
}

func (level3 *Level3) Close() error { return nil }

/*
Touch returns the last known touch for a symbol.
*/
func (level3 *Level3) Touch(symbol string) (float64, float64, float64, float64, bool) {
	level3.mu.RLock()
	defer level3.mu.RUnlock()

	state, found := level3.states[symbol]

	if !found || !state.hasPrevTouch {
		return 0, 0, 0, 0, false
	}

	return state.prevBid, state.prevAsk, state.prevBidQty, state.prevAskQty, true
}

/*
Step processes a Level3Data message, tracks the book touch, computes
attribution metrics, and projects the measurement.
*/
func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	bidPrice, askPrice, bidQty, askQty := level3.bestTouch(message)
	symbol := message.Symbol
	at := message.Timestamp
	sec := float64(at.Unix())
	nsec := float64(at.Nanosecond())

	level3.mu.Lock()
	state, found := level3.states[symbol]

	if !found {
		state = &level3State{}
		level3.states[symbol] = state
	}

	if state.hasTime {
		if sec < state.lastSec || (sec == state.lastSec && nsec < state.lastNsec) {
			level3.mu.Unlock()

			return nil
		}
	}

	state.lastSec = sec
	state.lastNsec = nsec
	state.hasTime = true

	withdrewBid := withdrawsPrice(message.Bids, state.retainedBid, state.hasRetainedBid)
	withdrewAsk := withdrawsPrice(message.Asks, state.retainedAsk, state.hasRetainedAsk)

	if bidPrice == 0 && askPrice == 0 && !withdrewBid && !withdrewAsk {
		level3.mu.Unlock()

		return nil
	}

	surrenderBid := withdrewBid && bidPrice == 0
	surrenderAsk := withdrewAsk && askPrice == 0

	if surrenderBid {
		state.hasRetainedBid = false
		state.retainedBid = 0
		state.retainedBidQty = 0
	}

	if surrenderAsk {
		state.hasRetainedAsk = false
		state.retainedAsk = 0
		state.retainedAskQty = 0
	}

	if bidPrice > 0 && (!state.hasRetainedBid || bidPrice >= state.retainedBid || withdrewBid) {
		state.retainedBid = bidPrice
		state.retainedBidQty = bidQty
		state.hasRetainedBid = true
	}

	if askPrice > 0 && (!state.hasRetainedAsk || askPrice <= state.retainedAsk || withdrewAsk) {
		state.retainedAsk = askPrice
		state.retainedAskQty = askQty
		state.hasRetainedAsk = true
	}

	complete := state.hasRetainedBid && state.hasRetainedAsk
	uncrossed := complete && state.retainedBid > 0 && state.retainedBid < state.retainedAsk

	if !uncrossed {
		level3.mu.Unlock()

		return nil
	}

	if !state.hasPrevTouch {
		state.prevBid = state.retainedBid
		state.prevAsk = state.retainedAsk
		state.prevBidQty = state.retainedBidQty
		state.prevAskQty = state.retainedAskQty
		state.prevSec = sec
		state.prevNsec = nsec
		state.hasPrevTouch = true

		state.withdrawalBidStd.Step(0)
		state.withdrawalAskStd.Step(0)
		state.retreatBidStd.Step(0)
		state.retreatAskStd.Step(0)
	}

	// Current touch
	curBid := state.retainedBid
	curAsk := state.retainedAsk
	curBidQty := state.retainedBidQty
	curAskQty := state.retainedAskQty

	// Previous touch
	prevBid := state.prevBid
	prevAsk := state.prevAsk
	prevBidQty := state.prevBidQty
	prevAskQty := state.prevAskQty

	deltaT := (sec - state.prevSec) + (nsec-state.prevNsec)*1e-9

	id := fmt.Sprintf("toxicity:level3:%s:%d", symbol, at.UnixNano())
	measurement := data.NewMeasurement[float64](id, symbol, "toxicity", at, at)
	measurement.Metadata = make(map[string]float64)

	putLevel3Metric(measurement, "best_price:bid", curBid, data.UnitRate)
	putLevel3Metric(measurement, "best_price:ask", curAsk, data.UnitRate)
	putLevel3Metric(measurement, "previous_best_price:bid", prevBid, data.UnitRate)
	putLevel3Metric(measurement, "previous_best_price:ask", prevAsk, data.UnitRate)
	putLevel3Metric(measurement, "touch_quantity:bid", curBidQty, data.UnitCount)
	putLevel3Metric(measurement, "touch_quantity:ask", curAskQty, data.UnitCount)
	putLevel3Metric(measurement, "previous_touch_quantity:bid", prevBidQty, data.UnitCount)
	putLevel3Metric(measurement, "previous_touch_quantity:ask", prevAskQty, data.UnitCount)
	putLevel3Metric(measurement, "unfilled_residual_quantity:bid", prevBidQty, data.UnitCount)
	putLevel3Metric(measurement, "unfilled_residual_quantity:ask", prevAskQty, data.UnitCount)

	logChangeBid := 0.0

	if prevBid > 0 && curBid > 0 {
		logChangeBid = math.Log(curBid / prevBid)
	}

	logChangeAsk := 0.0

	if prevAsk > 0 && curAsk > 0 {
		logChangeAsk = math.Log(curAsk / prevAsk)
	}

	putLevel3Metric(measurement, "touch_price_log_change:bid", logChangeBid, data.UnitDimensionless)
	putLevel3Metric(measurement, "touch_price_log_change:ask", logChangeAsk, data.UnitDimensionless)

	// Attribution Bid
	retreatedBidQty := 0.0
	retreatFractionBid := 0.0
	withdrawnBidQty := 0.0
	withdrawalFractionBid := 0.0
	replenishedBidQty := 0.0
	replenishmentFractionBid := 0.0

	if curBid < prevBid {
		retreatedBidQty = prevBidQty
		retreatFractionBid = 1.0
	} else if curBid == prevBid {
		if curBidQty < prevBidQty {
			withdrawnBidQty = prevBidQty - curBidQty

			if prevBidQty > 0 {
				withdrawalFractionBid = withdrawnBidQty / prevBidQty
			}
		} else if curBidQty > prevBidQty {
			replenishedBidQty = curBidQty - prevBidQty

			if prevBidQty > 0 {
				replenishmentFractionBid = replenishedBidQty / prevBidQty
			}
		}
	}

	putLevel3Metric(measurement, "retreated_quantity:bid", retreatedBidQty, data.UnitCount)
	putLevel3Metric(measurement, "net_withdrawn_quantity:bid", withdrawnBidQty, data.UnitCount)
	putLevel3Metric(measurement, "net_replenished_quantity:bid", replenishedBidQty, data.UnitCount)
	putLevel3Metric(measurement, "retreat_fraction:bid", retreatFractionBid, data.UnitDimensionless)
	putLevel3Metric(measurement, "net_withdrawal_fraction:bid", withdrawalFractionBid, data.UnitDimensionless)
	putLevel3Metric(measurement, "net_replenishment_fraction:bid", replenishmentFractionBid, data.UnitDimensionless)

	// Attribution Ask
	retreatedAskQty := 0.0
	retreatFractionAsk := 0.0
	withdrawnAskQty := 0.0
	withdrawalFractionAsk := 0.0
	replenishedAskQty := 0.0
	replenishmentFractionAsk := 0.0

	if curAsk > prevAsk {
		retreatedAskQty = prevAskQty
		retreatFractionAsk = 1.0
	} else if curAsk == prevAsk {
		if curAskQty < prevAskQty {
			withdrawnAskQty = prevAskQty - curAskQty

			if prevAskQty > 0 {
				withdrawalFractionAsk = withdrawnAskQty / prevAskQty
			}
		} else if curAskQty > prevAskQty {
			replenishedAskQty = curAskQty - prevAskQty

			if prevAskQty > 0 {
				replenishmentFractionAsk = replenishedAskQty / prevAskQty
			}
		}
	}

	putLevel3Metric(measurement, "retreated_quantity:ask", retreatedAskQty, data.UnitCount)
	putLevel3Metric(measurement, "net_withdrawn_quantity:ask", withdrawnAskQty, data.UnitCount)
	putLevel3Metric(measurement, "net_replenished_quantity:ask", replenishedAskQty, data.UnitCount)
	putLevel3Metric(measurement, "retreat_fraction:ask", retreatFractionAsk, data.UnitDimensionless)
	putLevel3Metric(measurement, "net_withdrawal_fraction:ask", withdrawalFractionAsk, data.UnitDimensionless)
	putLevel3Metric(measurement, "net_replenishment_fraction:ask", replenishmentFractionAsk, data.UnitDimensionless)

	// Rates
	if deltaT > 0 {
		putLevel3Metric(measurement, "retreat_rate:bid", retreatedBidQty/deltaT, data.UnitPerSecond)
		putLevel3Metric(measurement, "net_withdrawal_rate:bid", withdrawnBidQty/deltaT, data.UnitPerSecond)
		putLevel3Metric(measurement, "net_replenishment_rate:bid", replenishedBidQty/deltaT, data.UnitPerSecond)

		putLevel3Metric(measurement, "retreat_rate:ask", retreatedAskQty/deltaT, data.UnitPerSecond)
		putLevel3Metric(measurement, "net_withdrawal_rate:ask", withdrawnAskQty/deltaT, data.UnitPerSecond)
		putLevel3Metric(measurement, "net_replenishment_rate:ask", replenishedAskQty/deltaT, data.UnitPerSecond)
	}

	// Bid standardizers
	if withdrawalFractionBid > 0 {
		state.withdrawalBidSamples++
		z := state.withdrawalBidStd.Step(nmtypes.Number(withdrawalFractionBid))
		mean := state.withdrawalBidStd.Mean()
		div := withdrawalFractionBid - mean

		putLevel3Metric(measurement, "withdrawal_fraction_baseline:bid", mean, data.UnitDimensionless)
		putLevel3Metric(measurement, "withdrawal_fraction_divergence:bid", div, data.UnitDimensionless)
		putLevel3Metric(measurement, "withdrawal_fraction_zscore:bid", float64(z), data.UnitDimensionless)
	}

	if retreatFractionBid > 0 {
		state.retreatBidSamples++
		z := state.retreatBidStd.Step(nmtypes.Number(retreatFractionBid))
		mean := state.retreatBidStd.Mean()

		putLevel3Metric(measurement, "retreat_fraction_baseline:bid", mean, data.UnitDimensionless)
		putLevel3Metric(measurement, "retreat_fraction_zscore:bid", float64(z), data.UnitDimensionless)
	}

	// Ask standardizers
	if withdrawalFractionAsk > 0 {
		state.withdrawalAskSamples++
		z := state.withdrawalAskStd.Step(nmtypes.Number(withdrawalFractionAsk))
		mean := state.withdrawalAskStd.Mean()
		div := withdrawalFractionAsk - mean

		putLevel3Metric(measurement, "withdrawal_fraction_baseline:ask", mean, data.UnitDimensionless)
		putLevel3Metric(measurement, "withdrawal_fraction_divergence:ask", div, data.UnitDimensionless)
		putLevel3Metric(measurement, "withdrawal_fraction_zscore:ask", float64(z), data.UnitDimensionless)
	}

	if retreatFractionAsk > 0 {
		state.retreatAskSamples++
		z := state.retreatAskStd.Step(nmtypes.Number(retreatFractionAsk))
		mean := state.retreatAskStd.Mean()

		putLevel3Metric(measurement, "retreat_fraction_baseline:ask", mean, data.UnitDimensionless)
		putLevel3Metric(measurement, "retreat_fraction_zscore:ask", float64(z), data.UnitDimensionless)
	}

	// Advance previous touch
	state.prevBid = curBid
	state.prevAsk = curAsk
	state.prevBidQty = curBidQty
	state.prevAskQty = curAskQty
	state.prevSec = sec
	state.prevNsec = nsec

	measurement.Maturity = 1.0
	measurement.SNR = 0.0

	level3.mu.Unlock()
	measurement.Finalize()

	return measurement
}

func putLevel3Metric(measurement *data.Measurement[float64], name string, value float64, unit data.Unit) {
	measurement.PutMetric(data.NewMetric(
		name, value, nil, nil, unit, data.TimescaleInstantaneous,
	))
}

func (level3 *Level3) bestTouch(
	message kraken.Level3Data,
) (bidPrice, askPrice, bidQty, askQty float64) {
	for _, order := range message.Bids {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); price > bidPrice {
			bidPrice = price
			bidQty = order.OrderQty.Float64()
		}
	}

	for _, order := range message.Asks {
		if !order.Resting() {
			continue
		}

		if price := order.LimitPrice.Float64(); askPrice == 0 || price < askPrice {
			askPrice = price
			askQty = order.OrderQty.Float64()
		}
	}

	return bidPrice, askPrice, bidQty, askQty
}

func withdrawsPrice(orders []kraken.Level3Order, price float64, hasPrice bool) bool {
	if !hasPrice || price == 0 {
		return false
	}

	for _, order := range orders {
		if order.Event == "delete" && order.LimitPrice.Float64() == price {
			return true
		}
	}

	return false
}
