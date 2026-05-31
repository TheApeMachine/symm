package fluid

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/orderbook"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
FluidSymbol models one symbol's order book as a fluid field — divergence
(imbalance), vorticity (flow), turbulence (stationary price velocity excess),
viscosity (spread), and a Reynolds number combining them — and maps that onto
the mechanical perspective. SNR is the Reynolds number: a high-energy field
clears the noise floor, a calm laminar one does not.

The book is a maintained orderbook.Book. Liquidity flux — the field's vorticity
input — is measured as the change between the local book before and after each frame
is folded in, not between consecutive raw deltas, so it reflects genuine book churn
rather than the size of whatever slice the feed happened to send.
*/
type FluidSymbol struct {
	mu           sync.RWMutex
	symbol       string
	book         *orderbook.Book
	bookSequence market.BookSequence
	diverged     bool
	buyPressure  float64
	changePct    float64
	volume       float64
	last         float64
	bid          float64
	ask          float64
	pressure     *adaptive.EMA
	spreadBPS    float64
	flux         *fluxAccumulator
	priceFD      *adaptive.FracDiff
	fracScale    adaptive.AlphaEMA
	fracReturn   float64
	floor        *adaptive.SNR
}

// fracScaleAlpha smooths the running magnitude of the fractional price return,
// the baseline against which turbulence is measured as an excess over the norm.
const fracScaleAlpha = 0.05

func NewFluidSymbol(symbol string) *FluidSymbol {
	return &FluidSymbol{
		symbol:   symbol,
		book:     orderbook.NewBook(orderbook.MaintainDepth(config.System.BookDepthLevels)),
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

func (state *FluidSymbol) FeedBook(update market.BookUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.feedBookLocked(update)
}

func (state *FluidSymbol) feedBookLocked(update market.BookUpdate) {
	if !state.bookSequence.CanAccept(update) {
		return
	}

	if update.IsSnapshot() {
		state.bookSequence.AdmitSnapshot()
	}

	beforeBids := state.book.Bids()
	beforeAsks := state.book.Asks()

	state.applyFrameLocked(update)
	state.verifyLocked(uint32(update.Checksum))

	if state.diverged {
		return
	}

	afterBids := state.book.Bids()
	afterAsks := state.book.Asks()

	flux := sideChangeFlux(beforeBids, afterBids) + sideChangeFlux(beforeAsks, afterAsks)

	state.updateTouchLocked(afterBids, afterAsks)

	if flux <= 0 {
		return
	}

	state.flux.addBook(time.Now(), flux)
}

func (state *FluidSymbol) applyFrameLocked(update market.BookUpdate) {
	if update.IsSnapshot() {
		state.book.ApplySnapshot(update.BidLevels(), update.AskLevels())

		return
	}

	state.book.ApplyDelta(update.BidLevels(), update.AskLevels())
}

// verifyLocked compares the maintained book against the exchange checksum, reporting
// a divergence only on the transition into the diverged state so a persistent
// mismatch does not spam the hot path. The book corrects itself on the next snapshot.
func (state *FluidSymbol) verifyLocked(checksum uint32) {
	if checksum == 0 || !state.book.Ready() {
		return
	}

	matches := state.book.Verify(checksum)

	if !matches && !state.diverged {
		errnie.Error(fmt.Errorf("fluid: book checksum diverged for %s", state.symbol))
	}

	state.diverged = !matches
}

func (state *FluidSymbol) updateTouchLocked(bids, asks []orderbook.Level) {
	if len(bids) == 0 || len(asks) == 0 {
		return
	}

	bid := bids[0].Price
	ask := asks[0].Price
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

func sideChangeFlux(previous, updated []orderbook.Level) float64 {
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

	return state.book.Ready()
}

/*
Row returns the symbol's current fluid-field reading in the dashboard wire shape
(symbol, change_pct, vol, div, vort, turb, visc, re), or nil when the field has
no data yet.
*/
func (state *FluidSymbol) Row() map[string]any {
	state.mu.RLock()
	defer state.mu.RUnlock()

	if state.diverged {
		return nil
	}

	return state.wireRowLocked()
}

func (state *FluidSymbol) Measure() (perspectives.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.diverged {
		return perspectives.Measurement{}, false
	}

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

	return perspectives.WithGaugeFactors(perspectives.FinalizeMeasurement(perspectives.Measurement{
		Symbol:   state.symbol,
		Source:   perspectives.SourceFluid,
		Category: fluidCategory(divergence, turbulence, viscosity, re),
		Last:     state.last,
	}, re, "reynolds"), perspectives.GaugeFactorsFrom(row,
		"div", "vort", "turb_fd", "re", "visc",
	)), true
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
	bids := state.book.Bids()
	asks := state.book.Asks()
	imbalance := 0.0
	pressure := (state.buyPressure + 1) / 2
	visc := 1 / (1 + state.spreadBPS/100)

	if len(bids) > 0 && len(asks) > 0 {
		bidVolume := 0.0
		askVolume := 0.0

		for _, level := range bids {
			bidVolume += level.Qty
		}

		for _, level := range asks {
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
