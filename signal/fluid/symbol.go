package fluid

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
	"github.com/theapemachine/symm/toxicity"
)

// toxicLevelFilter returns a per-level predicate that excludes toxicity-flagged
// book levels (large, young, near-touch cancels) from the weighted imbalance
// (§16.3). Returns false for every level until the tracker flags one.
func toxicLevelFilter(symbol string) func(price float64) bool {
	return func(price float64) bool {
		return toxicity.IsToxic(symbol, price, time.Now())
	}
}

type FluidSymbol struct {
	mu              sync.RWMutex
	pair            asset.Pair
	bids            []market.BookLevel
	asks            []market.BookLevel
	prevBids        []market.BookLevel
	prevAsks        []market.BookLevel
	buyPressure     float64
	changePct       float64
	volume          float64
	last            float64
	bid             float64
	ask             float64
	pressure        *adaptive.EMA
	spreadBPS       float64
	flux            *fluxAccumulator
	priceFD         *adaptive.FracDiff
	fracScale       adaptive.AlphaEMA
	fracReturn      float64
	score           *numeric.Derived
	forecast        *learned.Forecast
}

// fracScaleAlpha smooths the running magnitude of the fractional price return, the baseline
// against which turbulence is measured as an excess over the norm.
const fracScaleAlpha = 0.05

func NewFluidSymbol(pair asset.Pair) *FluidSymbol {
	return &FluidSymbol{
		pair:     pair,
		pressure: adaptive.NewEMA(0),
		flux:     newFluxAccumulator(config.System.BookFluxWindow),
		priceFD:  adaptive.NewFracDiff(config.System.FractionalDiffOrder, config.System.FractionalDiffWidth),
		score: numeric.NewDerived(numeric.WithDynamics(
			adaptive.NewProduct(),
			adaptive.NewEMA(0),
		)),
		forecast: learned.NewForecast(0.35),
	}
}

func (state *FluidSymbol) FeedTicker(row market.TickerRow) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.changePct = row.ChangePct
	state.volume = row.Volume

	// Size one volume bar as a fixed slice of the day's base volume, so bars close on traded
	// activity rather than the wall clock and accelerate automatically when the tape speeds up.
	if row.Volume > 0 && config.System.VolumeClockBarsPerDay > 0 {
		state.flux.setTarget(row.Volume / config.System.VolumeClockBarsPerDay)
	}

	if row.Last > 0 {
		state.last = row.Last
		state.observePriceLocked(row.Last)
	}

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}
}

/*
observePriceLocked folds the latest price into the fractional differencing filter, producing a
stationary, memory-preserving price velocity and tracking its rolling magnitude for turbulence.
*/
func (state *FluidSymbol) observePriceLocked(price float64) {
	if price <= 0 {
		return
	}

	value, ok := state.priceFD.Push(math.Log(price))

	if !ok {
		return
	}

	state.fracReturn = value
	_ = state.fracScale.Update(math.Abs(value), fracScaleAlpha)
}

func (state *FluidSymbol) FeedBook(delta market.BookLevelsDelta) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.feedBookLocked(delta)
}

func (state *FluidSymbol) feedBookLocked(delta market.BookLevelsDelta) {
	flux := 0.0

	if len(state.prevBids) > 0 || len(state.prevAsks) > 0 {
		if delta.BidOK {
			flux += sideChangeFlux(state.prevBids, delta.Bids)
		}

		if delta.AskOK {
			flux += sideChangeFlux(state.prevAsks, delta.Asks)
		}
	}

	if delta.BidOK {
		state.bids = append([]market.BookLevel(nil), delta.Bids...)
		state.prevBids = append([]market.BookLevel(nil), delta.Bids...)
	}

	if delta.AskOK {
		state.asks = append([]market.BookLevel(nil), delta.Asks...)
		state.prevAsks = append([]market.BookLevel(nil), delta.Asks...)
	}

	if len(state.bids) > 0 && len(state.asks) > 0 {
		bid := state.bids[0].Price
		ask := state.asks[0].Price
		mid := (bid + ask) / 2

		state.bid = bid
		state.ask = ask

		if state.last <= 0 && mid > 0 {
			state.last = mid
		}

		if mid > 0 {
			state.spreadBPS = (ask - bid) / mid * 10000
		}
	}

	if flux <= 0 {
		return
	}

	state.flux.addBook(time.Now(), flux)
}

func sideChangeFlux(previous, updated []market.BookLevel) float64 {
	previousByPrice := make(map[float64]float64, len(previous))

	for _, level := range previous {
		previousByPrice[level.Price] = level.Volume
	}

	flux := 0.0
	seen := make(map[float64]bool, len(updated))

	for _, level := range updated {
		flux += math.Abs(level.Volume - previousByPrice[level.Price])
		seen[level.Price] = true
	}

	for price, volume := range previousByPrice {
		if seen[price] {
			continue
		}

		flux += volume
	}

	return flux
}

func (state *FluidSymbol) FeedTrade(at time.Time, qty float64) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.feedTradeLocked(at, qty)
}

func (state *FluidSymbol) FeedTradeSide(at time.Time, qty float64, side string) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.feedTradeLocked(at, qty)

	sign := -1.0

	if side == "buy" {
		sign = 1.0
	}

	state.buyPressure = errnie.Does(func() (float64, error) {
		return state.pressure.Next(0, sign)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()
}

func (state *FluidSymbol) feedTradeLocked(at time.Time, qty float64) {
	if qty <= 0 {
		return
	}

	state.flux.addTrade(at, qty)
}

func (state *FluidSymbol) HasBook() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return len(state.bids) > 0 && len(state.asks) > 0
}

func (state *FluidSymbol) BookStatus() (int, float64) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return len(state.bids), state.spreadBPS
}

func (state *FluidSymbol) bookFluxTrustworthy() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.bookFluxTrustworthyLocked()
}

func (state *FluidSymbol) bookFluxTrustworthyLocked() bool {
	bookFlux := state.flux.bookFlux()
	tradeFlux := state.flux.tradeFlux()

	if bookFlux <= 0 {
		return true
	}

	return tradeFlux/bookFlux >= config.System.MinFillToCancelRatio
}

func (state *FluidSymbol) Measure() (engine.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	row := state.wireRowLocked()

	if row == nil {
		return engine.Measurement{}, false
	}

	re, ok := row["re"].(float64)

	if !ok || re <= 0 {
		return engine.Measurement{}, false
	}

	bid := 0.0
	ask := 0.0
	mid := 0.0

	if len(state.bids) > 0 && len(state.asks) > 0 {
		bid = state.bids[0].Price
		ask = state.asks[0].Price
		mid = (bid + ask) / 2
	}

	if mid <= 0 && state.last > 0 {
		bid = state.bid
		ask = state.ask

		if bid <= 0 {
			bid = state.last
		}

		if ask <= 0 {
			ask = state.last
		}

		mid = state.last

		if bid > 0 && ask > 0 {
			mid = (bid + ask) / 2
		}
	}

	if mid <= 0 {
		return engine.Measurement{}, false
	}

	confidence := engine.ConfidenceFromScore(re)
	reason := "field_activity"

	if state.bookFluxTrustworthyLocked() {
		imbalance, imbalanceOK := market.WeightedDepthImbalanceFiltered(
			state.bids,
			state.asks,
			mid,
			config.System.BookDepthDecayLambda,
			toxicLevelFilter(state.pair.Wsname),
		)

		if imbalanceOK && imbalance != 0 {
			level1Imbalance, level1OK := market.Level1Imbalance(state.bids, state.asks)

			if level1OK && !market.IsSpoofSkew(
				imbalance,
				level1Imbalance,
				config.System.SpoofWeightedThreshold,
				config.System.SpoofLevel1Reject,
			) {
				flatImbalance, flatOK := market.FlatDepthImbalance(state.bids, state.asks)

				if !flatOK || !market.IsSpoofSkew(
					flatImbalance,
					level1Imbalance,
					config.System.SpoofWeightedThreshold,
					config.System.SpoofLevel1Reject,
				) {
					pressure := (state.buyPressure + 1) / 2

					if state.spreadBPS > 0 {
						pressure *= 1 / (1 + state.spreadBPS/100)
					}

					raw, err := state.score.Push(
						math.Abs(imbalance),
						pressure*state.forecast.Scale(),
					)

					if err != nil {
						errnie.Error(err)
					}

					if raw > 0 {
						confidence = engine.ConfidenceFromScore(raw)
						reason = "book_flow"
					}
				}
			}
		}
	}

	divergence, _ := row["div"].(float64)
	turbulence, _ := row["turb_fd"].(float64)
	viscosity, _ := row["visc"].(float64)

	return engine.Measurement{
		Type:       engine.Flow,
		Source:     fluidSource,
		Regime:     "fluid",
		Reason:     reason,
		Category:   fluidCategory(divergence, turbulence, viscosity, re),
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       mid,
		Bid:        bid,
		Ask:        ask,
	}, true
}

/*
fluidCategory maps the fluid-dynamics row onto the mechanical perspective.
Stationary turbulence dominating the field is turbulent (fragile); a wide spread
(low viscosity) is a viscous grind; a strong directional divergence on an
elevated Reynolds number is an inertial push; everything else is laminar
(smooth, low activity).
*/
func fluidCategory(divergence, turbulence, viscosity, reynolds float64) engine.Category {
	switch {
	case turbulence > 0 && turbulence >= math.Abs(divergence):
		return engine.CatTurbulent
	case viscosity > 0 && viscosity < 0.5:
		return engine.CatViscous
	case math.Abs(divergence) >= 0.2 && reynolds >= 0.2:
		return engine.CatInertial
	default:
		return engine.CatLaminar
	}
}

func (state *FluidSymbol) ApplyFeedback(feedback engine.PredictionFeedback) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if _, err := state.forecast.Next(
		0, feedback.PredictedReturn, feedback.ActualReturn,
	); err != nil {
		errnie.Error(err)
	}
}

func (state *FluidSymbol) wireRow() map[string]any {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.wireRowLocked()
}

func (state *FluidSymbol) wireRowLocked() map[string]any {
	imbalance := 0.0
	pressure := (state.buyPressure + 1) / 2
	visc := 1 / (1 + state.spreadBPS/100)

	if len(state.bids) > 0 && len(state.asks) > 0 {
		bidVolume := 0.0
		askVolume := 0.0

		for _, level := range state.bids {
			bidVolume += level.Volume
		}

		for _, level := range state.asks {
			askVolume += level.Volume
		}

		total := bidVolume + askVolume

		if total > 0 {
			imbalance = (bidVolume - askVolume) / total
		}
	}

	if state.volume <= 0 && state.changePct == 0 && imbalance == 0 && pressure == 0.5 {
		return nil
	}

	// Turbulence from the stationary, memory-preserving price velocity: the excess of the current
	// fractional return over its rolling norm. Calm or warming-up price action contributes zero, so
	// the Reynolds number only rises from price when the flow is genuinely turbulent — not because a
	// quiet book's spread has widened.
	turbulence := 0.0
	fracScale := state.fracScale.Value()

	if fracScale > 0 {
		turbulence = math.Max(0, math.Abs(state.fracReturn)/fracScale-1)
	}

	re := math.Max(
		math.Max(math.Abs(imbalance), math.Abs(pressure)),
		turbulence,
	) * state.forecast.Scale()

	return WireRow(map[string]any{
		"symbol":     state.pair.Wsname,
		"change_pct": state.changePct,
		"vol":        state.volume,
		"div":        imbalance,
		"vort":       state.buyPressure,
		"turb":       pressure * state.spreadBPS / 100,
		"turb_fd":    turbulence,
		"fd_ret":     state.fracReturn,
		"visc":       visc,
		"re":         re,
	})
}
