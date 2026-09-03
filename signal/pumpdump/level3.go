package pumpdump

import (
	"fmt"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

type level3State struct {
	retainedBid float64
	retainedAsk float64
}

/*
Level3 is the authoritative executable-touch market entity. It retains resting
bids and asks across one-sided updates and emits the uncrossed executable touch.
Zero Wire blocks, zero Frame allocations.
*/
type Level3 struct {
	states map[string]*level3State
	mu     sync.RWMutex
}

func NewLevel3() *Level3 {
	return &Level3{
		states: make(map[string]*level3State),
	}
}

func (l3 *Level3) Close() error {
	return nil
}

func (l3 *Level3) Step(msg kraken.Level3Data) *data.Measurement[float64] {
	l3.mu.Lock()
	defer l3.mu.Unlock()

	state, found := l3.states[msg.Symbol]

	if !found {
		state = &level3State{}
		l3.states[msg.Symbol] = state
	}

	var hasRestingBid, hasRestingAsk bool
	var newBid, newAsk float64

	for _, order := range msg.Bids {
		if order.Event == "delete" {
			continue
		}

		price := order.LimitPrice.Float64()

		if price > 0 && (price > newBid || !hasRestingBid) {
			newBid = price
			hasRestingBid = true
		}
	}

	for _, order := range msg.Asks {
		if order.Event == "delete" {
			continue
		}

		price := order.LimitPrice.Float64()

		if price > 0 && (price < newAsk || !hasRestingAsk) {
			newAsk = price
			hasRestingAsk = true
		}
	}

	wasCrossed := state.retainedBid > 0 && state.retainedAsk > 0 && state.retainedBid >= state.retainedAsk

	if hasRestingBid {
		if newBid > state.retainedBid || state.retainedBid == 0 || wasCrossed {
			state.retainedBid = newBid
		}
	}

	if hasRestingAsk {
		if newAsk < state.retainedAsk || state.retainedAsk == 0 || wasCrossed {
			state.retainedAsk = newAsk
		}
	}

	// Must have observed resting quotes on both sides
	if state.retainedBid <= 0 || state.retainedAsk <= 0 {
		return nil
	}

	// A message with no resting orders carries no fresh information
	if !hasRestingBid && !hasRestingAsk {
		return nil
	}

	// Crossed touches retain the fresh price but emit no measurement
	if state.retainedBid >= state.retainedAsk {
		return nil
	}

	midpoint := (state.retainedBid + state.retainedAsk) / 2.0
	spread := state.retainedAsk - state.retainedBid
	relativeSpread := spread / midpoint

	id := fmt.Sprintf("pumpdump:%s:%d", msg.Symbol, msg.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, msg.Symbol, "pumpdump", msg.Timestamp, msg.Timestamp)
	measurement.Metadata = make(map[string]float64)

	putPumpDumpMetric(measurement, "best_bid", state.retainedBid, data.UnitRate)
	putPumpDumpMetric(measurement, "best_ask", state.retainedAsk, data.UnitRate)
	putPumpDumpMetric(measurement, "midpoint", midpoint, data.UnitRate)
	putPumpDumpMetric(measurement, "spread", spread, data.UnitRate)
	putPumpDumpMetric(measurement, "relative_spread", relativeSpread, data.UnitDimensionless)
	// A retained touch is a direct reading with no estimator behind it, so it
	// declares no support and Finalize calls it whole.
	measurement.Finalize()

	return measurement
}
