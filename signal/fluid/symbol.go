package fluid

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
FluidSymbol models one symbol's order book as a fluid field — divergence
(imbalance), vorticity (flow), turbulence (stationary price velocity excess),
viscosity (spread), and a Reynolds number combining them — and maps that onto
the mechanical perspective. SNR is the Reynolds number: a high-energy field
clears the noise floor, a calm laminar one does not.
*/
type FluidSymbol struct {
	mu          sync.RWMutex
	symbol      string
	bids        []market.BookLevel
	asks        []market.BookLevel
	prevBids    []market.BookLevel
	prevAsks    []market.BookLevel
	buyPressure float64
	changePct   float64
	volume      float64
	last        float64
	bid         float64
	ask         float64
	pressure    *adaptive.EMA
	spreadBPS   float64
	flux        *fluxAccumulator
	priceFD     *adaptive.FracDiff
	fracScale   adaptive.AlphaEMA
	fracReturn  float64
	floor       *adaptive.SNR
}

// fracScaleAlpha smooths the running magnitude of the fractional price return,
// the baseline against which turbulence is measured as an excess over the norm.
const fracScaleAlpha = 0.05

func NewFluidSymbol(symbol string) *FluidSymbol {
	return &FluidSymbol{
		symbol:   symbol,
		pressure: adaptive.NewEMA(0),
		flux:     newFluxAccumulator(config.System.BookFluxWindow),
		priceFD:  adaptive.NewFracDiff(config.System.FractionalDiffOrder, config.System.FractionalDiffWidth),
		floor:    adaptive.NewSNR(),
	}
}

func (state *FluidSymbol) FeedTicker(row market.TickerUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.changePct = row.ChangePct
	state.volume = row.Volume

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

func (state *FluidSymbol) FeedBook(delta market.BookUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.feedBookLocked(delta)
}

func (state *FluidSymbol) feedBookLocked(delta market.BookUpdate) {
	flux := 0.0
	bidOK := len(delta.Bids) > 0
	askOK := len(delta.Asks) > 0

	if len(state.prevBids) > 0 || len(state.prevAsks) > 0 {
		if bidOK {
			flux += sideChangeFlux(state.prevBids, delta.Bids)
		}

		if askOK {
			flux += sideChangeFlux(state.prevAsks, delta.Asks)
		}
	}

	if bidOK {
		state.bids = append([]market.BookLevel(nil), delta.Bids...)
		state.prevBids = append([]market.BookLevel(nil), delta.Bids...)
	}

	if askOK {
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
		previousByPrice[level.Price] = level.Qty
	}

	flux := 0.0
	seen := make(map[float64]bool, len(updated))

	for _, level := range updated {
		flux += math.Abs(level.Qty - previousByPrice[level.Price])
		seen[level.Price] = true
	}

	for price, qty := range previousByPrice {
		if seen[price] {
			continue
		}

		flux += qty
	}

	return flux
}

func (state *FluidSymbol) FeedTradeSide(at time.Time, qty float64, side string) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if qty > 0 {
		state.flux.addTrade(at, qty)
	}

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

func (state *FluidSymbol) HasBook() bool {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return len(state.bids) > 0 && len(state.asks) > 0
}

/*
Row returns the symbol's current fluid-field reading in the dashboard wire shape
(symbol, change_pct, vol, div, vort, turb, visc, re), or nil when the field has
no data yet.
*/
func (state *FluidSymbol) Row() map[string]any {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.wireRowLocked()
}

func (state *FluidSymbol) Measure() (perspectives.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	row := state.wireRowLocked()

	if row == nil {
		return perspectives.Measurement{}, false
	}

	re, ok := row["re"].(float64)

	if !ok || re <= 0 {
		return perspectives.Measurement{}, false
	}

	divergence, _ := row["div"].(float64)
	turbulence, _ := row["turb_fd"].(float64)
	viscosity, _ := row["visc"].(float64)

	return perspectives.Measurement{
		Symbol:   state.symbol,
		Source:   perspectives.SourceFluid,
		Category: fluidCategory(divergence, turbulence, viscosity, re),
		SNR:      state.floor.Score(re),
		Last:     state.last,
	}, true
}

/*
fluidCategory maps the fluid-dynamics row onto the mechanical perspective:
stationary turbulence dominating the field is turbulent (fragile); a wide spread
(low viscosity) is a viscous grind; a strong directional divergence on an
elevated Reynolds number is an inertial push; everything else is laminar.
*/
func fluidCategory(divergence, turbulence, viscosity, reynolds float64) perspectives.CategoryType {
	switch {
	case turbulence > 0 && turbulence >= math.Abs(divergence):
		return perspectives.CategoryTurbulent
	case viscosity > 0 && viscosity < 0.5:
		return perspectives.CategoryViscous
	case math.Abs(divergence) >= 0.2 && reynolds >= 0.2:
		return perspectives.CategoryInertial
	default:
		return perspectives.CategoryLaminar
	}
}

func (state *FluidSymbol) wireRowLocked() map[string]any {
	imbalance := 0.0
	pressure := (state.buyPressure + 1) / 2
	visc := 1 / (1 + state.spreadBPS/100)

	if len(state.bids) > 0 && len(state.asks) > 0 {
		bidVolume := 0.0
		askVolume := 0.0

		for _, level := range state.bids {
			bidVolume += level.Qty
		}

		for _, level := range state.asks {
			askVolume += level.Qty
		}

		total := bidVolume + askVolume

		if total > 0 {
			imbalance = (bidVolume - askVolume) / total
		}
	}

	if state.volume <= 0 && state.changePct == 0 && imbalance == 0 && pressure == 0.5 {
		return nil
	}

	turbulence := 0.0
	fracScale := state.fracScale.Value()

	if fracScale > 0 {
		turbulence = math.Max(0, math.Abs(state.fracReturn)/fracScale-1)
	}

	re := math.Max(
		math.Max(math.Abs(imbalance), math.Abs(pressure)),
		turbulence,
	)

	return WireRow(map[string]any{
		"symbol":     state.symbol,
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
