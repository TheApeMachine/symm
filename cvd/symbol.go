package cvd

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const (
	cvdWindow         = 15 * time.Minute
	cvdForwardHorizon = 30 * time.Minute // pre-position; the pump may be minutes away
	cvdMinTrades      = 40               // trades in window before emitting
	cvdMinNetFraction = 0.60             // |net|/gross required for a directional read
	cvdPriceFlatBand  = 0.003            // |drift| <= this over the window counts as "flat"
	cvdSampleCap      = 4096
)

type tick struct {
	at     time.Time
	price  float64
	signed float64 // +volume on a taker buy, -volume on a taker sell
}

// CVDSymbol accumulates executed-flow (cumulative volume delta) for one symbol
// and emits an accumulation / distribution reading when flow is strongly
// one-sided while price is suppressed. Executed flow cannot be spoofed, so the
// reading warms the forward-edge model faster and cleaner than a book-derived
// one would.
type CVDSymbol struct {
	mu    sync.Mutex
	pair  asset.Pair
	ticks []tick
	bid   float64
	ask   float64
	last  float64
}

func NewCVDSymbol(pair asset.Pair) *CVDSymbol {
	return &CVDSymbol{pair: pair}
}

// FeedTrade ingests one executed trade. takerBuy is true when the aggressor
// lifted the ask (Kraken WS trade side == "buy" in this codebase's normalized
// form).
func (state *CVDSymbol) FeedTrade(price, volume float64, takerBuy bool, at time.Time) {
	if price <= 0 || volume <= 0 {
		return
	}

	signed := volume

	if !takerBuy {
		signed = -volume
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.last = price
	state.ticks = append(state.ticks, tick{at: at, price: price, signed: signed})

	if len(state.ticks) > cvdSampleCap {
		state.ticks = state.ticks[len(state.ticks)-cvdSampleCap:]
	}
}

// FeedQuote keeps the best bid/ask current for the emitted measurement.
func (state *CVDSymbol) FeedQuote(bid, ask, last float64) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if bid > 0 {
		state.bid = bid
	}

	if ask > 0 {
		state.ask = ask
	}

	if last > 0 {
		state.last = last
	}
}

func (state *CVDSymbol) trimLocked(now time.Time) {
	cutoff := now.Add(-cvdWindow)
	keep := 0

	for keep < len(state.ticks) && state.ticks[keep].at.Before(cutoff) {
		keep++
	}

	if keep > 0 {
		state.ticks = state.ticks[keep:]
	}
}

// Measure emits an accumulation (bullish) or distribution (bearish) reading
// when executed flow is strongly one-sided while price is suppressed. The
// strength of the suppression is the absorption signal: heavy net buying with a
// flat or falling price is hidden accumulation.
func (state *CVDSymbol) Measure(now time.Time) (engine.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.trimLocked(now)

	if len(state.ticks) < cvdMinTrades {
		return engine.Measurement{}, false
	}

	var net, gross, hi, lo float64

	hi = state.ticks[0].price
	lo = state.ticks[0].price

	for _, t := range state.ticks {
		net += t.signed
		gross += math.Abs(t.signed)

		if t.price > hi {
			hi = t.price
		}

		if t.price < lo {
			lo = t.price
		}
	}

	if gross <= 0 || lo <= 0 {
		return engine.Measurement{}, false
	}

	netFraction := net / gross // signed in [-1, 1]
	startPrice := state.ticks[0].price
	drift := (state.last - startPrice) / startPrice

	directional := (math.Abs(netFraction) - cvdMinNetFraction) /
		(1 - cvdMinNetFraction)

	if directional <= 0 {
		return engine.Measurement{}, false
	}

	directional = clampUnit(directional)

	// Bullish: net buying with price flat or falling = absorption.
	if netFraction > 0 && drift <= cvdPriceFlatBand {
		suppression := 1.0

		if drift > 0 {
			suppression = clampUnit(1 - drift/cvdPriceFlatBand)
		}

		reason := "cvd_divergence"

		if drift <= 0 {
			reason = "cvd_absorption"
		}

		return state.emit(engine.Momentum, "accumulation", reason,
			clampUnit(directional*suppression), now), true
	}

	// Bearish mirror: net selling with price flat or rising = distribution.
	if netFraction < 0 && drift >= -cvdPriceFlatBand {
		suppression := 1.0

		if drift < 0 {
			suppression = clampUnit(1 + drift/cvdPriceFlatBand)
		}

		return state.emit(engine.Dump, "distribution", "cvd_distribution",
			clampUnit(directional*suppression), now), true
	}

	return engine.Measurement{}, false
}

func (state *CVDSymbol) emit(
	mtype engine.MeasurementType,
	regime, reason string,
	confidence float64,
	now time.Time,
) engine.Measurement {
	return engine.Measurement{
		Type:       mtype,
		Source:     "cvd",
		Regime:     regime,
		Reason:     reason,
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       state.last,
		Bid:        state.bid,
		Ask:        state.ask,
		Timeframe: engine.Timeframe{
			Start: now.Unix(),
			End:   now.Add(cvdForwardHorizon).Unix(),
		},
	}
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}

	if value > 1 {
		return 1
	}

	return value
}
