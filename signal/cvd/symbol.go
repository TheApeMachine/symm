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

	if len(state.ticks) == 0 {
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
	tradeCount := len(state.ticks)
	category := cvdCategory(netFraction, drift, tradeCount)

	confidence, mtype, regime, reason, ok := state.confidenceForCategory(
		category, netFraction, drift, tradeCount,
	)

	if !ok {
		return engine.Measurement{}, false
	}

	return state.emit(mtype, regime, reason, category, confidence, now), true
}

func (state *CVDSymbol) confidenceForCategory(
	category engine.Category,
	netFraction float64,
	drift float64,
	tradeCount int,
) (confidence float64, mtype engine.MeasurementType, regime, reason string, ok bool) {
	directional := (math.Abs(netFraction) - cvdMinNetFraction) / (1 - cvdMinNetFraction)
	directional = clampUnit(directional)

	switch category {
	case engine.CatVolumeStarvation:
		confidence = engine.ConfidenceFromScore(float64(tradeCount) / float64(cvdMinTrades))

		if confidence <= 0 {
			return 0, 0, "", "", false
		}

		return confidence, engine.Momentum, "starvation", "cvd_starvation", true
	case engine.CatStochasticBalance:
		confidence = engine.ConfidenceFromScore(1 - math.Abs(netFraction))

		if confidence <= 0 {
			return 0, 0, "", "", false
		}

		return confidence, engine.Momentum, "balance", "cvd_balance", true
	case engine.CatHiddenAbsorption:
		suppression := 1.0
		mtype = engine.Momentum
		regime = "accumulation"
		reason = "cvd_divergence"

		if netFraction > 0 {
			if drift > 0 {
				suppression = clampUnit(1 - drift/cvdPriceFlatBand)
			}

			if drift <= 0 {
				reason = "cvd_absorption"
			}
		}

		if netFraction < 0 {
			mtype = engine.Dump
			regime = "distribution"
			reason = "cvd_distribution"

			if drift < 0 {
				suppression = clampUnit(1 + drift/cvdPriceFlatBand)
			}
		}

		confidence = clampUnit(directional * suppression)

		if confidence <= 0 {
			return 0, 0, "", "", false
		}

		return confidence, mtype, regime, reason, true
	case engine.CatAggressiveDrive:
		confidence = directional

		if confidence <= 0 {
			return 0, 0, "", "", false
		}

		if netFraction > 0 {
			return confidence, engine.Momentum, "drive", "cvd_drive", true
		}

		return confidence, engine.Dump, "drive", "cvd_drive", true
	default:
		return 0, 0, "", "", false
	}
}

func (state *CVDSymbol) emit(
	mtype engine.MeasurementType,
	regime, reason string,
	category engine.Category,
	confidence float64,
	now time.Time,
) engine.Measurement {
	return engine.Measurement{
		Type:       mtype,
		Source:     "cvd",
		Regime:     regime,
		Reason:     reason,
		Category:   category,
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
